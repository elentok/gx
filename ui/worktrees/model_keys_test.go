package worktrees

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMIsNotAChordPrefix(t *testing.T) {
	for _, b := range newWorktreesManager().Bindings() {
		if len(b.Seq) > 1 && b.Seq[0] == "m" {
			t.Errorf("unexpected m chord %v", b.Seq)
		}
	}
}

func TestRefreshKeyStillBound(t *testing.T) {
	km := newWorktreesManager()
	if match, _ := km.Process(tea.KeyPressMsg{Code: 'R', Text: "R"}); match == nil || match.ID != bindingRefresh {
		t.Fatalf("expected R to resolve to refresh, got %v", match)
	}
}
