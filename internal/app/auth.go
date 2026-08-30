package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "bluescale_session"

type administratorContextKey struct{}

type administrator struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type sessionStore struct {
	db       *sql.DB
	lifetime time.Duration
}

func newSessionStore(db *sql.DB, lifetime time.Duration) *sessionStore {
	return &sessionStore{db: db, lifetime: lifetime}
}

func hashSessionToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func (s *sessionStore) create() (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(raw)
	expiresAt := time.Now().Add(s.lifetime)
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix()); err != nil {
		return "", time.Time{}, err
	}
	if _, err := s.db.Exec(`INSERT INTO sessions (token_hash, expires_at) VALUES (?, ?)`, hashSessionToken(token), expiresAt.Unix()); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (s *sessionStore) get(token string) (bool, error) {
	var expiresAt int64
	err := s.db.QueryRow(`SELECT expires_at FROM sessions WHERE token_hash = ?`, hashSessionToken(token)).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if expiresAt <= time.Now().Unix() {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashSessionToken(token))
		return false, nil
	}
	return true, nil
}

func (s *sessionStore) delete(token string) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashSessionToken(token))
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", Expires: expiresAt,
		MaxAge: int(time.Until(expiresAt).Seconds()), HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (a *App) loadAdministrator() (administrator, error) {
	var account administrator
	err := a.db.QueryRow(`SELECT id, username FROM administrators WHERE id = 1`).
		Scan(&account.ID, &account.Username)
	return account, err
}

func currentAdministrator(r *http.Request) (administrator, bool) {
	account, ok := r.Context().Value(administratorContextKey{}).(administrator)
	return account, ok
}

func bearerToken(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func (a *App) authenticateRequest(r *http.Request) (administrator, bool, error) {
	if token := bearerToken(r); token != "" {
		valid, err := a.authenticateAPIToken(token)
		if err != nil {
			return administrator{}, false, err
		}
		if valid {
			account, err := a.loadAdministrator()
			if errors.Is(err, sql.ErrNoRows) {
				return administrator{}, false, nil
			}
			return account, err == nil, err
		}
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return administrator{}, false, nil
	}
	valid, err := a.sessions.get(cookie.Value)
	if err != nil || !valid {
		return administrator{}, false, err
	}
	account, err := a.loadAdministrator()
	if errors.Is(err, sql.ErrNoRows) {
		return administrator{}, false, nil
	}
	return account, err == nil, err
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account, valid, err := a.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法验证身份凭据")
			return
		}
		if !valid {
			if _, cookieErr := r.Cookie(sessionCookieName); cookieErr == nil {
				clearSessionCookie(w, a.requestIsHTTPS(r))
			}
			writeError(w, http.StatusUnauthorized, "请登录或提供有效的 API Token")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), administratorContextKey{}, account)))
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
	username, validationErr := normalizeAdministratorUsername(request.Username)
	if validationErr != nil {
		writeError(w, http.StatusBadRequest, validationErr.Error())
		return
	}
	request.Username = username
	if err := validateAdministratorPassword([]byte(request.Password)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
	if _, err := tx.Exec(`INSERT INTO administrators (id, username, password_hash) VALUES (1, ?, ?)`, request.Username, string(hash)); err != nil {
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
	security := a.currentSettings().Security
	clientIP := a.clientIP(r)
	if security.LimitLoginFailures {
		allowed, retryAfter := a.loginAllowed(clientIP, security.MaxLoginFailures)
		if !allowed {
			seconds := int(retryAfter.Round(time.Second).Seconds())
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeError(w, http.StatusTooManyRequests, "登录失败次数过多，请在 15 分钟后重试")
			return
		}
	}
	loginFailed := func() {
		if security.LimitLoginFailures {
			a.recordLoginFailure(clientIP)
		}
		writeError(w, http.StatusUnauthorized, "用户名或密码不正确")
	}
	var account administrator
	var passwordHash string
	err := a.db.QueryRow(`SELECT id, username, password_hash
		FROM administrators WHERE username = ? COLLATE NOCASE`, request.Username).
		Scan(&account.ID, &account.Username, &passwordHash)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(request.Password))
		loginFailed()
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(request.Password)); err != nil {
		loginFailed()
		return
	}
	a.clearLoginFailure(clientIP)
	token, expiresAt, err := a.sessions.create()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建登录会话")
		return
	}
	setSessionCookie(w, token, expiresAt, a.requestIsHTTPS(r))
	writeJSON(w, http.StatusOK, account)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		a.sessions.delete(cookie.Value)
	}
	clearSessionCookie(w, a.requestIsHTTPS(r))
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	account, ok := currentAdministrator(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	writeJSON(w, http.StatusOK, account)
}

type updateMeRequest struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (a *App) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	account, ok := currentAdministrator(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	var request updateMeRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	username, validationErr := normalizeAdministratorUsername(request.Username)
	if validationErr != nil {
		writeError(w, http.StatusBadRequest, validationErr.Error())
		return
	}
	request.Username = username
	if request.NewPassword != "" {
		if err := validateAdministratorPassword([]byte(request.NewPassword)); err != nil {
			writeError(w, http.StatusBadRequest, "新"+err.Error())
			return
		}
	}

	var err error
	if request.NewPassword == "" {
		_, err = a.db.Exec(`UPDATE administrators SET username = ? WHERE id = ?`, request.Username, account.ID)
	} else {
		if request.CurrentPassword == "" {
			writeError(w, http.StatusBadRequest, "修改密码时请输入当前密码")
			return
		}
		var passwordHash string
		if err := a.db.QueryRow(`SELECT password_hash FROM administrators WHERE id = ?`, account.ID).Scan(&passwordHash); err != nil {
			writeError(w, http.StatusInternalServerError, "无法验证当前密码")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(request.CurrentPassword)); err != nil {
			writeError(w, http.StatusUnauthorized, "当前密码不正确")
			return
		}
		newHash, hashErr := bcrypt.GenerateFromPassword([]byte(request.NewPassword), bcrypt.DefaultCost)
		if hashErr != nil {
			writeError(w, http.StatusInternalServerError, "无法更新密码")
			return
		}
		tx, txErr := a.db.Begin()
		if txErr != nil {
			writeError(w, http.StatusInternalServerError, "无法更新管理员信息")
			return
		}
		defer tx.Rollback()
		if _, txErr = tx.Exec(`UPDATE administrators SET username = ?, password_hash = ? WHERE id = ?`, request.Username, string(newHash), account.ID); txErr == nil {
			if cookie, cookieErr := r.Cookie(sessionCookieName); cookieErr == nil {
				_, txErr = tx.Exec(`DELETE FROM sessions WHERE token_hash <> ?`, hashSessionToken(cookie.Value))
			} else {
				_, txErr = tx.Exec(`DELETE FROM sessions`)
			}
		}
		if txErr == nil {
			txErr = tx.Commit()
		}
		err = txErr
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法更新管理员信息")
		return
	}
	updated, err := a.loadAdministrator()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取管理员信息")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
