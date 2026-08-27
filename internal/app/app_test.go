package app

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

const (
	testAdminDisplayName = "BlueScale"
	testAdminUsername    = "Admin"
	testAdminPassword    = "Admin123"
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
	client := newCookieClient(t)

	var status struct {
		Configured bool `json:"configured"`
	}
	doJSON(t, client, http.MethodGet, server.URL+"/api/status", nil, http.StatusOK, &status)
	if status.Configured {
		t.Fatal("new instance must not be configured")
	}
	setupTestAdministrator(t, client, server.URL)
	doJSON(t, client, http.MethodPost, server.URL+"/api/setup", setupPayload(), http.StatusConflict, nil)
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": "wrong-password",
	}, http.StatusUnauthorized, nil)

	var account administrator
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusOK, &account)
	if account.DisplayName != testAdminDisplayName || account.Username != testAdminUsername {
		t.Fatalf("unexpected administrator: %#v", account)
	}

	uploaded := uploadTestPNG(t, client, server.URL, "pixel.png")
	imageResponse, err := client.Get(server.URL + uploaded.URL)
	if err != nil {
		t.Fatal(err)
	}
	servedBytes, err := io.ReadAll(imageResponse.Body)
	imageResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if imageResponse.StatusCode != http.StatusOK || !bytes.Equal(servedBytes, testPNGBytes(t)) {
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

	doJSON(t, client, http.MethodDelete, server.URL+"/api/images", map[string]any{"ids": []int64{uploaded.ID}}, http.StatusNoContent, nil)
	deletedResponse, err := client.Get(server.URL + uploaded.URL)
	if err != nil {
		t.Fatal(err)
	}
	deletedResponse.Body.Close()
	if deletedResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted image status = %d, want 404", deletedResponse.StatusCode)
	}
}

func TestSessionSurvivesServerRestart(t *testing.T) {
	dataDir := t.TempDir()
	frontend := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}}
	application, err := New(Config{DataDir: dataDir, Frontend: frontend})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(application.Handler())
	client := newCookieClient(t)
	setupTestAdministrator(t, client, server.URL)
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusOK, nil)

	firstURL := mustParseURL(t, server.URL)
	cookies := client.Jar.Cookies(firstURL)
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName {
		t.Fatalf("unexpected session cookies: %#v", cookies)
	}
	server.Close()
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(Config{DataDir: dataDir, Frontend: frontend})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restartedServer := httptest.NewServer(reopened.Handler())
	t.Cleanup(restartedServer.Close)
	client.Jar.SetCookies(mustParseURL(t, restartedServer.URL), cookies)

	var current administrator
	doJSON(t, client, http.MethodGet, restartedServer.URL+"/api/me", nil, http.StatusOK, &current)
	if current.DisplayName != testAdminDisplayName {
		t.Fatalf("session restored the wrong administrator: %#v", current)
	}
	var storedHash string
	if err := reopened.db.QueryRow(`SELECT token_hash FROM sessions`).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == cookies[0].Value || storedHash != hashSessionToken(cookies[0].Value) {
		t.Fatal("session token was not stored as the expected digest")
	}
}

