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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	// webCookiePrefix gets the request port appended: browsers share cookies
	// across ports on one host, so servers on 127.0.0.1:A and :B would
	// otherwise overwrite each other's session.
	webCookiePrefix = "gx_web"
	// webTokenParam is the query parameter of the sign-in link. It is namespaced
	// because the Vite dev frontend appends its own ?t=<timestamp> to modules.
	webTokenParam   = "gx_token"
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

func newWebToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// loadOrCreateWebToken keeps the token across restarts, so bookmarked links and
// cookies stay valid until the file changes.
func loadOrCreateWebToken(stateDir string) (string, error) {
	token, err := ReadWebToken(stateDir)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return token, err
	}
	token, err = newWebToken()
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(WebTokenPath(stateDir), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(token + "\n")
	return token, errors.Join(err, f.Close())
}

// RotateWebToken replaces the token file. The web listener reads the file on
// every request, so the running server drops all old cookies at once.
func RotateWebToken(stateDir string) (string, error) {
	token, err := newWebToken()
	if err != nil {
		return "", err
	}
	// Rename keeps a concurrent request from reading a half-written file.
	tmp := WebTokenPath(stateDir) + ".tmp"
	if err := os.WriteFile(tmp, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, WebTokenPath(stateDir)); err != nil {
		return "", errors.Join(err, os.Remove(tmp))
	}
	return token, nil
}

// webCookieValue derives the cookie from the token, so a new token makes every
// old cookie invalid and the cookie never carries the token itself.
func webCookieValue(token string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("gx web session"))
	return hex.EncodeToString(mac.Sum(nil))
}

// webCookieName names the session cookie after the listener's port. The Host
// header is not used: a proxy or a spoofed Host may omit the port.
func webCookieName(ln net.Listener) string {
	if _, port, err := net.SplitHostPort(ln.Addr().String()); err == nil && port != "" {
		return webCookiePrefix + "_" + port
	}
	return webCookiePrefix
}

func sameSecret(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// requireToken lets a request through when it carries the session cookie. A
// valid ?gx_token= starts a session and redirects to the app root; anything else goes to deny.
//
// Plain HTTP means anyone on the LAN can sniff the ?gx_token= link and the cookie, and
// the cookie cannot be Secure. It lives 365 days, so rotating the token is the
// only revocation. The file is read on every request so rotation takes effect
// at once; do not cache it. Security design: map gx:webui-security.
func requireToken(stateDir, cookieName string, next, deny http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := ReadWebToken(stateDir)
		if err != nil {
			deny.ServeHTTP(w, r)
			return
		}
		want := webCookieValue(token)
		if t := r.URL.Query().Get(webTokenParam); t != "" {
			if !sameSecret(t, token) {
				deny.ServeHTTP(w, r)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name: cookieName, Value: want, Path: "/", MaxAge: webCookieMaxAge,
				HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if c, err := r.Cookie(cookieName); err == nil && sameSecret(c.Value, want) {
			next.ServeHTTP(w, r)
			return
		}
		deny.ServeHTTP(w, r)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("401: sign in on the web page.\n"))
}
