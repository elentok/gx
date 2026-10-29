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
}

func startAccountWeb(t *testing.T) *accountWeb {
	t.Helper()
	l := startLoginWeb(t)
	first, secret := l.signIn(t)
	return &accountWeb{loginWeb: l, first: first, second: l.goodLogin(t, secret)}
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
