package codexdoctor

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func pass(Input) (string, bool) { return "ok", true }
func fail(Input) (string, bool) { return "broken", false }

func fixtures(t *testing.T) Source {
	t.Helper()
	src, err := FixtureSource()
	if err != nil {
		t.Fatalf("FixtureSource: %v", err)
	}
	return src
}

func byName(results []Result) map[string]Result {
	m := map[string]Result{}
	for _, r := range results {
		m[r.Name] = r
	}
	return m
}

func TestRun_DefaultTableAgainstFixtures(t *testing.T) {
	t.Parallel()
	results := Run(Table, fixtures(t))
	if len(results) == 0 {
		t.Fatal("no checks ran")
	}
	if got := byName(results)["rollout-format"]; got.Status != Pass {
		t.Errorf("rollout-format = %s (%s), want PASS", got.Status, got.Detail)
	}
	for _, r := range results {
		if r.Status == Fail {
			t.Errorf("%s failed on fixtures: %s", r.Name, r.Detail)
		}
	}
}

func TestRun_LiveChecksSkipOnFixtures(t *testing.T) {
	t.Parallel()
	got := byName(Run(Table, fixtures(t)))["codex-version"]
	if got.Status != Skip || got.Detail != "live only" {
		t.Errorf("codex-version = %s (%s), want SKIP live only", got.Status, got.Detail)
	}
	for _, c := range Table {
		if c.Name == "codex-version" && c.Mode != Live {
			t.Error("codex-version should be a live check")
		}
	}
}

func TestRun_SkipsDependentsOfFailedOrSkippedChecks(t *testing.T) {
	t.Parallel()
	table := []Check{
		{Name: "bad", Mode: Fixture, Run: fail},
		{Name: "live", Mode: Live, Run: pass},
		{Name: "after-bad", Mode: Fixture, DependsOn: []string{"bad"}, Run: pass},
		{Name: "after-live", Mode: Fixture, DependsOn: []string{"live"}, Run: pass},
	}
	got := byName(Run(table, Source{}))
	if got["bad"].Status != Fail {
		t.Errorf("bad = %s, want FAIL", got["bad"].Status)
	}
	for _, name := range []string{"after-bad", "after-live"} {
		if got[name].Status != Skip || !strings.HasPrefix(got[name].Detail, "needs ") {
			t.Errorf("%s = %s (%s), want SKIP needs ...", name, got[name].Status, got[name].Detail)
		}
	}
}

