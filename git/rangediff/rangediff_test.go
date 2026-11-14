package rangediff_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/testutil"
)

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, _, err := git.Run(dir, nil, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func mustResolve(t *testing.T, dir, a, b string) rangediff.Range {
	t.Helper()
	r, err := rangediff.Resolve(dir, a, b)
	if err != nil {
		t.Fatalf("Resolve(%q,%q): %v", a, b, err)
	}
	return r
}

func listByPath(t *testing.T, dir string, r rangediff.Range) map[string]rangediff.File {
	t.Helper()
	list, err := rangediff.ListFiles(dir, r)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	m := map[string]rangediff.File{}
	for _, f := range list.Files {
		m[f.Path] = f
	}
	return m
}

func TestRefResolvesBranchTagAndSHA(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "branch", "feat")
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "second")
	mustGit(t, dir, "tag", "v1")
	head := mustGit(t, dir, "rev-parse", "HEAD")

	for _, tc := range []string{"main", "v1", head} {
		r := mustResolve(t, dir, "feat", tc)
		if r.SecondSHA != head {
			t.Errorf("ref %q: SecondSHA = %q, want %q", tc, r.SecondSHA, head)
		}
	}
	r := mustResolve(t, dir, "main", "feat")
	if r.FirstSHA != head || r.MergeBase == "" || r.NoCommonHistory {
		t.Errorf("unexpected range %+v", r)
	}
}

func TestRefUnknownAndOptionLikeRefsAreRefused(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)

	cases := []struct {
		first, second string
		ref           string
		side          rangediff.Side
	}{
		{"nope", "main", "nope", rangediff.First},
		{"main", "nope", "nope", rangediff.Second},
		{"main", "--output=x", "--output=x", rangediff.Second},
		{"--output=x", "main", "--output=x", rangediff.First},
	}
	for _, tc := range cases {
		_, err := rangediff.Resolve(dir, tc.first, tc.second)
		var ue *rangediff.UnknownRefError
		if !errors.As(err, &ue) {
			t.Fatalf("Resolve(%q,%q) err = %v, want UnknownRefError", tc.first, tc.second, err)
		}
		if ue.Ref != tc.ref || ue.Side != tc.side {
			t.Errorf("got ref=%q side=%v, want ref=%q side=%v", ue.Ref, ue.Side, tc.ref, tc.side)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "x")); err == nil {
		t.Error("option-like ref reached git: file x was created")
	}
}

func TestRangeIsThreeDot(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "checkout", "-b", "feat")
	testutil.WriteFile(t, dir, "feat.txt", "f\n")
	testutil.CommitAll(t, dir, "feat")
	mustGit(t, dir, "checkout", "main")
	testutil.WriteFile(t, dir, "main-only.txt", "m\n")
	testutil.CommitAll(t, dir, "main advanced")

	files := listByPath(t, dir, mustResolve(t, dir, "main", "feat"))
	if len(files) != 1 || files["feat.txt"].Status != "A" {
		t.Errorf("files = %+v, want only feat.txt added", files)
	}
}

func TestRangeNoCommonHistory(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "checkout", "--orphan", "other")
	testutil.WriteFile(t, dir, "o.txt", "o\n")
	testutil.CommitAll(t, dir, "orphan")

	r, err := rangediff.Resolve(dir, "main", "other")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !r.NoCommonHistory || r.FirstSHA == "" || r.SecondSHA == "" {
		t.Errorf("range = %+v, want NoCommonHistory with both SHAs", r)
	}
}

func TestFileListEmptyRange(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	list, err := rangediff.ListFiles(dir, mustResolve(t, dir, "main", "main"))
	if err != nil || len(list.Files) != 0 || list.Truncated {
		t.Errorf("list = %+v, err = %v, want empty", list, err)
	}
}

func TestFileListRenameAndCounts(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	body := strings.Repeat("some stable line of content\n", 20)
	testutil.WriteFile(t, dir, "old.txt", body)
	testutil.CommitAll(t, dir, "add old")
	mustGit(t, dir, "branch", "base")
	mustGit(t, dir, "mv", "old.txt", "new.txt")
	testutil.WriteFile(t, dir, "new.txt", body+"extra\n")
	testutil.CommitAll(t, dir, "rename")

	list, err := rangediff.ListFiles(dir, mustResolve(t, dir, "base", "main"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("files = %+v, want one", list.Files)
	}
	f := list.Files[0]
	if f.Status != "R" || f.OldPath != "old.txt" || f.Path != "new.txt" || f.Added != 1 || f.Deleted != 0 {
		t.Errorf("file = %+v", f)
	}
}

func TestFileListFlags(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "branch", "base")
	testutil.WriteFile(t, dir, "many-lines.txt", strings.Repeat("x\n", 2100))
	testutil.WriteFile(t, dir, "wide.txt", strings.Repeat(strings.Repeat("y", 99)+"\n", 700))
	testutil.WriteFile(t, dir, "small.txt", "hi\n")
	testutil.WriteFile(t, dir, "sub/go.sum", "h1:abc\n")
	testutil.WriteFile(t, dir, "pic.png", "\x89PNG\x00\x01\x02\x00binary")
	testutil.CommitAll(t, dir, "add")

	r := mustResolve(t, dir, "base", "main")
	files := listByPath(t, dir, r)
	for name, want := range map[string]struct{ large, binary, generated bool }{
		"many-lines.txt": {large: true},
		"wide.txt":       {large: true},
		"small.txt":      {},
		"sub/go.sum":     {generated: true},
		"pic.png":        {binary: true},
	} {
		f := files[name]
		if f.Large != want.large || f.Binary != want.binary || f.Generated != want.generated {
			t.Errorf("%s: %+v, want %+v", name, f, want)
		}
	}

	p, err := rangediff.Patch(dir, r, files["pic.png"])
	if err != nil || p != "" {
		t.Errorf("binary patch = %q, err %v, want none", p, err)
	}
}

func TestFileListTruncatedPastLimit(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	mustGit(t, dir, "branch", "base")
	for i := range rangediff.MaxFiles + 5 {
		testutil.WriteFile(t, dir, fmt.Sprintf("d/f%05d.txt", i), "x\n")
	}
	testutil.CommitAll(t, dir, "many")

	list, err := rangediff.ListFiles(dir, mustResolve(t, dir, "base", "main"))
	if err != nil {
		t.Fatal(err)
	}
	if !list.Truncated || len(list.Files) != rangediff.MaxFiles {
		t.Errorf("truncated=%v len=%d", list.Truncated, len(list.Files))
	}
}

func TestPatchLargeFileInFullAndRename(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	body := strings.Repeat("stable\n", 30)
	testutil.WriteFile(t, dir, "old.txt", body)
	testutil.CommitAll(t, dir, "old")
	mustGit(t, dir, "branch", "base")
	testutil.WriteFile(t, dir, "big.txt", strings.Repeat("line\n", 3000))
	mustGit(t, dir, "mv", "old.txt", "new.txt")
	testutil.CommitAll(t, dir, "both")

	r := mustResolve(t, dir, "base", "main")
	files := listByPath(t, dir, r)

	p, err := rangediff.Patch(dir, r, files["big.txt"])
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(p, "\n+line"); got != 3000 {
		t.Errorf("big patch has %d added lines, want 3000", got)
	}

	p, err = rangediff.Patch(dir, r, files["new.txt"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p, "rename from old.txt") || !strings.Contains(p, "rename to new.txt") {
		t.Errorf("rename patch = %q", p)
	}
}
