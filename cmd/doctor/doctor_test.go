package doctor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/testutil"
)

func setupDotBareForCmd(t *testing.T) (outerDir string) {
	t.Helper()
	src := testutil.TempRepo(t)
	cwd := t.TempDir()

	raw, err := git.CloneBare(src, "", cwd)
	if err != nil {
		t.Fatalf("CloneBare: %v", err)
	}
	outerDir, err = filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	repo, err := git.FindRepo(outerDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	if _, err := git.UpdateRemotes(*repo); err != nil {
		t.Fatalf("UpdateRemotes: %v", err)
	}

	branch := git.RemoteDefaultBranch(repo.Root)
	wtPath := filepath.Join(outerDir, branch)
	if err := git.AddWorktreeFromRemote(*repo, wtPath, branch, "origin/"+branch); err != nil {
		t.Fatalf("AddWorktreeFromRemote: %v", err)
	}

	return outerDir
}

// runDoctor executes `gx doctor args...` with d.
func runDoctorCmd(t *testing.T, d Deps, args ...string) error {
	t.Helper()
	return cmdtest.Run(t, d.Env, append([]string{"doctor"}, args...), New(d))
}

func TestDoctor_NoIssues(t *testing.T) {
	t.Parallel()
	outerDir := setupDotBareForCmd(t)

	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return outerDir, nil },
	}}

	if err := runDoctorCmd(t, d); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(stdout.String(), "No issues found") {
		t.Errorf("expected 'No issues found', got: %q", stdout.String())
	}
}

func TestDoctor_PrintsRuntimeTerminalInfo(t *testing.T) {
	t.Parallel()
	outerDir := setupDotBareForCmd(t)

	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return outerDir, nil },
		Getenv: func(key string) string {
			switch key {
			case "KITTY_LISTEN_ON":
				return "unix:/tmp/mykitty-70704"
			case "KITTY_WINDOW_ID":
				return "12"
			case "TMUX":
				return "/tmp/tmux-1000/default,123,0"
			default:
				return ""
			}
		},
	}}

	if err := runDoctorCmd(t, d); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Runtime:") {
		t.Fatalf("expected runtime section, got: %q", out)
	}
	if !strings.Contains(out, "terminal: kitty") {
		t.Fatalf("expected detected kitty terminal, got: %q", out)
	}
	if !strings.Contains(out, `KITTY_LISTEN_ON="unix:/tmp/mykitty-70704"`) {
		t.Fatalf("expected kitty env in output, got: %q", out)
	}
}

func TestDoctor_PauseWaitsForEnter(t *testing.T) {
	t.Parallel()
	outerDir := setupDotBareForCmd(t)

	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdin:  strings.NewReader("\n"),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return outerDir, nil },
	}}

	if err := runDoctorCmd(t, d, "--pause"); err != nil {
		t.Fatalf("doctor --pause: %v", err)
	}
	if !strings.Contains(stdout.String(), "Press Enter to exit...") {
		t.Fatalf("expected pause prompt, got: %q", stdout.String())
	}
}

