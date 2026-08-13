package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

func writeGateTicket(t *testing.T, dir, id string) string {
	t.Helper()
	marshaled, err := schema.MarshalTicket(schema.Ticket{
		ID:     schema.TicketID(id),
		Status: schema.StatusClaimed,
		Type:   schema.TypeTask,
	}, "body\n")
	if err != nil {
		t.Fatalf("MarshalTicket: %v", err)
	}
	path := filepath.Join(dir, id+"-ticket.md")
	if err := os.WriteFile(path, marshaled, 0644); err != nil {
		t.Fatalf("write ticket: %v", err)
	}
	return path
}

func readGateTicket(t *testing.T, path string) schema.Ticket {
	t.Helper()
	ticket, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatalf("ParseTicket: %v", err)
	}
	return ticket
}

func TestNotificationGate_UnderBudget_Allowed(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "notifications-state.json")
	ticketPath := writeGateTicket(t, dir, "01")
	base := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	result, err := notificationGateAt(statePath, "telegram", "iteration-stalled", ticketPath, base, true, nil)
	if err != nil {
		t.Fatalf("notificationGateAt: %v", err)
	}
	if result.Decision != Allowed || result.EdgeTriggered {
		t.Fatalf("result = %+v, want Allowed/no edge", result)
	}
}

func TestNotificationGate_PerSourceThreshold_Trips(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "notifications-state.json")
	ticketPath := writeGateTicket(t, dir, "01")
	base := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	var parked []string
	parkTicket := func(source, reason string) error {
		parked = append(parked, source+":"+reason)
		return nil
	}

	var last GateResult
	for i := range 5 {
		now := base.Add(time.Duration(i) * 10 * time.Second)
		result, err := notificationGateAt(statePath, "telegram", "iteration-stalled", ticketPath, now, false, parkTicket)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		last = result
	}

	if last.Decision != PerSourceMuted || !last.EdgeTriggered {
		t.Fatalf("5th call result = %+v, want PerSourceMuted/edge", last)
	}
	if len(parked) != 1 {
		t.Fatalf("parkTicket called %d times, want 1: %v", len(parked), parked)
	}

	ticket := readGateTicket(t, ticketPath)
	if len(ticket.Mutes) != 1 || ticket.Mutes[0].EventType != "iteration-stalled" {
		t.Fatalf("ticket.Mutes = %+v, want one iteration-stalled mute", ticket.Mutes)
	}
}

func TestNotificationGate_GlobalTrip_DominantSourceAlsoPerSourceMuted(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "notifications-state.json")
	dominant := writeGateTicket(t, dir, "01")
	base := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	// 15 events from the dominant source (below its own 5/60s per-source
	// threshold's window doesn't matter here — it's attribution, not the
	// threshold path, that's under test).
	now := base
	for i := range 15 {
		now = base.Add(time.Duration(i) * time.Second)
		if _, err := notificationGateAt(statePath, "telegram", "iteration-stalled", dominant, now, false, nil); err != nil {
			t.Fatalf("dominant event %d: %v", i, err)
		}
	}

	// 20 sends from a ticket-less filler source (never individually mutable)
	// trip the global budget on the 20th. Dominant is still 15 of the
	// resulting 35-event window (43%), past the 25% attribution share.
	var last GateResult
	for i := range 20 {
		now = base.Add(time.Duration(15+i) * time.Second)
		result, err := notificationGateAt(statePath, "telegram", "iteration-stalled", "cli", now, true, nil)
		if err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
		last = result
	}

	if last.Decision != GloballyMuted || !last.EdgeTriggered {
		t.Fatalf("last result = %+v, want GloballyMuted/edge", last)
	}

	dominantTicket := readGateTicket(t, dominant)
	if len(dominantTicket.Mutes) != 1 {
		t.Fatalf("dominant ticket.Mutes = %+v, want one mute (attributed)", dominantTicket.Mutes)
	}
}

