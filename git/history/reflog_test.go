package history

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", rev)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", rev, err)
	}
	return strings.TrimSpace(string(out))
}

func TestReflogEntries_SkipsCreationAndNonForward(t *testing.T) {
	dir := testutil.TempRepo(t)
	base := revParse(t, dir, "main")
	testutil.WriteFile(t, dir, "a.txt", "a")
	testutil.CommitAll(t, dir, "a")
	a := revParse(t, dir, "main")
	testutil.MustGitExported(t, dir, "reset", "--hard", base)
	testutil.WriteFile(t, dir, "b.txt", "b")
	testutil.CommitAll(t, dir, "b")
	b := revParse(t, dir, "main")

	got, err := ReflogEntries(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	want := []ReflogEntry{{base, a}, {base, b}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v (the reset a->base moves backwards and is skipped)", got, want)
	}
}

func TestReflogEntries_MissingLogIsEmpty(t *testing.T) {
	dir := testutil.TempRepo(t)
	got, err := ReflogEntries(dir, "no-such-branch")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestReflogTrailerCommitsIn_TwoTrailerLines(t *testing.T) {
	dir := testutil.TempRepo(t)
	base := revParse(t, dir, "main")
	testutil.WriteFile(t, dir, "a.txt", "a")
	testutil.CommitAll(t, dir, "squash\n\nRalph-Loop-Ticket: p:e/01\nRalph-Loop-Ticket: p:e/02")
	testutil.WriteFile(t, dir, "b.txt", "b")
	testutil.CommitAll(t, dir, "plain")

	got, err := TrailerCommitsIn(dir, base+"..main", "Ralph-Loop-Ticket")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !slices.Equal(got[0].Values, []string{"p:e/01", "p:e/02"}) {
		t.Fatalf("got %+v", got)
	}
}
