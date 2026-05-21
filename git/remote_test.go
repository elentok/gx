package git

import (
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestExtractPRURL(t *testing.T) {
	t.Parallel()
	// Simulated GitHub stderr from a first-time push
	githubOutput := `
remote: Create a pull request for 'my-branch' on GitHub by visiting:
remote:      https://github.com/elentok/gx/pull/new/my-branch
remote:
`
	got := ExtractPRURL(githubOutput)
	want := "https://github.com/elentok/gx/pull/new/my-branch"
	if got != want {
		t.Fatalf("ExtractPRURL() = %q, want %q", got, want)
	}

	if got := ExtractPRURL("remote: Everything up-to-date\n"); got != "" {
		t.Fatalf("ExtractPRURL() = %q, want empty", got)
	}
}

func TestExtractPRURL_StripsTerminalEscapes(t *testing.T) {
	t.Parallel()
	const want = "https://github.com/elentok/gx/pull/new/my-branch"
	output := "" +
		"remote: Create a pull request for 'my-branch' on GitHub by visiting:\n" +
		"remote: \x1b[32m\x1b]8;;" + want + "\x07" + want + "\x1b]8;;\x07\x1b[0m\n"

	if got := ExtractPRURL(output); got != want {
		t.Fatalf("ExtractPRURL() = %q, want %q", got, want)
	}
}

func TestIsNonFastForwardPushError_NonRunError(t *testing.T) {
	t.Parallel()
	// A non-RunError should return false
	if IsNonFastForwardPushError(fakeRemoteErr("something else")) {
		t.Error("expected false for non-RunError")
	}
}

func TestCheckFetchConfig_NoRemote(t *testing.T) {
	t.Parallel()
	// Plain repo with no remote → nil problem
	dir := testutil.TempRepo(t)
	if prob := CheckFetchConfig(dir); prob != nil {
		t.Errorf("expected nil for repo with no remote, got %+v", prob)
	}
}

func TestCheckFetchConfig_CorrectSetup(t *testing.T) {
	t.Parallel()
	dir := testutil.TempBareRepo(t)
	// TempBareRepo already sets remote.origin.fetch and fetches → should be fine
	if prob := CheckFetchConfig(dir); prob != nil {
		t.Errorf("expected nil for correctly configured bare repo, got: %s", prob.Description)
	}
}

func TestBranchRemote_FallsBackToOrigin(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	repo := Repo{Root: dir}
	// No upstream configured → should fall back to "origin"
	remote := BranchRemote(repo, "main")
	if remote != "origin" {
		t.Errorf("expected 'origin' fallback, got %q", remote)
	}
}

type fakeRemoteErr string

func (e fakeRemoteErr) Error() string { return string(e) }

func TestIsNonFastForwardPushError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "non fast forward",
			err: &RunError{
				Stderr: "! [rejected]        main -> main (non-fast-forward)\nerror: failed to push some refs",
			},
			want: true,
		},
		{
			name: "fetch first",
			err: &RunError{
				Stderr: "Updates were rejected because the remote contains work that you do not have locally. (fetch first)",
			},
			want: true,
		},
		{
			name: "other error",
			err: &RunError{
				Stderr: "fatal: could not read from remote repository",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNonFastForwardPushError(tt.err); got != tt.want {
				t.Fatalf("IsNonFastForwardPushError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListRemotes_NoRemotes(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	remotes, err := ListRemotes(Repo{Root: dir})
	if err != nil {
		t.Fatalf("ListRemotes: %v", err)
	}
	if len(remotes) != 0 {
		t.Errorf("expected no remotes, got %v", remotes)
	}
}

func TestListRemotes_WithRemote(t *testing.T) {
	t.Parallel()
	dir := testutil.TempBareRepo(t)
	remotes, err := ListRemotes(Repo{Root: dir})
	if err != nil {
		t.Fatalf("ListRemotes: %v", err)
	}
	if len(remotes) == 0 {
		t.Error("expected at least one remote (origin)")
	}
}

func TestPruneRemote_NoOp(t *testing.T) {
	t.Parallel()
	dir := testutil.TempBareRepo(t)
	if err := PruneRemote(Repo{Root: dir}, "origin"); err != nil {
		t.Errorf("PruneRemote: %v", err)
	}
}

func TestPruneAllRemotes(t *testing.T) {
	t.Parallel()
	dir := testutil.TempBareRepo(t)
	if err := PruneAllRemotes(Repo{Root: dir}); err != nil {
		t.Errorf("PruneAllRemotes: %v", err)
	}
}

func TestCheckFetchConfig_WrongRefspec(t *testing.T) {
	t.Parallel()
	dir := testutil.TempBareRepo(t)
	// Set an incorrect refspec
	testutil.MustGitExported(t, dir, "config", "remote.origin.fetch", "+refs/heads/main:refs/remotes/origin/main")
	prob := CheckFetchConfig(dir)
	if prob == nil {
		t.Error("expected non-nil problem for wrong refspec")
	}
	if len(prob.Commands) == 0 {
		t.Error("expected fix commands in problem")
	}
}
