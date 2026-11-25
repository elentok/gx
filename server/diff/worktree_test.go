package diff_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// worktreeRepo is a repo with a tracked a.txt and an ignore rule, plus a linked
// worktree on its own branch holding one commit, one uncommitted edit, one
// untracked file and one ignored file.
func worktreeRepo(t *testing.T, name string) (h *servertest.Harness, repo, wt, base string) {
	t.Helper()
	h, repo = start(t, map[string]string{"epic": ""})
	write(t, repo, ".gitignore", []byte("*.log\n"))
	commit(t, repo, "a.txt", "a\n", "add a")
	base = mustGit(t, repo, "rev-parse", "HEAD")
	wt = filepath.Join(t.TempDir(), name)
	mustGit(t, repo, "worktree", "add", "-q", "-b", name, wt)
	commit(t, wt, "committed.txt", "c\n", "committed")
	write(t, wt, "a.txt", []byte("a edited\n"))
	write(t, wt, "new.txt", []byte("n\n"))
	write(t, wt, "debug.log", []byte("x\n"))
	return h, repo, wt, base
}

func chatIn(t *testing.T, h *servertest.Harness, dir, fixOf string) string {
	t.Helper()
	res, err := h.Client.Sessions.Create(context.Background(), wire.CreateSessionRequest{
		Project: project, Dir: dir, Harness: string(agentfake.HarnessKind), FixOf: fixOf,
	})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	return res.Session.ID
}

func worktree(t *testing.T, h *servertest.Harness, target string) wire.DiffWorktree {
	t.Helper()
	got, err := h.Client.Reads.DiffWorktree(context.Background(), project, target)
	if err != nil {
		t.Fatalf("DiffWorktree(%q): %v", target, err)
	}
	return got
}

func statuses(t *testing.T, h *servertest.Harness, got wire.DiffWorktree) map[string]string {
	t.Helper()
	files, err := h.Client.Reads.DiffFiles(context.Background(), project, got.BaseSHA, got.SnapshotSHA, nil)
	if err != nil {
		t.Fatalf("DiffFiles: %v", err)
	}
	out := map[string]string{}
	for _, f := range files.Files {
		out[f.Path] = f.Status
	}
	return out
}

func assertSnapshot(t *testing.T, h *servertest.Harness, got wire.DiffWorktree, base, wt string) {
	t.Helper()
	if got.Reason != "" || got.BaseSHA != base || got.BaseLabel != "main@"+base[:7] ||
		got.HeadSHA != mustGit(t, wt, "rev-parse", "HEAD") || got.SnapshotSHA == "" || !got.Uncommitted {
		t.Fatalf("worktree = %+v, want base %s and an uncommitted snapshot", got, base)
	}
	files := statuses(t, h, got)
	if files["committed.txt"] == "" || files["a.txt"] == "" || files["new.txt"] != "A" {
		t.Errorf("files = %v, want committed, edited and added-untracked", files)
	}
	if _, ok := files["debug.log"]; ok {
		t.Errorf("files = %v, want the ignored file left out", files)
	}
}

func TestWorktreePlainChat(t *testing.T) {
	t.Parallel()
	h, _, wt, base := worktreeRepo(t, "feature")
	id := chatIn(t, h, wt, "")
	indexPath := filepath.Join(strings.TrimSpace(mustGit(t, wt, "rev-parse", "--absolute-git-dir")), "index")
	before, _ := os.ReadFile(indexPath)
	beforeStat, _ := os.Stat(indexPath)

	got := worktree(t, h, "chat:"+id)
	assertSnapshot(t, h, got, base, wt)

	after, _ := os.ReadFile(indexPath)
	afterStat, _ := os.Stat(indexPath)
	if sha256.Sum256(before) != sha256.Sum256(after) || !beforeStat.ModTime().Equal(afterStat.ModTime()) {
		t.Error("the worktree's index changed")
	}
	if _, err := os.Stat(indexPath + ".lock"); !os.IsNotExist(err) {
		t.Errorf("index.lock left behind (stat err %v)", err)
	}
}

