package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// UploadHandler guarda a conexão com o Redis e PostgreSQL
type UploadHandler struct {
	DB          *sql.DB
	RedisClient *redis.Client
}

// HandleUpload processa o form-data, salva o PDF/EPUB e enfileira a tarefa
func (h *UploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	// 1. Limita o tamanho do upload (ex: 50MB)
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		http.Error(w, "Arquivo muito grande", http.StatusBadRequest)
		return
	}

	// 2. Extrai o arquivo com a chave 'document'
	file, header, err := r.FormFile("document")
	if err != nil {
		http.Error(w, "Erro ao ler o arquivo enviado", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 3. Valida a extensão do arquivo no backend (PDF ou EPUB)
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".pdf" && ext != ".epub" {
		http.Error(w, "Formato de arquivo não suportado. Envie apenas PDF ou EPUB.", http.StatusBadRequest)
		return
	}

	// 4. Prepara o diretório de destino e sanitiza o nome contra Path Traversal
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		http.Error(w, "Erro ao preparar diretório de uploads", http.StatusInternalServerError)
		return
	}

	// Isolamos apenas o nome do arquivo para prevenir ataques de navegação de diretório
	safeFilename := filepath.Base(header.Filename)
	fileName := fmt.Sprintf("%d_%s", time.Now().Unix(), safeFilename)
	filePath := filepath.Join(uploadDir, fileName)

	// 5. Salva o arquivo no disco
	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Erro ao salvar o arquivo no disco", http.StatusInternalServerError)
		return
	}
	
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		os.Remove(filePath)
		http.Error(w, "Erro ao gravar conteúdo do arquivo", http.StatusInternalServerError)
		return
	}
	dst.Close()

	var workID int
	query := `INSERT INTO works (original_title) VALUES ($1) RETURNING id`
	
	err = h.DB.QueryRow(query, safeFilename).Scan(&workID)
	if err != nil {
		os.Remove(filePath)
		http.Error(w, "Erro ao criar registro no banco de dados", http.StatusInternalServerError)
		return
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		os.Remove(filePath)
		http.Error(w, "Erro ao resolver caminho do arquivo", http.StatusInternalServerError)
		return
	}

	// Enfileira no Redis
	ctx := context.Background()
	err = h.RedisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: "ingestion_tasks",
		Values: map[string]interface{}{
			"file_path": absPath,
			"work_id":   workID,
		},
	}).Err()

	if err != nil {
		os.Remove(filePath)
		http.Error(w, "Arquivo salvo, mas erro ao avisar a fila", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Upload concluído e enfileirado",
		"work_id": workID,
	})
}