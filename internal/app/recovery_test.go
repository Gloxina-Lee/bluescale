package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"testing/fstest"

	"golang.org/x/crypto/bcrypt"
)

func TestAdministratorRecoveryRequiresStoppedInstanceAndRevokesCredentials(t *testing.T) {
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
	if _, err := application.db.Exec(`INSERT INTO api_tokens (name, token_prefix, token_hash) VALUES ('recovery-test', 'bs_test…', 'test-hash')`); err != nil {
		t.Fatal(err)
	}

	if _, err := OpenAdministratorRecovery(dataDir); !errors.Is(err, ErrInstanceRunning) {
		t.Fatalf("recovery while service is running returned %v, want ErrInstanceRunning", err)
	}
	server.Close()
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}

	recovery, err := OpenAdministratorRecovery(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if username, err := recovery.Username(); err != nil || username != testAdminUsername {
		t.Fatalf("Username() = %q, %v", username, err)
	}
	if _, err := recovery.ResetUsername(" RecoveredAdmin "); err != nil {
		t.Fatal(err)
	}
	var sessions, tokens int
	if err := recovery.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err := recovery.db.QueryRow(`SELECT COUNT(*) FROM api_tokens`).Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 || tokens != 1 {
		t.Fatalf("after username reset sessions=%d tokens=%d, want 0 and 1", sessions, tokens)
	}

	newPassword := []byte("RecoveredPassword123!")
	if err := recovery.ResetPassword(newPassword, true); err != nil {
		t.Fatal(err)
	}
	var passwordHash string
	if err := recovery.db.QueryRow(`SELECT password_hash FROM administrators WHERE id = 1`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), newPassword); err != nil {
		t.Fatalf("recovered password did not match stored hash: %v", err)
	}
	if err := recovery.db.QueryRow(`SELECT COUNT(*) FROM api_tokens`).Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if tokens != 0 {
		t.Fatalf("API tokens after requested revocation = %d, want 0", tokens)
	}
	if err := recovery.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(Config{DataDir: dataDir, Frontend: frontend})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restartedServer := httptest.NewServer(reopened.Handler())
	t.Cleanup(restartedServer.Close)
	loginClient := newCookieClient(t)
	doJSON(t, loginClient, http.MethodPost, restartedServer.URL+"/api/login", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword,
	}, http.StatusUnauthorized, nil)
	doJSON(t, loginClient, http.MethodPost, restartedServer.URL+"/api/login", map[string]string{
		"username": "RecoveredAdmin", "password": string(newPassword),
	}, http.StatusOK, nil)
}

func TestAdministratorRecoveryValidatesCredentials(t *testing.T) {
	dataDir := t.TempDir()
	application, err := New(Config{DataDir: dataDir, Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(application.Handler())
	client := newCookieClient(t)
	setupTestAdministrator(t, client, server.URL)
	server.Close()
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}

	recovery, err := OpenAdministratorRecovery(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	if _, err := recovery.ResetUsername("bad\nusername"); err == nil {
		t.Fatal("username containing a control character was accepted")
	}
	if _, err := recovery.ResetUsername("bad\u202eusername"); err == nil {
		t.Fatal("username containing a bidi formatting character was accepted")
	}
	if err := recovery.ResetPassword([]byte("short"), false); err == nil {
		t.Fatal("short password was accepted")
	}
	if err := recovery.ResetPassword([]byte("1234567密"), false); err != nil {
		t.Fatalf("valid Unicode password was rejected: %v", err)
	}
}

func TestApplicationSecuresDataFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACLs are not represented by Unix mode bits")
	}
	dataDir := t.TempDir()
	application, err := New(Config{DataDir: dataDir, Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test")}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	for path, want := range map[string]uint32{
		dataDir:                                  0o700,
		application.imagesDir:                    0o700,
		application.dataDir + "/bluescale.db":    0o600,
		application.dataDir + "/.bluescale.lock": 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := uint32(info.Mode().Perm()); got != want {
			t.Errorf("%s permissions = %#o, want %#o", path, got, want)
		}
	}
}
