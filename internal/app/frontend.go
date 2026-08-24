package app

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

func (a *App) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/i/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
		return
	}
	requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if requested == "." || requested == "" {
		requested = "index.html"
	}
	content, err := fs.ReadFile(a.frontend, requested)
	if err != nil {
		requested = "index.html"
		content, err = fs.ReadFile(a.frontend, requested)
		if err != nil {
			http.Error(w, "前端资源尚未构建，请先运行 npm run build", http.StatusServiceUnavailable)
			return
		}
	}
	if contentType := mime.TypeByExtension(path.Ext(requested)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if requested == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(content)
}
