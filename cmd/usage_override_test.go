package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/spf13/cobra"
)

func TestRunUsageOverride_RefusesWhenNothingGates(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	dial := func() (*apiclient.Client, error) { return h.Client, nil }
	c := &cobra.Command{}
	c.SetContext(context.Background())
	var out bytes.Buffer
	c.SetOut(&out)

	err := runUsageOverride(dial, c, false)
	if err == nil || !strings.Contains(err.Error(), server.ReasonNotGated) {
		t.Fatalf("err = %v, want a %s refusal", err, server.ReasonNotGated)
	}

	out.Reset()
	if err := runUsageOverride(dial, c, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"reason": "`+server.ReasonNotGated+`"`) && !strings.Contains(out.String(), `"reason":"`+server.ReasonNotGated+`"`) {
		t.Errorf("json output %q lacks the refusal reason", out.String())
	}
}
