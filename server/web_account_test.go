package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

type accountLogin struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	LastIP     string `json:"last_ip"`
	ThisDevice bool   `json:"this_device"`
}

// accountWeb is a web listener with two web logins: first and second.
type accountWeb struct {
	*loginWeb
	first, second *http.Cookie
	secret        string
}

func startAccountWeb(t *testing.T) *accountWeb {
	t.Helper()
	l := startLoginWeb(t)
	first, secret := l.signIn(t)
	return &accountWeb{loginWeb: l, first: first, second: l.goodLogin(t, secret), secret: secret}
}

func (a *accountWeb) logins(t *testing.T, cookie *http.Cookie) []accountLogin {
	t.Helper()
	resp, body := a.post(t, "/v1/web/logins", `{}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: status %d, body %q", resp.StatusCode, body)
	}
	var out struct{ Logins []accountLogin }
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("list body %q: %v", body, err)
	}
	return out.Logins
}

func TestWebAccount_ListMarksThisDevice(t *testing.T) {
	t.Parallel()
	a := startAccountWeb(t)
	for _, c := range []*http.Cookie{a.first, a.second} {
		logins := a.logins(t, c)
		if len(logins) != 2 {
			t.Fatalf("%d web logins, want 2", len(logins))
		}
		this := 0
		for _, l := range logins {
			if l.ThisDevice {
				this++
			}
			if l.ID == "" || l.Name == "" || l.LastIP != "127.0.0.1" {
				t.Errorf("login %+v lacks an id, a name or the last IP", l)
			}
		}
		if this != 1 {
			t.Errorf("%d logins marked as this device, want 1", this)
		}
	}
	if a.logins(t, a.first)[0].ThisDevice == a.logins(t, a.second)[0].ThisDevice {
		t.Error("both cookies mark the same web login as this device")
	}
}

func TestWebAccount_RenameIsStoredAndLogged(t *testing.T) {
	t.Parallel()
	a := startAccountWeb(t)
	id := a.logins(t, a.first)[1].ID
	_, body := a.post(t, "/v1/web/logins/rename", `{"id":"`+id+`","name":"Kitchen tablet"}`, a.first)
	if reason := reasonOf(t, body); reason != "" {
		t.Fatalf("rename refused: %q", reason)
	}
	if got := a.logins(t, a.first)[1].Name; got != "Kitchen tablet" {
		t.Errorf("name %q, want the new one", got)
	}
	lines := logLines(t, readGxLog(t, a.loginWeb), "web login renamed")
	if len(lines) != 1 || lines[0]["ip"] != "127.0.0.1" || lines[0]["name"] != "Kitchen tablet" {
		t.Errorf("rename log lines %v, want one with the IP and the new name", lines)
	}
	_, body = a.post(t, "/v1/web/logins/rename", `{"id":"`+id+`","name":""}`, a.first)
	if got := reasonOf(t, body); got != server.ReasonLoginNameInvalid {
		t.Errorf("empty name reason %q, want %q", got, server.ReasonLoginNameInvalid)
	}
}

func TestWebAccount_RevokeEndsTheStreamAndTheNextRequestGets401(t *testing.T) {
	t.Parallel()
	a := startAccountWeb(t)
	var second accountLogin
	for _, l := range a.logins(t, a.second) {
		if l.ThisDevice {
			second = l
		}
	}
	req, _ := http.NewRequest(http.MethodGet, a.base+"/v1/events", nil)
	req.AddCookie(a.second)
	stream, err := webClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	ended := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, stream.Body); close(ended) }()

	_, body := a.post(t, "/v1/web/logins/revoke", `{"id":"`+second.ID+`"}`, a.first)
	if reason := reasonOf(t, body); reason != "" {
		t.Fatalf("revoke refused: %q", reason)
	}
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Error("the event stream stayed open after the revoke")
	}
	if resp, _ := a.get(t, "/v1/snapshot", a.second); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("snapshot after revoke: status %d, want 401", resp.StatusCode)
	}
	if resp, _ := a.get(t, "/v1/snapshot", a.first); resp.StatusCode != http.StatusOK {
		t.Errorf("snapshot of the other login: status %d, want 200", resp.StatusCode)
	}
	lines := logLines(t, readGxLog(t, a.loginWeb), "web login revoked")
	if len(lines) != 1 || lines[0]["ip"] != "127.0.0.1" || lines[0]["name"] != second.Name {
		t.Errorf("revoke log lines %v, want one with the IP and name %q", lines, second.Name)
	}
}

func TestWebAccount_RevokeAndSignOutForgetTheCookieRenewal(t *testing.T) {
	t.Parallel()
	a := startAccountWeb(t)
	a.logins(t, a.first)
	a.logins(t, a.second)
	if got := a.h.Server.WebRenewedCount(); got != 2 {
		t.Fatalf("renewal entries before = %d, want 2", got)
	}
	var second accountLogin
	for _, l := range a.logins(t, a.second) {
		if l.ThisDevice {
			second = l
		}
	}
	a.post(t, "/v1/web/logins/revoke", `{"id":"`+second.ID+`"}`, a.first)
	if got := a.h.Server.WebRenewedCount(); got != 1 {
		t.Errorf("renewal entries after revoke = %d, want 1", got)
	}
	a.post(t, "/v1/web/auth/sign-out", `{}`, a.first)
	if got := a.h.Server.WebRenewedCount(); got != 0 {
		t.Errorf("renewal entries after sign-out = %d, want 0", got)
	}
}

func TestWebAccount_UnknownIDIsRefusedWithItsReason(t *testing.T) {
	t.Parallel()
	a := startAccountWeb(t)
	for _, path := range []string{"/v1/web/logins/rename", "/v1/web/logins/revoke"} {
		_, body := a.post(t, path, `{"id":"nope","name":"x"}`, a.first)
		if got := reasonOf(t, body); got != server.ReasonLoginNotFound {
			t.Errorf("%s: reason %q, want %q", path, got, server.ReasonLoginNotFound)
		}
	}
	if got := len(a.logins(t, a.first)); got != 2 {
		t.Errorf("%d web logins after the refusals, want 2", got)
	}
}

func TestWebAccount_RoutesNeedTheCookieAndASameHostOrigin(t *testing.T) {
	t.Parallel()
	a := startAccountWeb(t)
	for _, path := range []string{"/v1/web/logins", "/v1/web/logins/rename", "/v1/web/logins/revoke"} {
		if resp, _ := a.post(t, path, `{"id":"x","name":"x"}`, nil); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without the cookie: status %d, want 401", path, resp.StatusCode)
		}
		req, _ := http.NewRequest(http.MethodPost, a.base+path, strings.NewReader(`{}`))
		req.Header.Set("Origin", "http://evil.example")
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(a.first)
		if resp, _ := doBody(t, req); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s with a foreign Origin: status %d, want 403", path, resp.StatusCode)
		}
	}
	if got := len(a.logins(t, a.first)); got != 2 {
		t.Errorf("%d web logins after the refused calls, want 2", got)
	}
}

const accountNewPassword = "a brand new passphrase"

// codeNow moves the clock one step on and returns a fresh code, since a code works once.
func (l *loginWeb) codeNow(t *testing.T, secret string) string {
	t.Helper()
	l.advance(31 * time.Second)
	return l.totp(t, secret)
}

func (a *accountWeb) changePassword(t *testing.T, cookie *http.Cookie, current, code, next string) string {
	t.Helper()
	_, body := a.post(t, "/v1/web/account/password",
		`{"password":"`+current+`","totp_code":"`+code+`","new_password":"`+next+`"}`, cookie)
	return reasonOf(t, body)
}

func (a *accountWeb) streamOf(t *testing.T, cookie *http.Cookie) <-chan struct{} {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, a.base+"/v1/events", nil)
	req.AddCookie(cookie)
	stream, err := webClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stream.Body.Close() })
	ended := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, stream.Body); close(ended) }()
	return ended
}

func (a *accountWeb) wantOnlyFirstLeft(t *testing.T, ended <-chan struct{}) {
	t.Helper()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Error("the other login's event stream stayed open")
	}
	if resp, _ := a.get(t, "/v1/snapshot", a.second); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("other login after the change: status %d, want 401", resp.StatusCode)
	}
	if got := len(a.logins(t, a.first)); got != 1 {
		t.Errorf("%d web logins left, want only the current one", got)
	}
}

func TestWebAccount_ChangePasswordRefusesBadChecksAndSignsOutTheOthers(t *testing.T) {
	t.Parallel()
	a := startAccountWeb(t)
	secret := a.secret
	if got := a.changePassword(t, a.first, "not the password", a.codeNow(t, secret), accountNewPassword); got != server.ReasonWrongCredentials {
		t.Errorf("wrong password reason %q, want %q", got, server.ReasonWrongCredentials)
	}
	if got := a.changePassword(t, a.first, loginTestPassword, "000000", accountNewPassword); got != server.ReasonWrongCredentials {
		t.Errorf("wrong code reason %q, want %q", got, server.ReasonWrongCredentials)
	}
	if got := a.changePassword(t, a.first, loginTestPassword, a.codeNow(t, secret), "short"); got != server.ReasonPasswordTooShort {
		t.Errorf("short password reason %q, want %q", got, server.ReasonPasswordTooShort)
	}
	if got := len(a.logins(t, a.first)); got != 2 {
		t.Fatalf("%d web logins after the refusals, want 2", got)
	}
	ended := a.streamOf(t, a.second)
	if got := a.changePassword(t, a.first, loginTestPassword, a.codeNow(t, secret), accountNewPassword); got != "" {
		t.Fatalf("change password refused: %q", got)
	}
	a.wantOnlyFirstLeft(t, ended)
	if res, cookie := a.attemptLogin(t, loginTestPassword, a.codeNow(t, secret)); cookie != nil {
		t.Errorf("old password still logs in: %+v", res)
	}
	if res, cookie := a.attemptLogin(t, accountNewPassword, a.codeNow(t, secret)); cookie == nil {
		t.Errorf("new password does not log in: %+v", res)
	}
	log := readGxLog(t, a.loginWeb)
	if lines := logLines(t, log, "web password changed"); len(lines) != 1 || lines[0]["ip"] != "127.0.0.1" {
		t.Errorf("password log lines %v, want one with the IP", lines)
	}
	if strings.Contains(log, accountNewPassword) {
		t.Error("the log holds the new password")
	}
}

func TestWebAccount_FiveWrongPasswordsLockTheIPForLogin(t *testing.T) {
	t.Parallel()
	a := startAccountWeb(t)
	for i := 0; i < 5; i++ {
		a.changePassword(t, a.first, "not the password", "000000", accountNewPassword)
	}
	if got := a.changePassword(t, a.first, loginTestPassword, a.codeNow(t, a.secret), accountNewPassword); got != server.ReasonRateLimited {
		t.Errorf("6th change password reason %q, want %q", got, server.ReasonRateLimited)
	}
	if res, _ := a.attemptLogin(t, loginTestPassword, a.totp(t, a.secret)); res.Reason != server.ReasonRateLimited {
		t.Errorf("login after the lock: %+v, want rate limited", res)
	}
}

func (a *accountWeb) resetBegin(t *testing.T, cookie *http.Cookie, password, code string) authAnswer {
	t.Helper()
	_, body := a.post(t, "/v1/web/account/authenticator/begin", `{"password":"`+password+`","totp_code":"`+code+`"}`, cookie)
	return decodeAuth(t, body)
}

func (a *accountWeb) resetFinish(t *testing.T, cookie *http.Cookie, code string) authAnswer {
	t.Helper()
	_, body := a.post(t, "/v1/web/account/authenticator/finish", `{"totp_code":"`+code+`"}`, cookie)
	return decodeAuth(t, body)
}

func TestWebAccount_AbandonedResetKeepsTheOldSecretAndFinishSwapsIt(t *testing.T) {
	t.Parallel()
	a := startAccountWeb(t)
	if got := a.resetBegin(t, a.first, "not the password", a.codeNow(t, a.secret)); got.Reason != server.ReasonWrongCredentials {
		t.Errorf("begin with a wrong password reason %q, want %q", got.Reason, server.ReasonWrongCredentials)
	}
	begun := a.resetBegin(t, a.first, loginTestPassword, a.codeNow(t, a.secret))
	if begun.Reason != "" || begun.Secret == "" || !strings.HasPrefix(begun.URI, "otpauth://") {
		t.Fatalf("begin = %+v, want a secret and an otpauth URI", begun)
	}
	a.goodLogin(t, a.secret) // abandoned: the old secret still works
	if got := a.resetFinish(t, a.first, "000000"); got.Reason != server.ReasonTOTPMismatch {
		t.Errorf("finish with a wrong code reason %q, want %q", got.Reason, server.ReasonTOTPMismatch)
	}
	ended := a.streamOf(t, a.second)
	if got := a.resetFinish(t, a.first, a.codeNow(t, begun.Secret)); got.Reason != "" {
		t.Fatalf("finish refused: %+v", got)
	}
	a.wantOnlyFirstLeft(t, ended)
	if res, cookie := a.attemptLogin(t, loginTestPassword, a.codeNow(t, a.secret)); cookie != nil {
		t.Errorf("old secret still logs in: %+v", res)
	}
	if res, cookie := a.attemptLogin(t, loginTestPassword, a.codeNow(t, begun.Secret)); cookie == nil {
		t.Errorf("new secret does not log in: %+v", res)
	}
	if lines := logLines(t, readGxLog(t, a.loginWeb), "web authenticator reset"); len(lines) != 1 {
		t.Errorf("reset log lines %v, want one", lines)
	}
}

func TestWebAccount_TOTPOffNeedsNoCodeAndRefusesTheResetRoutes(t *testing.T) {
	t.Parallel()
	w := startTOTPWeb(t, false)
	a := &accountWeb{loginWeb: w.loginWeb}
	a.first = w.setupWithoutTOTP(t)
	if got := a.changePassword(t, a.first, loginTestPassword, "", accountNewPassword); got != "" {
		t.Errorf("change password without a code refused: %q", got)
	}
	for _, path := range []string{"/v1/web/account/authenticator/begin", "/v1/web/account/authenticator/finish"} {
		_, body := a.post(t, path, `{"password":"`+accountNewPassword+`"}`, a.first)
		if got := reasonOf(t, body); got != server.ReasonTOTPOff {
			t.Errorf("%s reason %q, want %q", path, got, server.ReasonTOTPOff)
		}
	}
}

func TestWebAccount_NoSavedSecretLetsResetBeginTakeThePasswordAlone(t *testing.T) {
	t.Parallel()
	w := startTOTPWeb(t, false)
	a := &accountWeb{loginWeb: w.loginWeb}
	a.first = w.setupWithoutTOTP(t)
	w.reloadTOTP(t, true)
	begun := a.resetBegin(t, a.first, loginTestPassword, "")
	if begun.Reason != "" || begun.Secret == "" {
		t.Fatalf("add authenticator begin = %+v, want a secret", begun)
	}
	if got := a.resetFinish(t, a.first, a.codeNow(t, begun.Secret)); got.Reason != "" {
		t.Fatalf("finish refused: %+v", got)
	}
	if res, cookie := a.attemptLogin(t, loginTestPassword, a.codeNow(t, begun.Secret)); cookie == nil {
		t.Errorf("the first secret does not log in: %+v", res)
	}
}
