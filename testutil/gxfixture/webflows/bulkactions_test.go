package webflows

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func TestBulkActions_SeedsCardsAgentsWorkspacesAndArchivedEpics(t *testing.T) {
	t.Parallel()
	r := start(t, "bulk-actions")
	defer r.Stop(t)

	// Queue and held cards appear once the server has launched the workers.
	want := []string{
		"bulk-merged|finished", "bulk-unmerged|implementing", "bulk-queued|implementing",
		"bulk-held|on-hold", "bulk-done|ready-to-queue", "bulk-done-solo|finished", "bulk-early|designing",
	}
	var got, missing []string
	testutil.Poll(40*time.Second, 500*time.Millisecond, func() bool {
		var list wire.EpicList
		if err := json.Unmarshal([]byte(r.Gx(t, "server", "epics", "--json")), &list); err != nil {
			t.Fatal(err)
		}
		got = got[:0]
		merged := map[string]bool{}
		for _, c := range list.Epics {
			got = append(got, c.Name+"|"+string(c.Section))
			merged[c.Name] = !c.MergedAt.IsZero()
		}
		missing = slices.DeleteFunc(slices.Clone(want), func(w string) bool { return slices.Contains(got, w) })
		return len(missing) == 0 && merged["bulk-merged"] && !merged["bulk-unmerged"]
	})
	if len(missing) > 0 {
		t.Fatalf("missing cards %v\ngot:\n%s", missing, strings.Join(got, "\n"))
	}

	var sessions []wire.SessionInfo
	have := func() (design, fix, queue, exited bool) {
		for _, si := range sessions {
			design = design || si.Design
			fix = fix || si.FixOf != ""
			queue = queue || si.Queue
			exited = exited || si.Status.State == wire.SessionExited
		}
		return
	}
	if !testutil.Poll(60*time.Second, time.Second, func() bool {
		sessions = nil
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &sessions); err != nil {
			t.Fatal(err)
		}
		d, f, q, e := have()
		return d && f && q && e
	}) {
		d, f, q, e := have()
		t.Fatalf("agents: design=%v fix=%v queue=%v exited=%v, want all", d, f, q, e)
	}

	var ws wire.WorkspaceList
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "workspaces", "list", "--json")), &ws); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, w := range ws.Workspaces {
		names = append(names, w.Name)
	}
	for _, name := range []string{"Default workspace", spec.WorkspaceBulk} {
		if !slices.Contains(names, name) {
			t.Errorf("workspace %q missing from %v", name, names)
		}
	}

	archive := filepath.Join(r.Root(), "data", "gx", "tickets", "demo", ".archive")
	for name, wantStamp := range map[string]bool{"arch-stamped": true, "arch-stale": true, "arch-bare": false, "arch-pair": true, "arch-pair-impl": true, "bulk-clash": true} {
		b, err := os.ReadFile(filepath.Join(archive, name, "ticket.md"))
		if err != nil {
			t.Errorf("archived %s: %v", name, err)
			continue
		}
		if has := strings.Contains(string(b), "archived_at:"); has != wantStamp {
			t.Errorf("archived %s: archived_at present = %v, want %v", name, has, wantStamp)
		}
	}
	stale, _ := os.ReadFile(filepath.Join(archive, "arch-stale", "ticket.md"))
	if !strings.Contains(string(stale), "archived_workspace: "+spec.ArchivedWorkspaceGone) || slices.Contains(names, spec.ArchivedWorkspaceGone) {
		t.Errorf("arch-stale must name a workspace that does not exist; workspaces %v", names)
	}
}
