package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
)

func runStashify(args []string, d deps) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gx stashify <command> [args...]")
	}

	cwd, err := d.getwd()
	if err != nil {
		return err
	}

	cfg, _ := config.Load()
	nerd := cfg.UseNerdFontIcons

	changes, err := git.UncommittedChanges(cwd)
	if err != nil {
		return err
	}

	stashed := false
	if len(changes) > 0 {
		printBadge(d.stderr, nerd, " Stashing changes\u2026", "Stashing changes\u2026")
		if _, err := git.Stash(cwd); err != nil {
			return fmt.Errorf("stash failed: %w", err)
		}
		stashed = true
	}

	runLabel := strings.Join(args, " ")
	printBadge(d.stderr, nerd, "󱐋 Running "+runLabel+"\u2026", "Running "+runLabel+"\u2026")
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = cwd
	cmd.Stdin = d.stdin
	cmd.Stdout = d.stdout
	cmd.Stderr = d.stderr
	cmdErr := cmd.Run()
	if exitErr, ok := cmdErr.(*exec.ExitError); ok {
		cmdErr = &ExitError{Code: exitErr.ExitCode()}
	}

	if !stashed {
		return cmdErr
	}

	if cmdErr == nil {
		printBadge(d.stderr, nerd, " Unstashing changes\u2026", "Unstashing changes\u2026")
		if _, err := git.StashPop(cwd); err != nil {
			return fmt.Errorf("stash pop failed: %w", err)
		}
		return nil
	}

	fmt.Fprintf(d.stderr, "\nCommand failed: %v\n", cmdErr)
	confirmed, err := d.confirmForce("Pop stash anyway?")
	if err != nil {
		return err
	}
	if confirmed {
		if _, popErr := git.StashPop(cwd); popErr != nil {
			return fmt.Errorf("stash pop failed: %w", popErr)
		}
	}
	return cmdErr
}
