package registry_test

import (
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/config/registry"
)

var update = flag.Bool("update", false, "rewrite docs/config-schema.json and docs/project-schema.json")

// configDefaults reads defaults from config.Default() as they appear in
// config.json. The ticket store path depends on the machine, so it has none.
func configDefaults(t *testing.T) registry.Defaults {
	t.Helper()
	b, err := json.Marshal(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return func(k registry.KeyPath) (any, bool) {
		if k.Equal(registry.Key("ticket-store", "path")) {
			return nil, false
		}
		var cur any = doc
		for _, seg := range k {
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, false
			}
			if cur, ok = m[seg]; !ok {
				return nil, false
			}
		}
		return cur, cur != nil
	}
}

func checkSchemaFile(t *testing.T, path string, want []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s is stale: it is generated from the registry.\nRegenerate it with: go test ./config/registry -run TestGeneratedSchemas -update", path)
	}
}

func TestGeneratedSchemas(t *testing.T) {
	r := registry.Default()
	checkSchemaFile(t, "../../docs/config-schema.json", r.ConfigSchema(configDefaults(t)))
	checkSchemaFile(t, "../../docs/project-schema.json", r.ProjectSchema())
}

func TestSchemaFollowsRegistry(t *testing.T) {
	changed := registry.New(registry.Entry{
		Key: registry.Key("budget", "soft-limit"), Type: registry.TypeNumber,
		Section: registry.SectionBudget, Description: "edited",
	})
	if string(changed.ConfigSchema(configDefaults(t))) == string(registry.Default().ConfigSchema(configDefaults(t))) {
		t.Fatal("a changed registry produced the same schema")
	}
}
