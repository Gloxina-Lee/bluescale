package app

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	loginFailureWindow            = 15 * time.Minute
	maxTrackedLoginFailureSources = 4096
)

type loginFailure struct {
	Count       int
	LastFailure time.Time
}

func (a *App) sameOriginProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || bearerToken(r) != "" {
			next.ServeHTTP(w, r)
			return
		}
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
			writeError(w, http.StatusForbidden, "拒绝跨站请求")
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && !a.originMatchesRequest(r, origin) {
			writeError(w, http.StatusForbidden, "拒绝跨源请求")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) originMatchesRequest(r *http.Request, rawOrigin string) bool {
	origin, err := url.Parse(rawOrigin)
	if err != nil || origin.User != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") ||
		(origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	scheme := requestOriginScheme(r)
	if !strings.EqualFold(origin.Scheme, scheme) {
		return false
	}
	originHost, ok := canonicalOriginHost(origin.Host, scheme)
	if !ok {
		return false
	}
	requestHost, ok := canonicalOriginHost(r.Host, scheme)
	return ok && originHost == requestHost
}

func requestOriginScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	// Forwarded proto is used only to compare the browser Origin with the
	// externally visible URL. Real client IP headers remain gated by the
	// explicit reverse-proxy setting.
	forwardedProto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if strings.EqualFold(forwardedProto, "https") {
		return "https"
	}
	return "http"
}

func canonicalOriginHost(hostPort, scheme string) (string, bool) {
	parsed, err := url.Parse("//" + hostPort)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" {
		return "", false
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(strings.ToLower(parsed.Hostname()), port), true
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
	failure, tracked := a.loginFailures[ip]
	if !tracked && len(a.loginFailures) >= maxTrackedLoginFailureSources {
		for source, candidate := range a.loginFailures {
			if now.Sub(candidate.LastFailure) >= loginFailureWindow {
				delete(a.loginFailures, source)
			}
		}
		if len(a.loginFailures) >= maxTrackedLoginFailureSources {
			return
		}
	}
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
