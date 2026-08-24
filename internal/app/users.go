package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type userGroupRecord struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	IsSystem    bool        `json:"isSystem"`
	IsDefault   bool        `json:"isDefault"`
	Permissions permissions `json:"permissions"`
	UserCount   int         `json:"userCount"`
	CreatedAt   string      `json:"createdAt"`
}

type managedUserRecord struct {
	userAccount
	CreatedAt string `json:"createdAt"`
}

type groupRequest struct {
	Name        string      `json:"name"`
	Permissions permissions `json:"permissions"`
}

type userRequest struct {
	DisplayName string `json:"displayName"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	GroupID     int64  `json:"groupId"`
}

func parseResourceID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func validateGroupRequest(request *groupRequest) string {
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len([]rune(request.Name)) > 64 {
		return "用户组名称长度应为 1–64 个字符"
	}
	if !request.Permissions.Upload && !request.Permissions.ManageImages && !request.Permissions.ManageUsers {
		return "请至少为用户组选择一项权限"
	}
	return ""
}

func validateUserRequest(request *userRequest, passwordRequired bool) string {
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Username = strings.TrimSpace(request.Username)
	if request.DisplayName == "" || len([]rune(request.DisplayName)) > 64 {
		return "用户名称长度应为 1–64 个字符"
	}
	if request.Username == "" || len([]rune(request.Username)) > 64 {
		return "登录账号长度应为 1–64 个字符"
	}
	if passwordRequired && request.Password == "" {
		return "请设置登录密码"
	}
	if request.Password != "" && (len(request.Password) < 8 || len(request.Password) > 128) {
		return "密码长度应为 8–128 个字符"
	}
	return ""
}

func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

func (a *App) loadUserGroup(groupID int64) (userGroupRecord, error) {
	var group userGroupRecord
	var systemKey string
	var canUpload, canManageImages, canManageUsers int
	err := a.db.QueryRow(`SELECT g.id, g.name, COALESCE(g.system_key, ''),
		g.can_upload, g.can_manage_images, g.can_manage_users, COUNT(u.id), g.created_at
		FROM user_groups g LEFT JOIN users u ON u.group_id = g.id
		WHERE g.id = ? GROUP BY g.id`, groupID).
		Scan(&group.ID, &group.Name, &systemKey, &canUpload, &canManageImages, &canManageUsers, &group.UserCount, &group.CreatedAt)
	if err != nil {
		return userGroupRecord{}, err
	}
	group.IsSystem = systemKey != ""
	group.IsDefault = systemKey == "user"
	group.Permissions = permissions{Upload: canUpload == 1, ManageImages: canManageImages == 1, ManageUsers: canManageUsers == 1}
	return group, nil
}

func (a *App) handleListUserGroups(w http.ResponseWriter, _ *http.Request) {
	rows, err := a.db.Query(`SELECT g.id, g.name, COALESCE(g.system_key, ''),
		g.can_upload, g.can_manage_images, g.can_manage_users, COUNT(u.id), g.created_at
		FROM user_groups g LEFT JOIN users u ON u.group_id = g.id
		GROUP BY g.id
		ORDER BY CASE g.system_key WHEN 'admin' THEN 0 WHEN 'user' THEN 1 ELSE 2 END, lower(g.name)`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户组")
		return
	}
	defer rows.Close()
	groups := make([]userGroupRecord, 0)
	for rows.Next() {
		var group userGroupRecord
		var systemKey string
		var canUpload, canManageImages, canManageUsers int
		if err := rows.Scan(&group.ID, &group.Name, &systemKey, &canUpload, &canManageImages, &canManageUsers, &group.UserCount, &group.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "无法读取用户组")
			return
		}
		group.IsSystem = systemKey != ""
		group.IsDefault = systemKey == "user"
		group.Permissions = permissions{Upload: canUpload == 1, ManageImages: canManageImages == 1, ManageUsers: canManageUsers == 1}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户组")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (a *App) handleCreateUserGroup(w http.ResponseWriter, r *http.Request) {
	var request groupRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if message := validateGroupRequest(&request); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	result, err := a.db.Exec(`INSERT INTO user_groups (name, can_upload, can_manage_images, can_manage_users) VALUES (?, ?, ?, ?)`,
		request.Name, request.Permissions.Upload, request.Permissions.ManageImages, request.Permissions.ManageUsers)
	if isUniqueConstraintError(err) {
		writeError(w, http.StatusConflict, "用户组名称已存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建用户组")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取新用户组")
		return
	}
	group, err := a.loadUserGroup(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取新用户组")
		return
	}
	writeJSON(w, http.StatusCreated, group)
}

func (a *App) handleUpdateUserGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseResourceID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "用户组编号无效")
		return
	}
	var request groupRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if message := validateGroupRequest(&request); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	var systemKey string
	var currentlyManagesUsers int
	err = a.db.QueryRow(`SELECT COALESCE(system_key, ''), can_manage_users FROM user_groups WHERE id = ?`, id).Scan(&systemKey, &currentlyManagesUsers)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "用户组不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户组")
		return
	}
	if systemKey != "" {
		writeError(w, http.StatusForbidden, "内置用户组不可编辑")
		return
	}
	if currentlyManagesUsers == 1 && !request.Permissions.ManageUsers {
		var totalManagers, groupManagers int
		if err := a.db.QueryRow(`SELECT COUNT(*) FROM users u JOIN user_groups g ON g.id = u.group_id WHERE g.can_manage_users = 1`).Scan(&totalManagers); err != nil {
			writeError(w, http.StatusInternalServerError, "无法检查管理权限")
			return
		}
		if err := a.db.QueryRow(`SELECT COUNT(*) FROM users WHERE group_id = ?`, id).Scan(&groupManagers); err != nil {
			writeError(w, http.StatusInternalServerError, "无法检查管理权限")
			return
		}
		if totalManagers <= groupManagers {
			writeError(w, http.StatusConflict, "系统必须保留至少一个拥有用户管理权限的账号")
			return
		}
	}
	_, err = a.db.Exec(`UPDATE user_groups SET name = ?, can_upload = ?, can_manage_images = ?, can_manage_users = ?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
		request.Name, request.Permissions.Upload, request.Permissions.ManageImages, request.Permissions.ManageUsers, id)
	if isUniqueConstraintError(err) {
		writeError(w, http.StatusConflict, "用户组名称已存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法更新用户组")
		return
	}
	group, err := a.loadUserGroup(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户组")
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (a *App) handleDeleteUserGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseResourceID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "用户组编号无效")
		return
	}
	var systemKey string
	var userCount int
	err = a.db.QueryRow(`SELECT COALESCE(g.system_key, ''), COUNT(u.id) FROM user_groups g LEFT JOIN users u ON u.group_id = g.id WHERE g.id = ? GROUP BY g.id`, id).
		Scan(&systemKey, &userCount)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "用户组不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户组")
		return
	}
	if systemKey != "" {
		writeError(w, http.StatusForbidden, "内置用户组不可删除")
		return
	}
	if userCount > 0 {
		writeError(w, http.StatusConflict, "请先将该组中的用户移至其他用户组")
		return
	}
	if _, err := a.db.Exec(`DELETE FROM user_groups WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "无法删除用户组")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func scanManagedUser(scanner rowScanner) (managedUserRecord, error) {
	var user managedUserRecord
	var systemKey string
	var canUpload, canManageImages, canManageUsers int
	err := scanner.Scan(
		&user.ID, &user.DisplayName, &user.Username,
		&user.Group.ID, &user.Group.Name, &systemKey,
		&canUpload, &canManageImages, &canManageUsers, &user.CreatedAt,
	)
	if err != nil {
		return managedUserRecord{}, err
	}
	user.Group.IsSystem = systemKey != ""
	user.Group.IsDefault = systemKey == "user"
	user.Permissions = permissions{Upload: canUpload == 1, ManageImages: canManageImages == 1, ManageUsers: canManageUsers == 1}
	return user, nil
}

const managedUserSelect = `SELECT u.id, u.display_name, u.username, g.id, g.name,
	COALESCE(g.system_key, ''), g.can_upload, g.can_manage_images, g.can_manage_users, u.created_at
	FROM users u JOIN user_groups g ON g.id = u.group_id`

func (a *App) loadManagedUser(userID int64) (managedUserRecord, error) {
	return scanManagedUser(a.db.QueryRow(managedUserSelect+` WHERE u.id = ?`, userID))
}

func (a *App) handleListUsers(w http.ResponseWriter, _ *http.Request) {
	rows, err := a.db.Query(managedUserSelect + ` ORDER BY lower(u.display_name), u.id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户列表")
		return
	}
	defer rows.Close()
	users := make([]managedUserRecord, 0)
	for rows.Next() {
		user, err := scanManagedUser(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法读取用户列表")
			return
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户列表")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *App) resolveUserGroupID(groupID int64) (int64, permissions, error) {
	if groupID == 0 {
		err := a.db.QueryRow(`SELECT id FROM user_groups WHERE system_key = 'user'`).Scan(&groupID)
		if err != nil {
			return 0, permissions{}, err
		}
	}
	var groupPermissions permissions
	var canUpload, canManageImages, canManageUsers int
	err := a.db.QueryRow(`SELECT can_upload, can_manage_images, can_manage_users FROM user_groups WHERE id = ?`, groupID).
		Scan(&canUpload, &canManageImages, &canManageUsers)
	if err != nil {
		return 0, permissions{}, err
	}
	groupPermissions = permissions{Upload: canUpload == 1, ManageImages: canManageImages == 1, ManageUsers: canManageUsers == 1}
	return groupID, groupPermissions, nil
}

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var request userRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if message := validateUserRequest(&request, true); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	groupID, _, err := a.resolveUserGroupID(request.GroupID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "选择的用户组不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户组")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建用户")
		return
	}
	result, err := a.db.Exec(`INSERT INTO users (display_name, username, password_hash, group_id) VALUES (?, ?, ?, ?)`,
		request.DisplayName, request.Username, string(hash), groupID)
	if isUniqueConstraintError(err) {
		writeError(w, http.StatusConflict, "登录账号已存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建用户")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取新用户")
		return
	}
	user, err := a.loadManagedUser(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取新用户")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (a *App) countUserManagers() (int, error) {
	var count int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM users u JOIN user_groups g ON g.id = u.group_id WHERE g.can_manage_users = 1`).Scan(&count)
	return count, err
}

func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseResourceID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "用户编号无效")
		return
	}
	var request userRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if message := validateUserRequest(&request, false); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	current, err := a.loadManagedUser(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户")
		return
	}
	groupID, nextPermissions, err := a.resolveUserGroupID(request.GroupID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "选择的用户组不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户组")
		return
	}
	if current.Permissions.ManageUsers && !nextPermissions.ManageUsers {
		managerCount, err := a.countUserManagers()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法检查管理权限")
			return
		}
		if managerCount <= 1 {
			writeError(w, http.StatusConflict, "系统必须保留至少一个拥有用户管理权限的账号")
			return
		}
	}
	if request.Password == "" {
		_, err = a.db.Exec(`UPDATE users SET display_name = ?, username = ?, group_id = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, request.DisplayName, request.Username, groupID, id)
	} else {
		var hash []byte
		hash, err = bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
		if err == nil {
			_, err = a.db.Exec(`UPDATE users SET display_name = ?, username = ?, password_hash = ?, group_id = ?,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, request.DisplayName, request.Username, string(hash), groupID, id)
		}
	}
	if isUniqueConstraintError(err) {
		writeError(w, http.StatusConflict, "登录账号已存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法更新用户")
		return
	}
	user, err := a.loadManagedUser(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseResourceID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "用户编号无效")
		return
	}
	actor, _ := currentUser(r)
	if actor.ID == id {
		writeError(w, http.StatusConflict, "不能删除当前登录账号")
		return
	}
	target, err := a.loadManagedUser(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取用户")
		return
	}
	if target.Permissions.ManageUsers {
		managerCount, err := a.countUserManagers()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法检查管理权限")
			return
		}
		if managerCount <= 1 {
			writeError(w, http.StatusConflict, "系统必须保留至少一个拥有用户管理权限的账号")
			return
		}
	}
	if _, err := a.db.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "无法删除用户")
		return
	}
	a.sessions.deleteUser(id)
	w.WriteHeader(http.StatusNoContent)
}
