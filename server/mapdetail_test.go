package server_test

import (
	"net/http"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestEpicDetail_MapProgress(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	mapMD := func(front, body string) string { return "---\nstatus: open\nkind: map\n" + front + "---\n" + body }
	done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
	findings := "\n## Review findings\n\n- [x] a\n- [x] b\n- [ ] c\n- [ ] d\n- [ ] e\n"
	checked := "\n## Review findings\n\n- [x] a\n"

	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "m-chart", mapMD("design_step: chart\n", ""), done)
		writeEpicFiles(t, store, "m-claimed", mapMD("design_step: resolve-tickets\n", ""), map[string]string{
			"01-a.md": issueMD("01", "done", "", "A"),
			"02-b.md": issueMD("02", "claimed", "", "B"),
			"03-c.md": issueMD("03", "open", "blocked_by: [\"02\"]\n", "C"),
		})
		writeEpicFiles(t, store, "m-frontier", mapMD("design_step: resolve-tickets\n", ""), map[string]string{
			"01-a.md": issueMD("01", "open", "", "A"),
			"02-b.md": issueMD("02", "open", "blocked_by: [\"01\"]\n", "B"),
		})
		writeEpicFiles(t, store, "m-fog", mapMD("design_step: resolve-tickets\n", "\n## Not yet specified\n\n- the schema\n"), done)
		writeEpicFiles(t, store, "m-grill", mapMD("design_step: grill-findings\n", findings), done)
		writeEpicFiles(t, store, "m-create", mapMD("design_step: create-tickets\n", checked), done)
		writeEpicFiles(t, store, "m-create-impl", "---\nstatus: draft\n---\n", map[string]string{
			"01-a.md": issueMD("01", "open", "", "A"),
			"02-b.md": issueMD("02", "open", "", "B"),
		})
		writeEpicFiles(t, store, "m-back", mapMD("design_step: resolve-tickets\n", checked), map[string]string{
			"01-a.md": issueMD("01", "done", "", "A"),
			"02-b.md": issueMD("02", "open", "", "B"),
		})
		servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	})
	detail := func(epic string) server.EpicDetail {
		t.Helper()
		code, d, body := getEpicDetail(t, h, base, "proj:"+epic)
		if code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", epic, code, body)
		}
		return d
	}
	current := func(d server.EpicDetail) (done int, label string) {
		for _, s := range d.Steps {
			switch s.State {
			case server.MapStepDone:
				done++
			case server.MapStepCurrent:
				label = s.Label
			}
		}
		return done, label
	}

	cases := []struct {
		epic  string
		done  int
		label string
	}{
		{"m-chart", 0, "Chart · confirm destination"},
		{"m-claimed", 1, "Resolve tickets"},
		{"m-frontier", 1, "Resolve tickets"},
		{"m-fog", 1, "Resolve tickets · fog left"},
		{"m-grill", 4, "Grill findings · 2 of 5"},
		{"m-create", 5, "Create tickets · 2 tickets published"},
		{"m-back", 1, "Resolve tickets"},
	}
	for _, c := range cases {
		d := detail(c.epic)
		if len(d.Steps) != 8 {
			t.Fatalf("%s: %d steps, want 8", c.epic, len(d.Steps))
		}
		if done, label := current(d); done != c.done || label != c.label {
			t.Errorf("%s: %d done, current %q; want %d, %q (%+v)", c.epic, done, label, c.done, c.label, d.Steps)
		}
	}

	back := detail("m-back")
	if spec := back.Steps[2]; spec.Step != server.ProgressSpec || spec.State != server.MapStepTodo {
		t.Errorf("sent-back Spec step = %+v, want todo", spec)
	}

	claimed := detail("m-claimed")
	if r := rowOf(t, claimed, "proj:m-claimed/02"); !r.Current || r.Blocked {
		t.Errorf("claimed row = %+v, want current", r)
	}
	if r := rowOf(t, claimed, "proj:m-claimed/03"); r.Current || !r.Blocked {
		t.Errorf("blocked row = %+v, want blocked, not current", r)
	}
	frontier := detail("m-frontier")
	if r := rowOf(t, frontier, "proj:m-frontier/01"); !r.Current || r.Blocked {
		t.Errorf("frontier row = %+v, want current", r)
	}
	if r := rowOf(t, frontier, "proj:m-frontier/02"); r.Current || !r.Blocked {
		t.Errorf("blocked row = %+v, want blocked", r)
	}

	if got := detail("m-create").ImplAddress; got != "proj:m-create-impl" {
		t.Errorf("ImplAddress = %q", got)
	}
	if got := detail("m-chart").ImplAddress; got != "" {
		t.Errorf("ImplAddress before the impl epic exists = %q", got)
	}
}

func TestEpicDetail_LoopEpicReportsDraft(t *testing.T) {
	t.Parallel()
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "draft-loop", "---\nstatus: draft\n---\n", map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
		writeEpicFiles(t, store, "open-loop", "---\nstatus: open\n---\n", map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
	})
	for epic, want := range map[string]bool{"draft-loop": true, "open-loop": false} {
		if _, d, _ := getEpicDetail(t, h, base, "proj:"+epic); d.Draft != want {
			t.Errorf("%s: Draft = %v, want %v", epic, d.Draft, want)
		}
	}
}
