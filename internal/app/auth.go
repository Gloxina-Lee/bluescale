package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "bluescale_session"

const (
	permissionUpload       = "upload"
	permissionManageImages = "manageImages"
	permissionManageUsers  = "manageUsers"
)

type userContextKey struct{}

type permissions struct {
	Upload       bool `json:"upload"`
	ManageImages bool `json:"manageImages"`
	ManageUsers  bool `json:"manageUsers"`
}

func (p permissions) allows(permission string) bool {
	switch permission {
	case permissionUpload:
		return p.Upload
	case permissionManageImages:
		return p.ManageImages
	case permissionManageUsers:
		return p.ManageUsers
	default:
		return false
	}
}

type userGroupReference struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IsSystem  bool   `json:"isSystem"`
	IsDefault bool   `json:"isDefault"`
}

type userAccount struct {
	ID          int64              `json:"id"`
	DisplayName string             `json:"displayName"`
	Username    string             `json:"username"`
	Group       userGroupReference `json:"group"`
	Permissions permissions        `json:"permissions"`
}

type session struct {
	userID    int64
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

func (s *sessionStore) create(userID int64) (string, time.Time, error) {
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
	s.sessions[token] = session{userID: userID, expiresAt: expiresAt}
	return token, expiresAt, nil
}

func (s *sessionStore) get(token string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[token]
	if !ok || time.Now().After(current.expiresAt) {
		delete(s.sessions, token)
		return 0, false
	}
	return current.userID, true
}

func (s *sessionStore) delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *sessionStore) deleteUser(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, current := range s.sessions {
		if current.userID == userID {
			delete(s.sessions, token)
		}
	}
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

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUserAccount(scanner rowScanner) (userAccount, error) {
	var account userAccount
	var systemKey string
	var canUpload, canManageImages, canManageUsers int
	err := scanner.Scan(
		&account.ID, &account.DisplayName, &account.Username,
		&account.Group.ID, &account.Group.Name, &systemKey,
		&canUpload, &canManageImages, &canManageUsers,
	)
	if err != nil {
		return userAccount{}, err
	}
	account.Group.IsSystem = systemKey != ""
	account.Group.IsDefault = systemKey == "user"
	account.Permissions = permissions{
		Upload: canUpload == 1, ManageImages: canManageImages == 1, ManageUsers: canManageUsers == 1,
	}
	return account, nil
}

const accountSelect = `SELECT u.id, u.display_name, u.username, g.id, g.name,
	COALESCE(g.system_key, ''), g.can_upload, g.can_manage_images, g.can_manage_users
	FROM users u JOIN user_groups g ON g.id = u.group_id`

func (a *App) loadUserAccount(userID int64) (userAccount, error) {
	return scanUserAccount(a.db.QueryRow(accountSelect+` WHERE u.id = ?`, userID))
}

func currentUser(r *http.Request) (userAccount, bool) {
	user, ok := r.Context().Value(userContextKey{}).(userAccount)
	return user, ok
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		userID, ok := a.sessions.get(cookie.Value)
		if !ok {
			clearSessionCookie(w, requestIsHTTPS(r))
			writeError(w, http.StatusUnauthorized, "登录已过期，请重新登录")
			return
		}
		user, err := a.loadUserAccount(userID)
		if errors.Is(err, sql.ErrNoRows) {
			a.sessions.delete(cookie.Value)
			clearSessionCookie(w, requestIsHTTPS(r))
			writeError(w, http.StatusUnauthorized, "账号已不可用，请重新登录")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法读取登录账号")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	}
}

func (a *App) requirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		if !user.Permissions.allows(permission) {
			writeError(w, http.StatusForbidden, "当前用户组没有执行此操作的权限")
			return
		}
		next(w, r)
	})
}

func (a *App) handleStatus(w http.ResponseWriter, _ *http.Request) {
	var count int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取系统状态")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"configured": count > 0})
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
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
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
	var adminGroupID int64
	if err := tx.QueryRow(`SELECT id FROM user_groups WHERE system_key = 'admin'`).Scan(&adminGroupID); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取管理员组")
		return
	}
	if _, err := tx.Exec(`INSERT INTO users (display_name, username, password_hash, group_id) VALUES (?, ?, ?, ?)`, request.DisplayName, request.Username, string(hash), adminGroupID); err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存管理员账号")
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
	var user userAccount
	var passwordHash, systemKey string
	var canUpload, canManageImages, canManageUsers int
	err := a.db.QueryRow(`SELECT u.id, u.display_name, u.username, u.password_hash,
		g.id, g.name, COALESCE(g.system_key, ''), g.can_upload, g.can_manage_images, g.can_manage_users
		FROM users u JOIN user_groups g ON g.id = u.group_id WHERE u.username = ? COLLATE NOCASE`, request.Username).
		Scan(&user.ID, &user.DisplayName, &user.Username, &passwordHash, &user.Group.ID, &user.Group.Name, &systemKey, &canUpload, &canManageImages, &canManageUsers)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(request.Password))
		writeError(w, http.StatusUnauthorized, "账号或密码不正确")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(request.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "账号或密码不正确")
		return
	}
	user.Group.IsSystem = systemKey != ""
	user.Group.IsDefault = systemKey == "user"
	user.Permissions = permissions{Upload: canUpload == 1, ManageImages: canManageImages == 1, ManageUsers: canManageUsers == 1}
	token, expiresAt, err := a.sessions.create(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建登录会话")
		return
	}
	setSessionCookie(w, token, expiresAt, requestIsHTTPS(r))
	writeJSON(w, http.StatusOK, user)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		a.sessions.delete(cookie.Value)
	}
	clearSessionCookie(w, requestIsHTTPS(r))
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

type updateMeRequest struct {
	DisplayName     string `json:"displayName"`
	Username        string `json:"username"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (a *App) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	var request updateMeRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Username = strings.TrimSpace(request.Username)
	if request.DisplayName == "" || len([]rune(request.DisplayName)) > 64 {
		writeError(w, http.StatusBadRequest, "用户名称长度应为 1–64 个字符")
		return
	}
	if request.Username == "" || len([]rune(request.Username)) > 64 {
		writeError(w, http.StatusBadRequest, "登录账号长度应为 1–64 个字符")
		return
	}
	if request.NewPassword != "" && (len(request.NewPassword) < 8 || len(request.NewPassword) > 128) {
		writeError(w, http.StatusBadRequest, "新密码长度应为 8–128 个字符")
		return
	}

	var err error
	if request.NewPassword == "" {
		_, err = a.db.Exec(`UPDATE users SET display_name = ?, username = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, request.DisplayName, request.Username, user.ID)
	} else {
		if request.CurrentPassword == "" {
			writeError(w, http.StatusBadRequest, "修改密码时请输入当前密码")
			return
		}
		var passwordHash string
		if err := a.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, user.ID).Scan(&passwordHash); err != nil {
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
		_, err = a.db.Exec(`UPDATE users SET display_name = ?, username = ?, password_hash = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, request.DisplayName, request.Username, string(newHash), user.ID)
	}
	if isUniqueConstraintError(err) {
		writeError(w, http.StatusConflict, "登录账号已存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法更新个人信息")
		return
	}
	updated, err := a.loadUserAccount(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取个人信息")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
