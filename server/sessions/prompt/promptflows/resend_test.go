package promptflows_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

func (f promptFixture) seedCard(t *testing.T, a wire.SessionActionable) {
	t.Helper()
	meta := readMeta(t, f.h, f.id)
	meta["actionable"] = a
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.h.SessionsDir, f.id, "meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == a.Reason })
}

func (f promptFixture) resend(t *testing.T) (int, string) {
	t.Helper()
	return servertest.WebPostJSON(t, f.base+"/v1/sessions/run-action", fmt.Sprintf(`{"id":%q}`, f.id), f.ck, "", servertest.JSONPost(f.base))
}

func TestResend(t *testing.T) {
	t.Parallel()
	t.Run("unknown command card is refused while exited and resent after resume", func(t *testing.T) {
		t.Parallel()
		f := startPromptSession(t)
		f.seedCard(t, wire.SessionActionable{Kind: wire.DeliveryUnknownCommand, Reason: "unknown", Action: []string{"/nope"}, CopyText: "/nope", GxRaised: true})
		if res, err := f.h.Client.Sessions.Stop(f.ctx, f.id); err != nil || res.Refused {
			t.Fatalf("stop = %+v, %v", res, err)
		}
		waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })

		_, body := f.resend(t)
		if !strings.Contains(body, wire.ReasonSessionNotLive) || !strings.Contains(body, "restart the agent") {
			t.Errorf("resend while exited = %q, want the not-live refusal that says to restart", body)
		}
		if got := readMeta(t, f.h, f.id)["actionable"]; got == nil {
			t.Errorf("card gone after a refused resend")
		}

		if res, err := f.h.Client.Sessions.Resume(f.ctx, f.id); err != nil || res.Refused {
			t.Fatalf("resume = %+v, %v", res, err)
		}
		waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
		report(t, f.conv, agentfake.Control{State: "idle"})
		waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })

		if code, body := f.resend(t); code/100 != 2 {
			t.Fatalf("resend = %d %s", code, body)
		}
		waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == "" })
		waitForSubmitted(t, f.conv, "\"/nope\"\n")
	})
	for _, kind := range []string{wire.DeliverySendFailed, wire.DeliveryMaybeSent} {
		t.Run("web refuses a "+kind+" card", func(t *testing.T) {
			t.Parallel()
			f := startPromptSession(t)
			f.seedCard(t, wire.SessionActionable{Kind: kind, Reason: "card", Action: []string{"hi"}, CopyText: "hi", GxRaised: true})
			_, body := f.resend(t)
			if !strings.Contains(body, wire.ReasonSessionNoAction) {
				t.Errorf("resend = %q, want the no-action refusal", body)
			}
			if submitted(t, f.conv) != "" {
				t.Errorf("submitted = %q, want nothing sent", submitted(t, f.conv))
			}
		})
	}
}
