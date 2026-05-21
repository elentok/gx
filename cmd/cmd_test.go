package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestExecute_ListWorktrees(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
	}

	if err := execute([]string{"wt", "list"}, d); err != nil {
		t.Fatalf("execute wt list: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 worktrees, got %d: %v", len(lines), lines)
	}
	if lines[0] != "feature-a" || lines[1] != "feature-b" {
		t.Fatalf("unexpected worktree names: %v", lines)
	}
}

func TestExecute_LogDispatchesToRunLog(t *testing.T) {
	called := ""
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runLog: func(ref string) error {
			called = ref
			return nil
		},
	}

	if err := execute([]string{"log", "HEAD~2"}, d); err != nil {
		t.Fatalf("execute log: %v", err)
	}
	if called != "HEAD~2" {
		t.Fatalf("runLog called with %q, want %q", called, "HEAD~2")
	}
}

func TestExecute_WorktreeAbsPath(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
	}

	if err := execute([]string{"wt", "abs-path", "feature-a"}, d); err != nil {
		t.Fatalf("execute wt abs-path: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	want := repoDir + "/feature-a"
	if got != want {
		t.Fatalf("abs path = %q, want %q", got, want)
	}
}

func TestExecute_ListWorktrees_FromInsideWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtDir := repoDir + "/feature-a"
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return wtDir, nil },
	}

	if err := execute([]string{"wt", "list"}, d); err != nil {
		t.Fatalf("execute wt list: %v", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if strings.ContainsRune(line, '/') {
			t.Errorf("wt list output contains path separator: %q", line)
		}
	}
}

func TestExecute_WorktreeAbsPath_FromInsideWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtDir := repoDir + "/feature-a"
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return wtDir, nil },
	}

	if err := execute([]string{"wt", "abs-path", "feature-b"}, d); err != nil {
		t.Fatalf("execute wt abs-path: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	want := repoDir + "/feature-b"
	if got != want {
		t.Fatalf("abs path = %q, want %q", got, want)
	}
}

func TestExecute_WorktreeAbsPath_NotFound(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
	}

	err := execute([]string{"wt", "abs-path", "does-not-exist"}, d)
	if err == nil {
		t.Fatal("expected error for missing worktree")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_WorktreeAbsPath_MissingArg(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}

	err := execute([]string{"wt", "abs-path"}, d)
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}

func TestExecute_DefaultRunsStatus(t *testing.T) {
	called := 0
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runStatus: func(_ string) error {
			called++
			return nil
		},
	}

	if err := execute(nil, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if called != 1 {
		t.Fatalf("runStatus called %d times, want 1", called)
	}
}

func TestExecute_WorktreesAliases(t *testing.T) {
	for _, args := range [][]string{{"worktrees"}, {"wt"}} {
		t.Run(args[0], func(t *testing.T) {
			called := 0
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				runWorktrees: func(_ string) error {
					called++
					return nil
				},
			}
			if err := execute(args, d); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if called != 1 {
				t.Fatalf("runWorktrees called %d times, want 1", called)
			}
		})
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	d := deps{
		stdout:       bytes.NewBuffer(nil),
		stderr:       &stderr,
		runWorktrees: func(_ string) error { return nil },
	}
	err := execute([]string{"nope"}, d)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := stderr.String(); got == "" {
		t.Fatal("expected usage on stderr")
	}
}

func TestExecute_RunsPush(t *testing.T) {
	for _, name := range []string{"push", "ps"} {
		t.Run(name, func(t *testing.T) {
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				getwd: func() (string, error) {
					return "/tmp", errors.New("boom")
				},
			}
			if err := execute([]string{name}, d); err == nil {
				t.Fatal("expected propagated error")
			}
		})
	}
}

func TestExecute_RunsStatus(t *testing.T) {
	for _, name := range []string{"status", "s"} {
		t.Run(name, func(t *testing.T) {
			called := 0
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				runStatus: func(string) error {
					called++
					return nil
				},
			}

			if err := execute([]string{name}, d); err != nil {
				t.Fatalf("execute %s: %v", name, err)
			}
			if called != 1 {
				t.Fatalf("runStatus called %d times, want 1", called)
			}
		})
	}
}

