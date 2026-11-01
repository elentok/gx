package migrations

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const snapshotPath = "testdata/schema.snapshot"

var updateSnapshot = flag.Bool("update-schema-snapshot", false, "rewrite "+snapshotPath)

// schemaSnapshot renders the parts of the ticket schema a migration step may
// need to react to: struct fields, closed enums and the section names the
// loaders look for.
func schemaSnapshot() string {
	var b strings.Builder
	for _, v := range []any{schema.Ticket{}, tickets.Epic{}, tickets.EpicFields{}} {
		t := reflect.TypeOf(v)
		fmt.Fprintf(&b, "struct %s.%s\n", t.PkgPath()[strings.LastIndex(t.PkgPath(), "/")+1:], t.Name())
		for i := range t.NumField() {
			f := t.Field(i)
			fmt.Fprintf(&b, "  %s %s %s\n", f.Name, f.Type, f.Tag)
		}
	}
	enums := schema.Enums()
	names := make([]string, 0, len(enums))
	for name := range enums {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Fprintf(&b, "enum %s: %s\n", name, strings.Join(enums[name], " "))
	}
	fmt.Fprintf(&b, "epic server-stamped: %s\n", strings.Join(tickets.EpicServerStamped, " "))
	fmt.Fprintf(&b, "sections: %s | %s | %s\n", schema.PausedHeading, tickets.LegacyAnswerHeading, tickets.LegacyRepairHeading)
	return b.String()
}

// checkSnapshot returns an error naming the next action when got differs from
// the stored snapshot.
func checkSnapshot(stored, got string) error {
	if stored == got {
		return nil
	}
	return fmt.Errorf("the ticket schema changed but the snapshot did not.\n"+
		"Add a new named step to the registry (migrations/steps.go), then update the snapshot "+
		"with `go test ./migrations -run TestSchemaSnapshot -update-schema-snapshot`.\n"+
		"Never rename or reuse a released step name.\n--- stored\n%s--- current\n%s", stored, got)
}

func TestSchemaSnapshot(t *testing.T) {
	got := schemaSnapshot()
	if *updateSnapshot {
		if err := os.WriteFile(snapshotPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	stored, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkSnapshot(string(stored), got); err != nil {
		t.Error(err)
	}
}

func TestSchemaSnapshot_FailureNamesNextAction(t *testing.T) {
	err := checkSnapshot(schemaSnapshot()+"enum extra: x\n", schemaSnapshot())
	if err == nil {
		t.Fatal("a changed schema passed")
	}
	for _, want := range []string{"new named step", "update the snapshot"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message lacks %q: %v", want, err)
		}
	}
}