func TestMultiUserDatabaseMigratesToSingleUser(t *testing.T) {
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "bluescale.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	legacyHash, err := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	currentHash, err := bcrypt.GenerateFromPassword([]byte("current-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE administrators (
			id INTEGER PRIMARY KEY CHECK (id = 1), display_name TEXT NOT NULL,
			username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE user_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE,
			system_key TEXT UNIQUE, can_upload INTEGER NOT NULL,
			can_manage_images INTEGER NOT NULL, can_manage_users INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT, display_name TEXT NOT NULL,
			username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
			group_id INTEGER NOT NULL REFERENCES user_groups(id),
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE images (
			id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER REFERENCES users(id),
			original_name TEXT NOT NULL, storage_name TEXT NOT NULL UNIQUE,
			mime_type TEXT NOT NULL, size INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE sessions (
			token_hash TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id),
			expires_at INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO administrators (id, display_name, username, password_hash) VALUES (1, ?, ?, ?)`, "旧资料", "legacy", string(legacyHash)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_groups (id, name, system_key, can_upload, can_manage_images, can_manage_users) VALUES
		(1, 'Admin', 'admin', 1, 1, 1), (2, 'User', 'user', 1, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, display_name, username, password_hash, group_id) VALUES
		(1, '当前管理员', 'current-admin', ?, 1), (2, '第二账号', 'second', ?, 2)`, string(currentHash), string(legacyHash)); err != nil {
		t.Fatal(err)
	}
	storageNames := []string{
		"0123456789abcdef0123456789abcdef.png",
		"fedcba9876543210fedcba9876543210.png",
	}
	for index, storageName := range storageNames {
		if _, err := db.Exec(`INSERT INTO images (user_id, original_name, storage_name, mime_type, size) VALUES (?, ?, ?, 'image/png', ?)`, index+1, "legacy.png", storageName, len("image-data")); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	imagesDir := filepath.Join(dataDir, "images")
	for index, storageName := range storageNames {
		directory := filepath.Join(imagesDir, strconv.Itoa(index+1))
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, storageName), []byte("image-data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	application, err := New(Config{DataDir: dataDir, Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)
	client := newCookieClient(t)

	var migrated administrator
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": "current-admin", "password": "current-password",
	}, http.StatusOK, &migrated)
	if migrated.DisplayName != "当前管理员" {
		t.Fatalf("unexpected migrated administrator: %#v", migrated)
	}

	var listing struct {
		Images []imageRecord `json:"images"`
		Total  int           `json:"total"`
	}
	doJSON(t, client, http.MethodGet, server.URL+"/api/images", nil, http.StatusOK, &listing)
	if listing.Total != 2 || len(listing.Images) != 2 {
		t.Fatalf("all prior images were not retained: %#v", listing)
	}
	for _, storageName := range storageNames {
		if stored, err := os.ReadFile(filepath.Join(imagesDir, storageName)); err != nil || string(stored) != "image-data" {
			t.Fatalf("image %s was not migrated to shared storage: %v", storageName, err)
		}
	}
	for _, directory := range []string{"1", "2"} {
		if _, err := os.Stat(filepath.Join(imagesDir, directory)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy user image directory %s still exists", directory)
		}
	}

	for _, table := range []string{"users", "user_groups"} {
		exists, err := tableExists(application.db, table)
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("legacy table %s still exists", table)
		}
	}
	hasOwner, err := tableHasColumn(application.db, "images", "user_id")
	if err != nil {
		t.Fatal(err)
	}
	if hasOwner {
		t.Fatal("images.user_id still exists")
	}
	doJSON(t, client, http.MethodGet, server.URL+"/api/users", nil, http.StatusNotFound, nil)
	doJSON(t, client, http.MethodGet, server.URL+"/api/user-groups", nil, http.StatusNotFound, nil)
}

func TestProtectedRoutesRequireLogin(t *testing.T) {
	application, err := New(Config{
		DataDir:  t.TempDir(),
		Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	for _, path := range []string{"/api/me", "/api/settings", "/api/tokens"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		application.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s status = %d, want 401", path, recorder.Code)
		}
	}
	for _, path := range []string{"/api/images", "/api/albums"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		application.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("anonymous GET %s status = %d, want 200", path, recorder.Code)
		}
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/images", nil),
		httptest.NewRequest(http.MethodDelete, "/api/images", nil),
		httptest.NewRequest(http.MethodPut, "/api/images/visibility", nil),
		httptest.NewRequest(http.MethodPost, "/api/albums", nil),
	} {
		recorder := httptest.NewRecorder()
		application.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s %s status = %d, want 401", request.Method, request.URL.Path, recorder.Code)
		}
	}
}

