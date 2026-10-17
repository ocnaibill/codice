package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/mailer"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// KindleHandler sends a book to the e-reader of the person that asks, by e-mail: a Kindle takes books at an address of its own (and so do
// other readers). It needs the mail server of the instance (MailAdminHandler) and the address of the person.
type KindleHandler struct {
	DB          *sql.DB
	StorageRoot string
	SecretKey   string
}

const (
	// kindleDailyLimit is how many books a person may send in a day: a mail server is shared, and the Kindle has its own limits.
	kindleDailyLimit = 30
	// kindleMaxBytes is the most Amazon takes in a message.
	kindleMaxBytes = 50 << 20
)

// sendable are the formats a Kindle takes by e-mail, in the order they are the better choice.
var sendable = []struct{ format, contentType string }{
	{"epub", "application/epub+zip"}, {"pdf", "application/pdf"}, {"txt", "text/plain"},
}

func (h *KindleHandler) address(ctx context.Context, userID string) (string, error) {
	var email sql.NullString
	if err := h.DB.QueryRowContext(ctx, `SELECT kindle_email FROM users WHERE id = $1`, userID).Scan(&email); err != nil {
		return "", err
	}
	return email.String, nil
}

// Get says the address of the person and whether sending is on in this instance.
func (h *KindleHandler) Get(w http.ResponseWriter, r *http.Request) {
	email, err := h.address(r.Context(), currentUserID(r))
	if err != nil {
		http.Error(w, "Error reading the address", http.StatusInternalServerError)
		return
	}
	_, _, mailErr := MailConfig(r.Context(), h.DB, h.SecretKey)
	writeJSON(w, http.StatusOK, map[string]any{"email": email, "available": mailErr == nil})
}

// Put keeps the address ("" takes it away).
func (h *KindleHandler) Put(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(req.Email)
	if email != "" {
		var err error
		if email, err = mailer.Address(email); err != nil {
			http.Error(w, "Isto não é um endereço de e-mail.", http.StatusBadRequest)
			return
		}
	}
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE users SET kindle_email = NULLIF($2, '') WHERE id = $1`, currentUserID(r), email); err != nil {
		log.Println("Error saving the address of the e-reader:", err)
		http.Error(w, "Error saving", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": email})
}

// fileName is how the book is named in the message: "Title - Author.epub", with nothing a file name cannot have.
func fileName(title, author, format string) string {
	clean := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			switch {
			case strings.ContainsRune(`/\:*?"<>|`, r) || unicode.IsControl(r):
				b.WriteByte(' ')
			default:
				b.WriteRune(r)
			}
		}
		return strings.Join(strings.Fields(b.String()), " ")
	}
	name := clean(title)
	if a := clean(author); a != "" && !strings.EqualFold(a, "Unknown Author") {
		name += " - " + a
	}
	if name == "" {
		name = "livro"
	}
	if r := []rune(name); len(r) > 120 {
		name = strings.TrimSpace(string(r[:120]))
	}
	return name + "." + format
}

