package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestAgentsCreateAndList_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	dir := t.TempDir()

	var out bytes.Buffer
	req := server.CreateSessionRequest{Project: "proj", Dir: dir, Name: "spike", Harness: string(agentfake.HarnessKind)}
	if err := runAgentsCreate(ctx, h.Client, &out, true, req); err != nil {
		t.Fatal(err)
	}
	var created server.SessionResult
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("create output %q: %v", out.String(), err)
	}
	if created.Refused || created.Session == nil || created.Session.Name != "spike" {
		t.Fatalf("create = %+v, want the session", created)
	}

	out.Reset()
	if err := runAgentsList(ctx, h.Client, &out, true); err != nil {
		t.Fatal(err)
	}
	var listed []server.SessionInfo
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("list output %q: %v", out.String(), err)
	}
	if len(listed) != 1 || listed[0].ID != created.Session.ID || listed[0].Dir != dir {
		t.Errorf("list = %s, want session %s", out.String(), created.Session.ID)
	}

	out.Reset()
	if err := runAgentsCreate(ctx, h.Client, &out, true, server.CreateSessionRequest{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	var refused server.SessionResult
	if err := json.Unmarshal(out.Bytes(), &refused); err != nil || !refused.Refused || refused.Reason != server.ReasonSessionBadRequest {
		t.Errorf("create without project = %s, want refused %s", out.String(), server.ReasonSessionBadRequest)
	}
}

func TestAgentsStopAndRemove_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	created, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	id := created.Session.ID

	run := func(write sessionWriteFunc) server.SessionResult {
		t.Helper()
		var out bytes.Buffer
		if err := runAgentsSessionWrite(ctx, h.Client, &out, true, write, id); err != nil {
			t.Fatal(err)
		}
		var res server.SessionResult
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("output %q: %v", out.String(), err)
		}
		return res
	}
	if res := run((*apiclient.Client).RemoveSession); !res.Refused || res.Reason != server.ReasonSessionLive {
		t.Errorf("remove live = %+v, want refused %s", res, server.ReasonSessionLive)
	}
	if res := run((*apiclient.Client).StopSession); res.Refused || res.Session == nil || !res.Session.Stopped {
		t.Errorf("stop = %+v, want the stopped session", res)
	}
	if res := run((*apiclient.Client).RemoveSession); res.Refused {
		t.Errorf("remove = %+v, want removed", res)
	}
}