func TestAnonymousReadsOnlyPublicImages(t *testing.T) {
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
	adminClient := newCookieClient(t)
	setupTestAdministrator(t, adminClient, server.URL)
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusOK, nil)

	var album albumRecord
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/albums", map[string]string{"name": "公开相册"}, http.StatusCreated, &album)
	privateImage := uploadTestPNGToAlbums(t, adminClient, server.URL, "private.png", []int64{album.ID})
	if privateImage.IsPublic {
		t.Fatal("new uploads must be private by default")
	}

	publicClient := &http.Client{}
	type imageListing struct {
		Images []imageRecord `json:"images"`
		Total  int           `json:"total"`
	}
	var anonymousImages imageListing
	doJSON(t, publicClient, http.MethodGet, server.URL+"/api/images", nil, http.StatusOK, &anonymousImages)
	if anonymousImages.Total != 0 || len(anonymousImages.Images) != 0 {
		t.Fatalf("private image leaked into anonymous listing: %#v", anonymousImages)
	}
	var anonymousAlbums struct {
		Albums []albumRecord `json:"albums"`
	}
	doJSON(t, publicClient, http.MethodGet, server.URL+"/api/albums", nil, http.StatusOK, &anonymousAlbums)
	if len(anonymousAlbums.Albums) != 1 || anonymousAlbums.Albums[0].ImageCount != 0 {
		t.Fatalf("anonymous album count included private images: %#v", anonymousAlbums)
	}
	privateResponse, err := publicClient.Get(server.URL + privateImage.URL)
	if err != nil {
		t.Fatal(err)
	}
	privateResponse.Body.Close()
	if privateResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("anonymous private image status = %d, want 404", privateResponse.StatusCode)
	}

	doJSON(t, adminClient, http.MethodPut, server.URL+"/api/images/visibility", map[string]any{
		"ids": []int64{privateImage.ID}, "isPublic": true,
	}, http.StatusOK, nil)
	doJSON(t, publicClient, http.MethodGet, server.URL+"/api/images", nil, http.StatusOK, &anonymousImages)
	if anonymousImages.Total != 1 || len(anonymousImages.Images) != 1 || !anonymousImages.Images[0].IsPublic {
		t.Fatalf("public image missing from anonymous listing: %#v", anonymousImages)
	}
	doJSON(t, publicClient, http.MethodGet, server.URL+"/api/albums", nil, http.StatusOK, &anonymousAlbums)
	if anonymousAlbums.Albums[0].ImageCount != 1 {
		t.Fatalf("anonymous public album count = %d, want 1", anonymousAlbums.Albums[0].ImageCount)
	}
	publicResponse, err := publicClient.Get(server.URL + privateImage.URL)
	if err != nil {
		t.Fatal(err)
	}
	publicResponse.Body.Close()
	if publicResponse.StatusCode != http.StatusOK || publicResponse.Header.Get("Cache-Control") != "public, max-age=0, must-revalidate" {
		t.Fatalf("public image response status=%d cache=%q", publicResponse.StatusCode, publicResponse.Header.Get("Cache-Control"))
	}

	doJSON(t, adminClient, http.MethodPut, server.URL+"/api/images/visibility", map[string]any{
		"ids": []int64{privateImage.ID}, "isPublic": false,
	}, http.StatusOK, nil)
	revokedResponse, err := publicClient.Get(server.URL + privateImage.URL)
	if err != nil {
		t.Fatal(err)
	}
	revokedResponse.Body.Close()
	if revokedResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("image remained anonymous after becoming private: %d", revokedResponse.StatusCode)
	}
}

