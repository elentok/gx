package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/transcript"
)

func TestTranscriptPaths_FoundPathIsGlobbedOnce(t *testing.T) {
	t.Parallel()
	var c transcriptPaths
	finds := 0
	find := func() (string, bool) { finds++; return "/p/c.jsonl", true }
	now := time.Now()
	for range 3 {
		if path, ok := c.resolve("c", "idle", now, defaultTranscriptMissTTL, find); !ok || path != "/p/c.jsonl" {
			t.Fatalf("resolve = %q, %v", path, ok)
		}
	}
	if finds != 1 {
		t.Errorf("finds = %d, want 1", finds)
	}
}

func TestTranscriptPaths_ForgetMissGlobsAgain(t *testing.T) {
	t.Parallel()
	var c transcriptPaths
	finds := 0
	found := false
	find := func() (string, bool) { finds++; return "/p/c.jsonl", found }
	now := time.Now()
	c.resolve("c", "idle", now, defaultTranscriptMissTTL, find)
	found = true
	c.forgetMiss("c")
	if path, ok := c.resolve("c", "idle", now, defaultTranscriptMissTTL, find); !ok || path != "/p/c.jsonl" || finds != 2 {
		t.Fatalf("after forgetMiss: %q, %v, finds=%d", path, ok, finds)
	}
}

func TestTranscriptPaths_MissIsRememberedForTheTTL(t *testing.T) {
	t.Parallel()
	var c transcriptPaths
	finds := 0
	found := false
	find := func() (string, bool) { finds++; return "/p/c.jsonl", found }
	now := time.Now()
	c.resolve("c", "idle", now, defaultTranscriptMissTTL, find)
	if _, ok := c.resolve("c", "idle", now.Add(defaultTranscriptMissTTL-time.Millisecond), defaultTranscriptMissTTL, find); ok || finds != 1 {
		t.Fatalf("inside the TTL: ok=%v finds=%d, want a cached miss", ok, finds)
	}
	found = true
	if _, ok := c.resolve("c", "idle", now.Add(defaultTranscriptMissTTL), defaultTranscriptMissTTL, find); !ok || finds != 2 {
		t.Errorf("after the TTL: ok=%v finds=%d, want a fresh lookup that finds it", ok, finds)
	}
}

func TestTranscriptPaths_RepeatedMissesBackOff(t *testing.T) {
	t.Parallel()
	var c transcriptPaths
	finds := 0
	find := func() (string, bool) { finds++; return "", false }
	ttl := defaultTranscriptMissTTL
	now := time.Now()
	c.resolve("c", "idle", now, ttl, find)
	now = now.Add(ttl)
	c.resolve("c", "idle", now, ttl, find)
	if finds != 2 {
		t.Fatalf("finds = %d after the first TTL, want 2", finds)
	}
	c.resolve("c", "idle", now.Add(10*ttl-time.Millisecond), ttl, find)
	if finds != 2 {
		t.Fatalf("finds = %d inside the second TTL (10x), want 2", finds)
	}
	now = now.Add(10 * ttl)
	c.resolve("c", "idle", now, ttl, find)
	c.resolve("c", "idle", now.Add(100*ttl-time.Millisecond), ttl, find)
	if finds != 3 {
		t.Fatalf("finds = %d inside the third TTL (100x), want 3", finds)
	}
	c.resolve("c", "idle", now.Add(100*ttl), ttl, find)
	if finds != 4 {
		t.Errorf("finds = %d after the third TTL, want 4", finds)
	}
	c.resolve("c", "idle", now.Add(200*ttl-time.Millisecond), ttl, find)
	if finds != 4 {
		t.Errorf("finds = %d: the TTL must stay capped at 100x", finds)
	}
}

func TestTranscriptPaths_StatusChangeEndsABackedOffMiss(t *testing.T) {
	t.Parallel()
	var c transcriptPaths
	finds := 0
	find := func() (string, bool) { finds++; return "", false }
	now := time.Now()
	c.resolve("c", "idle", now, defaultTranscriptMissTTL, find)
	c.resolve("c", "working", now, defaultTranscriptMissTTL, find)
	if finds != 2 {
		t.Fatalf("finds = %d after a status change, want a fresh lookup", finds)
	}
	c.resolve("c", "working", now, defaultTranscriptMissTTL, find)
	if finds != 2 {
		t.Errorf("finds = %d, want the new miss cached", finds)
	}
}

// A copy of the transcript under another project dir makes a glob ambiguous
// and so a miss: finding the expected one proves the glob was never run.
func TestClaudeTranscript_StatsTheExpectedSlugDirBeforeGlobbing(t *testing.T) {
	t.Parallel()
	config := t.TempDir()
	cwd := "/work/proj"
	want := filepath.Join(config, "projects", transcript.Slugify(cwd), "conv.jsonl")
	other := filepath.Join(config, "projects", "-elsewhere", "conv.jsonl")
	for _, p := range []string{want, other} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{log: slog.New(slog.DiscardHandler), cfg: Config{Env: []string{"CLAUDE_CONFIG_DIR=" + config}}}
	got, ok := s.claudeTranscript("s1", sessionMeta{Dir: cwd, ConversationID: "conv"})
	if !ok || got != want {
		t.Fatalf("claudeTranscript = %q, %v, want %q", got, ok, want)
	}
}

func TestClaudeTranscript_ReadsTheConfigDirFromTheServerEnv(t *testing.T) {
	t.Parallel()
	config := t.TempDir()
	cwd := "/work/proj"
	want := filepath.Join(config, "projects", transcript.Slugify(cwd), "conv.jsonl")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{log: slog.New(slog.DiscardHandler), cfg: Config{Env: []string{"CLAUDE_CONFIG_DIR=" + config}}}
	got, ok := s.claudeTranscript("s1", sessionMeta{Dir: cwd, ConversationID: "conv"})
	if !ok || got != want {
		t.Fatalf("claudeTranscript = %q, %v, want %q", got, ok, want)
	}
}

// The cache is shared by the auth check and the reported-model read; this one
// covers the non-bool value type.
func TestFileCache_ReusesAStringUntilTheFileChanges(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "c.jsonl")
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	computes := 0
	compute := func(string) (string, error) { computes++; return "m", nil }
	var c fileCache[string]
	for range 2 {
		if got, err := c.read(path, compute); err != nil || got != "m" {
			t.Fatalf("read = %q, %v", got, err)
		}
	}
	if computes != 1 {
		t.Errorf("computes = %d after two reads of an unchanged file, want 1", computes)
	}
}

func TestAuthFailures_RechecksOnlyWhenTheFileChanged(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "c.jsonl")
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checks := 0
	check := func(string) (bool, error) { checks++; return true, nil }
	var c authFailures
	for range 2 {
		if auth, err := c.read(path, check); err != nil || !auth {
			t.Fatalf("read = %v, %v", auth, err)
		}
	}
	if checks != 1 {
		t.Fatalf("checks = %d after two reads of an unchanged file, want 1", checks)
	}
	if err := os.WriteFile(path, []byte("a\nbb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.read(path, check); err != nil {
		t.Fatal(err)
	}
	if checks != 2 {
		t.Errorf("checks = %d after the file grew, want 2", checks)
	}
}
