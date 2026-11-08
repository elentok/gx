package serverapi

import (
	"bytes"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestServerTips_HasJSONFlag(t *testing.T) {
	c, _, err := serverParent(Deps{}).Find([]string{"tips"})
	if err != nil || c.RunE == nil {
		t.Fatalf("server tips = %v, err %v; want a runnable verb", c, err)
	}
	if c.Flags().Lookup("json") == nil {
		t.Error("server tips has no --json")
	}
}

func TestServerTips_TextListsTheFolders(t *testing.T) {
	var buf bytes.Buffer
	tip := wire.Tip{ID: "spotlight", Title: "Keep worktrees out of Spotlight", Body: "steps", Folders: []string{"/repo/.worktrees"}}
	if err := printTips(&buf, []wire.Tip{tip}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{tip.Title, "/repo/.worktrees"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output lacks %q: %s", want, buf.String())
		}
	}
}
