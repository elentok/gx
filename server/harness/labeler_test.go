package harness

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	if !h.sessionRecordedUncached(runlog.IterationSession{Agent: agent.AgentClaude, Cwd: cwd, Session: "sess-1"}, false) {
		t.Fatal("transcript at the cwd slug not found")
	}
}

// A transcript that shows up in another project dir after the first miss is
// found for a recent run once the 15 s TTL passes, never for an old run.
func TestSessionRecordedOldRunSkipsGlob(t *testing.T) {
	home := t.TempDir()
	reg := projects.NewRegistry(func() projects.Env { return projects.Env{Env: []string{"HOME=" + home}} }, nil, nil, nil, nil, nil)
	h := &Harness{projects: reg}
	now := time.Now()
	recent := runlog.IterationSession{Agent: agent.AgentClaude, Cwd: "/work/a", Session: "recent", Started: now}
	old := runlog.IterationSession{Agent: agent.AgentClaude, Cwd: "/work/b", Session: "old", Started: now.Add(-oldRunAge - time.Minute)}
	for _, r := range []runlog.IterationSession{recent, old} {
		if h.sessionRecordedAt(r, now) {
			t.Fatalf("%s found before it exists", r.Session)
		}
	}
	for _, r := range []runlog.IterationSession{recent, old} {
		// Another cwd slug, so only the glob can find it.
		path := transcript.PathIn(home, "/elsewhere/"+r.Session, r.Session)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	later := now.Add(recordedMissTTL + time.Second)
	if !h.sessionRecordedAt(recent, later) {
		t.Error("recent run did not re-check after the miss TTL")
	}
	if h.sessionRecordedAt(old, later) {
		t.Error("old run re-globbed after the miss TTL")
	}
}
