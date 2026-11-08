package herdrrunner_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
)

const vimRule = "────────────────────────────────────────"

func vimPane(footer string) string {
	return strings.Join([]string{vimRule, "❯ ", vimRule, "  " + footer}, "\n")
}

func TestPromptLaunch_VimFooterReturnsTheVimErrorAndSendsNothing(t *testing.T) {
	for _, footer := range []string{"-- INSERT --", "-- NORMAL --"} {
		t.Run(footer, func(t *testing.T) {
			r, state, s, _ := startLaunch(t)
			harness.NoteEditorOverride(s.SessionID+footer, nil)
			s.SessionID += footer
			if err := state.SetPaneText(s.Label, vimPane(footer)); err != nil {
				t.Fatal(err)
			}
			err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, nil)
			if _, ok := errors.AsType[*claudeui.VimEditorError](err); !ok {
				t.Fatalf("PromptLaunch = %v, want *VimEditorError", err)
			}
			if got, _ := state.Received(s.Label); len(got) != 0 {
				t.Fatalf("received = %q, want nothing typed", got)
			}
		})
	}
}

func TestPromptLaunch_VimFooterOnPlainPromptOrNoHomeReturnsTheVimError(t *testing.T) {
	for name, tc := range map[string]struct {
		text   string
		noHome bool
	}{
		"non-slash prompt":     {text: "fix the bug"},
		"slash prompt no home": {text: launchText, noHome: true},
	} {
		t.Run(name, func(t *testing.T) {
			r, state, s, _ := startLaunch(t)
			if tc.noHome {
				r.Home = ""
			}
			s.SessionID += "-plain-" + name
			harness.NoteEditorOverride(s.SessionID, nil)
			if err := state.SetPaneText(s.Label, vimPane("-- NORMAL --")); err != nil {
				t.Fatal(err)
			}
			err := agentrunner.PromptLaunch(context.Background(), r, s, tc.text, nil)
			if _, ok := errors.AsType[*claudeui.VimEditorError](err); !ok {
				t.Fatalf("PromptLaunch = %v, want *VimEditorError", err)
			}
			if got, _ := state.Received(s.Label); len(got) != 0 {
				t.Fatalf("received = %q, want nothing typed", got)
			}
		})
	}
}

func TestPromptLaunch_NonSlashPromptWithoutVimFooterSendsOnce(t *testing.T) {
	r, state, s, _ := startLaunch(t)
	harness.NoteEditorOverride(s.SessionID, nil)
	if err := state.SetPaneText(s.Label, vimPane("? for shortcuts")); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), r, s, "fix the bug", nil); err != nil {
		t.Fatalf("PromptLaunch = %v, want nil", err)
	}
	if got, _ := state.Received(s.Label); !slices.Equal(got, []string{"fix the bug"}) {
		t.Fatalf("received = %q, want one send", got)
	}
}

func TestPromptLaunch_NoVimFooterSendsThePromptOnce(t *testing.T) {
	r, state, s, home := startLaunch(t)
	harness.NoteEditorOverride(s.SessionID, nil)
	writeFirstUser(t, home, launchRecord)
	if err := state.SetPaneText(s.Label, vimPane("? for shortcuts")); err != nil {
		t.Fatal(err)
	}
	if err := state.SlashStartsTurn(s.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, nil); err != nil {
		t.Fatalf("PromptLaunch = %v, want nil", err)
	}
	if got, _ := state.Received(s.Label); !slices.Equal(got, []string{launchText}) {
		t.Fatalf("received = %q, want one send", got)
	}
}

func TestPromptLaunch_SkippedVimCheckTypesDespiteTheFooter(t *testing.T) {
	r, state, s, home := startLaunch(t)
	harness.NoteEditorOverride(s.SessionID, nil)
	writeFirstUser(t, home, launchRecord)
	if err := state.SetPaneText(s.Label, vimPane("-- NORMAL --")); err != nil {
		t.Fatal(err)
	}
	if err := state.SlashStartsTurn(s.Label); err != nil {
		t.Fatal(err)
	}
	ctx := agentrunner.WithoutVimCheck(context.Background())
	if err := agentrunner.PromptLaunch(ctx, r, s, launchText, nil); err != nil {
		t.Fatalf("PromptLaunch = %v, want nil", err)
	}
	if got, _ := state.Received(s.Label); !slices.Equal(got, []string{launchText}) {
		t.Fatalf("received = %q, want one send", got)
	}
}

func TestPromptLaunch_CodexNeverChecksForVim(t *testing.T) {
	r, state, s := startCodex(t, nil)
	harness.NoteEditorOverride(s.SessionID, nil)
	if err := state.SetPaneText(s.Label, vimPane("-- NORMAL --")); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), r, s, "/status", nil); err != nil {
		t.Fatalf("PromptLaunch = %v, want nil", err)
	}
	if got, _ := state.Received(s.Label); !slices.Equal(got, []string{"/status"}) {
		t.Fatalf("received = %q, want one send", got)
	}
}
