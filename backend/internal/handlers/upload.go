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
	"time"

	"github.com/redis/go-redis/v9"
)

// UploadHandler guarda a conexão com o Redis
type UploadHandler struct {
	DB          *sql.DB 
	RedisClient *redis.Client
}

// HandleUpload processa o form-data, salva o PDF e enfileira a tarefa
func (h *UploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	// 1. Limita o tamanho do upload (ex: 50MB)
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		http.Error(w, "Arquivo muito grande", http.StatusBadRequest)
		return
	}

	// 2. Extrai o arquivo com a chave 'document' (a mesma que usamos no FormData do React)
	file, header, err := r.FormFile("document")
	if err != nil {
		http.Error(w, "Erro ao ler o arquivo enviado", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 3. Prepara o diretório de destino
	uploadDir := "./uploads"
	os.MkdirAll(uploadDir, os.ModePerm)

	// Gera um nome de arquivo único usando o timestamp para evitar colisões
	fileName := fmt.Sprintf("%d_%s", time.Now().Unix(), header.Filename)
	filePath := filepath.Join(uploadDir, fileName)

	// 4. Salva o arquivo no disco
	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Erro ao salvar o arquivo no disco", http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

var workID int
	query := `INSERT INTO works (original_title) VALUES ($1) RETURNING id`
	
	err = h.DB.QueryRow(query, header.Filename).Scan(&workID)
	if err != nil {
		http.Error(w, "Erro ao criar registro no banco de dados", http.StatusInternalServerError)
		return
	}

	ctx := context.Background()
	absPath, _ := filepath.Abs(filePath)

	// Agora passamos o workID real para a fila!
	err = h.RedisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: "ingestion_tasks",
		Values: map[string]interface{}{
			"file_path": absPath,
			"work_id":   workID,
		},
	}).Err()

	if err != nil {
		http.Error(w, "Arquivo salvo, mas erro ao avisar a fila", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Upload concluído e enfileirado",
		"work_id": workID, // Devolvemos o ID real para o React também
	})
}