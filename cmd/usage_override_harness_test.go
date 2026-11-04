package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func TestUsageOverrideCmd_NamesTheHarnessAndRefusesUnknownOnes(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	run := func(args ...string) error {
		c := newUsageCmd(deps{serverClient: func() (*apiclient.Client, error) { return h.Client, nil }})
		c.SetContext(context.Background())
		c.SetArgs(append([]string{"override"}, args...))
		c.SetOut(&bytes.Buffer{})
		c.SetErr(&bytes.Buffer{})
		return c.Execute()
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"codex"}, "no Codex usage window"},
		{[]string{"claude"}, "no Claude usage window"},
		{nil, "no Claude usage window"}, // the Default harness
	} {
		err := run(tc.args...)
		if err == nil || !strings.Contains(err.Error(), wire.ReasonNotGated) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("override %v = %v, want a %s refusal containing %q", tc.args, err, wire.ReasonNotGated, tc.want)
		}
	}
	if err := run("gemini"); err == nil || strings.Contains(err.Error(), wire.ReasonNotGated) {
		t.Errorf("override gemini = %v, want an invalid-argument error", err)
	}
}
