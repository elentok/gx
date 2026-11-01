package migrations

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tokenledger"
)

// attrStore is project "proj" with a ticket in a queue epic, one in one-offs
// and one in a map epic, under an isolated data dir.
func attrStore(t *testing.T) (config.Dirs, string, string) {
	t.Helper()
	home := t.TempDir()
	dirs := config.DirsFrom([]string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
	})
	root := filepath.Join(t.TempDir(), "proj")
	ticket := func(sessions string) string {
		return "---\nid: \"01\"\nstatus: done\ntype: implement\nsession_ids: [" + sessions + "]\n---\nBody.\n"
	}
	testutil.WriteFile(t, root, "project.json", `{}`)
	testutil.WriteFile(t, root, "queued/issues/01-a.md", ticket("q1"))
	testutil.WriteFile(t, root, "one-offs/issues/01-b.md", ticket("o1"))
	testutil.WriteFile(t, root, "mapped/ticket.md", "---\nstatus: open\nkind: map\n---\n")
	testutil.WriteFile(t, root, "mapped/issues/01-c.md", "---\nid: \"01\"\nstatus: done\ntype: grilling\nsession_ids: [m1]\n---\nBody.\n")
	data, err := dirs.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	return dirs, root, data
}

func TestAttributeGroupSources(t *testing.T) {
	dirs, root, data := attrStore(t)
	recDir, _ := conversations.DirIn(dirs)
	for _, r := range []conversations.Record{
		{ID: "d1", Harness: "claude", Ticket: "proj:mapped/01", Design: true},
		{ID: "x1", Harness: "claude", Ticket: "proj:queued/01"},
		{ID: "far", Harness: "claude", Ticket: "other:e/01", Design: true},
	} {
		if err := conversations.Save(recDir, r); err != nil {
			t.Fatal(err)
		}
	}
	testutil.WriteFile(t, data, "sessions/s1/meta.json", `{"id":"meta-design","harness":"claude","project":"proj","design":true}`)

	got, err := Attribute(dirs, root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"q1": tokenledger.GroupQueue, "o1": tokenledger.GroupOneOff, "m1": tokenledger.GroupDesign,
		"d1": tokenledger.GroupDesign, "meta-design": tokenledger.GroupDesign, "x1": tokenledger.GroupOther,
	}
	for id, group := range want {
		if a := got.Of(id); a.Group != group || a.Project != "proj" {
			t.Errorf("%s = %+v, want group %s in proj", id, a, group)
		}
	}
	if a := got.Of("far"); a.Project != "" || a.Group != tokenledger.GroupOther {
		t.Errorf("another project's record leaked: %+v", a)
	}
}

func TestAttributeNoProjectAndUnknown(t *testing.T) {
	dirs, root, data := attrStore(t)
	testutil.WriteFile(t, data, "sessions/s1/meta.json", `{"id":"loose","harness":"claude"}`)

	got, err := Attribute(dirs, root)
	if err != nil {
		t.Fatal(err)
	}
	if ids := got.NoProject(); !slices.Equal(ids, []string{"loose"}) {
		t.Errorf("NoProject = %v", ids)
	}
	if a := got.Of("never-seen"); a != (Attribution{Group: tokenledger.GroupOther, Harness: "claude"}) {
		t.Errorf("unknown = %+v, want Other, no project, Claude", a)
	}
}

func TestAttributeCodexOnlyWhereNamed(t *testing.T) {
	dirs, root, data := attrStore(t)
	recDir, _ := conversations.DirIn(dirs)
	if err := conversations.Save(recDir, conversations.Record{ID: "q1", Harness: "codex", Ticket: "proj:queued/01"}); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, data, "sessions/s1/meta.json", `{"id":"cx","harness":"codex","project":"proj"}`)
	testutil.WriteFile(t, data, "sessions/s2/meta.json", `{"id":"cl","harness":"claude","project":"proj"}`)

	got, err := Attribute(dirs, root)
	if err != nil {
		t.Fatal(err)
	}
	for id, h := range map[string]string{"q1": "codex", "cx": "codex", "cl": "claude", "o1": "claude", "never-seen": "claude"} {
		if a := got.Of(id); a.Harness != h {
			t.Errorf("%s harness = %s, want %s", id, a.Harness, h)
		}
	}
	if a := got.Of("q1"); a.Group != tokenledger.GroupQueue {
		t.Errorf("harness upgrade changed the group: %+v", a)
	}
}
