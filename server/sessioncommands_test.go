package server_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// commandsFixture is a server whose "claude" and "codex" are scripts that run
// the test binary as the fakes, answering the probes from cfg. Every probe
// appends a line to counter.
type commandsFixture struct {
	designsFixture
	counter string
	home    string
	config  string
}

func startCommands(t *testing.T, cfg agentfake.ProbeConfig, opts ...func(*server.Config)) commandsFixture {
	t.Helper()
	bin, home, config := t.TempDir(), t.TempDir(), t.TempDir()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	commands := filepath.Join(t.TempDir(), "commands.json")
	if err := os.WriteFile(commands, data, 0o644); err != nil {
		t.Fatal(err)
	}
	counter := filepath.Join(t.TempDir(), "probes")
	for name, marker := range map[string]string{"claude": agentfake.TUIEnv, "codex": agentfake.CodexEnv} {
		script := "#!/bin/sh\necho x >> '" + counter + "'\nexport " + marker + "=1 " + agentfake.CommandsFileEnv + "='" + commands + "'\nexec '" + os.Args[0] + "' \"$@\"\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	opts = append([]func(*server.Config){servertest.WithEnv("PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+home, "CLAUDE_CONFIG_DIR="+config)}, opts...)
	return commandsFixture{designsFixture: startDesigns(t, opts...), counter: counter, home: home, config: config}
}

func (f commandsFixture) probes(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(f.counter)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "x")
}

// get calls the route with query and decodes a 200 answer.
func (f commandsFixture) get(t *testing.T, query url.Values) (int, server.SessionCommandsResponse, server.SessionResult) {
	t.Helper()
	code, body := webDoCookie(t, http.MethodGet, f.base+"/v1/sessions/commands?"+query.Encode(), f.ck)
	var res server.SessionCommandsResponse
	var refusal server.SessionResult
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &res); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
	} else if err := json.Unmarshal([]byte(body), &refusal); err != nil {
		t.Fatalf("status %d body %q: %v", code, body, err)
	} else {
		t.Logf("status %d: %s", code, refusal.Message)
	}
	return code, res, refusal
}

func (f commandsFixture) projectQuery(kind string) url.Values {
	q := url.Values{"project": {"proj"}, "dir": {f.repo}}
	if kind != "" {
		q.Set("kind", kind)
	}
	return q
}

func names(list server.SessionCommandsResponse) map[string]server.SessionCommandEntry {
	out := map[string]server.SessionCommandEntry{}
	for _, e := range list.Entries {
		out[e.Name] = e
	}
	return out
}

// createSession starts a fake-harness session in a temp dir.
func (f commandsFixture) createSession(t *testing.T, kind string) string {
	t.Helper()
	res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: kind, Ticket: "proj:map"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	return res.Session.ID
}

// seedSession writes a session record of harnessName without starting it.
func (f commandsFixture) seedSession(t *testing.T, id, harnessName string) {
	t.Helper()
	dir := filepath.Join(f.h.SessionsDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{"id": id, "harness": harnessName, "project": "proj", "dir": t.TempDir()})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCommands_ClaudeMergesDedupesAndUnescapes(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{Entries: []agentfake.ProbeEntry{
		{Name: "myskill", Description: "run &lt;x&gt;", ArgumentHint: "[x]"},
		{Name: "compact", Description: "mine", Builtin: true},
		{Name: "srv:review (MCP)"},
	}})
	code, got, _ := f.get(t, f.projectQuery(""))
	if code != http.StatusOK || got.Degraded {
		t.Fatalf("code %d, degraded %v", code, got.Degraded)
	}
	byName := names(got)
	if byName["myskill"].Description != "run <x>" || byName["myskill"].ArgHint != "[x]" {
		t.Errorf("myskill = %+v", byName["myskill"])
	}
	if byName["mcp__srv__review"].Source != "mcp" {
		t.Errorf("mcp entry = %+v", byName["mcp__srv__review"])
	}
	if _, ok := byName["help"]; !ok {
		t.Errorf("built-ins are missing: %v", got.Entries)
	}
	count := 0
	for _, e := range got.Entries {
		if e.Name == "compact" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("compact listed %d times", count)
	}
}

func TestSessionCommands_ProbeLeavesNoTranscriptOrSession(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{Entries: []agentfake.ProbeEntry{{Name: "myskill"}}})
	if code, _, _ := f.get(t, f.projectQuery("")); code != http.StatusOK {
		t.Fatalf("code %d", code)
	}
	if f.probes(t) != 1 {
		t.Fatalf("probes = %d, want 1", f.probes(t))
	}
	for _, dir := range []string{f.home, f.config, f.h.SessionsDir} {
		left, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if len(left) != 0 {
			t.Errorf("probe left %v in %s", left, dir)
		}
	}
}

func TestSessionCommands_FailedProbeFallsBackToBuiltins(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{Fail: true})
	code, got, _ := f.get(t, f.projectQuery(""))
	if code != http.StatusOK || !got.Degraded {
		t.Fatalf("code %d, degraded %v", code, got.Degraded)
	}
	if _, ok := names(got)["compact"]; !ok {
		t.Errorf("built-ins are missing: %v", got.Entries)
	}
	f.get(t, f.projectQuery(""))
	if f.probes(t) != 2 {
		t.Errorf("a failed probe was cached: %d probes", f.probes(t))
	}
}

