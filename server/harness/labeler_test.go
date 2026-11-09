package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

func TestLabelerMemoizesPerEpicAndProject(t *testing.T) {
	l := (&Harness{}).Labeler()
	addr := tickets.Address{Project: "p", Epic: "e", ID: "01"}
	l.events["p\x00e"] = []runlog.Event{{}}
	l.defaults["p"] = agent.AgentCodex

	if got := l.runEvents(addr); len(got) != 1 {
		t.Fatalf("runEvents re-read instead of reusing the pass: %v", got)
	}
	if got := l.defaultHarness("p"); got != agent.AgentCodex {
		t.Fatalf("defaultHarness = %v, want the memoized codex", got)
	}
}

func TestSessionRecordedStatsTranscriptPathDirectly(t *testing.T) {
	home := t.TempDir()
	cwd := "/work/tree"
	path := transcript.PathIn(home, cwd, "sess-1")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := projects.NewRegistry(func() projects.Env { return projects.Env{Env: []string{"HOME=" + home}} }, nil, nil, nil, nil, nil)
	h := &Harness{projects: reg}
	if !h.sessionRecordedUncached(runlog.IterationSession{Agent: agent.AgentClaude, Cwd: cwd, Session: "sess-1"}) {
		t.Fatal("transcript at the cwd slug not found")
	}
}
