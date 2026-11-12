package epics

import (
	"testing"

	"github.com/elentok/gx/server/wire"
)

func implCard(addr string, section wire.CardSection, state wire.EpicListState) wire.EpicListRow {
	return wire.EpicListRow{Address: addr, CardKind: wire.CardKindImplementation, Section: section, State: state}
}

func unreadOf(rows []wire.EpicListRow) []bool {
	out := make([]bool, len(rows))
	for i, r := range rows {
		out[i] = r.Unread
	}
	return out
}

func TestSeen_FirstScanSeedsEverythingAsSeen(t *testing.T) {
	s := OpenSeen(t.TempDir())
	rows := []wire.EpicListRow{
		implCard("p:a", wire.SectionNeedsYou, wire.EpicListPaused),
		implCard("p:b", wire.SectionFinished, wire.EpicListFinished),
	}
	if err := s.Apply(rows); err != nil {
		t.Fatal(err)
	}
	if got := unreadOf(rows); got[0] || got[1] {
		t.Errorf("unread after seed = %v, want none", got)
	}
}

func TestSeen_NewStateIsUnreadUntilMarked(t *testing.T) {
	s := OpenSeen(t.TempDir())
	if err := s.Apply(nil); err != nil { // seeds
		t.Fatal(err)
	}
	rows := []wire.EpicListRow{implCard("p:a", wire.SectionNeedsYou, wire.EpicListPaused)}
	_ = s.Apply(rows)
	if !rows[0].Unread {
		t.Fatal("a card that entered Needs you after the seed must be unread")
	}
	if err := s.Mark("p:a", seenNeedsYou); err != nil {
		t.Fatal(err)
	}
	rows = []wire.EpicListRow{implCard("p:a", wire.SectionNeedsYou, wire.EpicListPaused)}
	_ = s.Apply(rows)
	if rows[0].Unread {
		t.Error("a marked card must read as seen")
	}
}

func TestSeen_NeedsYouThenWorkingThenNeedsYouIsUnreadAgain(t *testing.T) {
	s := OpenSeen(t.TempDir())
	_ = s.Apply(nil)
	_ = s.Mark("p:a", seenNeedsYou)
	working := []wire.EpicListRow{implCard("p:a", wire.SectionImplementing, wire.EpicListRunning)}
	_ = s.Apply(working)
	if working[0].Unread {
		t.Error("a Working card is never unread")
	}
	back := []wire.EpicListRow{implCard("p:a", wire.SectionNeedsYou, wire.EpicListPaused)}
	_ = s.Apply(back)
	if !back[0].Unread {
		t.Error("returning to Needs you must be unread again")
	}
}

func TestSeen_MergedIsUnreadAndOtherKindsNever(t *testing.T) {
	s := OpenSeen(t.TempDir())
	_ = s.Apply(nil)
	rows := []wire.EpicListRow{
		implCard("p:a", wire.SectionFinished, wire.EpicListFinished),
		{Address: "p:d", CardKind: wire.CardKindDesign, Section: wire.SectionNeedsYou},
		{Address: "p:o", CardKind: wire.CardKindOneOff, Section: wire.SectionNeedsYou},
	}
	_ = s.Apply(rows)
	if got := unreadOf(rows); !got[0] || got[1] || got[2] {
		t.Errorf("unread = %v, want only the merged Implementation card", got)
	}
}

func TestSeen_SurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s := OpenSeen(dir)
	_ = s.Apply(nil)
	_ = s.Mark("p:a", seenNeedsYou)
	rows := []wire.EpicListRow{implCard("p:a", wire.SectionNeedsYou, wire.EpicListPaused)}
	_ = OpenSeen(dir).Apply(rows)
	if rows[0].Unread {
		t.Error("seen state must persist across a reopen")
	}
}
