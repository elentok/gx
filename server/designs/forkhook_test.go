package designs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/wire/ops"
)

func TestForkHooks_SaveFailRefusesTheDesignDraftCreateAndStoresNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := startDesigns(t, func(c *server.Config) { c.ForkHookDir = dir })
	req := ops.DesignDraftCreateRequest{Project: "proj", Slug: "hooked", Idea: "an idea"}
	hook := filepath.Join(dir, chatdrafts.HookSaveFail)

	if err := os.WriteFile(hook, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if res, err := f.h.Client.Sessions.CreateDesignDraft(f.ctx, req); err == nil {
		t.Fatalf("armed create = %+v, want an error", res)
	}
	if items, err := drafts.New(f.h.StateDir).List(); err != nil || len(items) != 0 {
		t.Fatalf("drafts after the failed create = %+v, %v, want none", items, err)
	}

	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if res := f.createDraft(t, req); res.Draft == nil {
		t.Fatalf("disarmed create = %+v, want a draft", res)
	}
}

func TestForkHooks_UnsetDirRefusesNothing(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	if res := f.createDraft(t, ops.DesignDraftCreateRequest{Project: "proj", Slug: "plain", Idea: "an idea"}); res.Draft == nil {
		t.Fatalf("create with no hook dir = %+v, want a draft", res)
	}
}
