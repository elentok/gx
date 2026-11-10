package sessions_test

import (
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// A session whose holder died with the server ends Exited crashed, but the
// server dying is no news about it: its saved Unread stays and no banner fires.
func TestDeadSession_StartKeepsSavedUnread(t *testing.T) {
	t.Parallel()
	for name, wantUnread := range map[string]bool{"read stays read": false, "unread stays unread": true} {
		t.Run(name, func(t *testing.T) {
			h, ctx, evs, id, conv := startBannerSession(t)
			reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
			if !wantUnread {
				markRead(ctx, t, h, id)
			}
			before := len(h.Banners.Sent())
			killWhileDown(t, h, id)

			h.Restart(t)
			got := waitForSnapshot(ctx, t, h, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
			if got.Exit != wire.SessionExitCrashed || got.Unread.Unread != wantUnread {
				t.Errorf("session = %+v, want exited crashed, unread %v", got, wantUnread)
			}
			if after := len(h.Banners.Sent()); after != before {
				t.Errorf("banners went from %d to %d, want none from the start", before, after)
			}
		})
	}
}
