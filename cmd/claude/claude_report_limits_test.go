package claude

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/servertest"
	"github.com/spf13/cobra"
)

func runReport(t *testing.T, dial func() (*apiclient.Client, error), payload string, jsonOut bool) (string, error) {
	t.Helper()
	c := &cobra.Command{}
	c.SetContext(context.Background())
	var out bytes.Buffer
	c.SetOut(&out)
	err := runReportLimits(dial, c, []byte(payload), jsonOut)
	return out.String(), err
}

func TestLimitsReportVerb(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	dial := func() (*apiclient.Client, error) { return h.Client, nil }

	out, err := runReport(t, dial, `{"rate_limits":{"five_hour":{"used_percentage":34,"resets_at":4000000000},"seven_day":{"used_percentage":12,"resets_at":4000100000}},"model":"x"}`, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"five_hour"`, `"seven_day"`, `"received_at"`} {
		if !strings.Contains(out, want) {
			t.Errorf("two-window output %q lacks %s", out, want)
		}
	}

	if out, err = runReport(t, dial, `{"model":"x"}`, true); err != nil || out != "" {
		t.Errorf("no rate_limits: out=%q err=%v, want nothing sent and exit 0", out, err)
	}
	if out, err = runReport(t, dial, `{"rate_limits":{}}`, true); err != nil || out != "" {
		t.Errorf("no usable window: out=%q err=%v, want nothing sent and exit 0", out, err)
	}
}

func TestLimitsReportVerb_OneWindow(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	dial := func() (*apiclient.Client, error) { return h.Client, nil }
	out, err := runReport(t, dial, `{"rate_limits":{"seven_day":{"used_percentage":9,"resets_at":4000100000}}}`, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"seven_day"`) || strings.Contains(out, `"five_hour"`) {
		t.Errorf("output %q, want only seven_day", out)
	}
}
