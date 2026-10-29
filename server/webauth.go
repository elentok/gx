package server

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

// removeLegacyWebToken deletes the shared token file an older version left in the state dir.
func removeLegacyWebToken(stateDir string) error {
	err := os.Remove(filepath.Join(stateDir, "web-token"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("401: sign in on the web page.\n"))
}
