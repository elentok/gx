package rigs

import (
	"os"
	"path/filepath"
	"testing"
)

const initiativeLoopMD = "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"

// SeedInitiatives writes four Initiatives to project "proj" and a plain epic to
// project "other":
//
//	alpha     active; sketch a, designing b, broken c (Design "gone" is missing),
//	          designing d (Design "arch" is archived); one bad line per section
//	stopper   stopped at Milestone 2
//	finisher  finished
func SeedInitiatives(t *testing.T, store string) {
	t.Helper()
	initiative := func(name, body string) {
		WriteEpicFiles(t, store, name, "---\nstatus: open\nkind: initiative\n---\n\n## Destination\n\nGo somewhere.\n\n"+body, nil)
	}
	initiative("alpha", "## Milestones\n\n"+
		"- a | a name | goal: goal of a\n"+
		"- b | b name | goal: goal of b | design: db\n"+
		"- c | c name | goal: goal of c | design: gone\n"+
		"- d | d name | goal: goal of d | design: arch\n"+
		"not a milestone\n\n"+
		"## Assumptions\n\n"+
		"- A1 | Users want it | tested by: a | state: unknown | evidence: -\n"+
		"bad assumption\n\n"+
		"## Deferred\n\n"+
		"- Export | to: unplaced | why: unclear | from: db | state: open\n"+
		"bad deferred\n\n"+
		"## Gate notes\n\n"+
		"bad gate line\n")
	initiative("stopper", "## Milestones\n\n- x | x name | goal: gx\n- y | y name | goal: gy\n\n"+
		"## Gate notes\n\n### x\noutcome: continue\n### y\noutcome: stop\n")
	initiative("finisher", "## Milestones\n\n- m | m name | goal: gm\n\n"+
		"## Gate notes\n\n### m\noutcome: done\n")
	WriteEpicFiles(t, store, "db", "---\nstatus: open\nkind: map\ninitiative: alpha\nmilestone: b\n---\n", nil)
	WriteEpicFiles(t, store, ".archive/arch", "---\nstatus: open\nkind: map\ninitiative: alpha\nmilestone: d\ncompleted_at: 2026-01-02T10:00:00Z\n---\n", nil)
	WriteEpicFiles(t, store, "plain", initiativeLoopMD, map[string]string{"01-a.md": IssueMD("01", "open", "", "A")})

	other := filepath.Join(store, "other", "plain-two")
	if err := os.MkdirAll(filepath.Join(other, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "other", "project.json"), []byte(`{"name":"other"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "ticket.md"), []byte(initiativeLoopMD), 0o644); err != nil {
		t.Fatal(err)
	}
}
