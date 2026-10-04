package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

//go:embed all:dist
var distFS embed.FS

// DefaultAssets returns the embedded web assets filesystem.
func DefaultAssets() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return distFS
	}
	return sub
}

func init() {
	defaultAssets = DefaultAssets
	registerRoutes((*Server).registerAssetsRoutes)
}

func (s *Server) registerAssetsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/", s.handleUnknownAPI)
	mux.HandleFunc("/", s.handleAssets)
}

func (s *Server) handleUnknownAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, app.ErrorResult(app.NewInvalidRequestError(
		"Unknown API path.",
		"Check the API version and path.",
	)))
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cleaned := path.Clean(r.URL.Path)
	filePath := strings.TrimPrefix(cleaned, "/")

	if s.opts.Assets != nil && filePath != "" && filePath != "." {
		if f, err := s.opts.Assets.Open(filePath); err == nil {
			defer f.Close()
			if stat, err := f.Stat(); err == nil && !stat.IsDir() {
				if strings.HasPrefix(cleaned, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				if rs, ok := f.(io.ReadSeeker); ok {
					http.ServeContent(w, r, stat.Name(), stat.ModTime(), rs)
				} else {
					w.WriteHeader(http.StatusOK)
					if r.Method != http.MethodHead {
						_, _ = io.Copy(w, f)
					}
				}
				return
			}
		}
	}

	accept := r.Header.Get("Accept")
	if cleaned == "/" || strings.Contains(accept, "text/html") {
		var indexFile fs.File
		var stat fs.FileInfo
		var err error
		if s.opts.Assets != nil {
			indexFile, err = s.opts.Assets.Open("index.html")
			if err == nil {
				defer indexFile.Close()
				stat, err = indexFile.Stat()
			}
		}
		if err == nil && indexFile != nil && !stat.IsDir() {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if rs, ok := indexFile.(io.ReadSeeker); ok {
				http.ServeContent(w, r, "index.html", stat.ModTime(), rs)
			} else {
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = io.Copy(w, indexFile)
				}
			}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, "Skill Hub web UI is not built. Run make web-build, then rebuild skillhub.\n")
		}
		return
	}

	w.WriteHeader(http.StatusNotFound)
}
