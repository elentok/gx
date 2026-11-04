package chat

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTranscriptPaths_FoundPathIsGlobbedOnce(t *testing.T) {
	t.Parallel()
	var c TranscriptPaths
	finds := 0
	find := func() (string, bool) { finds++; return "/p/c.jsonl", true }
	now := time.Now()
	for range 3 {
		if path, ok := c.Resolve("c", "idle", now, DefaultTranscriptMissTTL, find); !ok || path != "/p/c.jsonl" {
			t.Fatalf("Resolve = %q, %v", path, ok)
		}
	}
	if finds != 1 {
		t.Errorf("finds = %d, want 1", finds)
	}
}

func TestTranscriptPaths_ForgetMissGlobsAgain(t *testing.T) {
	t.Parallel()
	var c TranscriptPaths
	finds := 0
	found := false
	find := func() (string, bool) { finds++; return "/p/c.jsonl", found }
	now := time.Now()
	c.Resolve("c", "idle", now, DefaultTranscriptMissTTL, find)
	found = true
	c.ForgetMiss("c")
	if path, ok := c.Resolve("c", "idle", now, DefaultTranscriptMissTTL, find); !ok || path != "/p/c.jsonl" || finds != 2 {
		t.Fatalf("after ForgetMiss: %q, %v, finds=%d", path, ok, finds)
	}
}

func TestTranscriptPaths_MissIsRememberedForTheTTL(t *testing.T) {
	t.Parallel()
	var c TranscriptPaths
	finds := 0
	found := false
	find := func() (string, bool) { finds++; return "/p/c.jsonl", found }
	now := time.Now()
	c.Resolve("c", "idle", now, DefaultTranscriptMissTTL, find)
	if _, ok := c.Resolve("c", "idle", now.Add(DefaultTranscriptMissTTL-time.Millisecond), DefaultTranscriptMissTTL, find); ok || finds != 1 {
		t.Fatalf("inside the TTL: ok=%v finds=%d, want a cached miss", ok, finds)
	}
	found = true
	if _, ok := c.Resolve("c", "idle", now.Add(DefaultTranscriptMissTTL), DefaultTranscriptMissTTL, find); !ok || finds != 2 {
		t.Errorf("after the TTL: ok=%v finds=%d, want a fresh lookup that finds it", ok, finds)
	}
}

func TestTranscriptPaths_RepeatedMissesBackOff(t *testing.T) {
	t.Parallel()
	var c TranscriptPaths
	finds := 0
	find := func() (string, bool) { finds++; return "", false }
	ttl := DefaultTranscriptMissTTL
	now := time.Now()
	c.Resolve("c", "idle", now, ttl, find)
	now = now.Add(ttl)
	c.Resolve("c", "idle", now, ttl, find)
	if finds != 2 {
		t.Fatalf("finds = %d after the first TTL, want 2", finds)
	}
	c.Resolve("c", "idle", now.Add(10*ttl-time.Millisecond), ttl, find)
	if finds != 2 {
		t.Fatalf("finds = %d inside the second TTL (10x), want 2", finds)
	}
	now = now.Add(10 * ttl)
	c.Resolve("c", "idle", now, ttl, find)
	c.Resolve("c", "idle", now.Add(100*ttl-time.Millisecond), ttl, find)
	if finds != 3 {
		t.Fatalf("finds = %d inside the third TTL (100x), want 3", finds)
	}
	c.Resolve("c", "idle", now.Add(100*ttl), ttl, find)
	if finds != 4 {
		t.Errorf("finds = %d after the third TTL, want 4", finds)
	}
	c.Resolve("c", "idle", now.Add(200*ttl-time.Millisecond), ttl, find)
	if finds != 4 {
		t.Errorf("finds = %d: the TTL must stay capped at 100x", finds)
	}
}

func TestTranscriptPaths_StatusChangeEndsABackedOffMiss(t *testing.T) {
	t.Parallel()
	var c TranscriptPaths
	finds := 0
	find := func() (string, bool) { finds++; return "", false }
	now := time.Now()
	c.Resolve("c", "idle", now, DefaultTranscriptMissTTL, find)
	c.Resolve("c", "working", now, DefaultTranscriptMissTTL, find)
	if finds != 2 {
		t.Fatalf("finds = %d after a status change, want a fresh lookup", finds)
	}
	c.Resolve("c", "working", now, DefaultTranscriptMissTTL, find)
	if finds != 2 {
		t.Errorf("finds = %d, want the new miss cached", finds)
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
	var c FileCache[string]
	for range 2 {
		if got, err := c.Read(path, compute); err != nil || got != "m" {
			t.Fatalf("Read = %q, %v", got, err)
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
	var c AuthFailures
	for range 2 {
		if auth, err := c.Read(path, check); err != nil || !auth {
			t.Fatalf("Read = %v, %v", auth, err)
		}
	}
	if checks != 1 {
		t.Fatalf("checks = %d after two reads of an unchanged file, want 1", checks)
	}
	if err := os.WriteFile(path, []byte("a\nbb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(path, check); err != nil {
		t.Fatal(err)
	}
	if checks != 2 {
		t.Errorf("checks = %d after the file grew, want 2", checks)
	}
}
