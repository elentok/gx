package cmdtest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/webauth"
)

// FinishWebSetup sets the web account up through the real routes, so the server
// stops showing a Setup code. It returns the first web login's cookie.
func FinishWebSetup(t *testing.T, h *servertest.Harness) *http.Cookie {
	t.Helper()
	cookie, _ := FinishWebSetupWithSecret(t, h)
	return cookie
}

// FinishWebSetupWithSecret is FinishWebSetup that also returns the
// authenticator secret.
func FinishWebSetupWithSecret(t *testing.T, h *servertest.Harness) (*http.Cookie, string) {
	t.Helper()
	var login *http.Cookie
	info, err := h.Client.Web.Info(context.Background())
	if err != nil || !info.SetupPending {
		t.Fatalf("web info %+v, %v; want setup pending", info, err)
	}
	client := &http.Client{Transport: servertest.WebTransport()}
	post := func(path, body string) []byte {
		req, err := http.NewRequest(http.MethodPost, info.BaseURL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", info.BaseURL)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if cs := resp.Cookies(); len(cs) > 0 {
			login = cs[0]
		}
		b, _ := io.ReadAll(resp.Body)
		return b
	}
	var start struct{ Secret string }
	if err := json.Unmarshal(post("/v1/web/auth/setup-start", `{"code":"`+info.SetupCode+`"}`), &start); err != nil || start.Secret == "" {
		t.Fatalf("setup-start: %v, secret %q", err, start.Secret)
	}
	totp, err := webauth.TOTPCode(start.Secret, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	post("/v1/web/auth/setup-finish", `{"code":"`+info.SetupCode+`","password":"correct horse battery","totp_code":"`+totp+`"}`)
	if after, _ := h.Client.Web.Info(context.Background()); after.SetupPending {
		t.Fatalf("setup still pending after setup-finish: %+v", after)
	}
	return login, start.Secret
}
