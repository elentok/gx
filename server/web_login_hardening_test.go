package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

type loginResult struct {
	Reason      string `json:"reason"`
	WaitSeconds int    `json:"wait_seconds"`
}

// attemptLogin posts a login and returns the refusal, with its cookie when it worked.
func (l *loginWeb) attemptLogin(t *testing.T, password, totpCode string) (loginResult, *http.Cookie) {
	t.Helper()
	resp, body := l.post(t, "/v1/web/auth/login", fmt.Sprintf(`{"password":%q,"totp_code":%q}`, password, totpCode), nil)
	var res loginResult
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("login body %q: %v", body, err)
	}
	if cs := resp.Cookies(); len(cs) == 1 {
		return res, cs[0]
	}
	return res, nil
}

// failRound sends one round of wrong passwords, none of which may be refused as rate-limited.
func (l *loginWeb) failRound(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if res, _ := l.attemptLogin(t, "not the password", "000000"); res.Reason != server.ReasonWrongCredentials {
			t.Fatalf("failed login %d: reason %q, want %q", i+1, res.Reason, server.ReasonWrongCredentials)
		}
	}
}

func (l *loginWeb) goodLogin(t *testing.T, secret string) *http.Cookie {
	t.Helper()
	l.advance(31 * time.Second)
	res, cookie := l.attemptLogin(t, loginTestPassword, l.totp(t, secret))
	if res.Reason != "" || cookie == nil {
		t.Fatalf("good login: %+v, cookie %v", res, cookie)
	}
	return cookie
}

