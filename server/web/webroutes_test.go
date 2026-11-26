package web_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
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
	for name, big := range map[string]string{
		"oversize": `{"message":"` + strings.Repeat("a", 64<<10) + `"}`,
		"padded":   `{"message":"m"}` + strings.Repeat(" ", 17<<10),
	} {
		code, body := postReport(t, base, big, servertest.JSONPost(base))
		if code != http.StatusOK || servertest.ReasonOf(t, body) != "too-large" {
			t.Errorf("%s: status %d body %q, want a too-large refusal", name, code, body)
		}
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

// brokenRuleBody is a complete report; override replaces or adds fields.
func brokenRuleBody(t *testing.T, override map[string]string) string {
	t.Helper()
	body := map[string]string{
		"rule": "text-box-name", "message": "a box has no name", "value": "search",
		"url": "http://h/#/setup?code=SECRET", "user_agent": "UA/1",
	}
	maps.Copy(body, override)
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// startBrokenRules starts a server that files broken rules into proj:follow-ups
// and returns it with its URL, a signed-in cookie and the epic's directory.
func startBrokenRules(t *testing.T) (*servertest.Harness, string, *http.Cookie, string) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.RecoverySettings.FollowUps = "proj:follow-ups"
	})
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return h, hs.WebURL, servertest.Login(t, h.StateDir, hs.WebURL), filepath.Join(store, "proj", "follow-ups")
}

func brokenRuleLines(t *testing.T, stateDir string) []string {
	t.Helper()
	logs, err := os.ReadFile(server.LogPath(stateDir))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for l := range strings.SplitSeq(string(logs), "\n") {
		if strings.Contains(l, `"broken rule"`) {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestWeb_BrokenRuleReportKeepsTheLoginAndWriteChecks(t *testing.T) {
	t.Parallel()
	if !slices.Contains(server.WebOnlyRoutes(), server.WebOnlyRoute{Pattern: "POST /v1/broken-rules"}) {
		t.Error("POST /v1/broken-rules is not on the web-only list as a signed-in route")
	}
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	url := base + "/v1/broken-rules"
	body := brokenRuleBody(t, nil)
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

func TestWeb_BrokenRuleReportRefusesBadReportsWithoutFilingThem(t *testing.T) {
	t.Parallel()
	h, base, cookie, epic := startBrokenRules(t)
	url := base + "/v1/broken-rules"
	for name, tc := range map[string]struct{ body, reason string }{
		"oversize":      {brokenRuleBody(t, map[string]string{"value": strings.Repeat("a", 17<<10)}), "too-large"},
		"padded":        {brokenRuleBody(t, nil) + strings.Repeat(" ", 17<<10), "too-large"},
		"no fields":     {`{}`, "incomplete"},
		"empty value":   {brokenRuleBody(t, map[string]string{"value": ""}), "incomplete"},
		"empty agent":   {brokenRuleBody(t, map[string]string{"user_agent": ""}), "incomplete"},
		"empty rule":    {brokenRuleBody(t, map[string]string{"rule": ""}), "incomplete"},
		"empty message": {brokenRuleBody(t, map[string]string{"message": ""}), "incomplete"},
		"empty URL":     {brokenRuleBody(t, map[string]string{"url": ""}), "incomplete"},
		"upper case":    {brokenRuleBody(t, map[string]string{"rule": "Text"}), "bad-rule"},
		"double hyphen": {brokenRuleBody(t, map[string]string{"rule": "a--b"}), "bad-rule"},
		"edge hyphen":   {brokenRuleBody(t, map[string]string{"rule": "a-"}), "bad-rule"},
		"long rule":     {brokenRuleBody(t, map[string]string{"rule": strings.Repeat("a", 65)}), "bad-rule"},
		"newline rule":  {brokenRuleBody(t, map[string]string{"rule": "a\n"}), "bad-rule"},
	} {
		code, body := servertest.WebPostJSON(t, url, tc.body, cookie, "", servertest.JSONPost(base))
		if code != http.StatusOK || servertest.ReasonOf(t, body) != tc.reason {
			t.Errorf("%s: status %d body %q, want a %s refusal", name, code, body, tc.reason)
		}
	}
	if code, _ := servertest.WebPostJSON(t, url, `{"rule":1}`, cookie, "", servertest.JSONPost(base)); code != http.StatusBadRequest {
		t.Errorf("bad JSON: status %d, want 400", code)
	}
	if lines := brokenRuleLines(t, h.StateDir); len(lines) != 0 {
		t.Errorf("refused reports were logged: %v", lines)
	}
	if _, err := os.Stat(epic); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("follow-ups epic: %v, want nothing filed", err)
	}
}

func TestWeb_BrokenRuleReportLogsOneLineAndFilesOneDraft(t *testing.T) {
	t.Parallel()
	h, base, cookie, epic := startBrokenRules(t)
	post := func() {
		t.Helper()
		code, body := servertest.WebPostJSON(t, base+"/v1/broken-rules", brokenRuleBody(t, map[string]string{"rule": strings.Repeat("a", 64)}), cookie, "", servertest.JSONPost(base))
		if code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
			t.Fatalf("status %d body %q, want an accepted report", code, body)
		}
	}
	post()
	lines := brokenRuleLines(t, h.StateDir)
	if len(lines) != 1 {
		t.Fatalf("%d broken rule lines, want 1: %v", len(lines), lines)
	}
	for _, want := range []string{`"level":"WARN"`, strings.Repeat("a", 64), "a box has no name", "search", "http://h/#/setup", "UA/1", "127.0.0.1"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("log line %s lacks %q", lines[0], want)
		}
	}
	post()
	files, err := filepath.Glob(filepath.Join(epic, "issues", "*.md"))
	if err != nil || len(files) != 1 {
		t.Fatalf("drafts = %v, %v; want one", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Seen again in the web UI on ") {
		t.Errorf("the repeat is not noted:\n%s", raw)
	}
	if strings.Contains(string(raw)+strings.Join(brokenRuleLines(t, h.StateDir), "\n"), "SECRET") {
		t.Error("the setup code in the hash query was kept")
	}
}

func TestWeb_BrokenRuleReportAnswers500WhenTheTicketCannotBeWritten(t *testing.T) {
	t.Parallel()
	_, base, cookie, epic := startBrokenRules(t)
	// A file where the epic directory goes makes the write fail.
	if err := os.WriteFile(epic, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	code, _ := servertest.WebPostJSON(t, base+"/v1/broken-rules", brokenRuleBody(t, nil), cookie, "", servertest.JSONPost(base))
	if code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", code)
	}
}

func TestWeb_BrokenRuleReportHasItsOwnPerIPLimit(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	report := func() string {
		t.Helper()
		code, body := servertest.WebPostJSON(t, base+"/v1/broken-rules", brokenRuleBody(t, nil), cookie, "", servertest.JSONPost(base))
		if code != http.StatusOK {
			t.Fatalf("status %d body %q", code, body)
		}
		return servertest.ReasonOf(t, body)
	}
	for i := range 20 {
		if reason := report(); reason != "" {
			t.Fatalf("report %d refused: %s", i+1, reason)
		}
	}
	if reason := report(); reason != "rate-limited" {
		t.Errorf("report 21: reason %q, want rate-limited", reason)
	}
	if n := len(brokenRuleLines(t, h.StateDir)); n != 20 {
		t.Errorf("%d broken rule lines, want 20", n)
	}
	if _, body := postReport(t, base, `{"message":"m"}`, servertest.JSONPost(base)); servertest.ReasonOf(t, body) != "" {
		t.Errorf("client error from the same IP: %q, want it accepted", body)
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
