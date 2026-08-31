package app

import (
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	publicCORSAllowMethods  = "GET, HEAD, OPTIONS"
	publicCORSAllowHeaders  = "Accept, Cache-Control, Content-Type, If-Modified-Since, If-None-Match, Range"
	publicCORSExposeHeaders = "Content-Disposition, Content-Length, Content-Range, CDN-Cache-Control, Location"
)

func normalizePublicCORSOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return raw, true
	}
	origin, err := url.Parse(raw)
	if err != nil || origin.User != nil || origin.Host == "" ||
		(origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
		return "", false
	}
	scheme := strings.ToLower(origin.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	canonicalHost, valid := canonicalOriginHost(origin.Host, scheme)
	if !valid {
		return "", false
	}
	hostname, port, err := net.SplitHostPort(canonicalHost)
	if err != nil {
		return "", false
	}
	defaultPort := "80"
	if scheme == "https" {
		defaultPort = "443"
	}
	if port == defaultPort {
		port = ""
	}
	serializedHost := hostname
	if port != "" {
		serializedHost = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		serializedHost = "[" + hostname + "]"
	}
	return scheme + "://" + serializedHost, true
}

func (a *App) applyPublicCORSHeaders(w http.ResponseWriter) bool {
	settings := a.currentSettings().Security
	if !settings.EnablePublicCORS {
		return false
	}
	origin, valid := normalizePublicCORSOrigin(settings.CORSAllowedOrigin)
	if !valid {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Expose-Headers", publicCORSExposeHeaders)
	return true
}

func (a *App) handleRandomCORSPreflight(w http.ResponseWriter, r *http.Request) {
	a.writePublicCORSPreflight(w, r)
}

func (a *App) handleImageCORSPreflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.currentSettings().Security.EnablePublicCORS {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	name := r.PathValue("name")
	if !validStorageName(name) {
		http.NotFound(w, r)
		return
	}
	var marker int
	err := a.db.QueryRow(`SELECT 1 FROM images WHERE storage_name = ? AND is_public = 1`, name).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取图片信息")
		return
	}
	a.writePublicCORSPreflight(w, r)
}

func (a *App) writePublicCORSPreflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.applyPublicCORSHeaders(w) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	requestedMethod := strings.ToUpper(strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")))
	if requestedMethod != "" && requestedMethod != http.MethodGet && requestedMethod != http.MethodHead {
		w.Header().Set("Allow", publicCORSAllowMethods)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", publicCORSAllowMethods)
	w.Header().Set("Access-Control-Allow-Headers", publicCORSAllowHeaders)
	w.Header().Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
}
