package prompt_test

import (
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/agentfake"
)

// composerMarker is the pointer glyph and U+00A0 claude draws on the composer row.
const composerMarker = "❯ "

func TestFirstPrompt_TypedOnTheComposerMarkerAloneWithNoClear(t *testing.T) {
	t.Parallel()
	_, _, _, _, conv := startFirstPromptSession(t, "fix the bug", agentfake.ProgramLogClearEnv+"=1")
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionWrite, Output: composerMarker}); err != nil {
		t.Fatal(err)
	}
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
	if got := submitted(t, conv); strings.Contains(got, agentfake.ProgramClearEntry) {
		t.Errorf("submitted log = %q, want no clear key before the first prompt", got)
	}
}

// The box and a header on its row are on screen, but no signal fired: the
// screen alone never starts the prompt.
func TestFirstPrompt_MatchingInputBoxAloneTypesNothingBeforeTheStartTimeout(t *testing.T) {
	t.Parallel()
	f := startFailingFirstPrompt(t, "fix the bug")
	got := f.gxActionable(t)
	if !strings.Contains(got.Reason, "still starting") {
		t.Errorf("reason = %q, want a still-starting reason", got.Reason)
	}
	// The Actionable came at the timeout, so anything typed would be logged by now.
	if s := submitted(t, f.conv); s != "" {
		t.Errorf("submitted = %q, want nothing typed", s)
	}
}
