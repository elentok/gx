package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

const askPayload = `{"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion","tool_use_id":"toolu_1","tool_input":{"questions":[{"question":"Which?","header":"Pick","multiSelect":false,"options":[{"label":"A","description":"first"},{"label":"B","description":"second"}]}]}}`

// runAskHook runs the hook command on stdin against client.
func runAskHook(t *testing.T, client func() (*apiclient.Client, error), stdin string) (stdout string, err error) {
	t.Helper()
	t.Setenv(holder.AgentSessionEnv, "s1")
	c := newAgentAskQuestionCmd(Deps{Env: cmdbase.Env{Env: cmdtest.EnvFor(t), NewClient: client}})
	var out bytes.Buffer
	c.SetIn(strings.NewReader(stdin))
	c.SetOut(&out)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = c.ExecuteContext(ctx)
	return out.String(), err
}

func TestAskQuestionHook_FailuresPrintNothingAndExitZero(t *testing.T) {
	h := servertest.Start(t)
	down := func() (*apiclient.Client, error) { return nil, errors.New("server down") }
	live := func() (*apiclient.Client, error) { return h.Client, nil }
	for _, tc := range []struct {
		name   string
		client func() (*apiclient.Client, error)
		stdin  string
	}{
		{"server down", down, askPayload},
		{"unreadable payload", live, "not json"},
		{"no questions", live, `{"tool_use_id":"t","tool_input":{"questions":[]}}`},
		{"no tool_use_id", live, `{"tool_input":{"questions":[{"question":"q"}]}}`},
	} {
		out, err := runAskHook(t, tc.client, tc.stdin)
		if err != nil || out != "" {
			t.Errorf("%s: stdout %q, err %v, want empty and nil", tc.name, out, err)
		}
	}
}

func TestAskQuestionHook_PrintsTheServersDecision(t *testing.T) {
	h := servertest.Start(t)
	live := func() (*apiclient.Client, error) { return h.Client, nil }
	for _, tc := range []struct {
		name string
		res  wire.AskHookResult
		want string
	}{
		{"allow", wire.AskHookResult{Decision: wire.AskHookAllow, UpdatedInput: json.RawMessage(`{"answers":{"Which?":"A"}}`)},
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"answers":{"Which?":"A"}}}}`},
		{"deny", wire.AskHookResult{Decision: wire.AskHookDeny, Reason: "use gx tickets ask"},
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"use gx tickets ask"}}`},
		{"release", wire.AskHookResult{Decision: wire.AskHookRelease}, ""},
	} {
		done := make(chan struct{})
		var held []wire.AskHookRequest
		go func() {
			defer close(done)
			deadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(deadline) {
				if held = h.Server.Sessions().HeldAsks("s1"); len(held) > 0 {
					h.Server.Sessions().ResolveAsk("s1", held[0].ToolUseID, tc.res)
					return
				}
				<-time.After(5 * time.Millisecond)
			}
		}()
		out, err := runAskHook(t, live, askPayload)
		<-done
		if err != nil || strings.TrimSpace(out) != tc.want {
			t.Errorf("%s: stdout %q, err %v, want %q", tc.name, out, err, tc.want)
		}
		if len(held) != 1 || held[0].ToolUseID != "toolu_1" || len(held[0].Questions) != 1 ||
			held[0].Questions[0].Header != "Pick" || len(held[0].Questions[0].Options) != 2 || held[0].Questions[0].Options[1].Description != "second" {
			t.Errorf("%s: registered %+v", tc.name, held)
		}
	}
}
