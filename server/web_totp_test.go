package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// totpWeb is a loginWeb whose config file the test rewrites and reloads.
type totpWeb struct {
	*loginWeb
	path, store string
}

func startTOTPWeb(t *testing.T, totp bool) *totpWeb {
	t.Helper()
	w := &totpWeb{loginWeb: &loginWeb{}, store: t.TempDir()}
	w.path = filepath.Join(t.TempDir(), "config.json")
	w.setTOTP(t, totp)
	w.h = servertest.StartWithStore(t, w.store, servertest.WithConfigFile(w.path), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.Clock = w.now
		c.Settings.WebTOTPOff = !totp
	})
	hs, err := w.h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake %+v, %v", hs, err)
	}
	w.base = webBase(hs.WebURL)
	return w
}

func (w *totpWeb) setTOTP(t *testing.T, on bool) {
	t.Helper()
	servertest.WriteConfig(t, w.path, w.store, map[string]any{
		"server": map[string]any{"web": map[string]any{"totp": on}},
	})
}

// reloadTOTP writes the setting and reloads the running server with it.
func (w *totpWeb) reloadTOTP(t *testing.T, on bool) {
	t.Helper()
	w.setTOTP(t, on)
	if res := reload(t, w.h); res.Refused {
		t.Fatalf("reload = %+v", res)
	}
}

// setupWithoutTOTP finishes setup with a password only and returns the cookie.
func (w *totpWeb) setupWithoutTOTP(t *testing.T) *http.Cookie {
	t.Helper()
	info, err := w.h.Client.WebInfo(context.Background())
	if err != nil || !info.SetupPending {
		t.Fatalf("web info %+v, %v", info, err)
	}
	_, body := w.post(t, "/v1/web/auth/setup-start", `{"code":"`+info.SetupCode+`"}`, nil)
	if res := decodeAuth(t, body); res.Secret != "" || res.URI != "" {
		t.Fatalf("setup-start with TOTP off handed out %q", body)
	}
	resp, body := w.post(t, "/v1/web/auth/setup-finish", `{"code":"`+info.SetupCode+`","password":"`+loginTestPassword+`"}`, nil)
	if reasonOf(t, body) != "" || len(resp.Cookies()) != 1 {
		t.Fatalf("setup-finish body %q, %d cookies", body, len(resp.Cookies()))
	}
	return resp.Cookies()[0]
}

type authAnswer struct {
	Refused             bool   `json:"refused"`
	Reason              string `json:"reason"`
	AuthenticatorNeeded bool   `json:"authenticator_needed"`
	PendingID           string `json:"pending_id"`
	Secret              string `json:"secret"`
	URI                 string `json:"uri"`
	TOTP                bool   `json:"totp_on"`
	SetupCodeRequired   bool   `json:"setup_code_required"`
}

func decodeAuth(t *testing.T, body string) authAnswer {
	t.Helper()
	var a authAnswer
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return a
}

func (w *totpWeb) login(t *testing.T, body string) (*http.Response, authAnswer) {
	t.Helper()
	resp, b := w.post(t, "/v1/web/auth/login", body, nil)
	return resp, decodeAuth(t, b)
}

func TestWebTOTP_OffTakesThePasswordAlone(t *testing.T) {
	t.Parallel()
	w := startTOTPWeb(t, false)
	if _, body := w.get(t, "/v1/web/auth", nil); decodeAuth(t, body).TOTP {
		t.Errorf("auth state %q, want totp false", body)
	}
	w.setupWithoutTOTP(t)

	resp, a := w.login(t, `{"password":"`+loginTestPassword+`","totp_code":"000000"}`)
	if a.Refused || a.AuthenticatorNeeded || len(resp.Cookies()) != 1 {
		t.Errorf("login with a stray code: %+v, %d cookies; want it ignored", a, len(resp.Cookies()))
	}
	if _, a := w.login(t, `{"password":"wrong password here"}`); a.Reason != server.ReasonWrongCredentials {
		t.Errorf("wrong password reason %q, want %q", a.Reason, server.ReasonWrongCredentials)
	}
	resp, body := w.post(t, "/v1/web/auth/add-authenticator", `{"pending_id":"x","totp_code":"000000"}`, nil)
	if reasonOf(t, body) != server.ReasonTOTPOff || len(resp.Cookies()) != 0 {
		t.Errorf("add-authenticator with TOTP off: %q, %d cookies; want %q", body, len(resp.Cookies()), server.ReasonTOTPOff)
	}
}

