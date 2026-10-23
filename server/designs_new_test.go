package server_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

func (f designsFixture) openNew(t *testing.T, slug, idea string) server.SessionResult {
	t.Helper()
	res, err := f.h.Client.OpenDesign(f.ctx, server.DesignOpenRequest{Epic: slug, NewDesign: true, Idea: idea, Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDesignsOpenNew_CreatesDesignSessionWithoutAnEpic(t *testing.T) {
	f := startDesigns(t)
	res := f.openNew(t, "fresh-idea", "a thing\nwith lines")
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	meta := readMeta(t, f.h, res.Session.ID)
	want := map[string]any{"dir": f.repo, "name": "fresh-idea", "ticket": "proj:fresh-idea", "design": true, "first_prompt": "/gx-design fresh-idea a thing\nwith lines"}
	for k, v := range want {
		if meta[k] != v {
			t.Errorf("meta[%s] = %v, want %v", k, meta[k], v)
		}
	}
	if !res.Session.Design {
		t.Errorf("session = %+v, want Design", res.Session)
	}
	list, err := f.h.Client.Epics(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range list.Epics {
		if strings.HasSuffix(e.Address, "fresh-idea") {
			t.Errorf("epic %s was created", e.Address)
		}
	}
}

func TestDesignsOpenNew_SecondCallReturnsTheFirstSession(t *testing.T) {
	f := startDesigns(t)
	first := f.openNew(t, "fresh-idea", "one")
	second := f.openNew(t, "fresh-idea", "two")
	if first.Session == nil || second.Session == nil || second.Session.ID != first.Session.ID {
		t.Fatalf("second = %+v, want session of %+v", second, first)
	}
	if got := len(f.live(t)); got != 1 {
		t.Errorf("live sessions = %d, want 1", got)
	}
	if fp, _ := readMeta(t, f.h, first.Session.ID)["first_prompt"].(string); fp != "/gx-design fresh-idea one" {
		t.Errorf("first_prompt = %q", fp)
	}
}

func TestDesignsOpenNew_Refusals(t *testing.T) {
	f := startDesigns(t)
	for name, tc := range map[string]struct{ slug, idea, want string }{
		"bad slug":     {"Bad Slug", "idea", server.ReasonSessionBadRequest},
		"path slug":    {"a/b", "idea", server.ReasonSessionBadRequest},
		"epic exists":  {"plain", "idea", server.ReasonEpicExists},
		"map exists":   {"map", "idea", server.ReasonEpicExists},
		"empty idea":   {"fresh-idea", " \n", server.ReasonEmptyPrompt},
		"idea too big": {"fresh-idea", strings.Repeat("a", 32*1024+1), server.ReasonTextTooLong},
	} {
		if res := f.openNew(t, tc.slug, tc.idea); !res.Refused || res.Reason != tc.want {
			t.Errorf("%s: open = %+v, want reason %q", name, res, tc.want)
		}
	}
	if got := len(f.live(t)); got != 0 {
		t.Errorf("live sessions = %d, want 0", got)
	}
}

func TestDesignsOpenNew_AcceptsPromptsBeforeTheMapExists(t *testing.T) {
	t.Setenv(agentfake.ProgramInputEnv, "1")
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	f := startDesigns(t)
	res := f.openNew(t, "fresh-idea", "idea")
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, f.h, id)["conversation_id"].(string)
	waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })

	first := strconv.Quote("/gx-design fresh-idea idea") + "\n"
	waitForSubmitted(t, conv, first)

	got, err := f.h.Client.PromptSession(f.ctx, server.SessionPromptRequest{ID: id, Text: "more detail"})
	if err != nil || got.Refused {
		t.Fatalf("prompt = %+v, %v", got, err)
	}
	waitForSubmitted(t, conv, first+strconv.Quote("more detail")+"\n")
}
