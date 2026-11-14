package diff_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

const project = "proj"

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, _, err := git.Run(dir, nil, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

// start runs a server whose project "proj" points at a fresh repo with the
// given epics (epic name to branch_name; empty means no branch_name).
func start(t *testing.T, epics map[string]string) (*servertest.Harness, string) {
	t.Helper()
	store := t.TempDir()
	repo := testutil.TempRepo(t)
	for name, branch := range epics {
		servertest.WriteTicketWith(t, store, project, name, "01", "t", servertest.TicketOpts{})
		if branch != "" {
			md := "---\nstatus: open\nbranch_name: " + branch + "\n---\n# E\n"
			if err := os.WriteFile(filepath.Join(store, project, name, "ticket.md"), []byte(md), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, project, repo)
	return servertest.StartWithStore(t, store), repo
}

func write(t *testing.T, repo, name string, data []byte) {
	t.Helper()
	p := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, repo, name, content, subject string) {
	t.Helper()
	write(t, repo, name, []byte(content))
	testutil.CommitAll(t, repo, subject)
}

func TestDiffRoutesResolveRefsAndEchoShas(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": ""})
	mustGit(t, repo, "branch", "base")
	mustGit(t, repo, "tag", "v1")
	commit(t, repo, "a.txt", "a\n", "add a")
	tip := mustGit(t, repo, "rev-parse", "HEAD")
	base := mustGit(t, repo, "rev-parse", "base")
	ctx := context.Background()

	for _, ref := range []string{"base", "v1", base} {
		got, err := h.Client.Reads.DiffFiles(ctx, project, ref, "main", nil)
		if err != nil {
			t.Fatalf("DiffFiles(%q): %v", ref, err)
		}
		if got.MergeBase != base || got.SecondSHA != tip || got.FirstSHA != base {
			t.Errorf("DiffFiles(%q) shas = %+v, want base %s tip %s", ref, got.DiffRange, base, tip)
		}
		if len(got.Files) != 1 || got.Files[0].Path != "a.txt" {
			t.Errorf("DiffFiles(%q) files = %+v", ref, got.Files)
		}
	}

	for _, tc := range []struct{ first, second, field string }{
		{"nope", "main", "first"}, {"base", "nope", "second"}, {"--output=x", "main", "first"}, {"base", "-x", "second"},
	} {
		_, err := h.Client.Reads.DiffFiles(ctx, project, tc.first, tc.second, nil)
		if err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "Unknown ref: "+tc.field) {
			t.Errorf("DiffFiles(%q, %q) err = %v, want 400 Unknown ref: %s", tc.first, tc.second, err, tc.field)
		}
	}

	if _, err := h.Client.Reads.DiffFiles(ctx, "nope", "base", "main", nil); err == nil {
		t.Error("an unknown project resolved, want an error")
	}
	if _, err := h.Client.Reads.DiffFiles(ctx, "", "base", "main", nil); err == nil {
		t.Error("an empty project resolved, want an error")
	}
}

func TestDiffRoutesNoCommonHistoryIsAResult(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": ""})
	mustGit(t, repo, "checkout", "--orphan", "orphan")
	mustGit(t, repo, "rm", "-rf", ".")
	commit(t, repo, "o.txt", "o\n", "orphan root")
	ctx := context.Background()

	files, err := h.Client.Reads.DiffFiles(ctx, project, "main", "orphan", nil)
	if err != nil || !files.NoCommonHistory || files.MergeBase != "" || len(files.Files) != 0 {
		t.Errorf("files = %+v, err %v; want no common history", files, err)
	}
	commits, err := h.Client.Reads.DiffCommits(ctx, project, "main", "orphan")
	if err != nil || !commits.NoCommonHistory {
		t.Errorf("commits = %+v, err %v; want no common history", commits, err)
	}
}

func TestDiffRoutesMissingCommitsAreNotAvailable(t *testing.T) {
	t.Parallel()
	h, _ := start(t, map[string]string{"epic": ""})
	ctx := context.Background()
	missing := strings.Repeat("ab", 20)

	files, err := h.Client.Reads.DiffFiles(ctx, project, missing, "main", nil)
	if err != nil || !files.CommitsUnavailable {
		t.Errorf("files = %+v, err %v; want commits_unavailable", files, err)
	}
	patch, err := h.Client.Reads.DiffPatch(ctx, project, "main", missing, "a.txt", nil)
	if err != nil || !patch.CommitsUnavailable {
		t.Errorf("patch = %+v, err %v; want commits_unavailable", patch, err)
	}
	commits, err := h.Client.Reads.DiffCommits(ctx, project, missing, missing)
	if err != nil || !commits.CommitsUnavailable {
		t.Errorf("commits = %+v, err %v; want commits_unavailable", commits, err)
	}
	blob, err := h.Client.Reads.DiffBlob(ctx, project, missing, missing, "a.txt")
	if err != nil || !blob.CommitsUnavailable {
		t.Errorf("blob = %+v, err %v; want commits_unavailable", blob, err)
	}
}

func TestDiffRoutesFilesRenameAndFlags(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": ""})
	commit(t, repo, "old.txt", strings.Repeat("keep this line\n", 20), "add old")
	mustGit(t, repo, "branch", "base")
	mustGit(t, repo, "mv", "old.txt", "new.txt")
	testutil.CommitAll(t, repo, "rename")
	commit(t, repo, "big.txt", strings.Repeat("x\n", rangediff.LargeLines+10), "big")
	commit(t, repo, "go.sum", "x\n", "lock")
	write(t, repo, "bin.dat", []byte{0, 1, 2, 3})
	testutil.CommitAll(t, repo, "binary")
	ctx := context.Background()

	got, err := h.Client.Reads.DiffFiles(ctx, project, "base", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]wire.DiffFile{}
	for _, f := range got.Files {
		byPath[f.Path] = f
	}
	if f := byPath["new.txt"]; f.Status != "R" || f.OldPath != "old.txt" {
		t.Errorf("new.txt = %+v, want a rename from old.txt", f)
	}
	if !byPath["big.txt"].Large || !byPath["go.sum"].Generated || !byPath["bin.dat"].Binary {
		t.Errorf("flags = %+v", byPath)
	}

	p, err := h.Client.Reads.DiffPatch(ctx, project, "base", "main", "big.txt", nil)
	if err != nil || !p.File.Large || !strings.Contains(p.Patch, "+x") {
		t.Errorf("big patch large=%v len=%d err %v, want the patch sent", p.File.Large, len(p.Patch), err)
	}
	p, err = h.Client.Reads.DiffPatch(ctx, project, "base", "main", "go.sum", nil)
	if err != nil || !p.File.Generated || p.Patch == "" {
		t.Errorf("go.sum patch = %+v, err %v", p, err)
	}
	if _, err := h.Client.Reads.DiffPatch(ctx, project, "base", "main", "missing.txt", nil); err == nil {
		t.Error("a patch of a path outside the list succeeded, want an error")
	}
}

func TestDiffRoutesPatchNarrowsToCommits(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": ""})
	mustGit(t, repo, "branch", "base")
	commit(t, repo, "f.txt", "one\n", "c1")
	c1 := mustGit(t, repo, "rev-parse", "HEAD")
	commit(t, repo, "other.txt", "x\n", "c2")
	commit(t, repo, "f.txt", "one\ntwo\n", "c3")
	c3 := mustGit(t, repo, "rev-parse", "HEAD")
	mustGit(t, repo, "checkout", "-q", "-b", "side", "base")
	commit(t, repo, "side.txt", "s\n", "side")
	mustGit(t, repo, "checkout", "-q", "main")
	mustGit(t, repo, "merge", "--no-ff", "-m", "merge side", "side")
	merge := mustGit(t, repo, "rev-parse", "HEAD")
	ctx := context.Background()

	got, err := h.Client.Reads.DiffPatch(ctx, project, "base", "main", "f.txt", []string{c3, c1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.CommitPatches) != 2 || got.CommitPatches[0].SHA != c1 || got.CommitPatches[1].SHA != c3 {
		t.Fatalf("commit patches = %+v, want c1 then c3 only", got.CommitPatches)
	}
	if strings.Contains(got.CommitPatches[0].Patch, "two") || !strings.Contains(got.CommitPatches[1].Patch, "+two") {
		t.Errorf("patches are not per commit: %+v", got.CommitPatches)
	}

	files, err := h.Client.Reads.DiffFiles(ctx, project, "base", "main", []string{merge})
	if err != nil || len(files.Files) != 0 {
		t.Errorf("merge-only narrowing files = %+v, err %v; want none", files.Files, err)
	}
	if _, err := h.Client.Reads.DiffFiles(ctx, project, "base", "main", []string{strings.Repeat("cd", 20)}); err == nil ||
		!strings.Contains(err.Error(), "400") {
		t.Errorf("unknown commit err = %v, want 400", err)
	}
}

func TestDiffRoutesCommitListTruncates(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": ""})
	mustGit(t, repo, "branch", "base")
	commit(t, repo, "late.txt", "x\n", "has a file")
	for i := range rangediff.MaxCommits {
		mustGit(t, repo, "commit", "--allow-empty", "-m", fmt.Sprintf("empty %d", i))
	}
	ctx := context.Background()

	commits, err := h.Client.Reads.DiffCommits(ctx, project, "base", "main")
	if err != nil || len(commits.Commits) != rangediff.MaxCommits || !commits.Truncated {
		t.Fatalf("commits = %d truncated=%v err %v", len(commits.Commits), commits.Truncated, err)
	}
	files, err := h.Client.Reads.DiffFiles(ctx, project, "base", "main", nil)
	if err != nil || len(files.Files) != 1 || files.Files[0].Path != "late.txt" {
		t.Errorf("files = %+v, err %v; want every file of the range", files.Files, err)
	}
}

func TestDiffRoutesCommitListCanonicalTickets(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": "", "renamed": "oldname"})
	mustGit(t, repo, "branch", "base")
	stamp := func(file, subject, trailer string) {
		commit(t, repo, file, subject+"\n", subject)
		if err := history.AppendTrailer(repo, "Ralph-Loop-Ticket", trailer); err != nil {
			t.Fatal(err)
		}
	}
	stamp("a.txt", "modern", "proj:epic/01")
	stamp("b.txt", "legacy", "epic/02")
	stamp("c.txt", "renamed", "oldname/03")
	commit(t, repo, "d.txt", "plain\n", "plain")
	mustGit(t, repo, "checkout", "-q", "-b", "side", "base")
	commit(t, repo, "s.txt", "s\n", "side")
	mustGit(t, repo, "checkout", "-q", "main")
	mustGit(t, repo, "merge", "--no-ff", "-m", "merge side", "side")

	got, err := h.Client.Reads.DiffCommits(context.Background(), project, "base", "main")
	if err != nil {
		t.Fatal(err)
	}
	bySubject := map[string]wire.DiffCommit{}
	for _, c := range got.Commits {
		bySubject[c.Subject] = c
	}
	for subject, want := range map[string]string{"modern": "proj:epic/01", "legacy": "proj:epic/02", "renamed": "proj:renamed/03", "plain": ""} {
		if c := bySubject[subject]; c.Ticket != want {
			t.Errorf("%s ticket = %q, want %q", subject, c.Ticket, want)
		}
	}
	if m := bySubject["merge side"]; !m.Merge || len(m.Files) != 0 {
		t.Errorf("merge commit = %+v, want a merge marker and no files", m)
	}
}

func TestDiffRoutesCommitListGroupsArchivedEpicTickets(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": "", ".archive/old": ""})
	mustGit(t, repo, "branch", "base")
	commit(t, repo, "a.txt", "a\n", "archived")
	if err := history.AppendTrailer(repo, "Ralph-Loop-Ticket", "proj:old/01"); err != nil {
		t.Fatal(err)
	}
	got, err := h.Client.Reads.DiffCommits(context.Background(), project, "base", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Commits) != 1 || got.Commits[0].Ticket != "proj:old/01" {
		t.Errorf("commits = %+v, want the archived epic's ticket address", got.Commits)
	}
}

func TestDiffRoutesBlob(t *testing.T) {
	t.Parallel()
	h, repo := start(t, map[string]string{"epic": ""})
	commit(t, repo, "text.txt", "old text\n", "text")
	mustGit(t, repo, "branch", "base")
	png := append([]byte("\x89PNG\r\n\x1a\n\x00"), make([]byte, 100)...)
	write(t, repo, "small.png", png)
	write(t, repo, "big.png", make([]byte, rangediff.MaxBlobBytes+1))
	write(t, repo, "logo.svg", []byte("<svg/>"))
	write(t, repo, "x.bin", []byte{0, 1, 2})
	write(t, repo, "text.txt", []byte("new text\n"))
	if err := os.Symlink("text.txt", filepath.Join(repo, "link")); err != nil {
		t.Fatal(err)
	}
	testutil.CommitAll(t, repo, "assets")
	base := mustGit(t, repo, "rev-parse", "base")
	tip := mustGit(t, repo, "rev-parse", "HEAD")
	write(t, repo, "text.txt", []byte("dirty on disk\n"))
	ctx := context.Background()

	blob := func(path string) wire.DiffBlobs {
		t.Helper()
		got, err := h.Client.Reads.DiffBlob(ctx, project, base, tip, path)
		if err != nil {
			t.Fatalf("DiffBlob(%q): %v", path, err)
		}
		return got
	}
	if got := blob("small.png"); got.New.Kind != wire.DiffBlobImage || got.Old.Kind != wire.DiffBlobAbsent ||
		got.New.ImageBase64 != base64.StdEncoding.EncodeToString(png) {
		t.Errorf("small.png = %+v", got)
	}
	for path, kind := range map[string]string{"big.png": wire.DiffBlobBinary, "logo.svg": wire.DiffBlobBinary, "x.bin": wire.DiffBlobBinary} {
		if got := blob(path); got.New.Kind != kind || got.New.ImageBase64 != "" {
			t.Errorf("%s = %+v, want kind %s and no bytes", path, got.New, kind)
		}
	}
	if got := blob("link"); got.New.Kind != wire.DiffBlobText || got.New.Text != "text.txt" {
		t.Errorf("symlink = %+v, want its target text", got.New)
	}
	if got := blob("text.txt"); got.Old.Text != "old text\n" || got.New.Text != "new text\n" {
		t.Errorf("text.txt = %+v, want committed content, not the dirty file", got)
	}

	for _, path := range []string{"nope.txt", "/etc/passwd", "../x", "a/../x", "-x", "text.txt\x00", ""} {
		if _, err := h.Client.Reads.DiffBlob(ctx, project, base, tip, path); err == nil || !strings.Contains(err.Error(), "400") {
			t.Errorf("DiffBlob(%q) err = %v, want 400", path, err)
		}
	}
	if _, err := h.Client.Reads.DiffBlob(ctx, project, "main", tip, "text.txt"); err == nil {
		t.Error("a typed ref reached the blob call, want a refusal")
	}
}
