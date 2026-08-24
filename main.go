package main

import (
	"embed"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"bluescale/internal/app"
)

//go:embed all:web/dist
var webFiles embed.FS

func main() {
	addr := envOr("BLUESCALE_ADDR", ":8080")
	dataDir := envOr("BLUESCALE_DATA_DIR", "data")

	frontend, err := fs.Sub(webFiles, "web/dist")
	if err != nil {
		log.Fatal(err)
	}

	application, err := app.New(app.Config{DataDir: dataDir, Frontend: frontend})
	if err != nil {
		log.Fatal(err)
	}
	defer application.Close()

	server := &http.Server{
		Addr:              addr,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	log.Printf("BlueScale is listening on %s", displayURL(addr))
	log.Fatal(server.ListenAndServe())
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func displayURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}
