package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/recovery"
)

func TestRecoveryCatalog_JSONPrintsCatalog(t *testing.T) {
	cat := recovery.Catalog{Enabled: true, Entries: []recovery.Entry{{
		ID: "T1", Type: events.LaunchFailed, Kind: events.AgentNameTaken,
		Executor: recovery.ExecutorRule, Authority: recovery.AuthorityLow, Verbs: []string{"retry"}, Enabled: true,
	}}}
	var out bytes.Buffer
	if err := runRecoveryCatalog(cat, true, &out); err != nil {
		t.Fatal(err)
	}
	var got CatalogInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || len(got.Entries) != 1 || got.Entries[0].ID != "T1" || got.Entries[0].Authority != recovery.AuthorityLow {
		t.Fatalf("unexpected payload: %s", out.String())
	}
	if _, ok := got.NotCatalogued["R13"]; !ok {
		t.Error("R13 missing from not_catalogued")
	}
}

func TestRecoveryCatalog_EmptyEntriesIsArray(t *testing.T) {
	var out bytes.Buffer
	if err := runRecoveryCatalog(recovery.Catalog{Enabled: true}, true, &out); err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["entries"]) != "[]" {
		t.Fatalf("entries = %s, want []", raw["entries"])
	}
}

func TestRecoveryCatalog_ViaRootCommand(t *testing.T) {
	root := newRootCmd(deps{})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"recovery", "catalog", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("not JSON: %s", out.String())
	}
}
