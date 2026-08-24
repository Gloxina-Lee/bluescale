package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
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

	var admin administrator
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
