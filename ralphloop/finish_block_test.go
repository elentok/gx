package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/tickets/schema"
)

const blockHead = "deadbeef"

// blockFixture writes a one-ticket epic of the given type whose body is body.
func blockFixture(t *testing.T, typ, body string) (scratchDir, path string) {
	t.Helper()
	scratchDir = fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: " + typ + "\n---\n# A\n\n" + body,
	})
	return scratchDir, filepath.Join(scratchDir, "epic", "issues", "01-a.md")
}

// agentReportsFinished makes every prompt end with the agent reporting
// finished; onBlock runs first when the prompt is a finish block.
func agentReportsFinished(t *testing.T, d deps.Deps, path string, onBlock func(path string)) {
	t.Helper()
	fakes.OnRunnerPrompt(d, func(_ agentrunner.Session, text string) error {
		if strings.Contains(text, "no ## Verified entry") && onBlock != nil {
			onBlock(path)
		}
		return ticketops.UpdateTicket(path, func(tk *schema.Ticket) { tk.IterationStatus = schema.IterationStatusFinished })
	})
}

func verifyAll(path string) {
	raw, _ := os.ReadFile(path)
	body := string(raw)
	for _, s := range schema.ParseStories(schema.ParseBody(body)) {
		body = schema.AppendVerified(body, schema.VerifiedEntry{Story: s, Verdict: "pass", Commit: blockHead})
	}
	_ = os.WriteFile(path, []byte(body), 0o644)
}

func countEvents(t *testing.T, scratchDir string, typ string) int {
	t.Helper()
	evs, _, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	n := 0
	for _, e := range evs {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func TestFinishIteration_VerifyFinishWithUncheckedStories_BlocksAndResends(t *testing.T) {
	t.Parallel()
	scratchDir, path := blockFixture(t, "verify", "## Stories\n\n- Story 1\n- Story 2\n")
	d, prompts, _ := fakes.FakeDeps()
	fakes.FakeRunner(d).PromptState = agentrunner.StateIdle
	agentReportsFinished(t, d, path, verifyAll)

	driveIteration(t, d, scratchDir, path, &fakes.RecordingSink{})

	if len(*prompts) != 2 {
		t.Fatalf("prompts = %v, want launch + one block", *prompts)
	}
	if got := (*prompts)[1]; !strings.Contains(got, "Story 1, Story 2") || strings.HasPrefix(got, "/") {
		t.Errorf("block prompt = %q, want plain text naming both units", got)
	}
	if n := countEvents(t, scratchDir, "finish-blocked"); n != 1 {
		t.Errorf("finish-blocked events = %d, want 1", n)
	}
}

func TestFinishIteration_VerifiedUncheckedWithReason_LetsFinishThrough(t *testing.T) {
	t.Parallel()
	body := "## Stories\n\n- Mock states: ready\n\n## Verified\n\n- Mock states: ready — unchecked skipped: no gx fixture @ " + blockHead + "\n"
	scratchDir, path := blockFixture(t, "verify", body)
	d, prompts, _ := fakes.FakeDeps()
	fakes.FakeRunner(d).PromptState = agentrunner.StateIdle
	agentReportsFinished(t, d, path, nil)

	driveIteration(t, d, scratchDir, path, &fakes.RecordingSink{})

	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want only the launch prompt (no finish block)", *prompts)
	}
	if n := countEvents(t, scratchDir, "finish-blocked"); n != 0 {
		t.Errorf("finish-blocked events = %d, want 0", n)
	}
}

func TestFinishIteration_VerifyFinishPastCap_ErrorsNamingUnits(t *testing.T) {
	t.Parallel()
	scratchDir, path := blockFixture(t, "verify", "## Stories\n\n- Story 1\n- Mock states: ready\n")
	d, prompts, _ := fakes.FakeDeps()
	fakes.FakeRunner(d).PromptState = agentrunner.StateIdle
	agentReportsFinished(t, d, path, nil)

	_, err := driveIterationErr(t, d, scratchDir, path, &fakes.RecordingSink{})

	if err == nil || !strings.Contains(err.Error(), "Story 1, Mock states: ready") {
		t.Fatalf("err = %v, want an error naming the unchecked units", err)
	}
	if want := 1 + maxFinishBlocks; len(*prompts) != want {
		t.Errorf("prompts = %d, want %d (launch + cap blocks)", len(*prompts), want)
	}
}

func TestFinishIteration_FinishWithoutBlock(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ typ, body string }{
		"verify without Stories":      {"verify", "## Verified\n"},
		"implement with Stories text": {"implement", "## Stories\n\n- Story 1\n"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scratchDir, path := blockFixture(t, c.typ, c.body)
			d, prompts, _ := fakes.FakeDeps()
			fakes.FakeRunner(d).PromptState = agentrunner.StateIdle
			agentReportsFinished(t, d, path, nil)

			driveIteration(t, d, scratchDir, path, &fakes.RecordingSink{})

			if len(*prompts) != 1 {
				t.Errorf("prompts = %v, want the launch only", *prompts)
			}
			if n := countEvents(t, scratchDir, "finish-blocked"); n != 0 {
				t.Errorf("finish-blocked events = %d, want 0", n)
			}
		})
	}
}
