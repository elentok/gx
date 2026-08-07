package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestExecute_PRsDispatchesToRunPRs(t *testing.T) {
	called := 0
	var gotAllRepos bool
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runPRs: func(allRepos bool) error {
			called++
			gotAllRepos = allRepos
			return nil
		},
	}

	if err := execute([]string{"prs"}, d); err != nil {
		t.Fatalf("execute prs: %v", err)
	}
	if called != 1 {
		t.Fatalf("runPRs called %d times, want 1", called)
	}
	if gotAllRepos {
		t.Fatal("expected allRepos false without --all flag")
	}
}

func TestExecute_PRsAllFlagDispatchesAllRepos(t *testing.T) {
	called := 0
	var gotAllRepos bool
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runPRs: func(allRepos bool) error {
			called++
			gotAllRepos = allRepos
			return nil
		},
	}

	if err := execute([]string{"prs", "--all"}, d); err != nil {
		t.Fatalf("execute prs --all: %v", err)
	}
	if called != 1 {
		t.Fatalf("runPRs called %d times, want 1", called)
	}
	if !gotAllRepos {
		t.Fatal("expected allRepos true with --all flag")
	}
}

func TestExecute_TicketsDispatchesToRunTickets(t *testing.T) {
	called := 0
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runTickets: func() error {
			called++
			return nil
		},
	}

	if err := execute([]string{"tickets"}, d); err != nil {
		t.Fatalf("execute tickets: %v", err)
	}
	if called != 1 {
		t.Fatalf("runTickets called %d times, want 1", called)
	}
}

func TestExecute_TicketsAliasTk(t *testing.T) {
	called := 0
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		runTickets: func() error {
			called++
			return nil
		},
	}

	if err := execute([]string{"tk"}, d); err != nil {
		t.Fatalf("execute tk: %v", err)
	}
	if called != 1 {
		t.Fatalf("runTickets called %d times, want 1", called)
	}
}

func TestExecute_DefaultRunsStatus(t *testing.T) {
	called := 0
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return testutil.TempRepo(t), nil },
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

func TestExecute_DefaultFromBareRootRunsWorktrees(t *testing.T) {
	bareRoot := testutil.TempBareRepo(t)

	statusCalled, worktreesCalled := 0, 0
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return bareRoot, nil },
		runStatus: func(_ string) error {
			statusCalled++
			return nil
		},
		runWorktrees: func(_ string) error {
			worktreesCalled++
			return nil
		},
	}

	if err := execute(nil, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if worktreesCalled != 1 || statusCalled != 0 {
		t.Fatalf("from bare root: runWorktrees=%d runStatus=%d, want 1 and 0", worktreesCalled, statusCalled)
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
	d := deps{
		stdout:       bytes.NewBuffer(nil),
		stderr:       bytes.NewBuffer(nil),
		runWorktrees: func(_ string) error { return nil },
	}
	err := execute([]string{"nope"}, d)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got: %v", err)
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