func TestFixtureVersionComesFromHeaders(t *testing.T) {
	t.Parallel()
	if got := fixtures(t).CodexVersion; got != "0.166.0" {
		t.Errorf("fixture version = %q, want 0.166.0", got)
	}
	meta := func(v string) []byte {
		return []byte(`{"type":"session_meta","payload":{"id":"a","cwd":"/","cli_version":"` + v + `"}}` + "\n")
	}
	src, err := LoadFixtures(fstest.MapFS{
		"new.jsonl":    {Data: meta("0.170.0")},
		"old.jsonl":    {Data: meta("0.167.1")},
		"sample.jsonl": {Data: []byte(`{"type":"event_msg","payload":{"type":"agent_message"}}` + "\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if src.CodexVersion != "0.167.1" {
		t.Errorf("version = %q, want the oldest 0.167.1", src.CodexVersion)
	}
}

func TestNewerThanFixtures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		installed, fixtures string
		want                bool
	}{
		{"0.167.0", "0.166.0", true},
		{"0.166.0", "0.166.0", false},
		{"0.165.9", "0.166.0", false},
		{"", "0.166.0", false},
	} {
		if got := NewerThanFixtures(tc.installed, tc.fixtures); got != tc.want {
			t.Errorf("NewerThanFixtures(%q, %q) = %v, want %v", tc.installed, tc.fixtures, got, tc.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()
	if got := ParseVersion("codex-cli 0.166.0\n"); got != "0.166.0" {
		t.Errorf("ParseVersion = %q", got)
	}
	if got := ParseVersion("something else"); got != "" {
		t.Errorf("ParseVersion of junk = %q, want empty", got)
	}
}

func TestCodexVersionCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		version string
		want    Status
	}{
		{"0.166.0", Pass},
		{"0.170.2", Pass},
		{"0.165.9", Fail},
		{"", Fail},
	} {
		src := Source{Live: true, CodexVersion: tc.version}
		got := byName(Run(Table, Source{Live: true, CodexVersion: src.CodexVersion, Session: fixtures(t).Session}))["codex-version"]
		if got.Status != tc.want {
			t.Errorf("version %q = %s (%s), want %s", tc.version, got.Status, got.Detail, tc.want)
		}
	}
}

func fixtureRollout(t *testing.T, name string) []byte {
	t.Helper()
	data, err := fs.ReadFile(embedded, "testdata/rollouts/"+name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// replaceIn renames a key everywhere in data, so no line keeps it.
func replaceIn(t *testing.T, data []byte, old, new string) []byte {
	t.Helper()
	if !bytes.Contains(data, []byte(old)) {
		t.Fatalf("fixture has no %q", old)
	}
	return bytes.ReplaceAll(data, []byte(old), []byte(new))
}

func TestRolloutFormat_NamesTheMissingField(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, file, old, new, want string
	}{
		{"session id", "plain-turn.jsonl", `","id":`, `","xid":`, "payload.id"},
		{"cwd", "plain-turn.jsonl", `"cwd":`, `"xcwd":`, "payload.cwd"},
		{"cli_version", "plain-turn.jsonl", `"cli_version":`, `"xcli_version":`, "payload.cli_version"},
		{"used_percent", "rate-limits-sample.jsonl", `"used_percent":`, `"xused_percent":`, "payload.rate_limits.primary.used_percent"},
		{"resets_at", "rate-limits-sample.jsonl", `"resets_at":`, `"xresets_at":`, "payload.rate_limits.primary.resets_at"},
		{"total_tokens", "plain-turn.jsonl", `"total_tokens":`, `"xtotal_tokens":`, "payload.info.total_token_usage.total_tokens"},
		{"turn_id", "plain-turn.jsonl", `"turn_id":`, `"xturn_id":`, "payload.turn_id"},
		{"role", "plain-turn.jsonl", `"role":`, `"xrole":`, "payload.role"},
	} {
		got := CheckRollout(replaceIn(t, fixtureRollout(t, tc.file), tc.old, tc.new))
		if !strings.HasPrefix(got, tc.want+" missing") {
			t.Errorf("%s: CheckRollout = %q, want %q missing", tc.name, got, tc.want)
		}
	}
}

func TestRolloutFormat_SyntheticLines(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, line, want string
	}{
		{"function_call ok", `{"type":"response_item","payload":{"type":"function_call","name":"n","call_id":"c","arguments":"{}"}}`, ""},
		{"function_call no arguments", `{"type":"response_item","payload":{"type":"function_call","name":"n","call_id":"c"}}`, "payload.arguments"},
		{"custom_tool_call no input", `{"type":"response_item","payload":{"type":"custom_tool_call","name":"n","call_id":"c"}}`, "payload.input"},
		{"custom_tool_call no name", `{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c","input":"x"}}`, "payload.name"},
		{"task_complete error ok", `{"type":"event_msg","payload":{"type":"task_complete","turn_id":"t","error":{"message":"m","codex_error_info":"x"}}}`, ""},
		{"task_complete null error", `{"type":"event_msg","payload":{"type":"task_complete","turn_id":"t","error":null}}`, ""},
		{"task_complete error no message", `{"type":"event_msg","payload":{"type":"task_complete","turn_id":"t","error":{"codex_error_info":"x"}}}`, "payload.error.message"},
		{"task_complete error no info", `{"type":"event_msg","payload":{"type":"task_complete","turn_id":"t","error":{"message":"m"}}}`, "payload.error.codex_error_info"},
		{"compacted no payload", `{"type":"compacted"}`, "payload"},
		{"compacted ok", `{"type":"compacted","payload":{"message":""}}`, ""},
		{"null secondary", `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":1}},"rate_limits":{"primary":{"used_percent":1,"resets_at":2},"secondary":null}}}`, ""},
		{"message no content", `{"type":"response_item","payload":{"type":"message","role":"user"}}`, "payload.content"},
	} {
		got := CheckRollout([]byte(tc.line + "\n"))
		switch {
		case tc.want == "" && got != "":
			t.Errorf("%s: CheckRollout = %q, want ok", tc.name, got)
		case tc.want != "" && !strings.HasPrefix(got, tc.want+" missing"):
			t.Errorf("%s: CheckRollout = %q, want %q missing", tc.name, got, tc.want)
		}
	}
}

func TestRolloutFormat_FailsTheCheckOnAMutatedFixture(t *testing.T) {
	t.Parallel()
	bad := replaceIn(t, fixtureRollout(t, "rate-limits-sample.jsonl"), `"used_percent":`, `"xused_percent":`)
	src, err := LoadFixtures(fstest.MapFS{"bad.jsonl": {Data: bad}})
	if err != nil {
		t.Fatal(err)
	}
	got := byName(Run(Table, src))["rollout-format"]
	if got.Status != Fail || !strings.Contains(got.Detail, "payload.rate_limits.primary.used_percent missing") {
		t.Errorf("rollout-format = %s (%s), want FAIL naming used_percent", got.Status, got.Detail)
	}
}

func TestLiveRollout(t *testing.T) {
	t.Parallel()
	base := fixtures(t)
	rec, _ := base.Session(RolloutsSession)

	if _, err := WithLiveRollout(t.TempDir(), rec); !errors.Is(err, ErrNoSession) {
		t.Errorf("empty CODEX_HOME err = %v, want ErrNoSession", err)
	}

	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "10", "30")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-x.jsonl"), fixtureRollout(t, "plain-turn.jsonl"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := WithLiveRollout(home, rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rollouts) != len(rec.Rollouts)+1 || !strings.HasSuffix(got.Rollouts[0].Name, "rollout-x.jsonl") {
		t.Errorf("rollouts = %d, first %q", len(got.Rollouts), got.Rollouts[0].Name)
	}
}