func TestWebTOTP_ReloadKeepsLoginsAndTheOldSecret(t *testing.T) {
	t.Parallel()
	w := startTOTPWeb(t, true)
	cookie, secret := w.signIn(t)

	w.reloadTOTP(t, false)
	if resp, _ := w.get(t, "/v1/snapshot", cookie); resp.StatusCode != http.StatusOK {
		t.Errorf("signed-in cookie after turning TOTP off: status %d, want 200", resp.StatusCode)
	}
	if resp, a := w.login(t, `{"password":"`+loginTestPassword+`"}`); a.Refused || len(resp.Cookies()) != 1 {
		t.Errorf("password-only login with TOTP off: %+v", a)
	}

	w.reloadTOTP(t, true)
	if resp, _ := w.get(t, "/v1/snapshot", cookie); resp.StatusCode != http.StatusOK {
		t.Errorf("signed-in cookie after turning TOTP on: status %d, want 200", resp.StatusCode)
	}
	w.advance(31 * time.Second)
	resp, a := w.login(t, `{"password":"`+loginTestPassword+`","totp_code":"`+w.totp(t, secret)+`"}`)
	if a.Refused || a.AuthenticatorNeeded || len(resp.Cookies()) != 1 {
		t.Errorf("login with the old secret after off and on: %+v", a)
	}
}

func TestWebTOTP_NoSavedSecretGoesThroughAddAuthenticator(t *testing.T) {
	t.Parallel()
	w := startTOTPWeb(t, false)
	w.setupWithoutTOTP(t)
	w.reloadTOTP(t, true)

	resp, a := w.login(t, `{"password":"`+loginTestPassword+`"}`)
	if a.Refused || !a.AuthenticatorNeeded || a.Secret == "" || a.PendingID == "" || a.URI == "" || len(resp.Cookies()) != 0 {
		t.Fatalf("login = %+v, %d cookies; want authenticator needed with a secret and no cookie", a, len(resp.Cookies()))
	}
	add := func(id, code string) (*http.Response, authAnswer) {
		resp, b := w.post(t, "/v1/web/auth/add-authenticator", `{"pending_id":"`+id+`","totp_code":"`+code+`"}`, nil)
		return resp, decodeAuth(t, b)
	}
	if resp, got := add(a.PendingID, "000000"); !got.Refused || len(resp.Cookies()) != 0 {
		t.Errorf("wrong code: %+v, %d cookies; want a refusal", got, len(resp.Cookies()))
	}
	if resp, got := add("stale", w.totp(t, a.Secret)); !got.Refused || len(resp.Cookies()) != 0 {
		t.Errorf("stale pending id: %+v, %d cookies; want a refusal", got, len(resp.Cookies()))
	}
	// The wrong code saved nothing: the same pending id still takes the right one.
	resp, got := add(a.PendingID, w.totp(t, a.Secret))
	if got.Refused || len(resp.Cookies()) != 1 {
		t.Fatalf("right code: %+v, %d cookies; want a cookie", got, len(resp.Cookies()))
	}
	if r, _ := w.get(t, "/v1/snapshot", resp.Cookies()[0]); r.StatusCode != http.StatusOK {
		t.Errorf("snapshot with the new cookie: status %d, want 200", r.StatusCode)
	}
	if _, got := add(a.PendingID, w.totp(t, a.Secret)); !got.Refused {
		t.Error("a pending id worked twice")
	}
	w.advance(31 * time.Second)
	if _, got := w.login(t, `{"password":"`+loginTestPassword+`"}`); !got.Refused {
		t.Errorf("password-only login after the authenticator was added: %+v, want a refusal", got)
	}
}

func TestWebTOTP_WrongAddAuthenticatorCodesLockTheIP(t *testing.T) {
	t.Parallel()
	w := startTOTPWeb(t, false)
	w.setupWithoutTOTP(t)
	w.reloadTOTP(t, true)
	_, a := w.login(t, `{"password":"`+loginTestPassword+`"}`)
	for range 5 {
		w.post(t, "/v1/web/auth/add-authenticator", `{"pending_id":"`+a.PendingID+`","totp_code":"000000"}`, nil)
	}
	if _, got := w.login(t, `{"password":"`+loginTestPassword+`"}`); got.Reason != server.ReasonRateLimited {
		t.Errorf("login after 5 wrong codes: %+v, want %q", got, server.ReasonRateLimited)
	}
}
