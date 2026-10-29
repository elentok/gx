package server_test

import (
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
)

// Every write the web mux serves is either a socket route or on the web-only
// list, and every web-only route is served by the mux.
func TestWeb_OnlyRoutesMatchTheMux(t *testing.T) {
	t.Parallel()
	socket := server.RoutePatterns()
	served := server.WebWritePatterns()
	listed := map[string]bool{}
	for _, route := range server.WebOnlyRoutes() {
		listed[route.Pattern] = true
		if !slices.Contains(served, route.Pattern) {
			t.Errorf("%s is on the web-only list but the web mux does not serve it", route.Pattern)
		}
		if slices.Contains(socket, route.Pattern) {
			t.Errorf("%s is on the web-only list but is also a socket route", route.Pattern)
		}
	}
	for _, pattern := range served {
		if !slices.Contains(socket, pattern) && !listed[pattern] {
			t.Errorf("%s is served by the web mux, is not a socket route, and is missing from the web-only list", pattern)
		}
	}
}

func postReport(t *testing.T, base, body string, headers map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/v1/client-errors", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := webClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestWeb_ClientErrorReportNeedsNoCookieAndLogsOneWarn(t *testing.T) {
	t.Parallel()
	h, base := startWeb(t)
	code, body := postReport(t, base, `{"message":"boom","stack":"at x","url":"http://h/#/q","user_agent":"UA/1"}`, jsonPost(base))
	if code != http.StatusOK || reasonOf(t, body) != "" {
		t.Fatalf("status %d body %q, want an accepted report", code, body)
	}
	logs, err := os.ReadFile(server.LogPath(h.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(string(logs), "\n") {
		if strings.Contains(l, `"client error"`) {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("%d client error lines, want 1:\n%s", len(lines), logs)
	}
	for _, want := range []string{`"level":"WARN"`, `boom`, `at x`, `http://h/#/q`, `UA/1`} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("log line %s lacks %q", lines[0], want)
		}
	}
}

func TestWeb_ClientErrorReportKeepsTheWriteChecks(t *testing.T) {
	t.Parallel()
	_, base := startWeb(t)
	if code, _ := postReport(t, base, `{}`, map[string]string{"Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("no Origin: status %d, want 403", code)
	}
	if code, _ := postReport(t, base, `{}`, map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("foreign Origin: status %d, want 403", code)
	}
	if code, _ := postReport(t, base, `{}`, map[string]string{"Origin": base, "Content-Type": "text/plain"}); code != http.StatusForbidden {
		t.Errorf("text/plain: status %d, want 403", code)
	}
}

func TestWeb_ClientErrorReportRefusesAnOversizeBody(t *testing.T) {
	t.Parallel()
	h, base := startWeb(t)
	big := `{"message":"` + strings.Repeat("a", 64<<10) + `"}`
	code, body := postReport(t, base, big, jsonPost(base))
	if code != http.StatusOK || reasonOf(t, body) != "too-large" {
		t.Errorf("status %d body %q, want a too-large refusal", code, body)
	}
	logs, _ := os.ReadFile(server.LogPath(h.StateDir))
	if strings.Contains(string(logs), `"client error"`) {
		t.Error("an oversize report was logged")
	}
}

func TestWeb_ClientErrorReportDropsReportsPastThePerIPLimit(t *testing.T) {
	t.Parallel()
	_, base := startWeb(t)
	refused := 0
	for range 60 {
		code, body := postReport(t, base, `{"message":"m"}`, jsonPost(base))
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if reasonOf(t, body) == "rate-limited" {
			refused++
		}
	}
	if refused == 0 {
		t.Error("no report was rate limited")
	}
}

func TestWeb_OtherRoutesStillNeedTheCookie(t *testing.T) {
	t.Parallel()
	_, base := startWeb(t)
	resp, err := webClient().Get(base + "/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
}
