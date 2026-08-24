package app

import (
	"database/sql"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Config struct {
	DataDir  string
	Frontend fs.FS
}

type App struct {
	db           *sql.DB
	dataDir      string
	imagesDir    string
	frontend     fs.FS
	sessions     *sessionStore
	setupMu      sync.Mutex
	maxFileBytes int64
}

func New(config Config) (*App, error) {
	if config.DataDir == "" {
		return nil, errors.New("data directory is required")
	}
	dataDir, err := filepath.Abs(config.DataDir)
	if err != nil {
		return nil, err
	}
	imagesDir := filepath.Join(dataDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", filepath.Join(dataDir, "bluescale.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := initializeDatabase(db); err != nil {
		db.Close()
		return nil, err
	}

	return &App{
		db:           db,
		dataDir:      dataDir,
		imagesDir:    imagesDir,
		frontend:     config.Frontend,
		sessions:     newSessionStore(24 * time.Hour),
		maxFileBytes: 25 << 20,
	}, nil
}

func (a *App) Close() error {
	return a.db.Close()
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", a.handleStatus)
	mux.HandleFunc("POST /api/setup", a.handleSetup)
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("POST /api/logout", a.requireAuth(a.handleLogout))
	mux.HandleFunc("GET /api/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("GET /api/images", a.requireAuth(a.handleListImages))
	mux.HandleFunc("POST /api/images", a.requireAuth(a.handleUploadImages))
	mux.HandleFunc("DELETE /api/images", a.requireAuth(a.handleDeleteImages))
	mux.HandleFunc("GET /i/{name}", a.handleServeImage)
	mux.HandleFunc("/", a.handleFrontend)

	return a.securityHeaders(a.logRequests(mux))
}

func (a *App) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
	})
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob: data:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
