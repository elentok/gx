package web_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/webperf"
)

// Every write the web mux serves is either a socket route or on the web-only
// list, and every web-only route is served by the mux.
func TestWeb_OnlyRoutesMatchTheMux(t *testing.T) {
	t.Parallel()
	socket := server.RoutePatterns()
	writes := server.WebWritePatterns()
	served := append(slices.Clone(writes), server.WebPublicGetPatterns()...)
	served = append(served, server.WebPrefixGetPatterns()...)
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
	for _, pattern := range writes {
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
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestWeb_ClientErrorReportNeedsNoCookieAndLogsOneWarn(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	code, body := postReport(t, base, `{"message":"boom","stack":"at x","url":"http://h/#/q","user_agent":"UA/1"}`, servertest.JSONPost(base))
	if code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
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
	_, base := servertest.StartWeb(t)
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
	h, base := servertest.StartWeb(t)
	big := `{"message":"` + strings.Repeat("a", 64<<10) + `"}`
	code, body := postReport(t, base, big, servertest.JSONPost(base))
	if code != http.StatusOK || servertest.ReasonOf(t, body) != "too-large" {
		t.Errorf("status %d body %q, want a too-large refusal", code, body)
	}
	logs, _ := os.ReadFile(server.LogPath(h.StateDir))
	if strings.Contains(string(logs), `"client error"`) {
		t.Error("an oversize report was logged")
	}
}

func TestWeb_ClientErrorReportDropsReportsPastThePerIPLimit(t *testing.T) {
	t.Parallel()
	_, base := servertest.StartWeb(t)
	refused := 0
	for range 60 {
		code, body := postReport(t, base, `{"message":"m"}`, servertest.JSONPost(base))
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if servertest.ReasonOf(t, body) == "rate-limited" {
			refused++
		}
	}
	if refused == 0 {
		t.Error("no report was rate limited")
	}
}

func TestWeb_OtherRoutesStillNeedTheCookie(t *testing.T) {
	t.Parallel()
	_, base := servertest.StartWeb(t)
	resp, err := servertest.WebClient().Get(base + "/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
}

func perfSample(id, metric string, value float64, extra string) string {
	return fmt.Sprintf(`{"id":%q,"metric":%q,"value":%v,"ts":%d%s}`, id, metric, value, time.Now().UnixMilli(), extra)
}

func perfBatch(version int, samples ...string) string {
	return fmt.Sprintf(`{"v":%d,"session":"s1","build_id":"abc123","web_version":"1.0","samples":[%s]}`, version, strings.Join(samples, ","))
}

func perfDayLines(t *testing.T, stateDir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stateDir, webperf.DirName, time.Now().Format(webperf.DayLayout)+".jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(strings.TrimSpace(string(b)))
}

func TestWeb_WebPerformanceKeepsTheLoginAndWriteChecks(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	url := base + "/v1/web-performance"
	body := perfBatch(1, perfSample("a", "lcp", 100, ""))
	if code, _ := servertest.WebPostJSON(t, url, body, nil, "", servertest.JSONPost(base)); code != http.StatusUnauthorized {
		t.Errorf("signed out: status %d, want 401", code)
	}
	if code, _ := servertest.WebPostJSON(t, url, body, cookie, "", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("foreign Origin: status %d, want 403", code)
	}
	if code, _ := servertest.WebPostJSON(t, url, body, cookie, "", map[string]string{"Origin": base, "Content-Type": "text/plain"}); code != http.StatusForbidden {
		t.Errorf("text/plain: status %d, want 403", code)
	}
}

func TestWeb_WebPerformanceStoresValidSamplesAndDropsTheRest(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	url := base + "/v1/web-performance"
	old := fmt.Sprintf(`{"id":"old","metric":"lcp","value":1,"ts":%d}`, time.Now().Add(-48*time.Hour).UnixMilli())
	body := perfBatch(1,
		perfSample("good-lcp", "lcp", 900, `,"ttfb":50,"input_delay":3,"action":"open-menu","data_source":"cache"`),
		perfSample("poor-inp", "inp", 300, `,"input_delay":10,"action":"open-menu"`),
		perfSample("bad-metric", "nope", 1, ""),
		perfSample("neg", "lcp", -1, ""),
		perfSample("route", "route_ready", 5, `,"route":"ticket/gx:epic/01"`),
		perfSample("query", "route_ready", 5, `,"route":"ticket?x=1"`),
		perfSample("enum", "route_ready", 5, `,"outcome":"weird"`),
		perfSample("long", "lcp", 5, `,"worst_chunk":"`+strings.Repeat("a", 65)+`"`),
		perfSample("url", "inp", 300, `,"action":"http://x/y?q=1"`),
		old,
	)
	code, out := servertest.WebPostJSON(t, url, body, cookie, "", servertest.JSONPost(base))
	if code != http.StatusOK || !strings.Contains(out, `"stored":2`) || !strings.Contains(out, `"dropped":8`) {
		t.Fatalf("status %d body %q, want 2 stored and 8 dropped", code, out)
	}
	lines := perfDayLines(t, h.StateDir)
	if len(lines) != 2 {
		t.Fatalf("%d stored lines, want 2: %v", len(lines), lines)
	}
	if strings.Contains(lines[0], "input_delay") || strings.Contains(lines[0], "action") || !strings.Contains(lines[0], `"ttfb":50`) || !strings.Contains(lines[0], `"data_source":"cache"`) {
		t.Errorf("good sample kept attribution or lost a field: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"input_delay":10`) || !strings.Contains(lines[1], `"action":"open-menu"`) {
		t.Errorf("poor INP lost its attribution: %s", lines[1])
	}
	if _, out := servertest.WebPostJSON(t, url, body, cookie, "", servertest.JSONPost(base)); !strings.Contains(out, `"stored":0`) {
		t.Errorf("a repeated batch answered %q, want 0 stored", out)
	}
}

func TestWeb_WebPerformanceKeepsAttributionPerMetricAndLooseVersion(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	url := base + "/v1/web-performance"
	body := strings.Replace(perfBatch(1,
		perfSample("inp", "inp", 300, `,"input_delay":10,"lcp_load_delay":7`),
		perfSample("lcp", "lcp", 3000, `,"lcp_load_delay":8,"input_delay":9,"action":"x"`),
		`{"id":"zero","metric":"lcp","value":1,"ts":0}`,
	), `"web_version":"1.0"`, `"web_version":"v1.2.3+5-gabc/x"`, 1)
	code, out := servertest.WebPostJSON(t, url, body, cookie, "", servertest.JSONPost(base))
	if code != http.StatusOK || !strings.Contains(out, `"stored":2`) || !strings.Contains(out, `"dropped":1`) {
		t.Fatalf("status %d body %q, want 2 stored and 1 dropped", code, out)
	}
	lines := perfDayLines(t, h.StateDir)
	if len(lines) != 2 {
		t.Fatalf("%d stored lines, want 2: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"input_delay":10`) || strings.Contains(lines[0], "lcp_load_delay") || !strings.Contains(lines[0], `v1.2.3+5-gabc/x`) {
		t.Errorf("poor INP kept the wrong fields: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"lcp_load_delay":8`) || strings.Contains(lines[1], "input_delay") || strings.Contains(lines[1], `"action"`) {
		t.Errorf("poor LCP kept the wrong fields: %s", lines[1])
	}
}

func TestWeb_WebPerformanceRefusesAndRejects(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	url := base + "/v1/web-performance"
	post := func(body string) (int, string) {
		return servertest.WebPostJSON(t, url, body, cookie, "", servertest.JSONPost(base))
	}
	many := make([]string, 51)
	for i := range many {
		many[i] = perfSample(fmt.Sprintf("m%d", i), "lcp", 1, "")
	}
	if code, out := post(perfBatch(1, many...)); code != http.StatusOK || servertest.ReasonOf(t, out) != "too-large" {
		t.Errorf("51 samples: status %d body %q, want too-large", code, out)
	}
	if code, out := post(perfBatch(1, perfSample("big", "lcp", 1, `,"action":"`+strings.Repeat("a", 33<<10)+`"`))); code != http.StatusOK || servertest.ReasonOf(t, out) != "too-large" {
		t.Errorf("33 KB: status %d body %q, want too-large", code, out)
	}
	if code, _ := post(`{not json`); code != http.StatusBadRequest {
		t.Errorf("bad JSON: status %d, want 400", code)
	}
	if code, _ := post(perfBatch(2, perfSample("v", "lcp", 1, ""))); code != http.StatusBadRequest {
		t.Errorf("schema 2: status %d, want 400", code)
	}
	if n := len(perfDayLines(t, h.StateDir)); n != 0 {
		t.Errorf("%d samples stored by refused or bad requests, want 0", n)
	}
	rateLimited := false
	for range 40 {
		if _, out := post(perfBatch(1)); strings.Contains(out, "rate-limited") {
			rateLimited = true
			break
		}
	}
	if !rateLimited {
		t.Error("no request was rate limited")
	}
}
