package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestAgentsAutoAdvance_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	created, err := h.Client.Sessions.Create(context.Background(), wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	var out bytes.Buffer
	cmdtest.RequireRefusalExit(t, runAgentsAutoAdvance(context.Background(), h.Client, &out, true, created.Session.ID, false))
	var res wire.SessionResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if !res.Refused || res.Reason != wire.ReasonSessionNotDesign {
		t.Errorf("auto-advance off = %+v, want %s", res, wire.ReasonSessionNotDesign)
	}
}

func TestAgentsAutoQueue_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	created, err := h.Client.Sessions.Create(context.Background(), wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	var out bytes.Buffer
	cmdtest.RequireRefusalExit(t, runAgentsAutoQueue(context.Background(), h.Client, &out, true, created.Session.ID, true))
	var res wire.SessionResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if !res.Refused || res.Reason != wire.ReasonSessionNotDesign {
		t.Errorf("auto-queue on = %+v, want %s", res, wire.ReasonSessionNotDesign)
	}
}

func TestAgentsAutoSwitchVerbs_AcceptARealIDAndOnOff(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	created, err := h.Client.Sessions.Create(context.Background(), wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	d := Deps{Env: cmdbase.Env{NewClient: func() (*apiclient.Client, error) { return h.Client, nil }}}
	for verb, newCmd := range map[string]func(Deps) *cobra.Command{"auto-advance": newAgentsAutoAdvanceCmd, "auto-queue": newAgentsAutoQueueCmd} {
		for _, sw := range []string{"on", "off"} {
			t.Run(verb+" "+sw, func(t *testing.T) {
				var out bytes.Buffer
				c := newCmd(d)
				c.SilenceUsage = true
				c.SetArgs([]string{created.Session.ID, sw, "--json"})
				c.SetOut(&out)
				c.SetErr(&bytes.Buffer{})
				cmdtest.RequireRefusalExit(t, c.Execute())
				var res wire.SessionResult
				if err := json.Unmarshal(out.Bytes(), &res); err != nil || !res.Refused || res.Reason != wire.ReasonSessionNotDesign {
					t.Errorf("output %q: %+v, %v; want refusal %s", out.String(), res, err, wire.ReasonSessionNotDesign)
				}
			})
		}
		c := newCmd(d)
		c.SetArgs([]string{created.Session.ID, "maybe"})
		c.SetOut(&bytes.Buffer{})
		c.SetErr(&bytes.Buffer{})
		if err := c.Execute(); err == nil || !strings.Contains(err.Error(), "maybe") {
			t.Errorf("%s with %q: err = %v, want an invalid-argument error", verb, "maybe", err)
		}
	}
}

func TestAgentsAutoQueueCancel_PrintsTheSessionResult(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	created, err := h.Client.Sessions.Create(context.Background(), wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	var out bytes.Buffer
	if err := runAgentsSessionWrite(context.Background(), h.Client, &out, true, (*apiclient.Sessions).CancelAutoQueue, created.Session.ID); err != nil {
		t.Fatal(err)
	}
	var res wire.SessionResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if res.Refused || res.Session == nil || res.Session.ID != created.Session.ID {
		t.Errorf("auto-queue cancel = %+v, want the session %s", res, created.Session.ID)
	}
}
