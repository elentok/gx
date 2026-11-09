package check

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"testing"
)

const schemaPath = "../docs/checks-schema.json"

var updateSchema = flag.Bool("update", false, "rewrite "+schemaPath)

// schemaStale returns an error naming the update command when the committed
// schema differs from the generated one.
func schemaStale(stored, got []byte) error {
	if bytes.Equal(stored, got) {
		return nil
	}
	return errors.New("docs/checks-schema.json is stale; regenerate it with `go test ./check -run TestSchemaSnapshot -update`")
}

func TestSchemaSnapshot(t *testing.T) {
	got := SchemaJSON()
	if *updateSchema {
		if err := os.WriteFile(schemaPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	stored, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := schemaStale(stored, got); err != nil {
		t.Error(err)
	}
}

func TestSchemaSnapshot_StaleNamesUpdateCommand(t *testing.T) {
	err := schemaStale(append(SchemaJSON(), ' '), SchemaJSON())
	if err == nil {
		t.Fatal("a stale schema passed")
	}
	want := "go test ./check -run TestSchemaSnapshot -update"
	if !bytes.Contains([]byte(err.Error()), []byte(want)) {
		t.Errorf("err = %v, want it to name %q", err, want)
	}
}
