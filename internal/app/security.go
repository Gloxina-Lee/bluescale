package app

import (
	"net"
	"net/http"
	"strings"
	"time"
)

const loginFailureWindow = 15 * time.Minute

type loginFailure struct {
	Count       int
	LastFailure time.Time
}

func (a *App) clientIP(r *http.Request) string {
	settings := a.currentSettings().Security
	if settings.ReverseProxyMode {
		value := strings.TrimSpace(r.Header.Get(settings.RealIPHeader))
		if settings.RealIPHeader == "X-Forwarded-For" {
			value = strings.TrimSpace(strings.Split(value, ",")[0])
		}
		if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
			return ip.String()
		}
		return host
	}
	if ip := net.ParseIP(strings.Trim(r.RemoteAddr, "[]")); ip != nil {
		return ip.String()
	}
	return r.RemoteAddr
}

func (a *App) requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	settings := a.currentSettings().Security
	return settings.ReverseProxyMode && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (a *App) loginAllowed(ip string, maximum int) (bool, time.Duration) {
	now := time.Now()
	a.loginFailuresMu.Lock()
	defer a.loginFailuresMu.Unlock()
	failure, ok := a.loginFailures[ip]
	if !ok || now.Sub(failure.LastFailure) >= loginFailureWindow {
		delete(a.loginFailures, ip)
		return true, 0
	}
	if failure.Count < maximum {
		return true, 0
	}
	return false, loginFailureWindow - now.Sub(failure.LastFailure)
}

func (a *App) recordLoginFailure(ip string) {
	now := time.Now()
	a.loginFailuresMu.Lock()
	defer a.loginFailuresMu.Unlock()
	failure := a.loginFailures[ip]
	if now.Sub(failure.LastFailure) >= loginFailureWindow {
		failure.Count = 0
	}
	failure.Count++
	failure.LastFailure = now
	a.loginFailures[ip] = failure
}

func (a *App) clearLoginFailure(ip string) {
	a.loginFailuresMu.Lock()
	defer a.loginFailuresMu.Unlock()
	delete(a.loginFailures, ip)
}

func (a *App) clearLoginFailures() {
	a.loginFailuresMu.Lock()
	defer a.loginFailuresMu.Unlock()
	clear(a.loginFailures)
}
