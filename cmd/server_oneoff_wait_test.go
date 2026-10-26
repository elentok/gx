package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// The snapshot carries a paused ticket under its class's wire status.
const (
	wireAnswer = string(schema.StatusLegacyAnswer)
	wireRepair = string(schema.StatusLegacyRepair)
)

// fakeSource serves one ticket whose status advances one step per snapshot.
// failFirst snapshots fail first, standing in for a server restart. recoveries,
// when set, is the ticket's recovery state at each step alongside statuses.
// investigation, when set, adds an investigate child that alone gets events,
// on a stream that then stays open.
type fakeSource struct {
	file          string
	statuses      []string
	recoveries    []string
	failFirst     int
	snaps         int
	investigation bool
	// gone drops the ticket from every snapshot, as after an archive move.
	gone  bool
	moved movedLookup
}

const investigationAddr = "gx:p/1a"

func (f *fakeSource) Snapshot(context.Context) (server.Snapshot, error) {
	f.snaps++
	if f.snaps <= f.failFirst {
		return server.Snapshot{}, errors.New("connection refused")
	}
	i := min(f.snaps-f.failFirst-1, len(f.statuses)-1)
	t := server.TicketInfo{Address: "gx:p/1", Status: f.statuses[i], File: f.file}
	if i < len(f.recoveries) {
		t.Recovery = f.recoveries[i]
	}
	snap := server.Snapshot{Seq: uint64(f.snaps), Tickets: []server.TicketInfo{t}}
	if f.gone {
		snap.Tickets = nil
	}
	if f.investigation {
		snap.Tickets = append(snap.Tickets, server.TicketInfo{Address: investigationAddr, Status: "claimed", Type: "investigate", Parent: "gx:p/1"})
	}
	return snap, nil
}

// Events delivers one event for the ticket, then ends like a dropped stream.
func (f *fakeSource) Events(ctx context.Context, _ uint64) (<-chan server.Event, error) {
	ch := make(chan server.Event, 1)
	if !f.investigation {
		ch <- server.Event{Address: "gx:p/1"}
		close(ch)
		return ch, nil
	}
	ch <- server.Event{Address: investigationAddr}
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func runWait(t *testing.T, f *fakeSource) (stdout string, code int) {
	t.Helper()
	return runWaitTimeout(t, f, 0)
}

func runWaitTimeout(t *testing.T, f *fakeSource, timeout time.Duration) (stdout string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := runOneOffWait(ctx, f, f.moved, &out, &errOut, "gx:p/1", timeout)
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return out.String(), exitErr.Code
	}
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	return out.String(), 0
}

func TestOneOffWait_DonePrintsResultAndExitsZero(t *testing.T) {
	file := filepath.Join(t.TempDir(), "1.md")
	body := "---\nid: 1\ntitle: t\nstatus: done\ntype: prompt\n---\n\n## Result\n\nall good\n\n## Comments\n\nnoise\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := runWait(t, &fakeSource{file: file, statuses: []string{"claimed", "done"}})
	if code != 0 || strings.TrimSpace(out) != "all good" {
		t.Errorf("code = %d, out = %q; want 0 and the Result only", code, out)
	}
}

func TestOneOffWait_ExitCodesByStatus(t *testing.T) {
	for status, want := range map[string]int{wireAnswer: 3, wireRepair: 4, "cancelled": 5} {
		if _, code := runWait(t, &fakeSource{statuses: []string{"open", status}}); code != want {
			t.Errorf("%s: code = %d; want %d", status, code, want)
		}
	}
}

// A manual pause is answer class (gx receives the reply), so --wait stops on it
// with 3 where it used to stop with 4.
func TestWaitExitCode_ManualPauseStopsWithThree(t *testing.T) {
	paused := tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: schema.PauseManual}
	if code, stop := waitExitCode(server.TicketInfo{Status: paused.LegacyStatus()}); !stop || code != 3 {
		t.Errorf("code, stop = %d, %v; want 3, true", code, stop)
	}
}

// A blocked-pane pause travels as the answer status and stops --wait with 3.
func TestWaitExitCode_BlockedPaneStopsWithThree(t *testing.T) {
	if code, stop := waitExitCode(server.TicketInfo{Status: wireAnswer}); !stop || code != 3 {
		t.Errorf("code, stop = %d, %v; want 3, true", code, stop)
	}
}

func TestOneOffWait_WaitsThroughPendingRecovery(t *testing.T) {
	const pending, escalated = server.RecoveryPending, server.RecoveryEscalated
	for name, tc := range map[string]struct {
		statuses, recoveries []string
		want                 int
	}{
		"recovered":    {[]string{"claimed", wireRepair, wireRepair, "claimed", "cancelled"}, []string{"", pending, pending}, 5},
		"escalated":    {[]string{"claimed", wireRepair, wireRepair}, []string{"", pending, escalated}, exitWaitEscalated},
		"hold expired": {[]string{"claimed", wireAnswer, wireAnswer}, []string{"", pending, ""}, 3},
	} {
		if _, code := runWait(t, &fakeSource{statuses: tc.statuses, recoveries: tc.recoveries}); code != tc.want {
			t.Errorf("%s: code = %d; want %d", name, code, tc.want)
		}
	}
}

func TestOneOffWait_WakesOnItsInvestigation(t *testing.T) {
	const pending, escalated = server.RecoveryPending, server.RecoveryEscalated
	for name, tc := range map[string]struct {
		statuses, recoveries []string
		want                 int
	}{
		"escalated": {[]string{wireRepair, wireRepair}, []string{pending, escalated}, exitWaitEscalated},
		"merged":    {[]string{wireRepair, wireRepair, "done"}, []string{pending, pending}, 0},
	} {
		f := &fakeSource{statuses: tc.statuses, recoveries: tc.recoveries, investigation: true}
		if _, code := runWait(t, f); code != tc.want {
			t.Errorf("%s: code = %d; want %d", name, code, tc.want)
		}
	}
}

func TestOneOffWait_TimeoutExitsSix(t *testing.T) {
	f := &fakeSource{statuses: []string{"claimed"}}
	if _, code := runWaitTimeout(t, f, 50*time.Millisecond); code != exitWaitTimeout {
		t.Errorf("code = %d; want %d", code, exitWaitTimeout)
	}
}

func TestOneOffWait_SurvivesServerRestart(t *testing.T) {
	old := waitRetry
	t.Cleanup(func() { waitRetry = old })
	waitRetry = time.Millisecond
	f := &fakeSource{statuses: []string{"claimed", wireRepair}, failFirst: 3}
	if _, code := runWait(t, f); code != 4 {
		t.Errorf("code = %d; want 4 after the outage", code)
	}
}

func TestOneOffWait_PrintsResultAfterTheTicketIsArchived(t *testing.T) {
	old := waitRetry
	t.Cleanup(func() { waitRetry = old })
	waitRetry = time.Millisecond
	file := filepath.Join(t.TempDir(), "01.md")
	body := "---\nid: 1\nstatus: done\ntype: prompt\n---\n\n## Result\n\nall good\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// The first snapshots fail (a restart), then the old address is gone.
	f := &fakeSource{statuses: []string{"claimed"}, failFirst: 2, gone: true,
		moved: func(address string) (server.TicketInfo, bool) {
			if address != "gx:p/1" {
				return server.TicketInfo{}, false
			}
			return server.TicketInfo{Address: "gx:one-offs-archived/1", Status: "done", File: file}, true
		}}
	out, code := runWait(t, f)
	if code != 0 || strings.TrimSpace(out) != "all good" {
		t.Errorf("code = %d, out = %q; want 0 and the Result", code, out)
	}
}