func TestAPITokenLifecycleAndAuthorization(t *testing.T) {
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
	adminClient := newCookieClient(t)
	setupTestAdministrator(t, adminClient, server.URL)
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusOK, nil)

	type createdToken struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	var first, second createdToken
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/tokens", map[string]string{"name": "自动上传"}, http.StatusCreated, &first)
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/tokens", map[string]string{"name": "备用"}, http.StatusCreated, &second)
	if !strings.HasPrefix(first.Token, apiTokenMarker) || first.Token == second.Token {
		t.Fatalf("unexpected generated tokens: %#v %#v", first, second)
	}
	var storedHash string
	if err := application.db.QueryRow(`SELECT token_hash FROM api_tokens WHERE id = ?`, first.ID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == first.Token || storedHash != hashSessionToken(first.Token) {
		t.Fatal("API token was not stored as the expected digest")
	}

	var createdAlbum albumRecord
	doJSONWithBearer(t, http.MethodPost, server.URL+"/api/albums", map[string]string{"name": "Token 创建"}, first.Token, http.StatusCreated, &createdAlbum)
	privateImage := uploadTestPNGWithBearer(t, server.URL, "token-private.png", first.Token)
	deletedImage := uploadTestPNGWithBearer(t, server.URL, "token-delete.png", first.Token)
	doJSONWithBearer(t, http.MethodDelete, server.URL+"/api/images", map[string]any{"ids": []int64{deletedImage.ID}}, first.Token, http.StatusNoContent, nil)
	deletedResponse, err := http.Get(server.URL + deletedImage.URL)
	if err != nil {
		t.Fatal(err)
	}
	deletedResponse.Body.Close()
	if deletedResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("token-authenticated delete did not remove image: %d", deletedResponse.StatusCode)
	}
	privateRequest, err := http.NewRequest(http.MethodGet, server.URL+privateImage.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	privateRequest.Header.Set("Authorization", "Bearer "+first.Token)
	privateResponse, err := http.DefaultClient.Do(privateRequest)
	if err != nil {
		t.Fatal(err)
	}
	privateResponse.Body.Close()
	if privateResponse.StatusCode != http.StatusOK || privateResponse.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("token private image status=%d cache=%q", privateResponse.StatusCode, privateResponse.Header.Get("Cache-Control"))
	}

	var detail apiTokenRecord
	doJSON(t, adminClient, http.MethodGet, server.URL+"/api/tokens/"+strconv.FormatInt(first.ID, 10), nil, http.StatusOK, &detail)
	if detail.LastUsedAt == nil || detail.CreatedAt == "" {
		t.Fatalf("token detail did not track timestamps: %#v", detail)
	}
	var tokenList struct {
		Tokens []apiTokenRecord `json:"tokens"`
		Total  int              `json:"total"`
	}
	doJSON(t, adminClient, http.MethodGet, server.URL+"/api/tokens", nil, http.StatusOK, &tokenList)
	if tokenList.Total != 2 || len(tokenList.Tokens) != 2 {
		t.Fatalf("unexpected API token list: %#v", tokenList)
	}

	doJSON(t, adminClient, http.MethodDelete, server.URL+"/api/tokens", map[string]any{"ids": []int64{first.ID, second.ID}}, http.StatusOK, nil)
	doJSONWithBearer(t, http.MethodPost, server.URL+"/api/albums", map[string]string{"name": "应失败"}, first.Token, http.StatusUnauthorized, nil)
	revokedRequest, err := http.NewRequest(http.MethodGet, server.URL+privateImage.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	revokedRequest.Header.Set("Authorization", "Bearer "+first.Token)
	revokedResponse, err := http.DefaultClient.Do(revokedRequest)
	if err != nil {
		t.Fatal(err)
	}
	revokedResponse.Body.Close()
	if revokedResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked token still read a private image: %d", revokedResponse.StatusCode)
	}
	var publicListing map[string]any
	doJSONWithBearer(t, http.MethodGet, server.URL+"/api/images", nil, first.Token, http.StatusOK, &publicListing)
}

