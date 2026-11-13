package settingsapi

import (
	"testing"

	"github.com/elentok/gx/config/registry"
)

func TestWritableDirectoriesProjectEditorUsesAdditions(t *testing.T) {
	e, ok := registry.Default().Lookup(registry.Key("agents", "writable-directories"))
	if !ok {
		t.Fatal("missing setting")
	}
	global := []byte(`{"agents":{"writable-directories":["/global/cache"]}}`)
	project := buildEntry(e, true, []byte(`{}`), global)
	if project.Override != "add" || string(project.Default) != "[]" || string(project.GlobalValue) != `["/global/cache"]` || project.Set {
		t.Fatalf("project editor = %+v", project)
	}
	globalEntry := buildEntry(e, false, []byte(`{}`), []byte(`{}`))
	if string(globalEntry.Default) != `["~/Library/pnpm"]` {
		t.Fatalf("global default = %s", globalEntry.Default)
	}
}