func TestSessionCommands_SecondCallWithinAgeDoesNotProbe(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{Entries: []agentfake.ProbeEntry{{Name: "myskill"}}})
	for range 2 {
		if code, _, _ := f.get(t, f.projectQuery("")); code != http.StatusOK {
			t.Fatalf("code %d", code)
		}
	}
	if f.probes(t) != 1 {
		t.Errorf("probes = %d, want 1", f.probes(t))
	}
}

func TestSessionCommands_ConfiguredAgeShortensTheCache(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{Entries: []agentfake.ProbeEntry{{Name: "myskill"}}},
		func(c *server.Config) { c.CommandListAge = 50 * time.Millisecond })
	if !testutil.Poll(10*time.Second, 20*time.Millisecond, func() bool {
		f.get(t, f.projectQuery(""))
		return f.probes(t) >= 2
	}) {
		t.Fatalf("probes = %d after the age passed, want 2", f.probes(t))
	}
}

func TestSessionCommands_ConcurrentCallsShareOneProbe(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{Delay: "500ms", Entries: []agentfake.ProbeEntry{{Name: "myskill"}}})
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, _, _ := f.get(t, f.projectQuery("")); code != http.StatusOK {
				t.Errorf("code %d", code)
			}
		}()
	}
	wg.Wait()
	if f.probes(t) != 1 {
		t.Errorf("probes = %d, want 1", f.probes(t))
	}
}

func TestSessionCommands_DirOutsideTheProjectIsRefused(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{})
	code, _, refusal := f.get(t, url.Values{"project": {"proj"}, "dir": {t.TempDir()}})
	if code != http.StatusBadRequest || refusal.Reason != server.ReasonSessionBadRequest {
		t.Errorf("code %d, refusal %+v", code, refusal)
	}
	if f.probes(t) != 0 {
		t.Errorf("a refused dir started the harness")
	}
}

func TestSessionCommands_ClaudeDollarListIsEmpty(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{})
	code, got, _ := f.get(t, f.projectQuery("$"))
	if code != http.StatusOK || len(got.Entries) != 0 || got.Degraded {
		t.Errorf("code %d, list %+v", code, got)
	}
}

func TestSessionCommands_SessionIsServedFromItsDir(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{Entries: []agentfake.ProbeEntry{{Name: "myskill"}}},
		servertest.WithTestHolder())
	id := f.createSession(t, string(agentfake.HarnessKind))
	if code, got, _ := f.get(t, url.Values{"session": {id}}); code != http.StatusOK || names(got)["myskill"].Name == "" {
		t.Errorf("live session: code %d, list %+v", code, got)
	}
	if res, err := f.h.Client.StopSession(f.ctx, id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	if code, got, _ := f.get(t, url.Values{"session": {id}}); code != http.StatusOK || names(got)["myskill"].Name == "" {
		t.Errorf("exited session: code %d, list %+v", code, got)
	}
}

func TestSessionCommands_CodexListsBuiltinsAndSkills(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{Entries: []agentfake.ProbeEntry{{Name: "myskill", Description: "mine"}}})
	f.seedSession(t, "cdx", "codex")
	code, got, _ := f.get(t, url.Values{"session": {"cdx"}})
	if code != http.StatusOK || names(got)["compact"].Source != "built-in" || len(got.Entries) == 0 {
		t.Errorf("/ list: code %d, %+v", code, got)
	}
	if f.probes(t) != 0 {
		t.Errorf("Codex's / list started the harness")
	}
	code, got, _ = f.get(t, url.Values{"session": {"cdx"}, "kind": {"$"}})
	if code != http.StatusOK || names(got)["myskill"].Source != "skill" {
		t.Errorf("$ list: code %d, %+v", code, got)
	}
}

func TestSessionCommands_Refusals(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{}, servertest.WithTestHolder())
	f.seedSession(t, "other", "no-such-harness")
	cases := []struct {
		name   string
		query  url.Values
		code   int
		reason string
	}{
		{"unknown session", url.Values{"session": {"nope"}}, http.StatusNotFound, server.ReasonSessionUnknown},
		{"harness without a list", url.Values{"session": {"other"}}, http.StatusBadRequest, server.ReasonPromptHarnessNotSupported},
		{"bad id", url.Values{"session": {"../x"}}, http.StatusBadRequest, server.ReasonSessionBadRequest},
		{"neither", url.Values{}, http.StatusBadRequest, server.ReasonSessionBadRequest},
		{"both", url.Values{"session": {"other"}, "project": {"proj"}, "dir": {"/"}}, http.StatusBadRequest, server.ReasonSessionBadRequest},
		{"project without dir", url.Values{"project": {"proj"}}, http.StatusBadRequest, server.ReasonSessionBadRequest},
		{"bad kind", url.Values{"session": {"other"}, "kind": {"!"}}, http.StatusBadRequest, server.ReasonSessionBadRequest},
	}
	for _, c := range cases {
		code, _, refusal := f.get(t, c.query)
		if code != c.code || refusal.Reason != c.reason {
			t.Errorf("%s: code %d, refusal %+v; want %d %s", c.name, code, refusal, c.code, c.reason)
		}
	}
}

func TestSessionCommands_NoResumeHarnessIsNotSupported(t *testing.T) {
	t.Parallel()
	f := startCommands(t, agentfake.ProbeConfig{}, servertest.WithTestHolder())
	id := f.createSession(t, string(agentfake.NoResumeHarnessKind))
	code, _, refusal := f.get(t, url.Values{"session": {id}})
	if code != http.StatusBadRequest || refusal.Reason != server.ReasonPromptHarnessNotSupported {
		t.Errorf("code %d, refusal %+v", code, refusal)
	}
}
