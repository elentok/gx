package merge

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

var testRoot = queue.RootRef{}

func TestRangeOf_GxMergeIsTheTargetsTipBeforeAndAfter(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	tip, err := git.RevParse(repo, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}

	base, got := rangeOf(slog.New(slog.DiscardHandler), testRoot, tickets.Epic{Name: "e"}, repo, "main", mergedByGx, "before")

	if base != "before" || got != tip {
		t.Errorf("range = %s..%s, want before..%s", base, got, tip)
	}
}

func TestRangeOf_FailedTipReadStampsNoRangeAndWarns(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	base, tip := rangeOf(log, testRoot, tickets.Epic{Name: "e"}, repo, "no-such-branch", mergedByGx, "before")

	if base != "" || tip != "" {
		t.Errorf("range = %s..%s, want none", base, tip)
	}
	if !strings.Contains(buf.String(), "stamping no range") {
		t.Errorf("no warning logged: %q", buf.String())
	}
}

func TestRangeOf_MapEpicGetsNoRangeInAnyStyle(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	for _, style := range []mergeStyle{mergedByGx, mergedByHand, mergedByPR} {
		base, tip := rangeOf(slog.New(slog.DiscardHandler), testRoot, tickets.Epic{Name: "m", IsMap: true}, repo, "main", style, "before")
		if base != "" || tip != "" {
			t.Errorf("style %d: range = %s..%s, want none", style, base, tip)
		}
	}
}
