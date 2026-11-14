package server

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/files"
)

const (
	rawPrefix   = "/v1/files"
	tokenPrefix = "/t/"
)

// raw serves GET /v1/files/<absolute path>. Every failure, whatever the rule,
// answers the same 404, so the route cannot be used to probe the disk.
func (h hostedFiles) raw(w http.ResponseWriter, r *http.Request) {
	rest, ok := strings.CutPrefix(r.URL.EscapedPath(), rawPrefix)
	if !ok || strings.HasPrefix(rest, tokenPrefix) {
		http.NotFound(w, r)
		return
	}
	path, ok := files.PathForm(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	hosted, ok := h.checker().Check(path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(hosted.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", files.ContentType(hosted.Path))
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(hosted.Path)}))
	}
	// ServeContent answers Range (206/416) and keeps the Content-Type set above.
	http.ServeContent(&keepCache{ResponseWriter: w, value: w.Header().Get("Cache-Control")}, r, "", hosted.ModTime, f)
}

// keepCache restores Cache-Control on the header write: ServeContent drops it
// from its error replies (416), which must stay no-store like the 200.
type keepCache struct {
	http.ResponseWriter
	value string
}

func (k *keepCache) WriteHeader(code int) {
	if k.value != "" {
		k.Header().Set("Cache-Control", k.value)
	}
	k.ResponseWriter.WriteHeader(code)
}

func (k *keepCache) Unwrap() http.ResponseWriter { return k.ResponseWriter }
