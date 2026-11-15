package claudeui_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/testutil/agentfake"
)

const launchText = "/gx-implement gx:epic/20"

const launchRecord = "<command-name>/gx-implement</command-name>\n<command-message>gx-implement</command-message>"

func TestCheckLaunchDelivered_CancelEndsLoopWithContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clock := time.Unix(1000, 0)
	polls := 0
	p := claudeui.LaunchProof{
		FirstUser: func() (string, bool, error) { return "", false, nil },
		Pane:      func() (string, error) { return "> ", nil },
		Resend:    func() error { return nil },
		Now:       func() time.Time { return clock },
		Sleep: func(d time.Duration) {
			clock = clock.Add(d)
			if polls++; polls == 2 {
				cancel()
			}
		},
	}
	err := claudeui.CheckLaunchDelivered(ctx, p, launchText, claudeui.DefaultLaunchProofCaps)
	if !errors.Is(err, context.Canceled) || errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("err = %v, want context.Canceled and not ErrNotDelivered", err)
	}
	if polls != 2 {
		t.Errorf("polls = %d, want the loop to stop on the poll after the cancel (2)", polls)
	}
}

// TestCheckLaunchDelivered drives the proof on a fake clock: each case says,
// per elapsed time, what the transcript and the pane show.
func TestCheckLaunchDelivered(t *testing.T) {
	const unknown = "Unknown command: /gx-implement"
	tests := []struct {
		name string
		// userAt is when the first user record appears (never when negative).
		userAt time.Duration
		// user is that record's content.
		user string
		// unknownAt is when the pane starts showing the unknown line (never when negative).
		unknownAt time.Duration
		wantErr   bool
		// wantResends is the exact resend count; wantMaxElapsed bounds the time spent.
		wantResends    int
		wantMaxElapsed time.Duration
	}{
		{name: "delivered at once", userAt: 0, user: launchRecord, unknownAt: -1},
		{name: "typed text counts as the command", userAt: 0, user: launchText, unknownAt: -1},
		{
			name: "unknown command fails in seconds, not after the grace", userAt: -1, unknownAt: 0,
			wantErr: true, wantResends: 1, wantMaxElapsed: 6 * time.Second,
		},
		{
			name: "registry fills after 10s", userAt: 10 * time.Second, user: launchRecord, unknownAt: -1,
			wantResends: 3,
		},
		{
			name: "no user record and no unknown line resends until the grace ends", userAt: -1, unknownAt: -1,
			wantErr: true, wantResends: 9, wantMaxElapsed: 31 * time.Second,
		},
		{
			name: "a different first user record is not the proof", userAt: 0, user: "hello", unknownAt: -1,
			wantErr: true, wantResends: 9, wantMaxElapsed: 31 * time.Second,
		},
		{
			// The command ran while the unknown line was still on the pane.
			name: "transcript showing the command never gets a resend", userAt: 1 * time.Second, user: launchRecord, unknownAt: 0,
			wantResends: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := time.Unix(1000, 0)
			start := clock
			resends := 0
			var reported []claudeui.NudgeDecision
			since := func() time.Duration { return clock.Sub(start) }
			p := claudeui.LaunchProof{
				FirstUser: func() (string, bool, error) {
					return tt.user, tt.userAt >= 0 && since() >= tt.userAt, nil
				},
				Pane: func() (string, error) {
					if tt.unknownAt >= 0 && since() >= tt.unknownAt {
						return unknown, nil
					}
					return "> ", nil
				},
				Resend: func() error { resends++; return nil },
				Report: func(d claudeui.NudgeDecision) { reported = append(reported, d) },
				Now:    func() time.Time { return clock },
				Sleep:  func(d time.Duration) { clock = clock.Add(d) },
			}
			err := claudeui.CheckLaunchDelivered(context.Background(), p, launchText, claudeui.DefaultLaunchProofCaps)
			if tt.wantErr != errors.Is(err, agentrunner.ErrNotDelivered) {
				t.Fatalf("err = %v, want ErrNotDelivered=%v", err, tt.wantErr)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("err = %v", err)
			}
			if resends != tt.wantResends {
				t.Errorf("resends = %d, want %d", resends, tt.wantResends)
			}
			if len(reported) != resends {
				t.Errorf("reported %v, want one prompt-nudged per resend", reported)
			}
			for _, d := range reported {
				if d != "prompt-nudged" {
					t.Errorf("decision = %q, want prompt-nudged", d)
				}
			}
			if tt.wantMaxElapsed > 0 && since() > tt.wantMaxElapsed {
				t.Errorf("took %s, want at most %s", since(), tt.wantMaxElapsed)
			}
		})
	}
}

// proofFor is a LaunchProof on a fake clock that checks for pasted records.
func proofFor(clock *time.Time, resends *int, first func() (string, bool, error)) claudeui.LaunchProof {
	return claudeui.LaunchProof{
		FirstUser:   first,
		CheckPasted: true,
		Pane:        func() (string, error) { return "> ", nil },
		Resend:      func() error { *resends++; return nil },
		Now:         func() time.Time { return *clock },
		Sleep:       func(d time.Duration) { *clock = clock.Add(d) },
	}
}