func TestExecute_RunsStatusWithPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "relative", args: []string{"status", "README.md"}, want: "README.md"},
		{name: "alias", args: []string{"s", "/tmp/file.txt"}, want: "/tmp/file.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				runStatus: func(path string) error {
					got = path
					return nil
				},
			}

			if err := execute(tc.args, d); err != nil {
				t.Fatalf("execute %v: %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("runStatus path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveStatusTargetPath(t *testing.T) {
	root := "/repo"
	cwd := "/repo/sub"

	got, err := resolveStatusTargetPath(root, cwd, "file.txt")
	if err != nil {
		t.Fatalf("resolveStatusTargetPath relative: %v", err)
	}
	if got != "sub/file.txt" {
		t.Fatalf("relative target = %q, want %q", got, "sub/file.txt")
	}

	abs := filepath.Join(root, "deep", "file.txt")
	got, err = resolveStatusTargetPath(root, cwd, abs)
	if err != nil {
		t.Fatalf("resolveStatusTargetPath absolute: %v", err)
	}
	if got != "deep/file.txt" {
		t.Fatalf("absolute target = %q, want %q", got, "deep/file.txt")
	}
}

func TestResolveStatusTargetPathRejectsOutsideWorktree(t *testing.T) {
	_, err := resolveStatusTargetPath("/repo", "/repo", "../other.txt")
	if err == nil {
		t.Fatal("expected error for path outside worktree")
	}
}

func TestExecute_PushAllowedInRegularRepo(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd: func() (string, error) {
			return repoDir, nil
		},
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	err := execute([]string{"push"}, d)
	if err == nil {
		t.Fatal("expected push failure in test repo without remote")
	}
	if strings.Contains(err.Error(), "must be run from a regular repo or linked worktree") {
		t.Fatalf("regular repo should be allowed, got: %v", err)
	}
}

func TestExecute_PushRejectedInBareRepo(t *testing.T) {
	repoDir := testutil.TempBareRepo(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd: func() (string, error) {
			return repoDir, nil
		},
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	err := execute([]string{"push"}, d)
	if err == nil {
		t.Fatal("expected error in bare repo")
	}
	if !strings.Contains(err.Error(), "must be run from a regular repo or linked worktree") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_PushConfirmsBeforeCheckingDivergence(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	remote := t.TempDir() + "/remote.git"
	testutil.MustGitExported(t, ".", "clone", "--bare", repoDir, remote)
	testutil.MustGitExported(t, repoDir, "remote", "add", "origin", remote)

	prompts := []string{}
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd: func() (string, error) {
			return repoDir, nil
		},
		confirmForce: func(prompt string) (bool, error) {
			prompts = append(prompts, prompt)
			return false, nil
		},
		choosePushDivergence: func(io.Reader, io.Writer, *git.PushDivergence) (int, error) {
			t.Fatalf("divergence chooser should not run before push confirmation")
			return 0, nil
		},
	}

	err := execute([]string{"push"}, d)
	if err == nil || err.Error() != "push aborted" {
		t.Fatalf("expected push aborted, got %v", err)
	}
	if len(prompts) != 1 {
		t.Fatalf("expected exactly one confirmation prompt, got %v", prompts)
	}
	if prompts[0] != "Push branch main to origin?" {
		t.Fatalf("unexpected confirmation prompt: %q", prompts[0])
	}
}

func TestExecute_Init(t *testing.T) {
	var stdout bytes.Buffer
	called := false
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		initConfig: func() (string, error) {
			called = true
			return "/tmp/gx/config.json", nil
		},
	}

	if err := execute([]string{"init"}, d); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	if !called {
		t.Fatal("expected initConfig to be called")
	}
	if !strings.Contains(stdout.String(), "Created config file at /tmp/gx/config.json") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestExecute_EditConfig_RequiresEditor(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		initConfig: func() (string, error) {
			return "/tmp/gx/config.json", nil
		},
		getenv: func(string) string { return "" },
	}

	err := execute([]string{"edit-config"}, d)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "$EDITOR is not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_EditConfig_RunsEditor(t *testing.T) {
	var stdout bytes.Buffer
	var gotEditor, gotPath string
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		initConfig: func() (string, error) {
			return "/tmp/gx/config.json", nil
		},
		getenv: func(k string) string {
			if k == "EDITOR" {
				return "vim"
			}
			return ""
		},
		runEditor: func(editor, path string, _ io.Reader, _, _ io.Writer) error {
			gotEditor = editor
			gotPath = path
			return nil
		},
	}

	if err := execute([]string{"edit-config"}, d); err != nil {
		t.Fatalf("execute edit-config: %v", err)
	}
	if gotEditor != "vim" {
		t.Fatalf("editor = %q, want %q", gotEditor, "vim")
	}
	if gotPath == "" {
		t.Fatal("expected non-empty config path")
	}
}

func TestRelativeDate_Zero(t *testing.T) {
	got := relativeDate(time.Time{})
	if got != "unknown time" {
		t.Fatalf("relativeDate(zero) = %q, want %q", got, "unknown time")
	}
}

func TestRelativeDate_NonZero(t *testing.T) {
	got := relativeDate(time.Now().Add(-time.Hour))
	if got == "" || got == "unknown time" {
		t.Fatalf("relativeDate(now-1h) = %q, expected a non-empty relative string", got)
	}
}

func TestRunEditorCommand_Success(t *testing.T) {
	path := t.TempDir() + "/file.txt"
	err := runEditorCommand("touch", path, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("runEditorCommand touch: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatal("expected file to be created by touch")
	}
}

func TestRunEditorCommand_Failure(t *testing.T) {
	err := runEditorCommand("false", "/irrelevant", bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected error from false command")
	}
}

func TestRunEditorCommand_MultiWordEditor(t *testing.T) {
	var stdout bytes.Buffer
	// Verify a multi-word $EDITOR (e.g. "code --wait") gets split correctly.
	err := runEditorCommand("echo hello", "/dev/null", bytes.NewBuffer(nil), &stdout, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("runEditorCommand multi-word: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello") {
		t.Fatalf("expected hello in output, got: %q", stdout.String())
	}
}

func TestExecute_Version(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		t.Run(args[0], func(t *testing.T) {
			var stdout bytes.Buffer
			d := deps{
				stdout: &stdout,
				stderr: bytes.NewBuffer(nil),
			}
			if err := execute(args, d); err != nil {
				t.Fatalf("execute %v: %v", args, err)
			}
		})
	}
}

func TestExecute_Help(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		t.Run(args[0], func(t *testing.T) {
			var stdout bytes.Buffer
			d := deps{
				stdout: &stdout,
				stderr: bytes.NewBuffer(nil),
			}
			if err := execute(args, d); err != nil {
				t.Fatalf("execute %v: %v", args, err)
			}
			if stdout.String() == "" {
				t.Fatal("expected usage output")
			}
		})
	}
}

func TestExecute_StatusWithTooManyArgs(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := execute([]string{"status", "a", "b"}, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_LogWithTooManyArgs(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := execute([]string{"log", "a", "b"}, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_ShowWithTooManyArgs(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := execute([]string{"show", "a", "b"}, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_StashifyNoArgs(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}
	err := execute([]string{"stashify"}, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_WtUnknownSubcommand(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := execute([]string{"wt", "bogus"}, d)
	if err == nil {
		t.Fatal("expected error for unknown wt subcommand")
	}
}

func TestExecute_RunInit_Error(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		initConfig: func() (string, error) {
			return "", errors.New("init failed")
		},
	}
	err := execute([]string{"init"}, d)
	if err == nil || !strings.Contains(err.Error(), "init failed") {
		t.Fatalf("expected init error, got: %v", err)
	}
}

func TestRunGitInteractive_Success(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	err := runGitInteractive(repoDir, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil), "status")
	if err != nil {
		t.Fatalf("runGitInteractive: %v", err)
	}
}

func TestRunGitInteractive_Failure(t *testing.T) {
	err := runGitInteractive(t.TempDir(), bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil), "status")
	if err == nil {
		t.Fatal("expected error for non-repo dir")
	}
}

func TestExecute_PushConfirmedNoRemote(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
		confirmForce: func(string) (bool, error) {
			return true, nil
		},
	}
	err := execute([]string{"push"}, d)
	// Should fail at git fetch (no remote), not at confirm
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() == "push aborted" {
		t.Fatalf("expected to get past confirm step, got push aborted")
	}
}

func TestExecute_PushConfirmError(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	wantErr := errors.New("confirm failed")
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
		confirmForce: func(string) (bool, error) {
			return false, wantErr
		},
	}
	err := execute([]string{"push"}, d)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestResolveStatusTargetPath_ExactRoot(t *testing.T) {
	_, err := resolveStatusTargetPath("/repo", "/repo", "/repo")
	if err == nil {
		t.Fatal("expected error for exact root target")
	}
}

func TestExecute_WtClone_NoArgs(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := execute([]string{"wt", "clone"}, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_WtClone_TooManyArgs(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := execute([]string{"wt", "clone", "a", "b", "c"}, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_WtClone_GetWdError(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "", errors.New("no dir") },
	}
	err := execute([]string{"wt", "clone", "https://example.com/repo.git"}, d)
	if err == nil || !strings.Contains(err.Error(), "no dir") {
		t.Fatalf("expected getwd error, got: %v", err)
	}
}

func TestExecute_RunLog_DispatchesWithRef(t *testing.T) {
	var got string
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runLog: func(ref string) error {
			got = ref
			return nil
		},
	}
	if err := execute([]string{"log", "abc123"}, d); err != nil {
		t.Fatalf("execute log abc123: %v", err)
	}
	if got != "abc123" {
		t.Fatalf("runLog ref = %q, want %q", got, "abc123")
	}
}

func TestExecute_RunShow_DispatchesWithRef(t *testing.T) {
	var got string
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runShow: func(ref string) error {
			got = ref
			return nil
		},
	}
	if err := execute([]string{"show", "HEAD"}, d); err != nil {
		t.Fatalf("execute show HEAD: %v", err)
	}
	if got != "HEAD" {
		t.Fatalf("runShow ref = %q, want %q", got, "HEAD")
	}
}

func TestExecute_RunShow_NoRef(t *testing.T) {
	var got string
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runShow: func(ref string) error {
			got = ref
			return nil
		},
	}
	if err := execute([]string{"show"}, d); err != nil {
		t.Fatalf("execute show: %v", err)
	}
	if got != "" {
		t.Fatalf("runShow ref = %q, want empty", got)
	}
}
