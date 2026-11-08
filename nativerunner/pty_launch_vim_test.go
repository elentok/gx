package nativerunner_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/testutil/agentfake"
)

var vimCaps = claudeui.LaunchProofCaps{Grace: 10 * time.Second, UnknownGrace: time.Minute, ResendEvery: time.Hour}

func typedKeys(t *testing.T, home string, s agentrunner.Session) string {
	t.Helper()
	in, err := os.ReadFile(agentfake.TUIInputPath(home, s.SessionID))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(in)
}

func TestPty_VimFooterAfterOverrideTripsTheCheckAndTypesNothing(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"INSERT", "NORMAL"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			spec := mode + ":1:" + filepath.Join(t.TempDir(), "launches")
			p, s := launchPty(t, vimCaps, agentfake.TUIVimWedgeEnv+"="+spec)

			err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil)

			if _, ok := errors.AsType[*claudeui.VimEditorError](err); !ok {
				t.Fatalf("PromptLaunch = %v; want a VimEditorError", err)
			}
			if got := typedKeys(t, ptyHome(p), s); got != "" {
				t.Errorf("keys typed into the wedged first launch = %q; want none", got)
			}
		})
	}
}

func TestPty_VimCheckSkippedTypesIntoTheFooterPane(t *testing.T) {
	t.Parallel()
	spec := "INSERT:1:" + filepath.Join(t.TempDir(), "launches")
	p, s := launchPty(t, vimCaps, agentfake.TUIVimWedgeEnv+"="+spec)

	_ = p.PromptLaunch(agentrunner.WithoutVimCheck(context.Background()), s, "/gx-implement x/1", nil)

	if got := typedKeys(t, ptyHome(p), s); !strings.Contains(got, "/gx-implement x/1") {
		t.Errorf("keys typed = %q; want the prompt typed anyway", got)
	}
}

func TestPty_VimRelaunchAfterTheCheckDeliversThePrompt(t *testing.T) {
	t.Parallel()
	spec := "INSERT:1:" + filepath.Join(t.TempDir(), "launches")
	p, s := launchPty(t, vimCaps, agentfake.TUIVimWedgeEnv+"="+spec)
	if err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil); err == nil {
		t.Fatal("first launch: want the vim check to trip")
	}
	if err := p.Stop(s); err != nil {
		t.Fatal(err)
	}
	s2, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(s2) })

	// The relaunch is healthy: the check passes and the prompt is typed. No
	// worker config for this cwd, so only the typed keys are asserted.
	_ = p.PromptLaunch(context.Background(), s2, "/gx-implement x/1", nil)

	if got := typedKeys(t, ptyHome(p), s2); !strings.Contains(got, "/gx-implement x/1") {
		t.Errorf("relaunch keys = %q; want the prompt typed", got)
	}
}

func TestPty_HealthyLaunchPassesTheVimCheck(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, vimCaps)

	err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil)

	if _, ok := errors.AsType[*claudeui.VimEditorError](err); ok {
		t.Fatalf("PromptLaunch = %v; want no vim trip on a healthy launch", err)
	}
	if got := typedKeys(t, ptyHome(p), s); !strings.Contains(got, "/gx-implement x/1") {
		t.Errorf("keys typed = %q; want the prompt", got)
	}
}
