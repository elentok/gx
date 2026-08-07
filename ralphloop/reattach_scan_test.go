package ralphloop

import (
	"errors"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

func TestScanForReattachable_ClaimedWithLiveTab_ProducesSignal(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	findWorkspace := func(label string) (string, error) {
		if label == "epic" {
			return "ws1", nil
		}
		return "", nil
	}
	tabList := func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: iterLabel("epic", "01")}}, nil
	}

	signals, err := ScanForReattachable(findWorkspace, tabList, epics)
	if err != nil {
		t.Fatalf("ScanForReattachable() error = %v", err)
	}
	if len(signals) != 1 || signals[0].EpicName != "epic" || signals[0].Ticket.Identifier != "01" {
		t.Fatalf("signals = %+v, want one signal for epic/01", signals)
	}

	// Detect-only: the ticket file on disk is untouched.
	reloaded, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load (reloaded): %v", err)
	}
	if reloaded[0].Tickets[0].Status != "claimed" {
		t.Fatalf("ticket status = %q, want unchanged \"claimed\"", reloaded[0].Tickets[0].Status)
	}
}

func TestScanForReattachable_NeedsAttentionWithLiveTab_ProducesSignal(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: needs-attention\ntype: task\n---\n# A\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	findWorkspace := func(label string) (string, error) { return "ws1", nil }
	tabList := func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: iterLabel("epic", "01")}}, nil
	}

	signals, err := ScanForReattachable(findWorkspace, tabList, epics)
	if err != nil {
		t.Fatalf("ScanForReattachable() error = %v", err)
	}
	if len(signals) != 1 || signals[0].Ticket.Identifier != "01" {
		t.Fatalf("signals = %+v, want one signal for 01", signals)
	}
}

func TestScanForReattachable_ClaimedWithNoLiveTab_ProducesNothing(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	findWorkspace := func(label string) (string, error) { return "ws1", nil }
	tabList := func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }

	signals, err := ScanForReattachable(findWorkspace, tabList, epics)
	if err != nil {
		t.Fatalf("ScanForReattachable() error = %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("signals = %+v, want none", signals)
	}
}

func TestScanForReattachable_NoWorkspace_ProducesNothingAndSkipsTabList(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	findWorkspace := func(label string) (string, error) { return "", nil }
	tabListCalled := false
	tabList := func(workspaceID string) ([]herdr.Tab, error) {
		tabListCalled = true
		return nil, nil
	}

	signals, err := ScanForReattachable(findWorkspace, tabList, epics)
	if err != nil {
		t.Fatalf("ScanForReattachable() error = %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("signals = %+v, want none", signals)
	}
	if tabListCalled {
		t.Fatal("tabList should not be called when no workspace exists")
	}
}

func TestScanForReattachable_NoClaimedOrNeedsAttentionTickets_ProducesNothing(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-a.md": "---\nid: \"02\"\nstatus: done\ntype: task\n---\n# B\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	findWorkspaceCalled := false
	findWorkspace := func(label string) (string, error) {
		findWorkspaceCalled = true
		return "ws1", nil
	}
	tabList := func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }

	signals, err := ScanForReattachable(findWorkspace, tabList, epics)
	if err != nil {
		t.Fatalf("ScanForReattachable() error = %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("signals = %+v, want none", signals)
	}
	if findWorkspaceCalled {
		t.Fatal("findWorkspace should not be called for an epic with no claimed/needs-attention tickets")
	}
}

func TestScanForReattachable_FindWorkspaceError_Propagates(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	wantErr := errors.New("herdr unreachable")
	findWorkspace := func(label string) (string, error) { return "", wantErr }
	tabList := func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }

	if _, err := ScanForReattachable(findWorkspace, tabList, epics); !errors.Is(err, wantErr) {
		t.Fatalf("ScanForReattachable() error = %v, want wrapping %v", err, wantErr)
	}
}
