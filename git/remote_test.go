package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestFixFetchConfig(t *testing.T) {
	t.Parallel()
	dir := testutil.TempBareRepo(t)
	testutil.MustGitExported(t, dir, "config", "remote.origin.fetch", "+refs/heads/main:refs/remotes/origin/main")
	if prob := CheckFetchConfig(dir); prob == nil {
		t.Fatal("expected a problem before fix")
	}
	if err := FixFetchConfig(dir); err != nil {
		t.Fatalf("FixFetchConfig: %v", err)
	}
	if prob := CheckFetchConfig(dir); prob != nil {
		t.Errorf("expected nil problem after fix, got: %s", prob.Description)
	}
}

// setupLocalAndBareRemote creates a bare "remote" repo and a regular "local" clone.
func setupLocalAndBareRemote(t *testing.T) (local, remote string) {
	t.Helper()
	src := testutil.TempRepo(t)
	remote = filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(filepath.Dir(remote), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	os.RemoveAll(remote)
	cmd := exec.Command("git", "clone", "--bare", src, remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone --bare: %v\n%s", err, out)
	}
	local = filepath.Join(t.TempDir(), "local")
	os.RemoveAll(local)
	cmd = exec.Command("git", "clone", remote, local)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone local: %v\n%s", err, out)
	}
	testutil.MustGitExported(t, local, "config", "user.email", "test@test.com")
	testutil.MustGitExported(t, local, "config", "user.name", "Test")
	return local, remote
}

func TestPush(t *testing.T) {
	t.Parallel()
	local, remote := setupLocalAndBareRemote(t)
	testutil.WriteFile(t, local, "pushed.txt", "pushed\n")
	testutil.CommitAll(t, local, "push test commit")

	if err := Push(local, "origin", "main"); err != nil {
		t.Fatalf("Push: %v", err)
	}

	out, _, err := run(remote, []string{"log", "--format=%s", "-n", "1"})
	if err != nil {
		t.Fatalf("git log remote: %v", err)
	}
	if !strings.Contains(out, "push test commit") {
		t.Errorf("commit not found in remote; log = %q", out)
	}
}

func TestPushBranch(t *testing.T) {
	t.Parallel()
	local, _ := setupLocalAndBareRemote(t)
	testutil.MustGitExported(t, local, "checkout", "-b", "feature")
	testutil.WriteFile(t, local, "feature.txt", "feature\n")
	testutil.CommitAll(t, local, "feature commit")

	prURL, output, err := PushBranch(local, "origin", "feature")
	if err != nil {
		t.Fatalf("PushBranch: %v\n%s", err, output)
	}
	_ = prURL // may be empty for local bare remote
}

func TestPull(t *testing.T) {
	t.Parallel()
	local, remote := setupLocalAndBareRemote(t)

	// Add a commit to remote via a second clone
	local2 := filepath.Join(t.TempDir(), "local2")
	os.RemoveAll(local2)
	cmd := exec.Command("git", "clone", remote, local2)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone local2: %v\n%s", err, out)
	}
	testutil.MustGitExported(t, local2, "config", "user.email", "test@test.com")
	testutil.MustGitExported(t, local2, "config", "user.name", "Test")
	testutil.WriteFile(t, local2, "remote_file.txt", "from remote\n")
	testutil.CommitAll(t, local2, "remote commit")
	testutil.MustGitExported(t, local2, "push", "origin", "main")

	out, err := Pull(local)
	if err != nil {
		t.Fatalf("Pull: %v\n%s", err, out)
	}
	log, _, err := run(local, []string{"log", "--format=%s", "-n", "1"})
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(log, "remote commit") {
		t.Errorf("pull didn't bring in remote commit; log = %q", log)
	}
}

func TestPushBranchForceWithLease(t *testing.T) {
	t.Parallel()
	local, _ := setupLocalAndBareRemote(t)
	// Push main, then amend locally and force-with-lease push
	testutil.WriteFile(t, local, "a.txt", "a\n")
	testutil.CommitAll(t, local, "commit a")
	testutil.MustGitExported(t, local, "push", "origin", "main")
	// Amend to diverge
	testutil.WriteFile(t, local, "a.txt", "amended\n")
	testutil.MustGitExported(t, local, "add", "a.txt")
	testutil.MustGitExported(t, local, "commit", "--amend", "--no-edit")

	if err := PushBranchForceWithLease(local, "origin", "main"); err != nil {
		t.Fatalf("PushBranchForceWithLease: %v", err)
	}
}

func TestPushBranchForce(t *testing.T) {
	t.Parallel()
	local, _ := setupLocalAndBareRemote(t)
	testutil.WriteFile(t, local, "b.txt", "b\n")
	testutil.CommitAll(t, local, "commit b")
	testutil.MustGitExported(t, local, "push", "origin", "main")
	// Amend to diverge
	testutil.WriteFile(t, local, "b.txt", "amended\n")
	testutil.MustGitExported(t, local, "add", "b.txt")
	testutil.MustGitExported(t, local, "commit", "--amend", "--no-edit")

	out, err := PushBranchForce(local, "origin", "main")
	if err != nil {
		t.Fatalf("PushBranchForce: %v\n%s", err, out)
	}
}
