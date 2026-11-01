package claudeui_test

import (
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
)

const launchText = "/gx-implement gx:epic/20"

const launchRecord = "<command-name>/gx-implement</command-name>\n<command-message>gx-implement</command-message>"

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
			err := claudeui.CheckLaunchDelivered(p, launchText, claudeui.DefaultLaunchProofCaps)
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