func TestWorktreeFixAgentChat(t *testing.T) {
	t.Parallel()
	h, _, wt, base := worktreeRepo(t, "fixer")
	id := chatIn(t, h, wt, project)
	assertSnapshot(t, h, worktree(t, h, "chat:"+id), base, wt)
}

func TestWorktreeDesign(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": ""})
	if !h.Server.WaitSettled(10e9) {
		t.Fatal("startup did not settle")
	}
	commit(t, repo, "a.txt", "a\n", "add a")
	base := mustGit(t, repo, "rev-parse", "HEAD")
	wt := filepath.Join(repo, ".worktrees", "epic-design")
	mustGit(t, repo, "worktree", "add", "-q", "-b", "epic-design", wt)
	write(t, wt, "a.txt", []byte("a edited\n"))

	got := worktree(t, h, "design:epic")
	if got.Reason != "" || got.BaseSHA != base || !got.Uncommitted {
		t.Fatalf("design worktree = %+v, want base %s", got, base)
	}
	if files := statuses(t, h, got); files["a.txt"] == "" {
		t.Errorf("files = %v, want a.txt", files)
	}
}

func TestWorktreeCleanHasNoUncommittedChanges(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": ""})
	commit(t, repo, "a.txt", "a\n", "add a")
	wt := filepath.Join(t.TempDir(), "clean")
	mustGit(t, repo, "worktree", "add", "-q", "-b", "clean", wt)
	commit(t, wt, "b.txt", "b\n", "b")

	got := worktree(t, h, "chat:"+chatIn(t, h, wt, ""))
	if got.Reason != "" || got.Uncommitted || got.SnapshotSHA == "" {
		t.Errorf("worktree = %+v, want a snapshot with uncommitted false", got)
	}
}

func TestWorktreeCountsLeftOutUntracked(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": ""})
	commit(t, repo, "a.txt", "a\n", "add a")
	wt := filepath.Join(t.TempDir(), "big")
	mustGit(t, repo, "worktree", "add", "-q", "-b", "big", wt)
	id := chatIn(t, h, wt, "")

	write(t, wt, "huge.bin", make([]byte, rangediff.MaxBlobBytes+1))
	if got := worktree(t, h, "chat:"+id); got.LeftOut != 1 {
		t.Errorf("one oversized file: left_out = %d, want 1", got.LeftOut)
	}
	if err := os.Remove(filepath.Join(wt, "huge.bin")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= rangediff.MaxSnapshotUntracked; i++ {
		write(t, wt, filepath.Join("many", strings.Repeat("f", 1)+string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676))+".txt"), []byte("x"))
	}
	got := worktree(t, h, "chat:"+id)
	if got.LeftOut != rangediff.MaxSnapshotUntracked+1 {
		t.Errorf("many files: left_out = %d, want %d", got.LeftOut, rangediff.MaxSnapshotUntracked+1)
	}
}

func TestWorktreeEmptyStates(t *testing.T) {
	t.Parallel()
	h, repo, wt, _ := worktreeRepo(t, "feature")

	other := testutil.TempRepo(t)
	foreign := chatIn(t, h, other, "")
	gone := chatIn(t, h, wt, "")
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	stepped := chatIn(t, h, repo, "")
	metaPath := filepath.Join(h.SessionsDir, stepped, "meta.json")
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	meta["flow"] = "triage"
	raw, _ = json.Marshal(meta)
	if err := os.WriteFile(metaPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	for target, reason := range map[string]string{
		"chat:nope":           wire.DiffNoWorktree,
		"chat:" + foreign:     wire.DiffNoWorktree,
		"chat:" + gone:        wire.DiffNoWorktree,
		"chat:" + stepped:     wire.DiffNoDiff,
		"design:epic":         wire.DiffNoWorktree,
		"design:missing-map":  wire.DiffNoWorktree,
		"ticket:proj:epic/01": wire.DiffNoWorktree,
		"other:x":             wire.DiffNoDiff,
	} {
		got := worktree(t, h, target)
		if got.Reason != reason || got.SnapshotSHA != "" {
			t.Errorf("%s = %+v, want reason %q", target, got, reason)
		}
	}
	if _, err := h.Client.Reads.DiffWorktree(context.Background(), project, "nocolon"); err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("a malformed target err = %v, want 400", err)
	}
}
