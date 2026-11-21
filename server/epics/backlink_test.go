package epics_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

const implLoopMD = "---\nstatus: open\n---\n"

func seedBackLinks(t *testing.T, store string) {
	t.Helper()
	open := map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")}
	initiativeMD := func(extra, milestones string) string {
		return "---\nstatus: open\nkind: initiative\n" + extra + "---\n\n## Destination\n\nGo.\n\n## Milestones\n\n" + milestones
	}
	design := func(name, initiative, milestone string) {
		fm := "---\nstatus: open\nkind: map\n"
		if initiative != "" {
			fm += "initiative: " + initiative + "\n"
		}
		if milestone != "" {
			fm += "milestone: " + milestone + "\n"
		}
		rigs.WriteEpicFiles(t, store, name, fm+"---\n", open)
	}
	rigs.WriteEpicFiles(t, store, "alpha", initiativeMD("", "- a | a name | goal: ga | design: other\n- b | b name | goal: gb | design: ok\n- c | c name | goal: gc | design: arch\n"), nil)
	rigs.WriteEpicFiles(t, store, ".archive/old", initiativeMD("archived_at: 2026-03-06T10:00:00Z\n", "- m | m name | goal: gm | design: ofold\n"), nil)

	design("ok", "alpha", "b")
	rigs.WriteEpicFiles(t, store, "ok-impl", implLoopMD, open)
	design("ofold", "old", "m")
	rigs.WriteEpicFiles(t, store, ".archive/arch", "---\nstatus: open\nkind: map\ninitiative: alpha\nmilestone: c\ncompleted_at: 2026-01-02T10:00:00Z\n---\n", nil)
	design("noinit", "ghost", "b")
	design("unknown", "alpha", "nope")
	design("mismatch", "alpha", "a")
	design("plain", "", "")
	rigs.WriteEpicFiles(t, store, "plain-impl", implLoopMD, open)
}

func listBackLinks(t *testing.T) (wire.EpicList, map[string]wire.EpicListRow) {
	t.Helper()
	h, base := rigs.StartEpicDetail(t, func(store string) { seedBackLinks(t, store) })
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	list := rigs.GetEpicList(t, code, body)
	byName := map[string]wire.EpicListRow{}
	for _, r := range list.Epics {
		byName[r.Name] = r
	}
	return list, byName
}

func TestEpicList_DesignAndImplCardCarryTheBackLink(t *testing.T) {
	t.Parallel()
	_, rows := listBackLinks(t)
	want := wire.BackLink{Milestone: 2, Initiative: "alpha", InitiativeAddress: "proj:alpha"}
	r, ok := rows["ok"]
	if !ok {
		t.Fatalf("no ok row in %+v", rows)
	}
	if r.BackLink != want {
		t.Errorf("ok back-link = %+v, want %+v", r.BackLink, want)
	}
	if len(r.Addresses) != 2 || r.Addresses[1] != "proj:ok-impl" {
		t.Errorf("ok card does not carry its -impl epic: %v", r.Addresses)
	}
	if _, listed := rows["ok-impl"]; listed {
		t.Error("the -impl epic got a row of its own")
	}
}

func TestEpicList_UnresolvedBackLinkIsEmptyWithoutError(t *testing.T) {
	t.Parallel()
	_, rows := listBackLinks(t)
	for _, name := range []string{"noinit", "unknown", "mismatch", "plain"} {
		r, ok := rows[name]
		if !ok {
			t.Errorf("%s is missing from the list", name)
			continue
		}
		if r.BackLink != (wire.BackLink{}) {
			t.Errorf("%s back-link = %+v, want none", name, r.BackLink)
		}
	}
}

func TestEpicList_ArchivedInitiativeAndDesignStillResolveBackLink(t *testing.T) {
	t.Parallel()
	_, rows := listBackLinks(t)
	want := wire.BackLink{Milestone: 1, Initiative: "old", InitiativeAddress: "proj:old"}
	if got := rows["ofold"].BackLink; got != want {
		t.Errorf("design of an archived Initiative back-link = %+v, want %+v", got, want)
	}
}

func TestEpicList_NeverListsAnInitiativeBackLink(t *testing.T) {
	t.Parallel()
	list, rows := listBackLinks(t)
	for _, name := range []string{"alpha", "old"} {
		if _, ok := rows[name]; ok {
			t.Errorf("Initiative %s has a row", name)
		}
	}
	for _, r := range list.Epics {
		if r.CardKind != wire.CardKindDesign && r.CardKind != wire.CardKindImplementation && r.CardKind != wire.CardKindFollowUps {
			t.Errorf("%s has unexpected card kind %q", r.Name, r.CardKind)
		}
	}
}

func TestEpicDetail_DesignAndImplCarryTheBackLink(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) { seedBackLinks(t, store) })
	want := wire.BackLink{Milestone: 2, Initiative: "alpha", InitiativeAddress: "proj:alpha"}
	for addr, w := range map[string]wire.BackLink{
		"proj:ok": want, "proj:ok-impl": want,
		"proj:noinit": {}, "proj:unknown": {}, "proj:mismatch": {}, "proj:plain": {}, "proj:plain-impl": {},
	} {
		code, d, body := rigs.GetEpicDetail(t, h, base, addr)
		if code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", addr, code, body)
		}
		if d.BackLink != w {
			t.Errorf("%s back-link = %+v, want %+v", addr, d.BackLink, w)
		}
	}
}

func TestEpicDetail_ArchivedDesignStillResolvesBackLink(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) { seedBackLinks(t, store) })
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics/archived/detail?address=proj:arch")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if !strings.Contains(body, `"initiative":"alpha"`) || !strings.Contains(body, `"milestone":3`) {
		t.Errorf("archived detail lacks the back-link: %s", body)
	}
}
