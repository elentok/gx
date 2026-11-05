package rowactions

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestEpicMenu_MergeLabelNamesTheBranch(t *testing.T) {
	items := epicMenuItems(epicMenuInput{branch: "main"})
	itemByLabel(t, items, "Merge into main")
}

func TestEpicMenu_AddEpicToQueueDimReasons(t *testing.T) {
	open := tickets.Epic{Name: "alpha", Status: "open"}
	items := epicMenuItems(epicMenuInput{epic: open})
	if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != nothingToQueue {
		t.Errorf("no queueable tickets: disabled = %q, want %q", got, nothingToQueue)
	}
	items = epicMenuItems(epicMenuInput{epic: open, queueable: true, serverReason: ServerNotRunning})
	if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != ServerNotRunning {
		t.Errorf("no server: disabled = %q, want %q", got, ServerNotRunning)
	}
	items = epicMenuItems(epicMenuInput{epic: open, queueable: true})
	if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != "" {
		t.Errorf("queueable: disabled = %q, want enabled", got)
	}
}

func TestEpicMenu_AddEpicToQueueRefusedEpics(t *testing.T) {
	for _, c := range []struct {
		name string
		epic tickets.Epic
		want string
	}{
		{"follow-ups", tickets.Epic{Name: "follow-ups", Status: "open"}, followUpsNotQueueable},
		{"follow-ups address", tickets.Epic{Name: "gx:follow-ups", Status: "open"}, followUpsNotQueueable},
		{"follow-ups-archived", tickets.Epic{Name: "follow-ups-archived", Status: "open"}, followUpsNotQueueable},
		{"draft", tickets.Epic{Name: "alpha", Status: "draft"}, "epic is draft, not open"},
		{"done", tickets.Epic{Name: "alpha", Status: "done"}, "epic is done, not open"},
		{"cancelled", tickets.Epic{Name: "alpha", Status: "cancelled"}, "epic is cancelled, not open"},
	} {
		items := epicMenuItems(epicMenuInput{epic: c.epic, queueable: true})
		if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != c.want {
			t.Errorf("%s: disabled = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEpicMenu_MergeBlockVerbsComeFromActions(t *testing.T) {
	done := tickets.Epic{Name: "gx:alpha", Status: "open", Tickets: []tickets.Ticket{{Status: "done"}}}
	labels := func(actions []string) []string {
		var out []string
		for _, it := range epicMenuItems(epicMenuInput{epic: done, actions: actions}) {
			if strings.HasPrefix(it.Label, "Merge") || strings.HasPrefix(it.Label, "Stash") {
				out = append(out, it.Label)
			}
		}
		return out
	}
	if got, want := labels([]string{"stash-merge", "merge"}), []string{"Stash, merge and restore", "Merge again"}; !slices.Equal(got, want) {
		t.Errorf("dirty-trunk labels = %v, want %v", got, want)
	}
	if got, want := labels([]string{"merge"}), []string{"Merge again"}; !slices.Equal(got, want) {
		t.Errorf("needs-rebase labels = %v, want %v", got, want)
	}
	if got, want := labels(nil), []string{"Merge"}; !slices.Equal(got, want) {
		t.Errorf("no actions labels = %v, want %v", got, want)
	}
}

func TestEpicMenu_CurrentStatusIsDim(t *testing.T) {
	items := epicMenuItems(epicMenuInput{epic: tickets.Epic{Status: "open"}})
	if got := itemByLabel(t, items, "Set open").Disabled; got != "already open" {
		t.Errorf("Set open disabled = %q, want already open", got)
	}
	if got := itemByLabel(t, items, "Set draft").Disabled; got != "" {
		t.Errorf("Set draft disabled = %q, want enabled", got)
	}
}
