package commandlist

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseCodexSkills_Recording(t *testing.T) {
	f, err := os.Open("testdata/codex-skills-list.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	got, err := parseCodexSkills(f)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Name: "dup-skill", Description: "A duplicated skill", Source: SourceSkill},
		{Name: "skill-creator", Description: "Create or update a Codex skill with appropriately scoped instructions and any needed supporting resources.", Source: SourceSkill},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].Description != want[i].Description || got[i].Source != want[i].Source {
			t.Errorf("entry %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A skill the user cannot run (off-skill is enabled:false in the recording) is
// left out of the command list.
func TestParseCodexSkills_LeavesOutDisabledSkill(t *testing.T) {
	f, err := os.Open("testdata/codex-skills-list.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := parseCodexSkills(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range got {
		if e.Name == "off-skill" {
			t.Errorf("entries hold the disabled skill: %+v", got)
		}
	}
}

func TestParseCodexSkills_ErrorReply(t *testing.T) {
	_, err := parseCodexSkills(strings.NewReader(`{"id":2,"error":{"message":"boom"}}` + "\n"))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want one naming boom", err)
	}
}

func TestParseCodexSkills_NoReply(t *testing.T) {
	if _, err := parseCodexSkills(strings.NewReader(`{"id":1,"result":{}}` + "\n")); err == nil {
		t.Fatal("want an error when the reply never arrives")
	}
}

// stubCodex puts a `codex` script first on PATH and returns the dir it logs to.
func stubCodex(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	logDir := t.TempDir()
	script := "#!/bin/sh\n" + strings.ReplaceAll(body, "$LOG", logDir)
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logDir
}

func TestProbeCodexSkills_RequestSequence(t *testing.T) {
	recording, err := filepath.Abs("testdata/codex-skills-list.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	logDir := stubCodex(t, `
echo "$@" > $LOG/args
pwd -P > $LOG/cwd
head -n 3 > $LOG/stdin
cat `+recording+`
exec sleep 30
`)
	dir := t.TempDir()

	got, err := ProbeCodexSkills(context.Background(), dir, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 entries", got)
	}

	if args := readTrim(t, filepath.Join(logDir, "args")); args != "app-server" {
		t.Errorf("args = %q", args)
	}
	wantDir, _ := filepath.EvalSymlinks(dir)
	if cwd := readTrim(t, filepath.Join(logDir, "cwd")); cwd != wantDir {
		t.Errorf("cwd = %q, want %q", cwd, wantDir)
	}

	lines := strings.Split(readTrim(t, filepath.Join(logDir, "stdin")), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdin lines = %q", lines)
	}
	var msgs [3]struct {
		ID     *int            `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	for i, l := range lines {
		if err := json.Unmarshal([]byte(l), &msgs[i]); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
	}
	if msgs[0].Method != "initialize" || msgs[0].ID == nil {
		t.Errorf("first request = %+v", msgs[0])
	}
	if msgs[1].Method != "initialized" || msgs[1].ID != nil {
		t.Errorf("second must be the initialized notification (no id): %+v", msgs[1])
	}
	if msgs[2].Method != "skills/list" || msgs[2].ID == nil || *msgs[2].ID != codexSkillsRequestID {
		t.Errorf("third request = %+v", msgs[2])
	}
	var params struct {
		Cwds        []string `json:"cwds"`
		ForceReload bool     `json:"forceReload"`
	}
	if err := json.Unmarshal(msgs[2].Params, &params); err != nil {
		t.Fatal(err)
	}
	if len(params.Cwds) != 1 || params.Cwds[0] != dir || !params.ForceReload {
		t.Errorf("skills/list params = %+v, want cwds [%s] and forceReload", params, dir)
	}
}

func TestProbeCodexSkills_Timeout(t *testing.T) {
	stubCodex(t, "exec sleep 30\n")
	old := codexProbeTimeout
	codexProbeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { codexProbeTimeout = old })

	start := time.Now()
	_, err := ProbeCodexSkills(context.Background(), t.TempDir(), os.Environ())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("returned after %v, the child was not killed", elapsed)
	}
}

const codexModelsPage1 = `{"id":2,"result":{"data":[` +
	`{"id":"gpt-a","model":"gpt-a-resolved","hidden":false,"defaultReasoningEffort":"medium",` +
	`"supportedReasoningEfforts":[{"reasoningEffort":"high","description":"x"},{"reasoningEffort":"low"}],"upgrade":"gpt-b"},` +
	`{"id":"gpt-old","model":"gpt-old","hidden":true,"defaultReasoningEffort":"low","supportedReasoningEfforts":[],` +
	`"upgrade":null,"upgradeInfo":{"model":"gpt-a"}}],"nextCursor":"c1"}}`

const codexModelsPage2 = `{"id":3,"result":{"data":[` +
	`{"id":"gpt-c","model":"gpt-c","hidden":false,"defaultReasoningEffort":"none","supportedReasoningEfforts":[],"upgrade":null}],` +
	`"nextCursor":null}}`

func TestProbeCodexModels_FollowsCursor(t *testing.T) {
	// Each page is only sent after the previous reply, so the stub reads a
	// request line before it answers.
	logDir := stubCodex(t, `
pwd -P > $LOG/cwd
read l1; read l2; read l3
echo "$l3" > $LOG/page1
echo '`+codexModelsPage1+`'
read l4
echo "$l4" > $LOG/page2
echo '`+codexModelsPage2+`'
exec sleep 30
`)
	dir := t.TempDir()

	got, err := ProbeCodexModels(context.Background(), dir, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	want := []Model{
		{Value: "gpt-a", ID: "gpt-a-resolved", Efforts: []string{"high", "low"}, DefaultEffort: "medium", Upgrade: "gpt-b"},
		{Value: "gpt-old", ID: "gpt-old", Hidden: true, Efforts: []string{}, DefaultEffort: "low", Upgrade: "gpt-a"},
		{Value: "gpt-c", ID: "gpt-c", Efforts: []string{}, DefaultEffort: "none"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	wantDir, _ := filepath.EvalSymlinks(dir)
	if cwd := readTrim(t, filepath.Join(logDir, "cwd")); cwd != wantDir {
		t.Errorf("cwd = %q, want %q", cwd, wantDir)
	}
	var p1, p2 struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal([]byte(readTrim(t, filepath.Join(logDir, "page1"))), &p1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(readTrim(t, filepath.Join(logDir, "page2"))), &p2); err != nil {
		t.Fatal(err)
	}
	if p1.Method != "model/list" || p1.Params["includeHidden"] != true || p1.Params["cursor"] != nil {
		t.Errorf("first page request = %+v", p1)
	}
	if p2.Method != "model/list" || p2.Params["includeHidden"] != true || p2.Params["cursor"] != "c1" {
		t.Errorf("second page request = %+v", p2)
	}
}

func TestProbeCodexModels_Failures(t *testing.T) {
	cases := map[string]string{
		"error reply":   `read a; read b; read c; echo '{"id":2,"error":{"message":"boom"}}'; exec sleep 30`,
		"no reply":      `read a; read b; read c; exit 0`,
		"error on page": `read a; read b; read c; echo '` + codexModelsPage1 + `'; read d; echo '{"id":3,"error":{"message":"late"}}'; exec sleep 30`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			stubCodex(t, body+"\n")
			if _, err := ProbeCodexModels(context.Background(), t.TempDir(), os.Environ()); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestProbeCodexModels_Timeout(t *testing.T) {
	stubCodex(t, "exec sleep 30\n")
	old := codexProbeTimeout
	codexProbeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { codexProbeTimeout = old })

	start := time.Now()
	_, err := ProbeCodexModels(context.Background(), t.TempDir(), os.Environ())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("returned after %v, the child was not killed", elapsed)
	}
}

func readTrim(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}