func TestDoctor_UnknownFlag(t *testing.T) {
	t.Parallel()
	d := Deps{Env: cmdbase.Env{
		Env:   cmdtest.EnvFor(t),
		Getwd: func() (string, error) { return "", errors.New("should not be called") },
	}}

	err := runDoctorCmd(t, d, "--wat")
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), `unknown doctor flag "--wat"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoctor_ReportsIssue(t *testing.T) {
	t.Parallel()
	outerDir := setupDotBareForCmd(t)

	// Corrupt the outer .git file.
	gitFile := filepath.Join(outerDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: wrong\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return outerDir, nil },
	}}

	if err := runDoctorCmd(t, d); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "gitdir: wrong") {
		t.Errorf("expected issue description in output, got: %q", out)
	}
	if !strings.Contains(out, "--fix") {
		t.Errorf("expected '--fix' hint in output, got: %q", out)
	}
}

func TestDoctor_Fix_AppliesWhenConfirmed(t *testing.T) {
	t.Parallel()
	outerDir := setupDotBareForCmd(t)

	gitFile := filepath.Join(outerDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: wrong\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	d := Deps{
		Env: cmdbase.Env{
			Env:    cmdtest.EnvFor(t),
			Stdout: &stdout,
			Stderr: bytes.NewBuffer(nil),
			Getwd:  func() (string, error) { return outerDir, nil },
		},
		ConfirmForce: func(string) (bool, error) { return true, nil },
	}

	if err := runDoctorCmd(t, d, "--fix"); err != nil {
		t.Fatalf("doctor --fix: %v", err)
	}

	data, err := os.ReadFile(gitFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "gitdir: ./.bare\n" {
		t.Errorf(".git content = %q, want %q", string(data), "gitdir: ./.bare\n")
	}
	if !strings.Contains(stdout.String(), "Fixed") {
		t.Errorf("expected 'Fixed' in output, got: %q", stdout.String())
	}
}

func TestDoctor_CheckBlockedForm_Pass(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	var gotTarget string
	d := Deps{
		Env: cmdbase.Env{
			Env:    cmdtest.EnvFor(t),
			Stdin:  strings.NewReader("\n"),
			Stdout: &stdout,
			Stderr: bytes.NewBuffer(nil),
		},
		ExplainAgent: func(target string) (herdr.AgentExplainResult, error) {
			gotTarget = target
			return herdr.AgentExplainResult{State: "blocked", MatchedRuleID: "live_blocked_form"}, nil
		},
	}

	if err := runDoctorCmd(t, d, "--check-blocked-form", "iter-01"); err != nil {
		t.Fatalf("doctor --check-blocked-form: %v", err)
	}
	if gotTarget != "iter-01" {
		t.Errorf("explainAgent target = %q, want %q", gotTarget, "iter-01")
	}
	if !strings.Contains(stdout.String(), "PASS: herdr matched live_blocked_form.") {
		t.Errorf("expected PASS message, got: %q", stdout.String())
	}
}

func TestDoctor_CheckBlockedForm_FailsOnWrongRule(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := Deps{
		Env: cmdbase.Env{
			Env:    cmdtest.EnvFor(t),
			Stdin:  strings.NewReader("\n"),
			Stdout: &stdout,
			Stderr: bytes.NewBuffer(nil),
		},
		ExplainAgent: func(target string) (herdr.AgentExplainResult, error) {
			return herdr.AgentExplainResult{State: "idle", MatchedRuleID: "live_prompt_box"}, nil
		},
	}

	err := runDoctorCmd(t, d, "--check-blocked-form", "iter-01")
	if err == nil {
		t.Fatal("expected error for mismatched rule")
	}
	if !strings.Contains(stdout.String(), "FAIL:") {
		t.Errorf("expected FAIL message, got: %q", stdout.String())
	}
}

func TestDoctor_Help_SaysBlockedFormIsHerdrOnly(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{Env: cmdtest.EnvFor(t), Stdout: &stdout, Stderr: &stdout}}

	if err := runDoctorCmd(t, d, "--help"); err != nil {
		t.Fatalf("doctor --help: %v", err)
	}
	out := strings.Join(strings.Fields(stdout.String()), " ")
	for _, want := range []string{"--check-blocked-form", "herdr only", "gx claude doctor"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor --help missing %q, got: %q", want, out)
		}
	}
}

func TestDoctor_CheckBlockedForm_RequiresTarget(t *testing.T) {
	t.Parallel()
	d := Deps{Env: cmdbase.Env{
		Env:   cmdtest.EnvFor(t),
		Getwd: func() (string, error) { return "", errors.New("should not be called") },
	}}

	err := runDoctorCmd(t, d, "--check-blocked-form")
	if err == nil {
		t.Fatal("expected error for missing target")
	}
	if !strings.Contains(err.Error(), "--check-blocked-form requires a pane target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoctor_Fix_SkipsWhenDeclined(t *testing.T) {
	t.Parallel()
	outerDir := setupDotBareForCmd(t)

	gitFile := filepath.Join(outerDir, ".git")
	original := []byte("gitdir: wrong\n")
	if err := os.WriteFile(gitFile, original, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	d := Deps{
		Env: cmdbase.Env{
			Env:    cmdtest.EnvFor(t),
			Stdout: &stdout,
			Stderr: bytes.NewBuffer(nil),
			Getwd:  func() (string, error) { return outerDir, nil },
		},
		ConfirmForce: func(string) (bool, error) { return false, nil },
	}

	if err := runDoctorCmd(t, d, "--fix"); err != nil {
		t.Fatalf("doctor --fix: %v", err)
	}

	data, _ := os.ReadFile(gitFile)
	if string(data) != string(original) {
		t.Errorf(".git file was modified despite declining fix")
	}
	if !strings.Contains(stdout.String(), "Skipped") {
		t.Errorf("expected 'Skipped' in output, got: %q", stdout.String())
	}
}

func TestDoctor_ReportsMissingTrunkWithoutFixing(t *testing.T) {
	outerDir := setupDotBareForCmd(t)
	repo, err := git.FindRepo(outerDir)
	if err != nil {
		t.Fatal(err)
	}
	cmdtest.RegisterWithTrunk(t, repo.Root, "develop")
	projectFile := filepath.Join(cmdtest.EnvValue(cmdtest.EnvFor(t), "XDG_DATA_HOME"), "gx", "tickets", "p", "project.json")
	before, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	d := Deps{
		Env: cmdbase.Env{
			Env:    cmdtest.EnvFor(t),
			Stdout: &stdout,
			Stderr: bytes.NewBuffer(nil),
			Getwd:  func() (string, error) { return outerDir, nil },
		},
		ConfirmForce: func(string) (bool, error) { t.Fatal("trunk check must not offer a fix"); return false, nil },
	}
	if err := runDoctorCmd(t, d, "--fix"); err != nil {
		t.Fatalf("doctor: %v", err)
	}

	out := stdout.String()
	if want := `project "p": trunk "develop" from project.json is not a local branch`; !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
	if !strings.Contains(out, "No automatic fix available") {
		t.Errorf("output = %q, want no automatic fix", out)
	}
	if after, _ := os.ReadFile(projectFile); !bytes.Equal(after, before) {
		t.Errorf("project.json changed: %s", after)
	}
	if git.RefExists(repo.Root, "refs/heads/develop") {
		t.Error("doctor created the develop branch")
	}
}

func TestDoctor_HealthyTrunkReportsNothing(t *testing.T) {
	outerDir := setupDotBareForCmd(t)
	repo, err := git.FindRepo(outerDir)
	if err != nil {
		t.Fatal(err)
	}
	cmdtest.RegisterWithTrunk(t, repo.Root, "main")

	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return outerDir, nil },
	}}
	if err := runDoctorCmd(t, d); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(stdout.String(), "No issues found") {
		t.Errorf("output = %q, want 'No issues found'", stdout.String())
	}
}
