package ralphloop

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// chatMembershipVerdicts is the enforcement mechanism this contract test
// checks, not merely test fixture data: every EventSink method must have an
// explicit yes/no verdict here, spelled out below rather than only listing
// the chat members, or a method added later with no verdict at all would
// pass silently — which is exactly how the iteration-started event fell
// through before this ticket.
//
// Chat members carry changes to what gx is running, what it's blocked on,
// and when it ends, at ticket granularity or coarser (see chatEventSink).
// Everything else — events inside one iteration, and startup reconciliation
// housekeeping — is TUI-only.
var chatMembershipVerdicts = map[string]bool{
	"EpicStarted":       true,
	"IterationStarted":  true,
	"IterationPaused":   true,
	"IterationResumed":  true,
	"IterationFinished": true,
	"TicketNeedsHuman":  true,
	"EpicParked":        true,
	"EpicComplete":      true,
	"DrainComplete":     true,
	// EpicFailed is the one documented exception: it is a chat member, but
	// no EventSink implementation dispatches it. It fires from the loop
	// registry's EpicFailureReporter (epic_failure_reporter.go), which
	// stays live across the run's sink close and drain and emits straight
	// to the transport afterward — the run itself has already returned by
	// the time the failure is recorded, so there is no run-scoped emitter
	// left to call this through the normal event stream.
	"EpicFailed": true,

	"TicketReverted":            false,
	"TicketReattached":          false,
	"TicketClaimed":             false,
	"TranscriptLine":            false,
	"ContextOccupancy":          false,
	"CherryPickStarted":         false,
	"ConflictResolutionStarted": false,
	"SmartZoneCompactStarted":   false,
	"SmartZoneFinishingUp":      false,
	"SmartZoneRecovered":        false,
	"TicketCleanupFinished":     false,
	"TicketRecovering":          false,
	"TicketRecovered":           false,
	"TicketUnrecoverable":       false,
	// NotificationFailed is TUI-only by construction, not by omission: it
	// only ever fires from chatEventSink calling it on its own embedded
	// EventSink when a chat send itself just failed, so treating it as a
	// chat member would risk looping a broken send back through chat.
	"NotificationFailed": false,
}

// missingVerdicts reports every method on iface that verdicts has no entry
// for, by reflection — the mechanism that fails a build the moment an
// EventSink method is added without an explicit chat verdict.
func missingVerdicts(iface reflect.Type, verdicts map[string]bool) []string {
	var missing []string
	for i := 0; i < iface.NumMethod(); i++ {
		name := iface.Method(i).Name
		if _, ok := verdicts[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func TestChatMembershipVerdicts_CoversEveryEventSinkMethod(t *testing.T) {
	t.Parallel()
	iface := reflect.TypeFor[EventSink]()

	if missing := missingVerdicts(iface, chatMembershipVerdicts); len(missing) > 0 {
		t.Errorf("chatMembershipVerdicts missing a verdict for: %v", missing)
	}

	// Catches the opposite drift too: a stale entry for a method the
	// interface no longer has, which would otherwise silently stop meaning
	// anything.
	if got, want := len(chatMembershipVerdicts), iface.NumMethod(); got != want {
		t.Errorf("chatMembershipVerdicts has %d entries, EventSink has %d methods", got, want)
	}
}

// TestChatMembershipVerdicts_FailsOnUnmappedMethod demonstrates the failure
// mode the contract test above exists to catch: a fixture interface with one
// method the verdict map was never updated for.
func TestChatMembershipVerdicts_FailsOnUnmappedMethod(t *testing.T) {
	t.Parallel()
	type fixtureWithUnmappedMethod interface {
		EventSink
		BrandNewLiveEvent(identifier string)
	}
	iface := reflect.TypeFor[fixtureWithUnmappedMethod]()

	missing := missingVerdicts(iface, chatMembershipVerdicts)
	if len(missing) != 1 || missing[0] != "BrandNewLiveEvent" {
		t.Fatalf("missing = %v, want exactly [BrandNewLiveEvent]", missing)
	}
}

// TestEpicComplete_ChatSinkIsNotificationOnly pins EventSink.EpicComplete's
// notification-only invariant (see its doc comment): Run's drain-exit path
// skips this call wholesale for a drained run rather than only suppressing
// its chat/toast half, which is only safe as long as no implementer persists
// state here. chatEventSink is the one implementer with enough behavior
// (reading scratchDir to render its message) to plausibly grow a write; this
// snapshots every ticket file's content and mtime around a call and fails if
// either changed — the shape any future non-notification state transition in
// this codebase would take (see StampEpicCompleted/MarkNeedsRepair, the
// existing examples of ticket state persisted to disk).
func TestEpicComplete_ChatSinkIsNotificationOnly(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: task\n---\n# A\n",
	})
	issuesDir := filepath.Join(scratchDir, "epic", "issues")

	before := snapshotDir(t, issuesDir)
	sink, _ := newFakeChatSink(t, &recordingSink{})
	sink.scratchDir = scratchDir
	sink.EpicComplete("epic", 1, 5)
	sink.Close()
	after := snapshotDir(t, issuesDir)

	if !reflect.DeepEqual(before, after) {
		t.Fatalf("issuesDir changed by EpicComplete: before=%#v after=%#v", before, after)
	}
}

// snapshotDir maps every regular file directly under dir to its content and
// mtime, so a caller can assert a call left dir untouched.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	snapshot := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("Info(%s): %v", entry.Name(), err)
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", entry.Name(), err)
		}
		snapshot[entry.Name()] = info.ModTime().String() + "\n" + string(content)
	}
	return snapshot
}
