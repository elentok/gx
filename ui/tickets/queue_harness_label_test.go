package tickets

import (
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/viewmodel"
)

func TestQueue_HarnessLabelsByAddress(t *testing.T) {
	label := func(h, tier, model, effort, note string) server.HarnessLabel {
		return server.HarnessLabel{Harness: h, Tier: tier, Model: model, Effort: effort, HarnessNote: note}
	}
	st := viewmodel.State{Tickets: []server.TicketInfo{
		{Address: "gx:a/01", HarnessLabel: label("", "", "", "", "follows default")},
		{Address: "gx:a/02", HarnessLabel: label("codex", "", "", "", "pinned")},
		{Address: "gx:a/03", HarnessLabel: label("claude", "", "", "", "keeps Claude (ran before)")},
		{Address: "gx:a/04", HarnessLabel: label("codex", "worker", "gpt-6.1-sol", "medium", "")},
		{Address: "gx:a/05"},
	}}
	m, _ := QueueModel{}.applyServerState(&st)
	want := map[string]string{
		"gx:a/01": "follows default",
		"gx:a/02": "Codex · pinned",
		"gx:a/03": "keeps Claude (ran before)",
		"gx:a/04": "Codex · Worker · gpt-6.1-sol medium",
	}
	if len(m.harnessLabels) != len(want) {
		t.Errorf("labels = %v, want %v (unlabelled row must have none)", m.harnessLabels, want)
	}
	for addr, w := range want {
		if got := m.harnessLabels[addr]; got != w {
			t.Errorf("%s: label %q, want %q", addr, got, w)
		}
	}
}
