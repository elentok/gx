package worktrees

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestCmdOpenURL_NonNil(t *testing.T) {
	cmd := cmdOpenURL("https://example.com")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cmdOpenURL")
	}
}

func TestCmdRebasePreflight_ReturnsMsg(t *testing.T) {
	repo := git.Repo{MainBranch: "main"}
	wt := git.Worktree{Name: "feature", Branch: "feature"}
	cmd := cmdRebasePreflight(repo, wt)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cmdRebasePreflight")
	}
	msg := cmd()
	if _, ok := msg.(rebasePreflightMsg); !ok {
		t.Fatalf("expected rebasePreflightMsg, got %T", msg)
	}
}

func TestCmdRebase_NoStash(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	wt := git.Worktree{Name: "main", Path: repoDir}
	repo := git.Repo{MainBranch: "main"}
	cmd := cmdRebase(repo, wt, false)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cmdRebase")
	}
	msg := cmd()
	if _, ok := msg.(rebaseResultMsg); !ok {
		t.Fatalf("expected rebaseResultMsg, got %T", msg)
	}
}

func TestCmdRebase_WithStash(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	wt := git.Worktree{Name: "main", Path: repoDir}
	repo := git.Repo{MainBranch: "main"}
	cmd := cmdRebase(repo, wt, true)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cmdRebase with stash")
	}
	msg := cmd()
	if _, ok := msg.(rebaseResultMsg); !ok {
		t.Fatalf("expected rebaseResultMsg, got %T", msg)
	}
}

func TestCmdRebaseRef_ExecutesAndReturnsMsg(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	wt := git.Worktree{Name: "main", Path: repoDir}
	cmd := cmdRebaseRef(wt, "HEAD", "initial log")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cmdRebaseRef")
	}
	msg := cmd()
	if _, ok := msg.(rebaseResultMsg); !ok {
		t.Fatalf("expected rebaseResultMsg, got %T", msg)
	}
}

func TestCmdStashPop_ExecutesAndReturnsMsg(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	cmd := cmdStashPop(repoDir, "rebase", "")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cmdStashPop")
	}
	msg := cmd()
	if _, ok := msg.(stashPopResultMsg); !ok {
		t.Fatalf("expected stashPopResultMsg, got %T", msg)
	}
}

func TestForcePushPrompt_ContainsBranch(t *testing.T) {
	wt := git.Worktree{Branch: "feature/my-branch"}
	prompt := forcePushPrompt(wt)
	if !strings.Contains(prompt, "feature/my-branch") {
		t.Fatalf("expected prompt to contain branch name, got %q", prompt)
	}
}

func TestPromptableJobOutputTitle_Default(t *testing.T) {
	title := promptableJobOutputTitle(99)
	if title == "" {
		t.Fatal("expected non-empty default title from promptableJobOutputTitle")
	}
}

func TestPromptableJobArgs_PushAndForcePush(t *testing.T) {
	repo := git.Repo{MainBranch: "main"}
	wt := git.Worktree{Name: "feature", Branch: "feature"}

	pushArgs := promptableJobArgs(repo, promptableJobPush, wt)
	if len(pushArgs) == 0 {
		t.Fatal("expected non-empty args for push job")
	}

	forcePushArgs := promptableJobArgs(repo, promptableJobForcePush, wt)
	if len(forcePushArgs) == 0 {
		t.Fatal("expected non-empty args for force-push job")
	}

	defaultArgs := promptableJobArgs(repo, 99, wt)
	if defaultArgs != nil {
		t.Fatalf("expected nil args for unknown job kind, got %v", defaultArgs)
	}
}

func TestPromptableJobLabel_AllKinds(t *testing.T) {
	wt := git.Worktree{Name: "feature"}
	labels := []string{
		promptableJobLabel(promptableJobPushFetch, wt),
		promptableJobLabel(promptableJobPush, wt),
		promptableJobLabel(promptableJobForcePush, wt),
		promptableJobLabel(99, wt),
	}
	for i, label := range labels[:3] {
		if label == "" {
			t.Errorf("expected non-empty label for kind %d", i)
		}
	}
}

func TestPromptableJobOutputTitle_KnownKinds(t *testing.T) {
	cases := []struct {
		kind promptableJobKind
		want string
	}{
		{promptableJobPushFetch, "Fetch output"},
		{promptableJobPush, "Push output"},
		{promptableJobForcePush, "Force-push output"},
	}
	for _, tc := range cases {
		got := promptableJobOutputTitle(tc.kind)
		if got != tc.want {
			t.Errorf("promptableJobOutputTitle(%d) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}