func TestNotificationGate_GlobalTrip_Diffuse_NoIndividualMute(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "notifications-state.json")
	base := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	sources := make([]string, 20)
	for i := range sources {
		sources[i] = writeGateTicket(t, dir, fmt.Sprintf("%02d", 10+i))
	}

	var last GateResult
	for i, source := range sources {
		now := base.Add(time.Duration(i) * time.Second)
		result, err := notificationGateAt(statePath, "telegram", "iteration-stalled", source, now, true, nil)
		if err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
		last = result
	}

	if last.Decision != GloballyMuted || !last.EdgeTriggered {
		t.Fatalf("last result = %+v, want GloballyMuted/edge", last)
	}

	for _, source := range sources {
		ticket := readGateTicket(t, source)
		if len(ticket.Mutes) != 0 {
			t.Fatalf("ticket %s Mutes = %+v, want none (diffuse trip)", source, ticket.Mutes)
		}
	}
}

func TestNotificationGate_SecondAttemptAgainstMutedSource_SuppressedNoSecondEdge(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "notifications-state.json")
	ticketPath := writeGateTicket(t, dir, "01")
	base := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	for i := range 5 {
		now := base.Add(time.Duration(i) * 10 * time.Second)
		if _, err := notificationGateAt(statePath, "telegram", "iteration-stalled", ticketPath, now, false, nil); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}

	result, err := notificationGateAt(statePath, "telegram", "iteration-stalled", ticketPath, base.Add(50*time.Second), false, nil)
	if err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if result.Decision != PerSourceMuted || result.EdgeTriggered {
		t.Fatalf("result = %+v, want PerSourceMuted/no edge", result)
	}
}

func TestNotificationGate_TransportAlreadyGloballyMuted_EveryAttemptSuppressed(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "notifications-state.json")
	ticketPath := writeGateTicket(t, dir, "01")
	base := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	if err := updateNotificationStateAt(statePath, func(state *NotificationState) {
		state.Transports["telegram"] = TransportState{Muted: true, Reason: "auto-trip"}
	}); err != nil {
		t.Fatalf("seed muted state: %v", err)
	}

	result, err := notificationGateAt(statePath, "telegram", "iteration-stalled", ticketPath, base, true, nil)
	if err != nil {
		t.Fatalf("notificationGateAt: %v", err)
	}
	if result.Decision != GloballyMuted || result.EdgeTriggered {
		t.Fatalf("result = %+v, want GloballyMuted/no edge", result)
	}
}

func TestNotificationGate_TicketlessSource_CountedNeverMuted(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "notifications-state.json")
	base := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	var last GateResult
	for i := range 10 {
		now := base.Add(time.Duration(i) * 5 * time.Second)
		result, err := notificationGateAt(statePath, "telegram", "iteration-stalled", "cli", now, false, nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		last = result
	}

	if last.Decision != Allowed || last.EdgeTriggered {
		t.Fatalf("result = %+v, want Allowed/no edge for a ticket-less source", last)
	}
}

func TestNotificationGate_NoParkTicketCallback_MutesWrittenParkSkipped(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "notifications-state.json")
	ticketPath := writeGateTicket(t, dir, "01")
	base := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	var last GateResult
	for i := range 5 {
		now := base.Add(time.Duration(i) * 10 * time.Second)
		result, err := notificationGateAt(statePath, "telegram", "iteration-stalled", ticketPath, now, false, nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		last = result
	}

	if last.Decision != PerSourceMuted || !last.EdgeTriggered {
		t.Fatalf("result = %+v, want PerSourceMuted/edge", last)
	}
	ticket := readGateTicket(t, ticketPath)
	if len(ticket.Mutes) != 1 {
		t.Fatalf("ticket.Mutes = %+v, want one mute written even without parkTicket", ticket.Mutes)
	}
}

func TestNotificationGate_EveryTrip_RecordedInHistoryRegardlessOfMuteOutcome(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "notifications-state.json")
	base := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	sources := make([]string, 20)
	for i := range sources {
		sources[i] = writeGateTicket(t, dir, fmt.Sprintf("%02d", 10+i))
	}
	for i, source := range sources {
		now := base.Add(time.Duration(i) * time.Second)
		if _, err := notificationGateAt(statePath, "telegram", "iteration-stalled", source, now, true, nil); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	state, err := loadNotificationStateAt(statePath)
	if err != nil {
		t.Fatalf("loadNotificationStateAt: %v", err)
	}
	trips := state.Transports["telegram"].Trips
	if len(trips) != 1 {
		t.Fatalf("Trips = %+v, want exactly one trip", trips)
	}
	if len(trips[0].Sources) != len(sources) {
		t.Fatalf("trip Sources = %+v, want breakdown for all %d sources", trips[0].Sources, len(sources))
	}
}
