package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCodexHomeDuplicates(t *testing.T) {
	t.Parallel()
	files := []SourceFile{
		{RelPath: "gx.md"},
		{RelPath: "gx-design/SKILL.md"},
		{RelPath: "gx-design/agents/openai.yaml"},
		{RelPath: "gx-tdd/SKILL.md"},
		{RelPath: "gx-merge/SKILL.md"},
	}
	home := t.TempDir()
	for _, name := range []string{"gx-design", "gx-merge", "other-skill"} {
		if err := os.MkdirAll(filepath.Join(home, "skills", name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// A top-level file named like a skill dir is not a skill directory.
	if err := os.WriteFile(filepath.Join(home, "skills", "gx.md"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	got := CodexHomeDuplicates(home, files)
	want := []string{
		filepath.Join(home, "skills", "gx-design"),
		filepath.Join(home, "skills", "gx-merge"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CodexHomeDuplicates = %v, want %v", got, want)
	}

	if got := CodexHomeDuplicates(t.TempDir(), files); len(got) != 0 {
		t.Errorf("empty codex home: got %v, want none", got)
	}
	if got := CodexHomeDuplicates("", files); len(got) != 0 {
		t.Errorf("unset codex home: got %v, want none", got)
	}
}
