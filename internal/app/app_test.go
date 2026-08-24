package app

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/crypto/bcrypt"
)

func TestCompleteImageLifecycle(t *testing.T) {
	frontend := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>test</title>")},
	}
	application, err := New(Config{DataDir: t.TempDir(), Frontend: fs.FS(frontend)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	var status struct {
		Configured bool `json:"configured"`
	}
	doJSON(t, client, http.MethodGet, server.URL+"/api/status", nil, http.StatusOK, &status)
	if status.Configured {
		t.Fatal("new instance must not be configured")
	}

	setup := map[string]string{
		"displayName":  "测试管理员",
		"username":     "admin",
		"password":     "secure-password",
		"databaseType": "sqlite",
	}
	doJSON(t, client, http.MethodPost, server.URL+"/api/setup", setup, http.StatusCreated, nil)
	doJSON(t, client, http.MethodPost, server.URL+"/api/setup", setup, http.StatusConflict, nil)
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{"username": "admin", "password": "wrong-password"}, http.StatusUnauthorized, nil)

	var admin userAccount
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{"username": "admin", "password": "secure-password"}, http.StatusOK, &admin)
	if admin.DisplayName != "测试管理员" {
		t.Fatalf("unexpected administrator: %#v", admin)
	}

	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	part, err := writer.CreateFormFile("files", "pixel.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	uploadRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/images", &multipartBody)
	if err != nil {
		t.Fatal(err)
	}
	uploadRequest.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResponse, err := client.Do(uploadRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer uploadResponse.Body.Close()
	if uploadResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(uploadResponse.Body)
		t.Fatalf("upload status = %d, body = %s", uploadResponse.StatusCode, body)
	}
	var uploaded struct {
		Images []imageRecord `json:"images"`
	}
	if err := json.NewDecoder(uploadResponse.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}
	if len(uploaded.Images) != 1 || !strings.HasPrefix(uploaded.Images[0].URL, "/i/") {
		t.Fatalf("unexpected upload response: %#v", uploaded)
	}

	imageResponse, err := client.Get(server.URL + uploaded.Images[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	servedBytes, err := io.ReadAll(imageResponse.Body)
	imageResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if imageResponse.StatusCode != http.StatusOK || !bytes.Equal(servedBytes, pngBytes) {
		t.Fatalf("Go image response did not preserve the uploaded file")
	}
	if imageResponse.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected image content type: %s", imageResponse.Header.Get("Content-Type"))
	}

	var listing struct {
		Images []imageRecord `json:"images"`
		Total  int           `json:"total"`
	}
	doJSON(t, client, http.MethodGet, server.URL+"/api/images", nil, http.StatusOK, &listing)
	if listing.Total != 1 || len(listing.Images) != 1 {
		t.Fatalf("unexpected image listing: %#v", listing)
	}

	doJSON(t, client, http.MethodDelete, server.URL+"/api/images", map[string]any{"ids": []int64{uploaded.Images[0].ID}}, http.StatusNoContent, nil)
	deletedResponse, err := client.Get(server.URL + uploaded.Images[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	deletedResponse.Body.Close()
	if deletedResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted image status = %d, want 404", deletedResponse.StatusCode)
	}
}

func TestMultiUserPermissionsAndCRUD(t *testing.T) {
	application, err := New(Config{
		DataDir: t.TempDir(),
		Frontend: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("test")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)

	adminClient := newCookieClient(t)
	setup := map[string]string{
		"displayName": "主管理员", "username": "admin", "password": "secure-password", "databaseType": "sqlite",
	}
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/setup", setup, http.StatusCreated, nil)
	var admin userAccount
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/login", map[string]string{"username": "admin", "password": "secure-password"}, http.StatusOK, &admin)
	if !admin.Permissions.Upload || !admin.Permissions.ManageImages || !admin.Permissions.ManageUsers || admin.Group.Name != "Admin" {
		t.Fatalf("unexpected admin permissions: %#v", admin)
	}

	var groupListing struct {
		Groups []userGroupRecord `json:"groups"`
	}
	doJSON(t, adminClient, http.MethodGet, server.URL+"/api/user-groups", nil, http.StatusOK, &groupListing)
	var adminGroup, defaultGroup userGroupRecord
	for _, group := range groupListing.Groups {
		if group.Name == "Admin" {
			adminGroup = group
		}
		if group.IsDefault {
			defaultGroup = group
		}
	}
	if adminGroup.ID == 0 || defaultGroup.ID == 0 || !defaultGroup.Permissions.Upload || defaultGroup.Permissions.ManageImages || defaultGroup.Permissions.ManageUsers {
		t.Fatalf("unexpected built-in groups: %#v", groupListing.Groups)
	}

	groupPayload := map[string]any{
		"name": "内容编辑", "permissions": map[string]bool{"upload": true, "manageImages": true, "manageUsers": false},
	}
	var customGroup userGroupRecord
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/user-groups", groupPayload, http.StatusCreated, &customGroup)
	groupPayload["name"] = "编辑团队"
	doJSON(t, adminClient, http.MethodPut, server.URL+"/api/user-groups/"+strconv.FormatInt(customGroup.ID, 10), groupPayload, http.StatusOK, &customGroup)
	if customGroup.Name != "编辑团队" {
		t.Fatalf("group was not updated: %#v", customGroup)
	}

	userPayload := map[string]any{
		"displayName": "编辑用户", "username": "editor", "password": "editor-password", "groupId": customGroup.ID,
	}
	var editor managedUserRecord
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/users", userPayload, http.StatusCreated, &editor)
	doJSON(t, adminClient, http.MethodDelete, server.URL+"/api/user-groups/"+strconv.FormatInt(customGroup.ID, 10), nil, http.StatusConflict, nil)

	editorClient := newCookieClient(t)
	var editorLogin userAccount
	doJSON(t, editorClient, http.MethodPost, server.URL+"/api/login", map[string]string{"username": "editor", "password": "editor-password"}, http.StatusOK, &editorLogin)
	doJSON(t, editorClient, http.MethodGet, server.URL+"/api/images", nil, http.StatusOK, nil)
	doJSON(t, editorClient, http.MethodPost, server.URL+"/api/images", nil, http.StatusBadRequest, nil)
	doJSON(t, editorClient, http.MethodGet, server.URL+"/api/users", nil, http.StatusForbidden, nil)

	var defaultUser managedUserRecord
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/users", map[string]any{
		"displayName": "普通用户", "username": "viewer", "password": "viewer-password", "groupId": 0,
	}, http.StatusCreated, &defaultUser)
	if !defaultUser.Group.IsDefault {
		t.Fatalf("new user did not receive default group: %#v", defaultUser)
	}
	defaultClient := newCookieClient(t)
	doJSON(t, defaultClient, http.MethodPost, server.URL+"/api/login", map[string]string{"username": "viewer", "password": "viewer-password"}, http.StatusOK, nil)
	doJSON(t, defaultClient, http.MethodPost, server.URL+"/api/images", nil, http.StatusBadRequest, nil)
	doJSON(t, defaultClient, http.MethodGet, server.URL+"/api/images", nil, http.StatusForbidden, nil)
	doJSON(t, defaultClient, http.MethodGet, server.URL+"/api/users", nil, http.StatusForbidden, nil)

	userPayload["password"] = ""
	userPayload["groupId"] = defaultGroup.ID
	doJSON(t, adminClient, http.MethodPut, server.URL+"/api/users/"+strconv.FormatInt(editor.ID, 10), userPayload, http.StatusOK, &editor)
	doJSON(t, adminClient, http.MethodDelete, server.URL+"/api/user-groups/"+strconv.FormatInt(customGroup.ID, 10), nil, http.StatusNoContent, nil)
	doJSON(t, adminClient, http.MethodDelete, server.URL+"/api/users/"+strconv.FormatInt(editor.ID, 10), nil, http.StatusNoContent, nil)
	doJSON(t, editorClient, http.MethodGet, server.URL+"/api/me", nil, http.StatusUnauthorized, nil)
	doJSON(t, adminClient, http.MethodDelete, server.URL+"/api/users/"+strconv.FormatInt(admin.ID, 10), nil, http.StatusConflict, nil)
	doJSON(t, adminClient, http.MethodPut, server.URL+"/api/user-groups/"+strconv.FormatInt(adminGroup.ID, 10), groupPayload, http.StatusForbidden, nil)
}

