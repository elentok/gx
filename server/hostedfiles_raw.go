package server

import (
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/elentok/gx/server/files"
)

const (
	rawPrefix   = "/v1/files"
	tokenPrefix = "/t/"
)

// raw serves GET /v1/files/<absolute path>. Every failure, whatever the rule,
// answers the same 404, so the route cannot be used to probe the disk. Range,
// download and the PDF rules are not here yet.
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
	w.Header().Set("Content-Length", strconv.FormatInt(hosted.Size, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.CopyN(w, f, hosted.Size)
}
