package webhttp_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func TestWeb_DetailIsServed(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	// The handler answers 404 for a malformed address; the mux's own 404 has another body.
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/tickets/detail?address=nope")
	if code == http.StatusUnauthorized || (code == http.StatusNotFound && !strings.Contains(body, "no ticket nope")) {
		t.Errorf("GET /v1/tickets/detail = %d %q, want it routed", code, body)
	}
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, base+"/v1/tickets/detail", nil); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/tickets/detail without cookie = %d, want 401", code)
	}
}

func TestWeb_EpicDetailIsServed(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics/detail?address=nope:none")
	if code == http.StatusUnauthorized || code == http.StatusMethodNotAllowed || code == http.StatusForbidden {
		t.Errorf("GET /v1/epics/detail with cookie = %d, want it routed", code)
	}
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, base+"/v1/epics/detail", nil); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/epics/detail without cookie = %d, want 401", code)
	}
}

func TestWeb_AnswerAndResumeNeedAStateToken(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	for _, path := range []string{"/v1/tickets/answer", "/v1/tickets/resume", "/v1/tickets/unpark"} {
		code, body := servertest.WebPostJSON(t, base+path, `{"address":"nope/none/01"}`, cookie, "", servertest.JSONPost(base))
		if code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonStale {
			t.Errorf("%s without token = %d %s, want a stale refusal", path, code, body)
		}
		code, body = servertest.WebPostJSON(t, base+path, `{"address":"nope/none/01","state":"1:paused"}`, cookie, "", servertest.JSONPost(base))
		if code != http.StatusOK || servertest.ReasonOf(t, body) == wire.ReasonStale {
			t.Errorf("%s with token = %d %s, want the handler to run", path, code, body)
		}
	}
}

func TestWeb_NoWithdrawRoute(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	for _, path := range []string{"/v1/tickets/withdraw", "/v1/tickets/undo"} {
		if code, _ := servertest.WebPostJSON(t, base+path, `{"state":"1:paused"}`, cookie, "", servertest.JSONPost(base)); code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404", path, code)
		}
	}
}

func TestWeb_AnswerWriteChecks(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	body := `{"address":"nope/none/01","state":"1:paused"}`

	if code, _ := servertest.WebPostJSON(t, base+"/v1/tickets/answer", body, cookie, "", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("foreign Origin = %d, want 403", code)
	}
	if code, _ := servertest.WebPostJSON(t, base+"/v1/tickets/answer", body, cookie, "", map[string]string{"Origin": "null", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("Origin null = %d, want 403", code)
	}
	if code, _ := servertest.WebPostJSON(t, base+"/v1/tickets/answer", body, cookie, "", map[string]string{"Origin": base, "Content-Type": "application/json; charset=utf-8"}); code != http.StatusOK {
		t.Errorf("JSON with charset = %d, want 200", code)
	}
	// The Host check runs before the token check.
	if code, _ := servertest.WebPostJSON(t, base+"/v1/tickets/answer", body, nil, "evil.example", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("wrong Host without cookie = %d, want 403", code)
	}
}
