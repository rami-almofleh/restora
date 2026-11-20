// Package ui embeds the built web interface so the API binary can serve it.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the embedded web interface. Unknown paths fall back to index.html
// so client-side routes work after a page reload.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	return newSPAHandler(sub), nil
}

type spaHandler struct {
	fsys fs.FS
	file http.Handler
}

func newSPAHandler(fsys fs.FS) *spaHandler {
	return &spaHandler{fsys: fsys, file: http.FileServerFS(fsys)}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}

	if info, err := fs.Stat(h.fsys, name); err == nil && !info.IsDir() {
		if strings.HasPrefix(name, "assets/") {
			// Build output in assets/ has content hashes in its names.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		h.file.ServeHTTP(w, r)
		return
	}

	index, err := fs.ReadFile(h.fsys, "index.html")
	if err != nil {
		http.Error(w, "The web interface is not built into this binary.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}
