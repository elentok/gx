package spec

import (
	"slices"
	"time"

	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/bgtx"
	"github.com/elentok/gx/tickets"
)

// BgProgressShapes are the --shape names of the bg-progress scenario; the first
// is the default.
var BgProgressShapes = []string{
	"claude", "codex", "old-codex", "single-task", "exited", "no-end",
	"subagent-with-inner-task", "many", "timed-out-shell", "growing-output",
}

// Keys of the exited shape's sessions in Output.Transcripts.
const (
	BgKeyExitedClaude = "exited-claude"
	BgKeyExitedCodex  = "exited-codex"
)

// BgProgressScenario is the bg-progress scenario in the given shape: Claude and
// Codex sessions whose transcripts and task files hold the shape's tasks (see
// bgtx). The single-session shapes are one Working Claude Design session of
// design-epic. False for a name that is not a shape.
func BgProgressScenario(shape string) (Scenario, bool) {
	const mapAddr = project + ":design-epic"
	var sessions []SessionSpec
	if b, ok := bgtx.Single[shape]; ok {
		sessions = []SessionSpec{bgClaudeSession(mapAddr, shape, agentfake.ChatSlowMarker+" background tasks", b.Tasks, bgSeed(b.Build, nil))}
	} else if m, ok := bgMulti[shape]; ok {
		sessions = m(mapAddr)
	} else {
		return Scenario{}, false
	}
	return Scenario{
		Epics:       []Epic{{Project: project, Name: "design-epic"}},
		DesignSteps: map[string]string{mapAddr: string(tickets.StepResolveTickets)},
		Chat:        true,
		Codex:       slices.ContainsFunc(sessions, func(s SessionSpec) bool { return s.Harness == "codex" }),
		Sessions:    sessions,
	}, true
}

// bgSeed is a session's seed func; adjust sets the seed's own fields.
func bgSeed(build func(b *bgtx.Lines), adjust func(*bgtx.Seed)) func(time.Time, bgtx.Paths) bgtx.Seed {
	return func(now time.Time, p bgtx.Paths) bgtx.Seed {
		l := bgtx.NewLines(now, p)
		build(l)
		seed := l.Seed()
		if adjust != nil {
			adjust(&seed)
		}
		return seed
	}
}

func bgClaudeSession(mapAddr, key, prompt string, tasks int, seed func(time.Time, bgtx.Paths) bgtx.Seed) SessionSpec {
	return SessionSpec{Project: project, Prompt: prompt, Link: mapAddr, Key: key, BgTasks: tasks, BgSeed: seed}
}

func bgCodexSession(key string, tasks int, build func(b *bgtx.Lines), adjust func(*bgtx.Seed)) SessionSpec {
	return SessionSpec{Project: project, Harness: "codex", Prompt: "background tasks", Key: key, BgTasks: tasks, BgSeed: bgSeed(build, adjust)}
}

func bgVersion(v string) func(*bgtx.Seed) { return func(s *bgtx.Seed) { s.Version = v } }

// bgMulti maps a shape of a Codex session, or of more than one, to its sessions.
var bgMulti = map[string]func(mapAddr string) []SessionSpec{
	"codex": func(string) []SessionSpec {
		return []SessionSpec{
			bgCodexSession("codex-items", 3, bgtx.Codex(true, "00000000-0000-4000-8000-0000000000c2"), nil),
			bgCodexSession("codex-cells", 3, bgtx.Codex(false, "00000000-0000-4000-8000-0000000000c3"), bgVersion(bgtx.CodexCellVersion)),
		}
	},
	"old-codex": func(string) []SessionSpec {
		return []SessionSpec{bgCodexSession("old-codex", 0, bgtx.OldCodex, func(s *bgtx.Seed) { s.Version, s.Working = bgtx.CodexOldVersion, true })}
	},
	"exited": func(mapAddr string) []SessionSpec {
		claude := bgClaudeSession(mapAddr, BgKeyExitedClaude, "background tasks", 0, bgSeed(bgtx.ExitedClaude, nil))
		codex := bgCodexSession(BgKeyExitedCodex, 0, bgtx.ExitedCodex, nil)
		claude.Crash, codex.Crash = true, true
		return []SessionSpec{claude, codex}
	},
	// The Claude session is the second one so the Monitor's 20 s are not spent
	// seeding the Codex session.
	"no-end": func(mapAddr string) []SessionSpec {
		claude := bgClaudeSession(mapAddr, "no-end-claude", "background tasks", 5, bgSeed(bgtx.NoEndClaude, func(s *bgtx.Seed) { s.Age = bgtx.NoEndAge }))
		return []SessionSpec{bgCodexSession("no-end-codex", 1, bgtx.NoEndCodex, nil), claude}
	},
}