// Send mails a file of a work to the address of the person. The file is the one asked for, or the EPUB of the work, or its PDF.
func (h *KindleHandler) Send(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var req struct {
		FileID *int64 `json:"fileId"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req)
	userID := currentUserID(r)

	cfg, settings, err := MailConfig(r.Context(), h.DB, h.SecretKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	to, err := h.address(r.Context(), userID)
	if err != nil {
		http.Error(w, "Error reading the address", http.StatusInternalServerError)
		return
	}
	if to == "" {
		http.Error(w, "Diga o e-mail do seu leitor nas preferências da conta.", http.StatusConflict)
		return
	}
	var sent int
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'kindle.send' AND at > now() - interval '1 day'`, userID).Scan(&sent); err == nil && sent >= kindleDailyLimit {
		http.Error(w, fmt.Sprintf("Você já enviou %d livros hoje: tente de novo amanhã.", kindleDailyLimit), http.StatusTooManyRequests)
		return
	}

	var title, author string
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT w.original_title, COALESCE(au.names, '')`+catalogFrom+`
		WHERE w.id = $1 AND w.retired_at IS NULL`, workID).Scan(&title, &author); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		log.Println("Error finding the work to send:", err)
		http.Error(w, "Error sending", http.StatusInternalServerError)
		return
	}

	// the file: the one asked for, else the best format the work has
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT f.id, lower(COALESCE(f.format, '')), COALESCE(f.size_bytes, 0), l.mode, l.root, l.path
		FROM files f JOIN editions e ON e.id = f.edition_id
		JOIN storage_locations l ON l.file_id = f.id AND l.state = 'ok'
		WHERE e.work_id = $1 AND f.availability = 'available' ORDER BY e.is_primary DESC, f.id`, workID)
	if err != nil {
		http.Error(w, "Error sending", http.StatusInternalServerError)
		return
	}
	type candidate struct {
		id         int64
		format     string
		size       int64
		mode, root string
		rel        string
	}
	var found []candidate
	for rows.Next() {
		var c candidate
		var root, rel sql.NullString
		if err := rows.Scan(&c.id, &c.format, &c.size, &c.mode, &root, &rel); err != nil {
			rows.Close()
			http.Error(w, "Error sending", http.StatusInternalServerError)
			return
		}
		c.root, c.rel = root.String, rel.String
		found = append(found, c)
	}
	rows.Close()
	var chosen *candidate
	contentType := ""
	if req.FileID != nil {
		for i := range found {
			if found[i].id == *req.FileID {
				chosen = &found[i]
			}
		}
		if chosen == nil {
			http.Error(w, "That file is not one of this book's files", http.StatusBadRequest)
			return
		}
	} else {
		for _, want := range sendable {
			for i := range found {
				if found[i].format == want.format && chosen == nil {
					chosen = &found[i]
				}
			}
		}
	}
	if chosen != nil {
		for _, s := range sendable {
			if s.format == chosen.format {
				contentType = s.contentType
			}
		}
	}
	if chosen == nil || contentType == "" {
		http.Error(w, "Este livro não tem um arquivo que o Kindle receba por e-mail (EPUB, PDF ou TXT).", http.StatusUnprocessableEntity)
		return
	}
	limit := int64(settings.MaxMB) << 20
	if limit == 0 || limit > kindleMaxBytes {
		limit = int64(defaultMailMB) << 20
	}
	full, ok := storage.AbsPath(h.StorageRoot, chosen.mode, chosen.root, chosen.rel)
	if !ok {
		http.Error(w, "O arquivo não está disponível.", http.StatusNotFound)
		return
	}
	if chosen.mode == "referenced" {
		realFile, err1 := filepath.EvalSymlinks(full)
		realRoot, err2 := filepath.EvalSymlinks(chosen.root)
		if err1 != nil || err2 != nil || !isWithin(realFile, realRoot) {
			http.Error(w, "O arquivo não está disponível.", http.StatusNotFound)
			return
		}
		full = realFile
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.Error(w, "O arquivo não está disponível.", http.StatusNotFound)
		return
	}
	if info.Size() > limit {
		http.Error(w, fmt.Sprintf("O arquivo tem %d MB e o limite de envio é de %d MB.", info.Size()>>20, limit>>20), http.StatusRequestEntityTooLarge)
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		http.Error(w, "O arquivo não está disponível.", http.StatusNotFound)
		return
	}
	name := fileName(title, author, chosen.format)
	err = mailer.Send(r.Context(), cfg, mailer.Message{To: to, Subject: title,
		Body:   "Enviado pelo Códice para o seu leitor: " + name + ".",
		Attach: &mailer.Attachment{Filename: name, ContentType: contentType, Data: data}})
	if err != nil {
		log.Println("Error sending a book by e-mail:", err)
		http.Error(w, "Não foi possível enviar: "+err.Error(), http.StatusBadGateway)
		return
	}
	// the entry says which book and how big, and never the address of the person
	_ = audit.Record(r.Context(), h.DB, userID, "kindle.send", "work", fmt.Sprint(workID), map[string]any{"fileId": chosen.id, "format": chosen.format, "bytes": info.Size()})
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "to": to, "fileName": name, "bytes": info.Size()})
}
