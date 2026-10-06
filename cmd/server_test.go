package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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
	if !strings.Contains(out.String(), "agent_name_taken\tcause: herdr") {
		t.Errorf("herdr attribute missing:\n%s", out.String())
	}
}

func TestServerEventsKinds_JSONCarriesHerdrCause(t *testing.T) {
	var out bytes.Buffer
	if err := runServerEventsKinds(true, &out); err != nil {
		t.Fatal(err)
	}
	var got []KindInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(events.Kinds()) {
		t.Fatalf("got %d kinds, want %d", len(got), len(events.Kinds()))
	}
	for _, i := range got {
		if want := events.Kind(i.Kind).CauseHerdr(); i.CauseHerdr != want {
			t.Errorf("%s: cause_herdr=%v, want %v", i.Kind, i.CauseHerdr, want)
		}
	}
}

func TestServerEventsKinds_ViaRootCommand(t *testing.T) {
	root := newRootCmd(deps{})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"server", "events", "kinds", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) {
		t.Errorf("not JSON: %s", out.String())
	}
}
