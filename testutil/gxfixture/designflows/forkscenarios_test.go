package designflows

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func TestForkScenarios_SourceHoldsAChatACodexChatAndADesignInForkWs(t *testing.T) {
	t.Parallel()
	r := start(t, "fork-source")
	defer r.Stop(t)
	var sessions []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &sessions); err != nil {
		t.Fatal(err)
	}
	placed := map[string]wire.SessionPlacement{}
	for _, p := range r.snapshot(t).Placements {
		placed[p.Session] = p
	}
	var claude, codex, design int
	for _, si := range sessions {
		p := placed[si.ID]
		switch {
		case p.Kind == wire.PlacementEpic && strings.HasSuffix(p.Epic, ":fork-design"):
			design++
		case p.Workspace != spec.WorkspaceFork:
		case si.Harness == "codex":
			codex++
		default:
			claude++
		}
	}
	// A Design's workspace is its map's membership, not its session's placement.
	members, err := os.ReadFile(filepath.Join(r.StateDir, "workspaces.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(members), "fork-design") {
		t.Errorf("workspaces.json has no fork-design member:\n%s", members)
	}
	if claude != 1 || codex != 1 || design != 1 {
		t.Errorf("fork-ws holds claude %d, codex %d, design %d sessions, want one each; sessions %+v placements %+v",
			claude, codex, design, sessions, placed)
	}
}

func TestForkScenarios_ChatDraftHoldsThreeDraftsInForkWs(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-draft")
	defer r.Stop(t)
	web := r.Web(t)
	// name|harness|model|effort|folder|brief
	want := map[string]string{
		"claude-draft":   "claude|opus|max|source|" + spec.BriefForkClaude,
		"codex-draft":    "codex|gpt-5.5-codex|high|source|" + spec.BriefForkCodex,
		"worktree-draft": "claude|opus|max|worktree|" + spec.BriefForkWorktree,
	}
	rows := r.snapshot(t).ChatDrafts
	if len(rows) != len(want) {
		t.Fatalf("chat draft rows = %+v, want %d", rows, len(want))
	}
	for _, row := range rows {
		if row.Workspace != spec.WorkspaceFork {
			t.Errorf("row %+v: workspace = %q, want %q", row, row.Workspace, spec.WorkspaceFork)
		}
		var read ops.ChatDraftResult
		if err := json.Unmarshal([]byte(postRaw(t, web, "/v1/agents/drafts/read", ops.ChatDraftReadRequest{ID: row.ID})), &read); err != nil || read.Draft == nil {
			t.Fatalf("read %s: %v %+v", row.ID, err, read)
		}
		d := read.Draft
		got := d.Harness + "|" + d.Model + "|" + d.Effort + "|" + d.Folder + "|" + d.Brief
		if got != want[d.Name] {
			t.Errorf("draft %q = %q, want %q", d.Name, got, want[d.Name])
		}
	}
}
