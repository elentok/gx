package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

const loginTestPassword = "correct horse battery"

// loginWeb is a web listener on a clock the test can move.
type loginWeb struct {
	h      *servertest.Harness
	base   string
	offset atomic.Int64
}

func (l *loginWeb) now() time.Time { return time.Now().Add(time.Duration(l.offset.Load())) }

func (l *loginWeb) advance(d time.Duration) { l.offset.Add(int64(d)) }

func startLoginWeb(t *testing.T) *loginWeb {
	t.Helper()
	l := &loginWeb{}
	l.h = servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.Clock = l.now
	})
	hs, err := l.h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake %+v, %v", hs, err)
	}
	l.base = webBase(hs.WebURL)
	return l
}

// post sends a JSON write the way the page does and returns the response and body.
func (l *loginWeb) post(t *testing.T, path, body string, cookie *http.Cookie) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, l.base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range jsonPost(l.base) {
		req.Header.Set(k, v)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return doBody(t, req)
}

func (l *loginWeb) get(t *testing.T, path string, cookie *http.Cookie) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, l.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return doBody(t, req)
}

func doBody(t *testing.T, req *http.Request) (*http.Response, string) {
	t.Helper()
	resp, err := webClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func (l *loginWeb) totp(t *testing.T, secret string) string {
	t.Helper()
	code, err := server.TOTPCode(secret, l.now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// signIn sets the account up through the real routes and returns the cookie.
func (l *loginWeb) signIn(t *testing.T) (*http.Cookie, string) {
	t.Helper()
	info, err := l.h.Client.WebInfo(context.Background())
	if err != nil || !info.SetupPending || info.SetupCode == "" {
		t.Fatalf("web info %+v, %v; want setup pending with a code", info, err)
	}
	_, body := l.post(t, "/v1/web/auth/setup-start", `{"code":"`+info.SetupCode+`"}`, nil)
	var start struct{ Secret string }
	if err := json.Unmarshal([]byte(body), &start); err != nil || start.Secret == "" {
		t.Fatalf("setup-start body %q: %v", body, err)
	}
	resp, body := l.post(t, "/v1/web/auth/setup-finish",
		`{"code":"`+info.SetupCode+`","password":"`+loginTestPassword+`","totp_code":"`+l.totp(t, start.Secret)+`"}`, nil)
	if reasonOf(t, body) != "" || len(resp.Cookies()) != 1 {
		t.Fatalf("setup-finish body %q, %d cookies", body, len(resp.Cookies()))
	}
	return resp.Cookies()[0], start.Secret
}

func TestWebLogin_SetupPendingServesThePageAndNothingElse(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	if resp, _ := l.get(t, "/v1/snapshot", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("snapshot: status %d, want 401", resp.StatusCode)
	}
	resp, _ := l.get(t, "/", nil)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("/: status %d type %q, want 200 text/html", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if _, body := l.get(t, "/v1/web/auth", nil); !strings.Contains(body, `"setup"`) ||
		!strings.Contains(body, `"totp_on":true`) || !strings.Contains(body, `"setup_code_required":false`) {
		t.Errorf("auth state %q, want setup with totp_on and no setup code required", body)
	}
}

func TestWebLogin_AuthStateCarriesTheRootFingerprintWithoutACookie(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	info, err := l.h.Client.WebInfo(context.Background())
	if err != nil || info.Fingerprint == "" {
		t.Fatalf("web info %+v, %v; want a fingerprint", info, err)
	}
	_, body := l.get(t, "/v1/web/auth", nil)
	var state struct {
		Fingerprint string `json:"fingerprint"`
		ProfilePath string `json:"profile_path"`
		CerPath     string `json:"cer_path"`
	}
	if err := json.Unmarshal([]byte(body), &state); err != nil {
		t.Fatalf("auth state %q: %v", body, err)
	}
	if state.Fingerprint != info.Fingerprint {
		t.Errorf("auth state fingerprint %q, web info %q", state.Fingerprint, info.Fingerprint)
	}
	for _, path := range []string{state.ProfilePath, state.CerPath} {
		if resp, _ := l.get(t, path, nil); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %q: status %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestWebLogin_HandshakeFlagsPendingSetupUntilItIsDone(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	hs, err := l.h.Client.Handshake(context.Background())
	if err != nil || !hs.WebSetupPending {
		t.Fatalf("handshake %+v, %v; want setup pending", hs, err)
	}
	l.signIn(t)
	if hs, err = l.h.Client.Handshake(context.Background()); err != nil || hs.WebSetupPending {
		t.Errorf("handshake after setup %+v, %v; want no pending setup", hs, err)
	}
}

func TestWebLogin_SetupFinishSetsTheCookieAndOpensTheApp(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	cookie, _ := l.signIn(t)
	if !strings.HasPrefix(cookie.Name, "__Host-") || !cookie.Secure || !cookie.HttpOnly ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" ||
		cookie.MaxAge != 90*24*60*60 {
		t.Errorf("cookie %+v lacks an attribute of the spec", cookie)
	}
	auth, err := server.OpenWebAuth(l.h.StateDir, l.now)
	if err != nil {
		t.Fatal(err)
	}
	logins := auth.List()
	if len(logins) != 1 {
		t.Fatalf("%d web logins, want 1", len(logins))
	}
	if resp, _ := l.get(t, "/v1/snapshot", cookie); resp.StatusCode != http.StatusOK {
		t.Errorf("snapshot with the cookie: status %d, want 200", resp.StatusCode)
	}
	if _, body := l.get(t, "/v1/web/auth", cookie); !strings.Contains(body, logins[0].ID) {
		t.Errorf("auth state %q, want signed in as %s", body, logins[0].ID)
	}
	if info, _ := l.h.Client.WebInfo(context.Background()); info.SetupPending || info.SetupCode != "" {
		t.Errorf("web info after setup %+v, want no pending setup", info)
	}
}

func TestWebLogin_WrongPasswordAndWrongCodeGiveTheSameReason(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	_, secret := l.signIn(t)
	l.advance(31 * time.Second)
	_, wrongPassword := l.post(t, "/v1/web/auth/login", `{"password":"not the password","totp_code":"`+l.totp(t, secret)+`"}`, nil)
	_, wrongCode := l.post(t, "/v1/web/auth/login", `{"password":"`+loginTestPassword+`","totp_code":"000000"}`, nil)
	if got := reasonOf(t, wrongPassword); got != server.ReasonWrongCredentials {
		t.Errorf("wrong password reason %q, want %q", got, server.ReasonWrongCredentials)
	}
	if got := reasonOf(t, wrongCode); got != server.ReasonWrongCredentials {
		t.Errorf("wrong code reason %q, want %q", got, server.ReasonWrongCredentials)
	}
	resp, body := l.post(t, "/v1/web/auth/login", `{"password":"`+loginTestPassword+`","totp_code":"`+l.totp(t, secret)+`"}`, nil)
	if reasonOf(t, body) != "" && reasonOf(t, body) != server.ReasonRateLimited {
		t.Fatalf("good login body %q", body)
	}
	if reasonOf(t, body) == "" && len(resp.Cookies()) != 1 {
		t.Errorf("good login set %d cookies, want 1", len(resp.Cookies()))
	}
}

func TestWebLogin_SetupRefusalsNameTheirReason(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	info, _ := l.h.Client.WebInfo(context.Background())
	if _, body := l.post(t, "/v1/web/auth/setup-start", `{"code":"0000 0000"}`, nil); reasonOf(t, body) != server.ReasonSetupCodeWrong {
		t.Errorf("wrong Setup code: body %q, want %q", body, server.ReasonSetupCodeWrong)
	}
	_, body := l.post(t, "/v1/web/auth/setup-start", `{"code":"`+info.SetupCode+`"}`, nil)
	var start struct{ Secret string }
	_ = json.Unmarshal([]byte(body), &start)
	finish := func(password, code string) string {
		_, b := l.post(t, "/v1/web/auth/setup-finish", `{"code":"`+info.SetupCode+`","password":"`+password+`","totp_code":"`+code+`"}`, nil)
		return reasonOf(t, b)
	}
	if got := finish("short", l.totp(t, start.Secret)); got != server.ReasonPasswordTooShort {
		t.Errorf("short password reason %q, want %q", got, server.ReasonPasswordTooShort)
	}
	if got := finish(loginTestPassword, "000000"); got != server.ReasonTOTPMismatch {
		t.Errorf("wrong TOTP code reason %q, want %q", got, server.ReasonTOTPMismatch)
	}
}

func TestWebLogin_SignOutClearsTheCookieAndEndsStreams(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	cookie, _ := l.signIn(t)
	req, _ := http.NewRequest(http.MethodGet, l.base+"/v1/events", nil)
	req.AddCookie(cookie)
	stream, err := webClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	ended := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, stream.Body); close(ended) }()

	resp, _ := l.post(t, "/v1/web/auth/sign-out", `{}`, cookie)
	cleared := resp.Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Errorf("sign out cookies %+v, want one that clears the cookie", cleared)
	}
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Error("the event stream stayed open after sign out")
	}
	if resp, _ := l.get(t, "/v1/snapshot", cookie); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("snapshot after sign out: status %d, want 401", resp.StatusCode)
	}
}

func TestWebLogin_AuthWritesKeepTheWriteChecks(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	for _, path := range []string{"/v1/web/auth/setup-start", "/v1/web/auth/setup-password", "/v1/web/auth/setup-finish", "/v1/web/auth/login"} {
		for name, headers := range map[string]map[string]string{
			"wrong Origin": {"Origin": "http://evil.example", "Content-Type": "application/json"},
			"form post":    {"Origin": l.base, "Content-Type": "application/x-www-form-urlencoded"},
		} {
			req, _ := http.NewRequest(http.MethodPost, l.base+path, strings.NewReader(`{}`))
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			if resp, _ := doBody(t, req); resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s with %s: status %d, want 403", path, name, resp.StatusCode)
			}
		}
	}
}

func TestWebLogin_CookieIsRenewedAtMostOncePerMinute(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	cookie, _ := l.signIn(t)
	renewed := func() bool {
		resp, _ := l.get(t, "/v1/snapshot", cookie)
		return len(resp.Cookies()) == 1
	}
	if !renewed() {
		t.Error("first use did not renew the cookie")
	}
	if renewed() {
		t.Error("a second use within a minute renewed the cookie")
	}
	l.advance(61 * time.Second)
	if !renewed() {
		t.Error("use after a minute did not renew the cookie")
	}
}
