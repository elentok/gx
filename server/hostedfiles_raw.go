package server

import (
	"mime"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/files"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire/ops"
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
	download := r.URL.Query().Get("download") == "1"
	gate := h.s.web.Gate()
	if loginID := webauth.LoginIDFrom(r.Context()); !download && loginID != "" && gate != nil && files.KindOf(hosted.Path) == ops.HostedKindHTML {
		token := gate.MintFileToken(filepath.Dir(path), loginID)
		http.Redirect(w, r, webauth.FileTokenPrefix+token+rest, http.StatusFound)
		return
	}
	serveHosted(w, r, hosted, download)
}

// token serves GET /v1/files/t/<token>/<absolute path>: any file under the
// token's folder, for the login the token was made for. Anything the token
// does not cover answers 302 to the stable raw link, which mints a fresh token
// when the cookie is valid and answers 401 when not.
func (h hostedFiles) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	rest := strings.TrimPrefix(r.URL.EscapedPath(), webauth.FileTokenPrefix)
	tokenStr, escaped, ok := strings.Cut(rest, "/")
	gate := h.s.web.Gate()
	if !ok || gate == nil {
		http.NotFound(w, r)
		return
	}
	escaped = "/" + escaped
	tok, valid := gate.VerifyFileToken(tokenStr)
	path, formOK := files.PathForm(escaped)
	if !formOK {
		// A request that climbs out with ".." goes back to the stable link of the cleaned path.
		if cleaned := pathpkg.Clean(escaped); valid && cleaned != escaped {
			if _, ok := files.PathForm(cleaned); ok {
				http.Redirect(w, r, rawPrefix+cleaned, http.StatusFound)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	if !valid || !gate.LoginLive(tok.LoginID) || (path != tok.Dir && !strings.HasPrefix(path, strings.TrimSuffix(tok.Dir, "/")+"/")) {
		http.Redirect(w, r, rawPrefix+escaped, http.StatusFound)
		return
	}
	hosted, ok := h.checker().Check(path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	serveHosted(w, r, hosted, false)
}

func serveHosted(w http.ResponseWriter, r *http.Request, hosted files.Hosted, download bool) {
	f, err := os.Open(hosted.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", files.ContentType(hosted.Path))
	if download {
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
