package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"bluescale/internal/app"
)

func TestAdministratorCLIRecovery(t *testing.T) {
	dataDir := createConfiguredDataDir(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"admin", "show-username", "--data-dir", dataDir}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("show-username exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"Admin"`) {
		t.Fatalf("show-username output = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"admin", "reset-username", "--data-dir", dataDir, "--yes", "CLIAdmin"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("reset-username exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"CLIAdmin"`) {
		t.Fatalf("reset-username output = %q", stdout.String())
	}

	const newPassword = "CLIRecoveredPassword123!"
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"admin", "reset-password", "--data-dir", dataDir, "--yes", "--password-stdin", "--revoke-api-tokens"}, strings.NewReader(newPassword+"\n"), &stdout, &stderr); code != 0 {
		t.Fatalf("reset-password exit=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), newPassword) || strings.Contains(stderr.String(), newPassword) {
		t.Fatal("password was exposed in command output")
	}

	application, err := app.New(app.Config{DataDir: dataDir, Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)
	response := postJSON(t, server.URL+"/api/login", map[string]string{"username": "CLIAdmin", "password": newPassword})
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login with recovered credentials status=%d, want 200", response.StatusCode)
	}
}

func TestAdministratorCLICancelAndPasswordStdinSafety(t *testing.T) {
	dataDir := createConfiguredDataDir(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"admin", "reset-username", "--data-dir", dataDir, "CancelledName"}, strings.NewReader("no\n"), &stdout, &stderr); code != 0 {
		t.Fatalf("cancelled reset exit=%d stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"admin", "show-username", "--data-dir", dataDir}, strings.NewReader(""), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), `"Admin"`) {
		t.Fatalf("username changed after cancellation: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"admin", "reset-password", "--data-dir", dataDir, "--password-stdin"}, strings.NewReader("PasswordThatMustNotBeConsumed\n"), &stdout, &stderr); code == 0 {
		t.Fatal("--password-stdin without --yes was accepted")
	}
	if !strings.Contains(stderr.String(), "必须同时指定 --yes") {
		t.Fatalf("unexpected password-stdin error: %s", stderr.String())
	}
}

func createConfiguredDataDir(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	application, err := app.New(app.Config{DataDir: dataDir, Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(application.Handler())
	response := postJSON(t, server.URL+"/api/setup", map[string]string{"username": "Admin", "password": "Admin123", "databaseType": "sqlite"})
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("setup status=%d, want 201", response.StatusCode)
	}
	server.Close()
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

func postJSON(t *testing.T, url string, payload any) *http.Response {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(url, "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	return response
}
