package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
)

// webPostJSON sends a POST with a JSON body. A nil cookie sends none; host, when
// set, replaces the Host header. It returns the status and the body.
func webPostJSON(t *testing.T, url, body string, cookie *http.Cookie, host string, headers map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if host != "" {
		req.Host = host
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

func jsonPost(base string) map[string]string {
	return map[string]string{"Origin": base, "Content-Type": "application/json"}
}

func reasonOf(t *testing.T, body string) string {
	t.Helper()
	var res struct {
		Refused bool   `json:"refused"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return res.Reason
}

func TestWeb_DetailIsServed(t *testing.T) {
	h, base := startWeb(t)
	code, _ := webDo(t, h, http.MethodGet, base+"/v1/tickets/detail?address=nope")
	if code == http.StatusNotFound || code == http.StatusUnauthorized {
		t.Errorf("GET /v1/tickets/detail = %d, want it routed", code)
	}
	if code, _ := webDoCookie(t, http.MethodGet, base+"/v1/tickets/detail", nil); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/tickets/detail without cookie = %d, want 401", code)
	}
}

func TestWeb_EpicDetailIsServed(t *testing.T) {
	h, base := startWeb(t)
	code, _ := webDo(t, h, http.MethodGet, base+"/v1/epics/detail?address=nope:none")
	if code == http.StatusUnauthorized || code == http.StatusMethodNotAllowed || code == http.StatusForbidden {
		t.Errorf("GET /v1/epics/detail with cookie = %d, want it routed", code)
	}
	if code, _ := webDoCookie(t, http.MethodGet, base+"/v1/epics/detail", nil); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/epics/detail without cookie = %d, want 401", code)
	}
}

func TestWeb_AnswerAndUnparkNeedAStateToken(t *testing.T) {
	h, base := startWeb(t)
	cookie := login(t, h.StateDir, base)
	for _, path := range []string{"/v1/tickets/answer", "/v1/tickets/resume", "/v1/tickets/unpark"} {
		code, body := webPostJSON(t, base+path, `{"address":"nope/none/01"}`, cookie, "", jsonPost(base))
		if code != http.StatusOK || reasonOf(t, body) != server.ReasonStale {
			t.Errorf("%s without token = %d %s, want a stale refusal", path, code, body)
		}
		code, body = webPostJSON(t, base+path, `{"address":"nope/none/01","state":"1:needs-answer"}`, cookie, "", jsonPost(base))
		if code != http.StatusOK || reasonOf(t, body) == server.ReasonStale {
			t.Errorf("%s with token = %d %s, want the handler to run", path, code, body)
		}
	}
}

func TestWeb_NoWithdrawRoute(t *testing.T) {
	h, base := startWeb(t)
	cookie := login(t, h.StateDir, base)
	for _, path := range []string{"/v1/tickets/withdraw", "/v1/tickets/undo"} {
		if code, _ := webPostJSON(t, base+path, `{"state":"1:needs-answer"}`, cookie, "", jsonPost(base)); code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404", path, code)
		}
	}
}

func TestWeb_AnswerWriteChecks(t *testing.T) {
	h, base := startWeb(t)
	cookie := login(t, h.StateDir, base)
	body := `{"address":"nope/none/01","state":"1:needs-answer"}`

	if code, _ := webPostJSON(t, base+"/v1/tickets/answer", body, cookie, "", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("foreign Origin = %d, want 403", code)
	}
	if code, _ := webPostJSON(t, base+"/v1/tickets/answer", body, cookie, "", map[string]string{"Origin": "null", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("Origin null = %d, want 403", code)
	}
	if code, _ := webPostJSON(t, base+"/v1/tickets/answer", body, cookie, "", map[string]string{"Origin": base, "Content-Type": "application/json; charset=utf-8"}); code != http.StatusOK {
		t.Errorf("JSON with charset = %d, want 200", code)
	}
	// The Host check runs before the token check.
	if code, _ := webPostJSON(t, base+"/v1/tickets/answer", body, nil, "evil.example", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("wrong Host without cookie = %d, want 403", code)
	}
}
