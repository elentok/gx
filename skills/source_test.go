package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestScanSource_WhatCountsAsASkill(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	elsewhere := t.TempDir()
	mk := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk(filepath.Join(src, "plain", "SKILL.md"), "x")
	mk(filepath.Join(src, "nomarker", "other.md"), "x")
	mk(filepath.Join(src, "loose.md"), "x")
	mk(filepath.Join(elsewhere, "linked", "SKILL.md"), "x")
	mk(filepath.Join(elsewhere, "real-marker.md"), "x")
	if err := os.Symlink(filepath.Join(elsewhere, "linked"), filepath.Join(src, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "symmarker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(elsewhere, "real-marker.md"), filepath.Join(src, "symmarker", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	got, err := ScanSource(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"linked", "plain", "symmarker"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ScanSource = %v, want %v", got, want)
	}
}

func TestSourceID_IsStableAndFileSafe(t *testing.T) {
	t.Parallel()
	a, b := SourceID("/a/b"), SourceID("/a/c")
	if a == b || a != SourceID("/a/b") {
		t.Errorf("ids: %s %s", a, b)
	}
	if filepath.Base(a) != a {
		t.Errorf("id not a file name: %s", a)
	}
}
