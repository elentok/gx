package designflows

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
)

// gxWithStdin runs a CLI verb against r and reports its output and whether it exited 0.
func (r *running) gxWithStdin(t *testing.T, stdin string, args ...string) (out string, ok bool) {
	t.Helper()
	cmd := exec.Command(fixturetest.GxBin, args...)
	cmd.Env = r.Env
	cmd.Stdin = strings.NewReader(stdin)
	b, err := cmd.CombinedOutput()
	return string(b), err == nil
}

func (r *running) snapshot(t *testing.T) wire.Snapshot {
	t.Helper()
	var snap wire.Snapshot
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "snapshot", "--json")), &snap); err != nil {
		t.Fatal(err)
	}
	return snap
}

func forkHook(t *testing.T, r *running, name string, armed bool) {
	t.Helper()
	path := filepath.Join(r.ForkHookDir, name)
	var err error
	if armed {
		err = os.WriteFile(path, nil, 0o644)
	} else {
		err = os.Remove(path)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestForkHooks_SaveFailFailsBothDraftCreatesAndStoresNothing(t *testing.T) {
	t.Parallel()
	r := start(t, "fork-hooks")
	defer r.Stop(t)
	chatArgs := []string{"agents", "drafts", "create", "--project", project, "--brief", "-", "--folder", "trunk", "--json"}
	designArgs := []string{"designs", "drafts", "create", "--project", project, "--slug", "hooked", "--idea", "-", "--json"}

	forkHook(t, r, "save-fail", true)
	if out, ok := r.gxWithStdin(t, "a brief", chatArgs...); ok {
		t.Fatalf("chat draft create worked with save-fail armed:\n%s", out)
	}
	if out, ok := r.gxWithStdin(t, "an idea", designArgs...); ok {
		t.Fatalf("design draft create worked with save-fail armed:\n%s", out)
	}
	if snap := r.snapshot(t); len(snap.ChatDrafts) != 0 || len(snap.DesignDrafts) != 0 {
		t.Fatalf("drafts after the failed creates: chat %+v, design %+v", snap.ChatDrafts, snap.DesignDrafts)
	}

	forkHook(t, r, "save-fail", false)
	if out, ok := r.gxWithStdin(t, "a brief", chatArgs...); !ok {
		t.Fatalf("chat draft create after disarming:\n%s", out)
	}
	if out, ok := r.gxWithStdin(t, "an idea", designArgs...); !ok {
		t.Fatalf("design draft create after disarming:\n%s", out)
	}
	if snap := r.snapshot(t); len(snap.ChatDrafts) != 1 || len(snap.DesignDrafts) != 1 {
		t.Fatalf("drafts after the creates: chat %+v, design %+v", snap.ChatDrafts, snap.DesignDrafts)
	}
}

func TestForkHooks_StartFailLeavesTheDraftWithItsWorktreeThenStartWorks(t *testing.T) {
	t.Parallel()
	r := start(t, "fork-hooks")
	defer r.Stop(t)
	out, ok := r.gxWithStdin(t, "a brief", "agents", "drafts", "create", "--project", project,
		"--brief", "-", "--folder", "worktree", "--json")
	if !ok {
		t.Fatalf("chat draft create:\n%s", out)
	}
	var created ops.ChatDraftResult
	if err := json.Unmarshal([]byte(out), &created); err != nil || created.Draft == nil {
		t.Fatalf("create output %q: %+v, %v", out, created, err)
	}
	id := created.Draft.ID
	web := r.Web(t)

	forkHook(t, r, "start-fail", true)
	if data, ok := web.Try(t, "/v1/agents/drafts/start", ops.ChatDraftReadRequest{ID: id}); ok {
		t.Fatalf("start worked with start-fail armed: %s", data)
	}
	var read ops.ChatDraftResult
	if err := json.Unmarshal([]byte(postRaw(t, web, "/v1/agents/drafts/read", ops.ChatDraftReadRequest{ID: id})), &read); err != nil {
		t.Fatal(err)
	}
	if read.Draft == nil || read.Draft.WorktreePath == "" || read.Draft.StartError == "" {
		t.Fatalf("draft after the failed start = %+v, want its worktree path and the error", read.Draft)
	}
	if _, err := os.Stat(read.Draft.WorktreePath); err != nil {
		t.Errorf("worktree folder: %v", err)
	}

	forkHook(t, r, "start-fail", false)
	web.Post(t, "/v1/agents/drafts/start", ops.ChatDraftReadRequest{ID: id})
	if snap := r.snapshot(t); len(snap.ChatDrafts) != 0 {
		t.Fatalf("chat drafts after a good start = %+v, want none", snap.ChatDrafts)
	}
}

func postRaw(t *testing.T, w *web, path string, body any) string {
	t.Helper()
	data, _ := w.Try(t, path, body)
	_, after, _ := strings.Cut(data, ": ")
	return after
}
