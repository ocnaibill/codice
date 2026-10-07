package handlers

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// authorsOf is the SQL that names a work's authors, for a column of works called w.
const authorsOf = `COALESCE((SELECT string_agg(p.name, ', ' ORDER BY c.position, p.name)
	FROM work_contributors c JOIN person p ON p.id = c.person_id WHERE c.work_id = w.id AND c.role = 'author'), '')`

// queryRows runs a query and gives each row as a map by its column names, in the order of the columns of the JSON a person reads:
// times as RFC 3339, JSON columns as JSON, nothing as null. The columns are named in the SQL, in camelCase, so what a person
// reads is what the query says.
func queryRows(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, name := range cols {
			switch v := values[i].(type) {
			case []byte:
				if t := types[i].DatabaseTypeName(); t == "JSON" || t == "JSONB" {
					row[name] = json.RawMessage(v)
				} else {
					row[name] = string(v)
				}
			case time.Time:
				row[name] = v.UTC().Format(time.RFC3339)
			default:
				row[name] = v
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type exportFile struct {
	zw *zip.Writer
}

func (e exportFile) add(name string, write func(io.Writer) error) error {
	w, err := e.zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return err
	}
	return write(w)
}

func (e exportFile) json(name string, v any) error {
	return e.add(name, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(v)
	})
}

// exportQueries are the parts of a person's data that are a list of rows, each with the file it goes in. Every query takes the
// account as $1 and nothing else: what is in the file is only ever what belongs to that account (FL-09).
var exportQueries = []struct {
	file  string
	query string
}{
	{"favoritos.json", `
		SELECT w.original_title AS title, ` + authorsOf + ` AS authors, f.created_at AS "addedAt", (w.retired_at IS NULL) AS "inLibrary"
		FROM favorites f JOIN works w ON w.id = f.work_id WHERE f.user_id = $1 ORDER BY f.created_at, w.original_title`},
	{"conceitos.json", `
		SELECT name, description, to_jsonb(aliases) AS aliases, created_at AS "createdAt", updated_at AS "updatedAt"
		FROM concepts WHERE user_id = $1 ORDER BY lower(name)`},
	{"relacoes.json", `
		SELECT type, source_kind AS "sourceKind", source_label AS "source", target_kind AS "targetKind", target_label AS "target",
		       origin, comment, created_at AS "createdAt", updated_at AS "updatedAt"
		FROM relations WHERE user_id = $1 ORDER BY created_at, id`},
	{"tags.json", `
		SELECT t AS name, count(*) AS notes FROM notes n, unnest(n.tags) AS t WHERE n.user_id = $1 GROUP BY t ORDER BY lower(t)`},
	{"sessoes.json", `
		SELECT user_agent AS "device", ip, created_at AS "createdAt", last_seen_at AS "lastSeenAt", expires_at AS "expiresAt"
		FROM sessions WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now() ORDER BY created_at`},
	{"aplicativos.json", `
		SELECT name, created_at AS "createdAt", last_used_at AS "lastUsedAt", revoked_at AS "revokedAt"
		FROM app_tokens WHERE user_id = $1 ORDER BY created_at`},
	{"entradas.json", `
		SELECT result, method, ip, user_agent AS "device", count, at AS "firstAt", last_at AS "lastAt"
		FROM login_events WHERE user_id = $1 ORDER BY id`},
}

const exportReadme = `Seus dados no Códice
====================

Arquivo gerado em %s, para a conta "%s".

O que está aqui (tudo é seu, e só você pôde pedir este arquivo):

  conta.json         seus dados de cadastro e as suas preferências (inclusive as do leitor)
  anotacoes.md       as suas anotações, destaques e marcadores, para ler
  anotacoes.json     o mesmo, com todos os campos (posição exata no livro, cor, tags)
  favoritos.json     as obras que você favoritou
  leitura.json       o seu progresso em cada livro, as conclusões e as posições equivalentes que você aceitou
  conceitos.json     os conceitos que você criou
  relacoes.json      as relações entre os seus conceitos e anotações
  tags.json          as suas tags e quantas anotações cada uma tem
  sessoes.json       onde a sua conta está aberta agora (aparelho, endereço, último uso)
  aplicativos.json   os aplicativos que você liberou (nome e datas; a senha deles nunca é guardada)
  entradas.json      o registro das entradas na sua conta, as tentativas que falharam inclusive

O que NÃO está aqui: os livros (são do acervo, não seus), a sua senha e qualquer segredo (tokens, chaves).
%s
Este arquivo some do servidor 24 horas depois de pronto, ou quando você o apagar na tela "Meus dados".
Os arquivos .json estão em UTF-8; as datas, em UTC.
`

// BuildDataExport writes the ZIP of a person's own data to w (see DataExportHandler). It reads only rows of that account.
func BuildDataExport(ctx context.Context, db *sql.DB, userID string, w io.Writer, now time.Time) error {
	zw := zip.NewWriter(w)
	out := exportFile{zw}

	accounts, err := queryRows(ctx, db, `
		SELECT username, email, role, created_at AS "createdAt", name_order AS "nameOrder", display_name AS "displayName", reader_prefs AS "reader", blocked_at AS "blockedAt"
		FROM users WHERE id = $1`, userID)
	if err != nil || len(accounts) != 1 {
		if err == nil {
			err = fmt.Errorf("the account is gone")
		}
		return err
	}
	identities, err := queryRows(ctx, db, `SELECT provider, created_at AS "linkedAt" FROM external_identities WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return err
	}
	account := accounts[0]
	account["directory"] = identities
	if err := out.json("conta.json", account); err != nil {
		return err
	}

	notes, err := exportNotes(ctx, db, userID)
	if err != nil {
		return err
	}
	if err := out.json("anotacoes.json", map[string]any{"exportedAt": now.UTC(), "count": len(notes), "notes": notes}); err != nil {
		return err
	}
	if err := out.add("anotacoes.md", func(w io.Writer) error {
		_, err := io.WriteString(w, markdownExport(notes, now))
		return err
	}); err != nil {
		return err
	}

	reading := map[string]any{}
	for key, query := range map[string]string{
		"progresso": `
			SELECT w.original_title AS title, ` + authorsOf + ` AS authors, f.format, rp.percent_complete AS "percent",
			       rp.completed_at AS "completedAt", rp.reading_seconds AS "readingSeconds", rp.device,
			       rp.last_opened_at AS "lastOpenedAt", rp.updated_at AS "updatedAt", rp.locator
			FROM reading_progress rp JOIN files f ON f.id = rp.file_id JOIN editions e ON e.id = f.edition_id JOIN works w ON w.id = e.work_id
			WHERE rp.user_id = $1 ORDER BY rp.updated_at, w.original_title`,
		"conclusoes": `
			SELECT w.original_title AS title, ` + authorsOf + ` AS authors, rc.format, rc.completed_at AS "completedAt"
			FROM reading_completions rc JOIN works w ON w.id = rc.work_id WHERE rc.user_id = $1 ORDER BY rc.completed_at, rc.id`,
		"obrasFinalizadas": `
			SELECT w.original_title AS title, ` + authorsOf + ` AS authors, s.finished_at AS "finishedAt"
			FROM work_reading_state s JOIN works w ON w.id = s.work_id WHERE s.user_id = $1 ORDER BY s.finished_at`,
		"posicoesEquivalentes": `
			SELECT w.original_title AS title, a.method, a.confidence, a.accepted_at AS "acceptedAt",
			       a.source_locator AS "from", a.destination_locator AS "to"
			FROM equivalent_position_acceptances a JOIN works w ON w.id = a.work_id WHERE a.user_id = $1 ORDER BY a.accepted_at`,
	} {
		rows, err := queryRows(ctx, db, query, userID)
		if err != nil {
			return err
		}
		reading[key] = rows
	}
	if err := out.json("leitura.json", reading); err != nil {
		return err
	}

	for _, q := range exportQueries {
		rows, err := queryRows(ctx, db, q.query, userID)
		if err != nil {
			return fmt.Errorf("%s: %w", q.file, err)
		}
		if err := out.json(q.file, rows); err != nil {
			return err
		}
	}

	limitNote := ""
	if len(notes) >= exportCap {
		limitNote = fmt.Sprintf("\nAtenção: as anotações têm um limite de %d por arquivo; se você tem mais, as mais antigas estão aqui e as mais novas ficaram de fora.\n", exportCap)
	}
	if err := out.add("LEIA-ME.txt", func(w io.Writer) error {
		_, err := io.WriteString(w, fmt.Sprintf(exportReadme, now.Format("02/01/2006 15:04"), strings.ReplaceAll(fmt.Sprint(account["username"]), "\n", " "), limitNote))
		return err
	}); err != nil {
		return err
	}
	return zw.Close()
}

// exportNotes reads every note of a person, oldest first, up to exportCap.
func exportNotes(ctx context.Context, db *sql.DB, userID string) ([]Note, error) {
	rows, err := db.QueryContext(ctx, noteSelect+` WHERE n.user_id = $1 ORDER BY n.created_at, n.id LIMIT `+fmt.Sprint(exportCap), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}
