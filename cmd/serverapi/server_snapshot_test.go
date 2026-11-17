package serverapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/events"
)

func TestServerEventsKinds_TextListsEveryKind(t *testing.T) {
	var out bytes.Buffer
	if err := runServerEventsKinds(false, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != len(events.Kinds()) {
		t.Fatalf("got %d lines, want %d", len(lines), len(events.Kinds()))
	}
	for _, k := range events.Kinds() {
		if !strings.Contains(out.String(), string(k)) {
			t.Errorf("kind %q missing from output", k)
		}
	}
}

func TestServerEventsKinds_ViaRootCommand(t *testing.T) {
	var out bytes.Buffer
	if err := execute(t, []string{"server", "events", "kinds", "--json"}, Deps{Env: cmdbase.Env{Stdout: &out}}); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) {
		t.Errorf("not JSON: %s", out.String())
	}
}