func TestUpdateAdministratorProfile(t *testing.T) {
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
	setupTestAdministrator(t, client, server.URL)
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusOK, nil)

	var updated administrator
	doJSON(t, client, http.MethodPut, server.URL+"/api/me", map[string]string{
		"displayName": "新的名称", "username": "Renamed", "currentPassword": "", "newPassword": "",
	}, http.StatusOK, &updated)
	if updated.DisplayName != "新的名称" || updated.Username != "Renamed" {
		t.Fatalf("profile was not updated: %#v", updated)
	}
	doJSON(t, client, http.MethodPut, server.URL+"/api/me", map[string]string{
		"displayName": "新的名称", "username": "Renamed", "currentPassword": "wrong-password", "newPassword": "new-secure-password",
	}, http.StatusUnauthorized, nil)
	doJSON(t, client, http.MethodPut, server.URL+"/api/me", map[string]string{
		"displayName": "新的名称", "username": "Renamed", "currentPassword": testAdminPassword, "newPassword": "new-secure-password",
	}, http.StatusOK, nil)

	newClient := newCookieClient(t)
	doJSON(t, newClient, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": "Renamed", "password": testAdminPassword,
	}, http.StatusUnauthorized, nil)
	doJSON(t, newClient, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": "Renamed", "password": "new-secure-password",
	}, http.StatusOK, nil)
}

func TestSettingsControlUploadLimitsConversionAndRenaming(t *testing.T) {
	dataDir := t.TempDir()
	application, err := New(Config{
		DataDir:  dataDir,
		Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	setupTestAdministrator(t, client, server.URL)
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusOK, nil)

	var settings applicationSettings
	doJSON(t, client, http.MethodGet, server.URL+"/api/settings", nil, http.StatusOK, &settings)
	if settings.Upload.MaxImageSizeMB != 25 || settings.Upload.MaxImagesPerUpload != 50 {
		t.Fatalf("unexpected default upload settings: %#v", settings.Upload)
	}
	settings.Upload.ConvertImages = true
	settings.Upload.TargetImageFormat = "jpeg"
	settings.Upload.CompressionQuality = 76
	settings.Upload.RenameImages = true
	settings.Upload.RenameMethod = "uuid_v4"
	settings.Upload.StripUUIDHyphens = false
	doJSON(t, client, http.MethodPut, server.URL+"/api/settings", settings, http.StatusOK, &settings)

	converted := uploadTestPNG(t, client, server.URL, "透明像素.png")
	if converted.MimeType != "image/jpeg" || !regexp.MustCompile(`^[0-9a-f-]{36}\.jpg$`).MatchString(converted.StorageName) || converted.StorageName[14] != '4' {
		t.Fatalf("unexpected converted image: %#v", converted)
	}
	response, err := client.Get(server.URL + converted.URL)
	if err != nil {
		t.Fatal(err)
	}
	convertedBytes, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/jpeg" || len(convertedBytes) < 2 || convertedBytes[0] != 0xff || convertedBytes[1] != 0xd8 {
		t.Fatalf("converted image was not served as JPEG: status=%d type=%q bytes=%x", response.StatusCode, response.Header.Get("Content-Type"), convertedBytes[:min(4, len(convertedBytes))])
	}

	settings.Upload.ConvertImages = false
	settings.Upload.RenameImages = false
	doJSON(t, client, http.MethodPut, server.URL+"/api/settings", settings, http.StatusOK, nil)
	firstNamed := uploadTestPNG(t, client, server.URL, "my photo.png")
	secondNamed := uploadTestPNG(t, client, server.URL, "my photo.png")
	if firstNamed.StorageName != "my photo.png" || secondNamed.StorageName != "my photo-2.png" {
		t.Fatalf("original-name collision handling failed: %q, %q", firstNamed.StorageName, secondNamed.StorageName)
	}
	settings.Upload.RenameImages = true
	settings.Upload.RenameMethod = "uuid_v5"
	settings.Upload.StripUUIDHyphens = true
	doJSON(t, client, http.MethodPut, server.URL+"/api/settings", settings, http.StatusOK, nil)
	firstV5 := uploadTestPNG(t, client, server.URL, "content.png")
	secondV5 := uploadTestPNG(t, client, server.URL, "content-copy.png")
	v5Pattern := regexp.MustCompile(`^[0-9a-f]{32}\.png$`)
	if !v5Pattern.MatchString(firstV5.StorageName) || firstV5.StorageName[12] != '5' || firstV5.StorageName == secondV5.StorageName {
		t.Fatalf("unexpected UUIDv5 names: %q, %q", firstV5.StorageName, secondV5.StorageName)
	}

	settings.Upload.MaxImageSizeMB = 1
	settings.Upload.MaxImagesPerUpload = 1
	doJSON(t, client, http.MethodPut, server.URL+"/api/settings", settings, http.StatusOK, nil)
	status, body := uploadTestFiles(t, client, server.URL, []testUploadFile{
		{name: "one.png", data: testPNGBytes(t)},
		{name: "two.png", data: testPNGBytes(t)},
	})
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte("单次最多上传 1 张图片")) {
		t.Fatalf("upload-count limit status=%d body=%s", status, body)
	}
	oversized := append(append([]byte(nil), testPNGBytes(t)...), make([]byte, (1<<20)+1)...)
	status, body = uploadTestFiles(t, client, server.URL, []testUploadFile{{name: "large.png", data: oversized}})
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte("超过 1 MB 限制")) {
		t.Fatalf("upload-size limit status=%d body=%s", status, body)
	}

	settings.Upload.MaxImageSizeMB = 0
	doJSON(t, client, http.MethodPut, server.URL+"/api/settings", settings, http.StatusBadRequest, nil)
	var persisted string
	if err := application.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, applicationSettingsKey).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(persisted, `"maxImageSizeMB":1`) {
		t.Fatalf("settings were not persisted: %s", persisted)
	}
}