func TestCheckLaunchDelivered_WorkingAgentWithoutRecordGetsNoResendAndFailsAtGrace(t *testing.T) {
	start := time.Unix(1000, 0)
	clock := start
	resends := 0
	p := proofFor(&clock, &resends, func() (string, bool, error) { return "", false, nil })
	p.Working = func() (bool, error) { return true, nil }
	var reported []claudeui.NudgeDecision
	p.Report = func(d claudeui.NudgeDecision) { reported = append(reported, d) }

	err := claudeui.CheckLaunchDelivered(context.Background(), p, launchText, claudeui.DefaultLaunchProofCaps)

	if !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("err = %v, want ErrNotDelivered at the grace end", err)
	}
	if resends != 0 || len(reported) != 0 {
		t.Errorf("resends = %d, reported = %v; want none while working", resends, reported)
	}
	if got := clock.Sub(start); got < claudeui.DefaultLaunchProofCaps.Grace {
		t.Errorf("ended after %s, want the grace clock to keep running to %s", got, claudeui.DefaultLaunchProofCaps.Grace)
	}
}

// TestCheckLaunchDelivered_NoResendIntoAWorkingAgentForEveryCaller gives each
// launch path its own Working source, shaped as that caller reads its runner,
// and expects no resend while the agent works and no record exists.
func TestCheckLaunchDelivered_NoResendIntoAWorkingAgentForEveryCaller(t *testing.T) {
	tests := []struct {
		name        string
		checkPasted bool
		working     func() (bool, error)
	}{
		{"herdr runner", true, func() (bool, error) { return true, nil }},
		{"PTY Claude", true, func() (bool, error) { return true, nil }},
		{"PTY Codex", false, func() (bool, error) { return true, nil }},
		{"first prompt", true, func() (bool, error) { return true, nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := time.Unix(1000, 0)
			resends := 0
			p := proofFor(&clock, &resends, func() (string, bool, error) { return "", false, nil })
			p.CheckPasted = tt.checkPasted
			p.Working = tt.working

			err := claudeui.CheckLaunchDelivered(context.Background(), p, launchText, claudeui.DefaultLaunchProofCaps)

			if !errors.Is(err, agentrunner.ErrNotDelivered) {
				t.Fatalf("err = %v, want ErrNotDelivered at the grace end", err)
			}
			if resends != 0 {
				t.Errorf("resends = %d, want none into a working agent", resends)
			}
		})
	}
}

func TestCheckLaunchDelivered_PastedRecordIsItsOwnNotDeliveredError(t *testing.T) {
	clock := time.Unix(1000, 0)
	resends := 0
	pasted := "<pasted_content>\n" + launchText + "\n</pasted_content>"
	p := proofFor(&clock, &resends, func() (string, bool, error) { return pasted, true, nil })

	err := claudeui.CheckLaunchDelivered(context.Background(), p, launchText, claudeui.DefaultLaunchProofCaps)

	if !errors.Is(err, claudeui.ErrPastedLaunch) || !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("err = %v, want ErrPastedLaunch wrapping ErrNotDelivered", err)
	}
	if _, hung := errors.AsType[*claudeui.HungPaneError](err); hung {
		t.Errorf("err = %v, must not be a hung pane", err)
	}
	if !strings.Contains(err.Error(), "the launch command was pasted as plain text and the skill did not run") {
		t.Errorf("err = %q, want the pasted wording", err)
	}
	p.CheckPasted = false
	if err := claudeui.CheckLaunchDelivered(context.Background(), p, launchText, claudeui.DefaultLaunchProofCaps); errors.Is(err, claudeui.ErrPastedLaunch) {
		t.Errorf("err = %v, want no pasted check when CheckPasted is off", err)
	}
}

func TestNewRecordSource_ResumedTranscriptIgnoresItsEarlierLaunchRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := agentfake.AppendMessage(path, "user", launchText); err != nil {
		t.Fatal(err)
	}
	source := claudeui.NewRecordSource(func() (string, bool, error) { return path, true, nil })

	if content, ok, err := source(); err != nil || ok {
		t.Fatalf("before the new send: %q %v %v; want the old record not to count", content, ok, err)
	}
	if err := agentfake.AppendMessage(path, "user", launchText); err != nil {
		t.Fatal(err)
	}
	content, ok, err := source()
	if err != nil || !ok || !claudeui.LaunchedBy(content, launchText) {
		t.Fatalf("after the new send: %q %v %v; want the new record", content, ok, err)
	}
}

func TestNewRecordSource_FreshSessionCountsZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	exists := false
	source := claudeui.NewRecordSource(func() (string, bool, error) { return path, exists, nil })

	if _, ok, _ := source(); ok {
		t.Fatal("record before the transcript exists")
	}
	exists = true
	if err := agentfake.AppendMessage(path, "user", launchText); err != nil {
		t.Fatal(err)
	}
	if content, ok, err := source(); err != nil || !ok || content != launchText {
		t.Fatalf("got %q %v %v; want the first record", content, ok, err)
	}
}