func readGxLog(t *testing.T, l *loginWeb) string {
	t.Helper()
	b, err := os.ReadFile(server.LogPath(l.h.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// logLines returns the parsed log lines whose msg is msg.
func logLines(t *testing.T, log, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(log, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil && m["msg"] == msg {
			out = append(out, m)
		}
	}
	return out
}

func TestWebLoginLimits_FiveWrongSetupCodesLockLoginsToo(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	for i := 0; i < 5; i++ {
		if _, body := l.post(t, "/v1/web/auth/setup-start", `{"code":"0000 0000"}`, nil); reasonOf(t, body) != server.ReasonSetupCodeWrong {
			t.Fatalf("wrong Setup code %d: body %q", i+1, body)
		}
	}
	res, _ := l.attemptLogin(t, loginTestPassword, "000000")
	if res.Reason != server.ReasonRateLimited || res.WaitSeconds < 55 || res.WaitSeconds > 60 {
		t.Errorf("login after 5 wrong Setup codes: %+v, want rate-limited for about a minute", res)
	}
}

func TestWebLoginLimits_WaitGrowsPerRoundAndStopsAtAnHour(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	l.signIn(t)
	// Seconds each round makes the next attempt wait: 1, 2, 4, ... minutes, then the cap.
	want := []int{60, 120, 240, 480, 960, 1920, 3600}
	for round, wait := range want {
		l.failRound(t, 5)
		res, _ := l.attemptLogin(t, "not the password", "000000")
		if res.Reason != server.ReasonRateLimited || res.WaitSeconds < wait-5 || res.WaitSeconds > wait {
			t.Fatalf("round %d: %+v, want rate-limited for about %ds", round+1, res, wait)
		}
		l.advance(time.Duration(wait+1) * time.Second)
	}
}

func TestWebLoginLimits_SuccessClearsTheFailureCount(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	_, secret := l.signIn(t)
	l.failRound(t, 4)
	l.goodLogin(t, secret)
	l.failRound(t, 4)
}

func TestWebLoginLimits_IdleLoginExpiresAfterNinetyDays(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	cookie, _ := l.signIn(t)
	day := 24 * time.Hour

	l.advance(89 * day)
	resp, _ := l.get(t, "/v1/snapshot", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("day 89: status %d, want 200", resp.StatusCode)
	}
	if cs := resp.Cookies(); len(cs) != 1 || cs[0].MaxAge != 90*24*60*60 {
		t.Errorf("day 89 cookies %+v, want one renewed to 90 days", cs)
	}
	l.advance(89 * day)
	if resp, _ := l.get(t, "/v1/snapshot", cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("89 days after the last use: status %d, want 200", resp.StatusCode)
	}
	l.advance(90 * day)
	if resp, _ := l.get(t, "/v1/snapshot", cookie); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("90 days idle: status %d, want 401", resp.StatusCode)
	}
}

func TestWebLoginLimits_RestartedServerAcceptsTheOldCookie(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	cookie, _ := l.signIn(t)
	l.h.Restart(t)
	hs, err := l.h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake %+v, %v", hs, err)
	}
	l.base = webBase(hs.WebURL)
	u, err := url.Parse(l.base)
	if err != nil {
		t.Fatal(err)
	}
	// The cookie name carries the listener's port, which the new server may not reuse.
	name := cookie.Name[:strings.LastIndex(cookie.Name, "_")+1] + u.Port()
	if resp, _ := l.get(t, "/v1/snapshot", &http.Cookie{Name: name, Value: cookie.Value}); resp.StatusCode != http.StatusOK {
		t.Errorf("snapshot with the old cookie after a restart: status %d, want 200", resp.StatusCode)
	}
}

func TestWebLoginLimits_StateFileHoldsOnlyTheCookieHash(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	cookie, _ := l.signIn(t)
	data, err := os.ReadFile(filepath.Join(l.h.StateDir, "web-auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), cookie.Value) {
		t.Error("web-auth.json holds the cookie value")
	}
	if strings.Contains(string(data), loginTestPassword) {
		t.Error("web-auth.json holds the password")
	}
	var file struct {
		Logins []struct {
			CookieHash string `json:"cookie_hash"`
		} `json:"logins"`
	}
	if err := json.Unmarshal(data, &file); err != nil || len(file.Logins) != 1 || len(file.Logins[0].CookieHash) != 64 {
		t.Errorf("web-auth.json logins %+v (%v), want one with a 64-digit SHA-256", file.Logins, err)
	}
}

func TestWebLoginLimits_TwoLoginsFromOneIPMakeTwoWebLogins(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	_, secret := l.signIn(t)
	l.goodLogin(t, secret)
	auth, err := server.OpenWebAuth(l.h.StateDir, l.now)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(auth.List()); got != 2 {
		t.Errorf("%d web logins, want 2", got)
	}
}

func TestWebLoginLog_EachEventWritesOneLineWithIPAndName(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	l.post(t, "/v1/web/auth/setup-start", `{"code":"0000 0000"}`, nil)
	cookie, secret := l.signIn(t)
	l.failRound(t, 1)
	l.goodLogin(t, secret)
	l.post(t, "/v1/web/auth/sign-out", `{}`, cookie)

	log := readGxLog(t, l)
	auth, err := server.OpenWebAuth(l.h.StateDir, l.now)
	if err != nil {
		t.Fatal(err)
	}
	logins := auth.List()
	if len(logins) != 1 {
		t.Fatalf("%d web logins left, want the second one", len(logins))
	}
	for _, c := range []struct {
		msg    string
		count  int
		reason string
	}{
		{"web login", 2, ""},
		{"web login refused", 1, server.ReasonWrongCredentials},
		{"web setup refused", 1, server.ReasonSetupCodeWrong},
		{"web sign out", 1, ""},
	} {
		lines := logLines(t, log, c.msg)
		if len(lines) != c.count {
			t.Errorf("%q: %d log lines, want %d", c.msg, len(lines), c.count)
			continue
		}
		for _, m := range lines {
			if m["ip"] != "127.0.0.1" {
				t.Errorf("%q line %v: want ip 127.0.0.1", c.msg, m)
			}
			if m["reason"] != nil && m["reason"] != c.reason || m["reason"] == nil && c.reason != "" {
				t.Errorf("%q line %v: want reason %q", c.msg, m, c.reason)
			}
			if c.reason == "" && (m["name"] == nil || m["name"] == "") {
				t.Errorf("%q line %v: want the web login name", c.msg, m)
			}
		}
	}
}

func TestWebLoginLog_HoldsNoSecret(t *testing.T) {
	t.Parallel()
	l := startLoginWeb(t)
	info, err := l.h.Client.WebInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cookie, secret := l.signIn(t)
	code := l.totp(t, secret)
	l.failRound(t, 1)
	l.goodLogin(t, secret)
	l.post(t, "/v1/web/auth/sign-out", `{}`, cookie)
	report, _ := json.Marshal(map[string]string{
		"message": "boom", "url": "https://gx.example/#/setup?code=" + info.SetupCode, "user_agent": "test",
	})
	l.post(t, "/v1/client-errors", string(report), nil)

	log := readGxLog(t, l)
	for name, secret := range map[string]string{
		"password": loginTestPassword, "wrong password": "not the password", "TOTP code": code,
		"TOTP secret": secret, "cookie value": cookie.Value, "Setup code": info.SetupCode,
	} {
		if strings.Contains(log, secret) {
			t.Errorf("gx.log holds the %s", name)
		}
	}
	if len(logLines(t, log, "client error")) != 1 {
		t.Error("the error report left no line to check")
	}
}
