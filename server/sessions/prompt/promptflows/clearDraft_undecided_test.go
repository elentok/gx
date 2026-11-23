package promptflows_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// A read that can't decide, with no resize behind it (the ❯ row carries the
// logo), must raise the untrusted-read notice like a resized one. The old code
// took the "no draft" branch and wiped notices instead.
func TestClearDraft_UndecidedReadWithoutResizeRaisesTheNotice(t *testing.T) {
	t.Parallel()
	f := fromPrompt(rigs.StartPromptSessionAs(t, firstPromptGrace, wire.CreateSessionRequest{Name: "old"}, servertest.WithEnv(agentfake.ProgramDraftEnv+"=█ logo")))
	if _, err := f.h.Client.Sessions.Rename(f.ctx, wire.SessionRenameRequest{ID: f.id, Name: "new name"}); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool {
		return slices.Contains(s.Notices, wire.SessionNoticeUntrustedRead)
	})
	if !servertest.Poll(10*time.Second, func() bool { return strings.Contains(submitted(t, f.conv), "/rename new name") }) {
		t.Errorf("submitted = %q, want the rename typed on", submitted(t, f.conv))
	}
}
