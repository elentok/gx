package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestUsageSummary_JSONVerb(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	d := deps{serverClient: func() (*apiclient.Client, error) { return h.Client, nil }}
	c := newUsageCmd(d)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"summary", "--json"})
	c.SetContext(context.Background())
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var u server.UsageSummary
	if err := json.Unmarshal(out.Bytes(), &u); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if u.Mode != server.UsageModeSpending {
		t.Errorf("mode = %q, want spending", u.Mode)
	}
}
