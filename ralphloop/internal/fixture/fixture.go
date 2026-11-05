// Package fixture holds the file and transcript helpers ralphloop's tests
// share. It imports no ralphloop package, so every ralphloop package's tests
// can use it. Helpers that need Deps, EventSink or runnerfake belong in fakes.
package fixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// homeDir is the dir the last SetHomeEnv call pointed $HOME at.
var homeDir string

// HomeDir is the dir SetHomeEnv pointed $HOME at, or "" when none is active.
// Tests that use SetHomeEnv are not parallel, so no parallel test sees it.
func HomeDir() string { return homeDir }

// SetHomeEnv points $HOME at dir for the rest of the test, restored on
// cleanup via a manual os.Setenv/os.Unsetenv pair rather than t.Setenv, which
// panics once a test has called t.Parallel(). Production code in this
// package that still reads $HOME from the process env sees dir, so
// this remains a process-wide mutation — a test using it must stay
// non-parallel with any other test that also touches $HOME.
func SetHomeEnv(t *testing.T, dir string) {
	t.Helper()
	homeDir = dir
	t.Cleanup(func() { homeDir = "" })
	prev, had := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatalf("Setenv HOME: %v", err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv("HOME", prev)
		} else {
			os.Unsetenv("HOME")
		}
	})
}

// TicketPath is the path of ticket file in epicName's issues dir under scratchDir.
func TicketPath(scratchDir, epicName, file string) string {
	return filepath.Join(scratchDir, epicName, "issues", file)
}

// WriteEpic builds a fixture epic directory under a fresh temp dir's
// .scratch/{name}/issues/, with one ticket file per entry in tickets (each a
// "NN-slug.md" -> content pair), and returns the scratch dir.
func WriteEpic(t *testing.T, epicName string, tickets map[string]string) string {
	t.Helper()
	// t.TempDir() always ends in "001", and the merge lock dir is keyed by that
	// basename, so parallel tests would share one lock. Use a unique name.
	scratchDir, err := os.MkdirTemp("", "gx-epic-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(scratchDir) })
	issuesDir := filepath.Join(scratchDir, epicName, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(scratchDir, epicName))
	for name, content := range tickets {
		if err := os.WriteFile(filepath.Join(issuesDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	return scratchDir
}

// WriteTicket writes content to a fresh temp dir's 01-some-ticket.md and
// returns its path.
func WriteTicket(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-some-ticket.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// WriteFrontmatterTicket writes a minimal valid ticket file with the given
// status and returns its path.
func WriteFrontmatterTicket(t *testing.T, status string) string {
	t.Helper()
	return WriteTicket(t, "---\nid: \"01\"\nstatus: "+status+"\ntype: implement\n---\n# Ticket\n\nBody text.\n")
}

func MustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(raw)
}

func MustParse(t *testing.T, path string) schema.Ticket {
	t.Helper()
	ticket, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatalf("schema.ParseTicket: %v", err)
	}
	return ticket
}

// CanonicalAddr is the address logEvent stamps for ticket id in epic under
// scratchDir (no project.json in tests, so the project is the dir name).
func CanonicalAddr(scratchDir, epic, id string) string {
	return filepath.Base(scratchDir) + ":" + epic + "/" + id
}

// EpicScratchDir returns a fresh scratch dir with epicName's directory
// already created, since logEvent drops events for a missing epic dir.
func EpicScratchDir(t *testing.T, epicName string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, epicName), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return dir
}

// WriteFakeTranscript writes a session transcript under a fake
// <home>/.claude/projects/<slug>/<sessionID>.jsonl, with one assistant line per
// (model, inputTokens, cacheReadTokens) entry, one second apart starting at
// start.
func WriteFakeTranscript(t *testing.T, home, cwd, sessionID string, start time.Time, turns ...[3]any) {
	t.Helper()
	if home == "" {
		t.Fatal("WriteFakeTranscript requires a home")
	}
	path := transcript.PathIn(home, cwd, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	var lines []string
	for i, turn := range turns {
		ts := start.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano)
		model := turn[0].(string)
		input := turn[1].(int)
		cacheRead := turn[2].(int)
		lines = append(lines, `{"type":"assistant","timestamp":"`+ts+`","message":{"model":"`+model+`","usage":{"input_tokens":`+strconv.Itoa(input)+`,"cache_read_input_tokens":`+strconv.Itoa(cacheRead)+`,"output_tokens":10}}}`)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// codexRolloutTestEpoch anchors synthetic rollout JSONL timestamps,
// mirroring herdrfake.RegisterCodexRollout's own fixed epoch.
var codexRolloutTestEpoch = time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)

// appendCodexRolloutLine appends one JSONL record to a synthetic Codex
// rollout file at path, in the shape codexsession's reader expects (mirroring
// herdrfake.CodexRollout's private appendLine) — needed because
// herdrfake.State.Register claims "agent prompt"/"agent wait" globally by
// verb+noun with no per-pane scoping, so RegisterCodexRollout can't coexist
// with a second, differently-behaved Codex pane in the same test.
func appendCodexRolloutLine(path string, line int, value map[string]any) error {
	value["timestamp"] = codexRolloutTestEpoch.Add(time.Duration(line) * time.Millisecond).Format(time.RFC3339Nano)
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(encoded, '\n'))
	return err
}

func WriteCodexSessionMeta(path string, line int, sessionID, cwd string) error {
	return appendCodexRolloutLine(path, line, map[string]any{
		"type":    "session_meta",
		"payload": map[string]any{"id": sessionID, "cwd": cwd},
	})
}

func WriteCodexUsageLine(path string, line, contextTokens, totalTokens int) error {
	return appendCodexRolloutLine(path, line, map[string]any{
		"type": "event_msg",
		"payload": map[string]any{
			"type": "token_count",
			"info": map[string]any{
				"last_token_usage":  map[string]any{"input_tokens": contextTokens},
				"total_token_usage": map[string]any{"total_tokens": totalTokens},
			},
			"rate_limits": map[string]any{
				"primary":   map[string]any{"used_percent": 0, "resets_at": 0},
				"secondary": map[string]any{"used_percent": 0, "resets_at": 0},
			},
		},
	})
}
