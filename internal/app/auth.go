package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "bluescale_session"

type adminContextKey struct{}

type administrator struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"displayName"`
	Username    string `json:"username"`
}

type session struct {
	admin     administrator
	expiresAt time.Time
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
	lifetime time.Duration
}

func newSessionStore(lifetime time.Duration) *sessionStore {
	return &sessionStore{sessions: make(map[string]session), lifetime: lifetime}
}

func (s *sessionStore) create(admin administrator) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(raw)
	expiresAt := time.Now().Add(s.lifetime)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, existing := range s.sessions {
		if time.Now().After(existing.expiresAt) {
			delete(s.sessions, key)
		}
	}
	s.sessions[token] = session{admin: admin, expiresAt: expiresAt}
	return token, expiresAt, nil
}

func (s *sessionStore) get(token string) (administrator, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[token]
	if !ok || time.Now().After(current.expiresAt) {
		delete(s.sessions, token)
		return administrator{}, false
	}
	return current.admin, true
}

func (s *sessionStore) delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		admin, ok := a.sessions.get(cookie.Value)
		if !ok {
			clearSessionCookie(w, requestIsHTTPS(r))
			writeError(w, http.StatusUnauthorized, "登录已过期，请重新登录")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), adminContextKey{}, admin)))
	}
}

func (a *App) handleStatus(w http.ResponseWriter, _ *http.Request) {
	var count int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM administrators`).Scan(&count); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取系统状态")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"configured": count == 1})
}

type setupRequest struct {
	DisplayName  string `json:"displayName"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	DatabaseType string `json:"databaseType"`
}

func (a *App) handleSetup(w http.ResponseWriter, r *http.Request) {
	var request setupRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Username = strings.TrimSpace(request.Username)
	if request.DisplayName == "" || len([]rune(request.DisplayName)) > 64 {
		writeError(w, http.StatusBadRequest, "管理员名称长度应为 1–64 个字符")
		return
	}
	if request.Username == "" || len([]rune(request.Username)) > 64 {
		writeError(w, http.StatusBadRequest, "账号长度应为 1–64 个字符")
		return
	}
	if len(request.Password) < 8 || len(request.Password) > 128 {
		writeError(w, http.StatusBadRequest, "密码长度应为 8–128 个字符")
		return
	}
	if request.DatabaseType != "sqlite" {
		writeError(w, http.StatusBadRequest, "目前仅支持 SQLite")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建管理员")
		return
	}

	a.setupMu.Lock()
	defer a.setupMu.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法开始首次配置")
		return
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM administrators`).Scan(&count); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取配置状态")
		return
	}
	if count != 0 {
		writeError(w, http.StatusConflict, "系统已完成首次配置")
		return
	}
	if _, err := tx.Exec(`INSERT INTO administrators (id, display_name, username, password_hash) VALUES (1, ?, ?, ?)`, request.DisplayName, request.Username, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存管理员")
		return
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES ('database_type', ?)`, request.DatabaseType); err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存数据库设置")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "无法完成首次配置")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "首次配置已完成"})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("not-the-right-password"), bcrypt.DefaultCost)

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	request.Username = strings.TrimSpace(request.Username)
	var admin administrator
	var passwordHash string
	err := a.db.QueryRow(`SELECT id, display_name, username, password_hash FROM administrators WHERE username = ?`, request.Username).
		Scan(&admin.ID, &admin.DisplayName, &admin.Username, &passwordHash)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(request.Password))
		writeError(w, http.StatusUnauthorized, "账号或密码不正确")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(request.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "账号或密码不正确")
		return
	}
	token, expiresAt, err := a.sessions.create(admin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建登录会话")
		return
	}
	setSessionCookie(w, token, expiresAt, requestIsHTTPS(r))
	writeJSON(w, http.StatusOK, admin)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		a.sessions.delete(cookie.Value)
	}
	clearSessionCookie(w, requestIsHTTPS(r))
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	admin, ok := r.Context().Value(adminContextKey{}).(administrator)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	writeJSON(w, http.StatusOK, admin)
}

func constantTimeStringEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
