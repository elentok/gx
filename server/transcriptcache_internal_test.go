package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTranscriptPaths_FoundPathIsGlobbedOnce(t *testing.T) {
	var c transcriptPaths
	finds := 0
	find := func() (string, bool) { finds++; return "/p/c.jsonl", true }
	now := time.Now()
	for range 3 {
		if path, ok := c.resolve("c", now, defaultTranscriptMissTTL, find); !ok || path != "/p/c.jsonl" {
			t.Fatalf("resolve = %q, %v", path, ok)
		}
	}
	if finds != 1 {
		t.Errorf("finds = %d, want 1", finds)
	}
}

func TestTranscriptPaths_MissIsRememberedForTheTTL(t *testing.T) {
	var c transcriptPaths
	finds := 0
	found := false
	find := func() (string, bool) { finds++; return "/p/c.jsonl", found }
	now := time.Now()
	c.resolve("c", now, defaultTranscriptMissTTL, find)
	if _, ok := c.resolve("c", now.Add(defaultTranscriptMissTTL-time.Millisecond), defaultTranscriptMissTTL, find); ok || finds != 1 {
		t.Fatalf("inside the TTL: ok=%v finds=%d, want a cached miss", ok, finds)
	}
	found = true
	if _, ok := c.resolve("c", now.Add(defaultTranscriptMissTTL), defaultTranscriptMissTTL, find); !ok || finds != 2 {
		t.Errorf("after the TTL: ok=%v finds=%d, want a fresh lookup that finds it", ok, finds)
	}
}

func TestAuthFailures_RechecksOnlyWhenTheFileChanged(t *testing.T) {
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