func TestImageConversionTargets(t *testing.T) {
	for _, target := range []string{"jpeg", "png", "webp", "avif"} {
		t.Run(target, func(t *testing.T) {
			var converted bytes.Buffer
			mimeType, extension, err := convertImage(&converted, bytes.NewReader(testPNGBytes(t)), "image/png", target, 80)
			if err != nil {
				t.Fatal(err)
			}
			if supportedImageTypes[mimeType] != extension {
				t.Fatalf("mismatched conversion result: %q %q", mimeType, extension)
			}
			config, err := decodeImageConfig(bytes.NewReader(converted.Bytes()), mimeType)
			if err != nil {
				t.Fatalf("converted output cannot be decoded: %v", err)
			}
			if config.Width != 1 || config.Height != 1 {
				t.Fatalf("unexpected converted dimensions: %#v", config)
			}
		})
	}
}

func TestLoginFailureLimitAndProxySourceIP(t *testing.T) {
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
	adminClient := newCookieClient(t)
	setupTestAdministrator(t, adminClient, server.URL)
	doJSON(t, adminClient, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusOK, nil)
	settings := defaultApplicationSettings()
	settings.Security.LimitLoginFailures = true
	settings.Security.MaxLoginFailures = 2
	doJSON(t, adminClient, http.MethodPut, server.URL+"/api/settings", settings, http.StatusOK, nil)

	limitedClient := newCookieClient(t)
	for range 2 {
		doJSON(t, limitedClient, http.MethodPost, server.URL+"/api/login", map[string]string{
			"username": testAdminUsername, "password": "wrong-password",
		}, http.StatusUnauthorized, nil)
	}
	doJSON(t, limitedClient, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusTooManyRequests, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	request.RemoteAddr = "10.0.0.9:3210"
	request.Header.Set("X-Forwarded-For", "203.0.113.18, 10.0.0.2")
	if got := application.clientIP(request); got != "10.0.0.9" {
		t.Fatalf("proxy header was trusted while proxy mode was disabled: %q", got)
	}
	settings.Security.ReverseProxyMode = true
	settings.Security.RealIPHeader = "X-Forwarded-For"
	application.settingsMu.Lock()
	application.settings = settings
	application.settingsMu.Unlock()
	if got := application.clientIP(request); got != "203.0.113.18" {
		t.Fatalf("proxy source IP = %q, want 203.0.113.18", got)
	}

	var logs bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	log.SetOutput(&logs)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})
	recorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(logs.String(), "203.0.113.18 GET /api/status") {
		t.Fatalf("request log did not include source IP: status=%d log=%q", recorder.Code, logs.String())
	}
}

