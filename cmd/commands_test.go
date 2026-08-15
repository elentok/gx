package cmd

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/spf13/cobra"
)

func TestNewEpicScopedCmd_ResolvesEpicArgAndInvokesRun(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	epicDir := filepath.Join(repoDir, ".scratch", "widget-epic")
	testutil.Mkdir(t, epicDir)

	var gotEpicPath string
	var gotArgs []string
	d := deps{getwd: func() (string, error) { return repoDir, nil }}
	cmd := newEpicScopedCmd(d, "widget <epic>", "test command", func(epicPath string, args []string, out io.Writer) error {
		gotEpicPath = epicPath
		gotArgs = args
		_, err := io.WriteString(out, "ok")
		return err
	})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"widget-epic"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if gotEpicPath != epicDir {
		t.Errorf("epicPath = %q, want %q", gotEpicPath, epicDir)
	}
	if len(gotArgs) != 1 || gotArgs[0] != "widget-epic" {
		t.Errorf("args = %v, want [widget-epic]", gotArgs)
	}
	if stdout.String() != "ok" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "ok")
	}
}

func TestNewEpicScopedCmd_GetwdErrorPropagates(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("boom")
	d := deps{getwd: func() (string, error) { return "", wantErr }}
	cmd := newEpicScopedCmd(d, "widget <epic>", "test command", func(string, []string, io.Writer) error {
		t.Fatal("run should not be called when getwd fails")
		return nil
	})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"widget-epic"})
	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute error = %v, want %v", err, wantErr)
	}
}

func TestNewEpicScopedCmd_RequiresExactlyOneArg(t *testing.T) {
	t.Parallel()
	d := deps{getwd: func() (string, error) { return "", nil }}
	cmd := newEpicScopedCmd(d, "widget <epic>", "test command", func(string, []string, io.Writer) error {
		t.Fatal("run should not be called with a missing arg")
		return nil
	})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for missing epic argument")
	}
}

func TestNewEpicScopedCmd_ValidArgsFunctionListsEpicNames(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	testutil.Mkdir(t, filepath.Join(repoDir, ".scratch", "widget-epic"))
	testutil.Mkdir(t, filepath.Join(repoDir, ".scratch", "bugs-05"))

	d := deps{getwd: func() (string, error) { return repoDir, nil }}
	cmd := newEpicScopedCmd(d, "widget <epic>", "test command", func(string, []string, io.Writer) error { return nil })

	names, directive := cmd.ValidArgsFunction(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	want := map[string]bool{"widget-epic": true, "bugs-05": true}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want exactly %v", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected epic name %q", n)
		}
	}
}

func TestNewEpicScopedCmd_ValidArgsFunctionErrorsWhenGetwdFails(t *testing.T) {
	t.Parallel()
	d := deps{getwd: func() (string, error) { return "", errors.New("boom") }}
	cmd := newEpicScopedCmd(d, "widget <epic>", "test command", func(string, []string, io.Writer) error { return nil })

	_, directive := cmd.ValidArgsFunction(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveError {
		t.Errorf("directive = %v, want ShellCompDirectiveError", directive)
	}
}
