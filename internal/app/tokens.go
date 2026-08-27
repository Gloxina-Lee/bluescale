package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const apiTokenMarker = "bsk_"

type apiTokenRecord struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	CreatedAt  string  `json:"createdAt,omitempty"`
	LastUsedAt *string `json:"lastUsedAt,omitempty"`
}

func initializeAPITokenDatabase(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			token_prefix TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			last_used_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_created_at ON api_tokens(created_at DESC, id DESC)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) authenticateAPIToken(token string) (bool, error) {
	if !strings.HasPrefix(token, apiTokenMarker) || len(token) < 32 {
		return false, nil
	}
	var id int64
	err := a.db.QueryRow(`SELECT id FROM api_tokens WHERE token_hash = ?`, hashSessionToken(token)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = a.db.Exec(`UPDATE api_tokens SET last_used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, id)
	return err == nil, err
}

func (a *App) handleListAPITokens(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	rows, err := a.db.Query(`SELECT id, name, token_prefix FROM api_tokens ORDER BY created_at DESC, id DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取 API Token")
		return
	}
	defer rows.Close()
	tokens := make([]apiTokenRecord, 0)
	for rows.Next() {
		var token apiTokenRecord
		if err := rows.Scan(&token.ID, &token.Name, &token.Prefix); err != nil {
			writeError(w, http.StatusInternalServerError, "无法读取 API Token")
			return
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取 API Token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens, "total": len(tokens)})
}

type createAPITokenRequest struct {
	Name string `json:"name"`
}

func (a *App) handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request createAPITokenRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		request.Name = "API Token"
	}
	if len([]rune(request.Name)) > 64 {
		writeError(w, http.StatusBadRequest, "Token 名称不能超过 64 个字符")
		return
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		writeError(w, http.StatusInternalServerError, "无法生成 API Token")
		return
	}
	rawToken := apiTokenMarker + base64.RawURLEncoding.EncodeToString(random)
	prefix := rawToken[:12] + "…"
	result, err := a.db.Exec(`INSERT INTO api_tokens (name, token_prefix, token_hash) VALUES (?, ?, ?)`, request.Name, prefix, hashSessionToken(rawToken))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存 API Token")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取 API Token")
		return
	}
	var token apiTokenRecord
	if err := a.db.QueryRow(`SELECT id, name, token_prefix, created_at, last_used_at FROM api_tokens WHERE id = ?`, id).
		Scan(&token.ID, &token.Name, &token.Prefix, &token.CreatedAt, &token.LastUsedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取 API Token")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": token.ID, "name": token.Name, "prefix": token.Prefix, "createdAt": token.CreatedAt,
		"lastUsedAt": token.LastUsedAt, "token": rawToken,
	})
}

func (a *App) handleGetAPIToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "Token ID 无效")
		return
	}
	var token apiTokenRecord
	err = a.db.QueryRow(`SELECT id, name, token_prefix, created_at, last_used_at FROM api_tokens WHERE id = ?`, id).
		Scan(&token.ID, &token.Name, &token.Prefix, &token.CreatedAt, &token.LastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "API Token 不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取 API Token")
		return
	}
	writeJSON(w, http.StatusOK, token)
}

type deleteAPITokensRequest struct {
	IDs []int64 `json:"ids"`
}

func (a *App) handleDeleteAPITokens(w http.ResponseWriter, r *http.Request) {
	var request deleteAPITokensRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	ids, err := normalizeIDs(request.IDs, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择 1–200 个 API Token")
		return
	}
	result, err := a.db.Exec(`DELETE FROM api_tokens WHERE id IN (`+placeholders(len(ids))+`)`, anyArguments(ids)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法删除 API Token")
		return
	}
	deleted, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}