func TestAlbumsFilteringPaginationAndRelationships(t *testing.T) {
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
	setupTestAdministrator(t, client, server.URL)
	doJSON(t, client, http.MethodPost, server.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusOK, nil)

	createAlbum := func(name string) albumRecord {
		t.Helper()
		var album albumRecord
		doJSON(t, client, http.MethodPost, server.URL+"/api/albums", map[string]string{"name": name}, http.StatusCreated, &album)
		return album
	}
	travel := createAlbum("旅行")
	favorites := createAlbum("收藏")
	archive := createAlbum("待合并")
	doJSON(t, client, http.MethodPost, server.URL+"/api/albums", map[string]string{"name": "旅行"}, http.StatusConflict, nil)

	unassigned := uploadTestPNG(t, client, server.URL, "unassigned.png")
	shared := uploadTestPNGToAlbums(t, client, server.URL, "shared.png", []int64{travel.ID, favorites.ID})
	archived := uploadTestPNGToAlbums(t, client, server.URL, "archived.png", []int64{archive.ID})

	type listingResponse struct {
		Images     []imageRecord `json:"images"`
		Total      int           `json:"total"`
		Page       int           `json:"page"`
		PageSize   int           `json:"pageSize"`
		TotalPages int           `json:"totalPages"`
	}
	var pageTwo listingResponse
	doJSON(t, client, http.MethodGet, server.URL+"/api/images?page=2&pageSize=1", nil, http.StatusOK, &pageTwo)
	if pageTwo.Total != 3 || pageTwo.Page != 2 || pageTwo.PageSize != 1 || pageTwo.TotalPages != 3 || len(pageTwo.Images) != 1 {
		t.Fatalf("unexpected paginated listing: %#v", pageTwo)
	}
	var pngListing listingResponse
	doJSON(t, client, http.MethodGet, server.URL+"/api/images?format=png&pageSize=2", nil, http.StatusOK, &pngListing)
	if pngListing.Total != 3 || len(pngListing.Images) != 2 || pngListing.TotalPages != 2 {
		t.Fatalf("unexpected format-filtered listing: %#v", pngListing)
	}
	doJSON(t, client, http.MethodGet, server.URL+"/api/images?format=tiff", nil, http.StatusBadRequest, nil)
	doJSON(t, client, http.MethodGet, server.URL+"/api/images?pageSize=201", nil, http.StatusBadRequest, nil)

	var travelListing listingResponse
	doJSON(t, client, http.MethodGet, server.URL+"/api/images?album="+strconv.FormatInt(travel.ID, 10), nil, http.StatusOK, &travelListing)
	if travelListing.Total != 1 || travelListing.Images[0].ID != shared.ID {
		t.Fatalf("unexpected album-filtered listing: %#v", travelListing)
	}
	var unassignedListing listingResponse
	doJSON(t, client, http.MethodGet, server.URL+"/api/images?album=none", nil, http.StatusOK, &unassignedListing)
	if unassignedListing.Total != 1 || unassignedListing.Images[0].ID != unassigned.ID {
		t.Fatalf("unexpected unassigned-image listing: %#v", unassignedListing)
	}
	doJSON(t, client, http.MethodPost, server.URL+"/api/images/albums", map[string]any{
		"imageIds": []int64{unassigned.ID}, "albumIds": []int64{travel.ID, archive.ID},
	}, http.StatusNoContent, nil)
	doJSON(t, client, http.MethodDelete, server.URL+"/api/images/albums", map[string]any{
		"imageIds": []int64{shared.ID}, "albumIds": []int64{favorites.ID},
	}, http.StatusNoContent, nil)

	doJSON(t, client, http.MethodPost, server.URL+"/api/albums/merge", map[string]any{
		"ids": []int64{travel.ID, favorites.ID, archive.ID}, "targetId": travel.ID,
	}, http.StatusOK, nil)
	var albumsPayload struct {
		Albums []albumRecord `json:"albums"`
		Total  int           `json:"total"`
	}
	doJSON(t, client, http.MethodGet, server.URL+"/api/albums", nil, http.StatusOK, &albumsPayload)
	if albumsPayload.Total != 1 || len(albumsPayload.Albums) != 1 || albumsPayload.Albums[0].ID != travel.ID || albumsPayload.Albums[0].ImageCount != 3 {
		t.Fatalf("albums were not merged as a relation union: %#v", albumsPayload)
	}

	doJSON(t, client, http.MethodDelete, server.URL+"/api/albums", map[string]any{"ids": []int64{travel.ID}}, http.StatusOK, nil)
	var afterAlbumDelete listingResponse
	doJSON(t, client, http.MethodGet, server.URL+"/api/images", nil, http.StatusOK, &afterAlbumDelete)
	if afterAlbumDelete.Total != 3 {
		t.Fatalf("deleting an album deleted image records: %#v", afterAlbumDelete)
	}
	for _, image := range []imageRecord{unassigned, shared, archived} {
		response, err := client.Get(server.URL + image.URL)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("image %d disappeared after album deletion: %d", image.ID, response.StatusCode)
		}
	}
}

