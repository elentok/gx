package rest

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func breakCfg() config.MentalHealthConfig {
	cfg := config.DefaultMentalHealthConfig()
	cfg.Enabled = true
	cfg.Break = config.MentalHealthBreak{Enabled: true, WorkMinutes: 3, BreakMinutes: 2}
	return cfg
}

func TestRest_VerbsReadActiveAndUnlock(t *testing.T) {
	clock := servertest.NewRestClock(time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC))
	h := servertest.Start(t, servertest.WithRestClock(clock), servertest.WithMentalHealth(breakCfg()))
	ctx := context.Background()
	call := func(jsonOut bool, do func(context.Context, *apiclient.Client, []string) (wire.RestResult, error), args ...string) (string, error) {
		var out bytes.Buffer
		err := run(ctx, h.Client, &out, jsonOut, args, do)
		return out.String(), err
	}
	status := func(ctx context.Context, cl *apiclient.Client, _ []string) (wire.RestResult, error) {
		st, err := cl.Rest.Status(ctx)
		return wire.RestResult{Status: st}, err
	}
	active := func(ctx context.Context, cl *apiclient.Client, _ []string) (wire.RestResult, error) {
		return cl.Rest.Active(ctx)
	}
	unlock := func(ctx context.Context, cl *apiclient.Client, args []string) (wire.RestResult, error) {
		return cl.Rest.Unlock(ctx, strings.Join(args, " "))
	}

	if out, err := call(false, active); err != nil || out != "no rest period; break warning for 10:02; work 1\n" {
		t.Errorf("active = %q, %v", out, err)
	}
	for range 2 {
		clock.Advance(time.Minute)
		if _, err := call(false, active); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := call(false, status); err != nil || !strings.HasPrefix(out, "break until ") {
		t.Errorf("status during the Break = %q, %v", out, err)
	}
	if _, err := call(false, unlock, "hello"); err == nil || !strings.Contains(err.Error(), wire.ReasonRestSentenceMismatch) {
		t.Errorf("wrong sentence error = %v, want the refusal reason", err)
	}
	if out, err := call(true, unlock, "hello"); err == nil || !strings.Contains(out, `"refused": true`) && !strings.Contains(out, `"refused":true`) {
		t.Errorf("--json wrong sentence = %q, %v; want a refusal and a non-zero exit", out, err)
	}
	if out, err := call(false, unlock, "I'm", "Dana", "and", "I'm", "addicted", "to", "AI"); err != nil || out != "no rest period; work 0\n" {
		t.Errorf("unlock = %q, %v", out, err)
	}
}

func TestRest_RefusesWhenNoServerRuns(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "gxr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cl := apiclient.New(dir + "/none.sock")
	var out bytes.Buffer
	err = run(context.Background(), cl, &out, false, nil, func(ctx context.Context, cl *apiclient.Client, _ []string) (wire.RestResult, error) {
		return cl.Rest.Active(ctx)
	})
	if err == nil || !strings.Contains(err.Error(), wire.ReasonServerNotRunning) {
		t.Errorf("error = %v, want the server-not-running refusal", err)
	}
}