func TestLegacyAdministratorMigration(t *testing.T) {
	dataDir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "bluescale.db"))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE administrators (
		id INTEGER PRIMARY KEY CHECK (id = 1), display_name TEXT NOT NULL,
		username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO administrators (id, display_name, username, password_hash) VALUES (1, ?, ?, ?)`, "旧管理员", "legacy", string(hash)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	application, err := New(Config{DataDir: dataDir, Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	var migrated userAccount
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{"username": "legacy", "password": "legacy-password"}, http.StatusOK, &migrated)
	if migrated.DisplayName != "旧管理员" || migrated.Group.Name != "Admin" || !migrated.Permissions.ManageUsers {
		t.Fatalf("legacy administrator was not migrated: %#v", migrated)
	}
}

func TestProtectedRouteRequiresLogin(t *testing.T) {
	application, err := New(Config{
		DataDir: t.TempDir(),
		Frontend: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("test")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	request := httptest.NewRequest(http.MethodGet, "/api/images", nil)
	recorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestUpdateCurrentUserProfile(t *testing.T) {
	application, err := New(Config{
		DataDir:  t.TempDir(),
		Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	doJSON(t, client, http.MethodPost, server.URL+"/api/setup", map[string]string{
		"displayName": "初始管理员", "username": "admin", "password": "secure-password", "databaseType": "sqlite",
	}, http.StatusCreated, nil)
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{"username": "admin", "password": "secure-password"}, http.StatusOK, nil)

	var updated userAccount
	doJSON(t, client, http.MethodPut, server.URL+"/api/me", map[string]string{
		"displayName": "新的昵称", "username": "renamed", "currentPassword": "", "newPassword": "",
	}, http.StatusOK, &updated)
	if updated.DisplayName != "新的昵称" || updated.Username != "renamed" {
		t.Fatalf("profile was not updated: %#v", updated)
	}
	doJSON(t, client, http.MethodPut, server.URL+"/api/me", map[string]string{
		"displayName": "新的昵称", "username": "renamed", "currentPassword": "wrong-password", "newPassword": "new-secure-password",
	}, http.StatusUnauthorized, nil)
	doJSON(t, client, http.MethodPut, server.URL+"/api/me", map[string]string{
		"displayName": "新的昵称", "username": "renamed", "currentPassword": "secure-password", "newPassword": "new-secure-password",
	}, http.StatusOK, nil)

	newClient := newCookieClient(t)
	doJSON(t, newClient, http.MethodPost, server.URL+"/api/login", map[string]string{"username": "renamed", "password": "secure-password"}, http.StatusUnauthorized, nil)
	doJSON(t, newClient, http.MethodPost, server.URL+"/api/login", map[string]string{"username": "renamed", "password": "new-secure-password"}, http.StatusOK, nil)
}

func doJSON(t *testing.T, client *http.Client, method, url string, body any, expectedStatus int, destination any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s status = %d, want %d; body = %s", method, url, response.StatusCode, expectedStatus, responseBody)
	}
	if destination != nil {
		if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
			t.Fatal(err)
		}
	}
}

func newCookieClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}
