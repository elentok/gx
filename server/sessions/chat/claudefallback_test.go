package chat

import (
	"testing"

	"github.com/elentok/gx/agentrunner/claudeui"
)

func TestAddDirCard(t *testing.T) {
	cards := AddDirCard(claudeui.AddDirDialog{Dir: "/x/pnpm", ID: "adddir-ab"}, 3)
	if len(cards) != 1 || cards[0].ID != "adddir-ab-3" || !cards[0].Fallback {
		t.Fatalf("cards = %+v", cards)
	}
	q := cards[0].Questions[0]
	if q.Title != "Add /x/pnpm?" || len(q.Options) != 2 || q.Options[0].ID != "1" || q.Options[1].Label != "No, exit" {
		t.Errorf("question = %+v", q)
	}
}
