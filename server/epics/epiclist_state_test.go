package epics

import (
	"github.com/elentok/gx/server/wire"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

// epicStateValues parses wire/epiclist.go for every EpicState constant, so a
// new state cannot slip past the guard below.
func epicStateValues(t *testing.T) []wire.EpicState {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "../wire/epiclist.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []wire.EpicState
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "EpicState" {
				continue
			}
			for _, v := range vs.Values {
				s, err := strconv.Unquote(v.(*ast.BasicLit).Value)
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, wire.EpicState(s))
			}
		}
	}
	return out
}

func TestEpicListState_EveryEpicStateHasItsOwn(t *testing.T) {
	t.Parallel()
	states := epicStateValues(t)
	if len(states) == 0 {
		t.Fatal("found no EpicState constants")
	}
	for _, s := range states {
		e := tickets.Epic{Name: "epic"}
		want := wire.EpicListState(s)
		switch s {
		case wire.EpicStateFinished:
			// Finished is decided by merged_at, not by the queue state.
			e.MergedAt = time.Unix(100, 0)
		case wire.EpicStateWaiting:
			// A waiting epic is queued, ready or has no open ticket, depending
			// on the queue and its tickets; it has no list state of its own.
			continue
		}
		if got := epicListState(e, false, false, s); got != want {
			t.Errorf("epicListState(%q) = %q, want %q", s, got, want)
		}
	}
}

func TestCardTimeFor_QueuedFallsBackToQueuedAt(t *testing.T) {
	t.Parallel()
	queuedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	row := wire.EpicListRow{Section: wire.SectionImplementing, State: wire.EpicListQueued, QueuedAt: queuedAt}
	kind, start := cardTimeFor(row, cardTimeSources{})
	if kind != wire.CardTimeElapsed || !start.Equal(queuedAt) {
		t.Errorf("time = %q %v, want elapsed from queued_at", kind, start)
	}
	enq := queuedAt.Add(time.Hour)
	_, start = cardTimeFor(row, cardTimeSources{enqueuedAt: enq})
	if !start.Equal(enq) {
		t.Errorf("start = %v, want the enqueue time %v", start, enq)
	}
}

func TestCardTimeFor_SectionMapsToKind(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		name string
		row  wire.EpicListRow
		src  cardTimeSources
		kind wire.CardTimeKind
	}{
		{"needs you", wire.EpicListRow{Section: wire.SectionNeedsYou, WaitingSince: at}, cardTimeSources{}, wire.CardTimeWaiting},
		{"running", wire.EpicListRow{Section: wire.SectionImplementing, State: wire.EpicListRunning}, cardTimeSources{startedAt: at}, wire.CardTimeElapsed},
		{"ready to queue", wire.EpicListRow{Section: wire.SectionReadyToQueue, CreatedAt: at}, cardTimeSources{}, wire.CardTimeCreated},
		{"ready to design", wire.EpicListRow{Section: wire.SectionReadyToDesign, CreatedAt: at}, cardTimeSources{}, wire.CardTimeCreated},
		{"finished", wire.EpicListRow{Section: wire.SectionFinished, CompletedAt: at}, cardTimeSources{}, wire.CardTimeFinished},
		{"no start", wire.EpicListRow{Section: wire.SectionNeedsYou}, cardTimeSources{}, ""},
		{"no section", wire.EpicListRow{CreatedAt: at}, cardTimeSources{}, ""},
	} {
		kind, start := cardTimeFor(tc.row, tc.src)
		if kind != tc.kind || (kind != "" && !start.Equal(at)) {
			t.Errorf("%s: time = %q %v, want %q %v", tc.name, kind, start, tc.kind, at)
		}
	}
}
