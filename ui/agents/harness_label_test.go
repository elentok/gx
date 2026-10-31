package agents

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
)

func TestTerminalTitle_ShowsHarnessLabel(t *testing.T) {
	cases := []struct {
		name string
		s    server.SessionInfo
		want string
	}{
		{"claude", server.SessionInfo{ID: "a", Project: "gx", Status: idleStatus, Harness: "claude", Tier: "worker", Model: "sonnet-4.6", Effort: "high"}, "Claude · Worker · sonnet-4.6 high"},
		{"codex", server.SessionInfo{ID: "a", Project: "gx", Status: idleStatus, Harness: "codex", Tier: "strongest", Model: "gpt-6.1-sol", Effort: "high"}, "Codex · Strongest · gpt-6.1-sol high"},
		{"design", server.SessionInfo{ID: "a", Project: "gx", Status: idleStatus, Design: true, Harness: "claude", Tier: "worker"}, "Claude · Worker · CLI default"},
		{"switched", server.SessionInfo{ID: "a", Project: "gx", Status: idleStatus, Harness: "claude", Tier: "worker", Model: "opus", ModelSwitched: true}, "switched by hand"},
	}
	for _, c := range cases {
		m := openedModel(t, &fakeSessionClient{}, "", c.s)
		m.width = 160
		got := ansi.Strip(m.terminalTitle(c.s, lipgloss.NewStyle()))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: title %q lacks %q", c.name, got, c.want)
		}
	}
}

func TestTerminalTitle_NarrowWidthCutsTheLabelFirst(t *testing.T) {
	s := server.SessionInfo{ID: "a", Project: "gx", Status: idleStatus, Harness: "codex", Tier: "strongest", Model: "gpt-6.1-sol", Effort: "high"}
	m := openedModel(t, &fakeSessionClient{}, "", s)
	m.width = 50
	title := ansi.Strip(m.terminalTitle(s, lipgloss.NewStyle()))
	if strings.Contains(title, "gpt-6.1-sol high") || !strings.Contains(title, "…") {
		t.Errorf("label not cut at a narrow width: %q", title)
	}
	if !strings.Contains(title, "idle") {
		t.Errorf("status word lost: %q", title)
	}
}
