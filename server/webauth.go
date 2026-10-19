package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	webCookieName   = "gx_web"
	webCookieMaxAge = 365 * 24 * 60 * 60
)

// WebTokenPath is the file that holds the web listener's token.
func WebTokenPath(stateDir string) string {
	return filepath.Join(stateDir, "web-token")
}

// ReadWebToken returns the token the web listener checks, without creating it.
func ReadWebToken(stateDir string) (string, error) {
	data, err := os.ReadFile(WebTokenPath(stateDir))
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("%s is empty", WebTokenPath(stateDir))
	}
	return token, nil
}

// loadOrCreateWebToken keeps the token across restarts, so bookmarked links and
// cookies stay valid until the file changes.
func loadOrCreateWebToken(stateDir string) (string, error) {
	token, err := ReadWebToken(stateDir)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return token, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token = hex.EncodeToString(raw)
	f, err := os.OpenFile(WebTokenPath(stateDir), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(token + "\n")
	return token, errors.Join(err, f.Close())
}

// webCookieValue derives the cookie from the token, so a new token makes every
// old cookie invalid and the cookie never carries the token itself.
func webCookieValue(token string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("gx web session"))
	return hex.EncodeToString(mac.Sum(nil))
}

func sameSecret(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// requireToken lets a request through when it carries the session cookie. A
// valid ?t= starts a session and redirects to the app root; anything else is a 401.
func requireToken(token string, next http.Handler) http.Handler {
	want := webCookieValue(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("t"); t != "" {
			if !sameSecret(t, token) {
				unauthorized(w)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name: webCookieName, Value: want, Path: "/", MaxAge: webCookieMaxAge,
				HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if c, err := r.Cookie(webCookieName); err == nil && sameSecret(c.Value, want) {
			next.ServeHTTP(w, r)
			return
		}
		unauthorized(w)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("401: open the link printed by `gx server status` to sign in.\n"))
}
