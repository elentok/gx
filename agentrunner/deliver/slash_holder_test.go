package deliver_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/testutil/agentfake"
)

func slashPolicy(text string, extra *[]claudeui.NudgeDecision) deliver.Policy {
	p := deliver.PolicyFor(text)
	p.Slash.Settle = 300 * time.Millisecond
	p.Slash.Gap = 300 * time.Millisecond
	p.Report = func(d claudeui.NudgeDecision) { *extra = append(*extra, d) }
	return p
}

func TestSlashHolder_SwallowedFirstEnterLandsWithOneExtraEnter(t *testing.T) {
	h := holdInput(t)
	h.Control(agentfake.Control{Action: agentfake.ActionSwallowEnter})
	var extra []claudeui.NudgeDecision
	if err := deliver.Deliver(h.target(t, harness.ClaudeTyping()), "/local", slashPolicy("/local", &extra)); err != nil {
		t.Fatal(err)
	}
	h.waitSubmitted("\"/local\"\n")
	if len(extra) != 1 {
		t.Errorf("extra Enters = %v, want one", extra)
	}
}

func TestSlashHolder_TurnStartEndsTheSendAtOnce(t *testing.T) {
	h := holdInput(t)
	var extra []claudeui.NudgeDecision
	if err := deliver.Deliver(h.target(t, harness.ClaudeTyping()), "/clear", slashPolicy("/clear", &extra)); err != nil {
		t.Fatal(err)
	}
	h.waitSubmitted("\"/clear\"\n")
	if len(extra) != 0 {
		t.Errorf("extra Enters = %v, want none", extra)
	}
}
