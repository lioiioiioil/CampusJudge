// Package web embeds the buildless interface in the Go executable.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed index.html app.js styles.css lib/*.js views/*.js
var assets embed.FS

func Handler() http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-cache")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !publicAsset(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func publicAsset(requestPath string) bool {
	if requestPath == "/" {
		return true
	}
	asset := strings.TrimPrefix(requestPath, "/")
	if !fs.ValidPath(asset) {
		return false
	}
	if asset == "app.js" || asset == "styles.css" {
		return true
	}
	moduleDirectory := strings.HasPrefix(asset, "lib/") || strings.HasPrefix(asset, "views/")
	return moduleDirectory && strings.HasSuffix(asset, ".js")
}