func setupPayload() map[string]string {
	return map[string]string{
		"displayName": testAdminDisplayName, "username": testAdminUsername,
		"password": testAdminPassword, "databaseType": "sqlite",
	}
}

func setupTestAdministrator(t *testing.T, client *http.Client, serverURL string) {
	t.Helper()
	doJSON(t, client, http.MethodPost, serverURL+"/api/setup", setupPayload(), http.StatusCreated, nil)
}

func uploadTestPNG(t *testing.T, client *http.Client, serverURL, filename string) imageRecord {
	t.Helper()
	return uploadTestPNGToAlbums(t, client, serverURL, filename, nil)
}

func uploadTestPNGToAlbums(t *testing.T, client *http.Client, serverURL, filename string, albumIDs []int64) imageRecord {
	return uploadTestPNGWithCredential(t, client, serverURL, filename, albumIDs, "")
}

func uploadTestPNGWithBearer(t *testing.T, serverURL, filename, token string) imageRecord {
	t.Helper()
	return uploadTestPNGWithCredential(t, http.DefaultClient, serverURL, filename, nil, token)
}

func uploadTestPNGWithCredential(t *testing.T, client *http.Client, serverURL, filename string, albumIDs []int64, token string) imageRecord {
	t.Helper()
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	encodedAlbumIDs, err := json.Marshal(albumIDs)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("albumIds", string(encodedAlbumIDs)); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(testPNGBytes(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, serverURL+"/api/images", &multipartBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("upload status = %d, body = %s", response.StatusCode, body)
	}
	var payload struct {
		Images []imageRecord `json:"images"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Images) != 1 || !strings.HasPrefix(payload.Images[0].URL, "/i/") {
		t.Fatalf("unexpected upload response: %#v", payload)
	}
	return payload.Images[0]
}

type testUploadFile struct {
	name string
	data []byte
}

func uploadTestFiles(t *testing.T, client *http.Client, serverURL string, files []testUploadFile) (int, []byte) {
	t.Helper()
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	for _, file := range files {
		part, err := writer.CreateFormFile("files", file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, serverURL+"/api/images", &multipartBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body
}

func testPNGBytes(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	return data
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

func doJSONWithBearer(t *testing.T, method, url string, body any, token string, expectedStatus int, destination any) {
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
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
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

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
