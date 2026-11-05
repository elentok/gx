package gitops

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/git"
)

func runStashify(args []string, d Deps) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gx stashify <command> [args...]")
	}

	cwd, err := d.Getwd()
	if err != nil {
		return err
	}

	cfg, _ := d.Dirs().Load()
	nerd := cfg.UseNerdFontIcons

	changes, err := git.UncommittedChanges(cwd)
	if err != nil {
		return err
	}

	stashed := false
	if len(changes) > 0 {
		printBadge(d.Stderr, nerd, " Stashing changes\u2026", "Stashing changes\u2026")
		if _, err := git.Stash(cwd); err != nil {
			return fmt.Errorf("stash failed: %w", err)
		}
		stashed = true
	}

	runLabel := strings.Join(args, " ")
	printBadge(d.Stderr, nerd, "󱐋 Running "+runLabel+"\u2026", "Running "+runLabel+"\u2026")
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = cwd
	cmd.Env = append([]string{}, d.Env.Env...)
	cmd.Stdin = d.Stdin
	cmd.Stdout = d.Stdout
	cmd.Stderr = d.Stderr
	cmdErr := cmd.Run()
	if exitErr, ok := cmdErr.(*exec.ExitError); ok {
		cmdErr = &cmdbase.ExitError{Code: exitErr.ExitCode()}
	}

	if !stashed {
		return cmdErr
	}

	if cmdErr == nil {
		printBadge(d.Stderr, nerd, " Unstashing changes\u2026", "Unstashing changes\u2026")
		if _, err := git.StashPop(cwd); err != nil {
			return fmt.Errorf("stash pop failed: %w", err)
		}
		return nil
	}

	fmt.Fprintf(d.Stderr, "\nCommand failed: %v\n", cmdErr)
	confirmed, err := d.ConfirmForce("Pop stash anyway?")
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
