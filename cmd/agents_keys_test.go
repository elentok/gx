package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestAgentsKeys_RefusalIsAnError(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir())
	var out bytes.Buffer
	err := runAgentsKeys(context.Background(), h.Client, &out, false, server.SessionKeysRequest{ID: "nope", Key: "Esc"})
	if err == nil || !strings.Contains(err.Error(), server.ReasonSessionUnknown) {
		t.Errorf("err = %v, want it to name %s", err, server.ReasonSessionUnknown)
	}
}

func TestAgentsKeys_JSONCarriesTheRefusal(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir())
	var out bytes.Buffer
	_ = runAgentsKeys(context.Background(), h.Client, &out, true, server.SessionKeysRequest{ID: "nope", Text: "hi"})
	var got server.SessionResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || !got.Refused || got.Reason != server.ReasonSessionUnknown {
		t.Errorf("output %q: %+v, %v; want refusal %s", out.String(), got, err, server.ReasonSessionUnknown)
	}
}

func TestAgentsKeys_NeedsExactlyOneInput(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][]string{
		"none": {"s1"},
		"both": {"s1", "--key", "Esc", "--text", "x"},
	} {
		t.Run(name, func(t *testing.T) {
			c := newAgentsKeysCmd(deps{})
			c.SetArgs(args)
			c.SetOut(&bytes.Buffer{})
			c.SetErr(&bytes.Buffer{})
			if err := c.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one") {
				t.Errorf("err = %v, want exactly-one error", err)
			}
		})
	}
}
