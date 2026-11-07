package projects

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
)

func TestWorktreeDir_ResolvesOncePerProjectFileVersion(t *testing.T) {
	store := t.TempDir()
	projectDir := filepath.Join(store, "proj")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pjson := filepath.Join(projectDir, config.ProjectFileName)
	write := func(repo string, mtime time.Time) {
		t.Helper()
		if err := os.WriteFile(pjson, []byte(`{"repo":"`+repo+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(pjson, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Now()
	write("/repo-a", base)

	var finds []string
	reg := &Registry{
		cfg: func() Env { return Env{TicketStore: store} },
		findRepo: func(dir string) (*git.Repo, error) {
			finds = append(finds, dir)
			return &git.Repo{Root: dir, WorktreeDir: dir + "/.wt"}, nil
		},
	}

	for range 3 {
		if got := reg.WorktreeDir("proj"); got != "/repo-a/.wt" {
			t.Fatalf("WorktreeDir = %q, want /repo-a/.wt", got)
		}
	}
	if len(finds) != 1 {
		t.Fatalf("repo resolved %d times over 3 calls, want 1", len(finds))
	}

	write("/repo-b", base.Add(time.Minute))
	if got := reg.WorktreeDir("proj"); got != "/repo-b/.wt" {
		t.Fatalf("after project.json changed, WorktreeDir = %q, want /repo-b/.wt", got)
	}
	if len(finds) != 2 {
		t.Fatalf("repo resolved %d times, want 2", len(finds))
	}
}
