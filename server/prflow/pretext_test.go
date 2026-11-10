package prflow_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server/epics/prstate"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestParseText(t *testing.T) {
	cases := []struct {
		name, in, title, body, err string
	}{
		{"valid", "Title: Add x\nBody:\nLine one\n\nLine two\n", "Add x", "Line one\n\nLine two", ""},
		{"body on the label line", "Title: T\nBody: inline\nmore", "T", "inline\nmore", ""},
		{"no title", "Body:\nsomething", "", "", "no Title:"},
		{"empty title", "Title:   \nBody:\nx", "", "", "no Title:"},
		{"no body", "Title: T\n", "", "", "no Body:"},
		{"empty body", "Title: T\nBody:\n  \n", "", "", "Body: section is empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			title, body, err := prflow.ParseText(c.in)
			if c.err != "" {
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Fatalf("err = %v, want %q", err, c.err)
				}
				return
			}
			if err != nil || title != c.title || body != c.body {
				t.Fatalf("got %q %q %v, want %q %q", title, body, err, c.title, c.body)
			}
		})
	}
}

func TestParseText_LongBodyIsTrimmedWhenUsed(t *testing.T) {
	_, body, err := prflow.ParseText("Title: T\nBody:\n" + strings.Repeat("line\n", 100000))
	if err != nil || !strings.Contains(body, "trimmed") {
		t.Fatalf("body len %d, err %v; want a trimmed body", len(body), err)
	}
}

func TestStartPRText_NeverASecondWhileOneIsOpen(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	job := prflow.TextJob{Epic: "proj:epic-a", Worktree: "/wt/epic-a", Skill: "my-skill"}
	id, started, err := prflow.StartPRText(f.EpicDir(), job)
	if err != nil || !started || id == "" {
		t.Fatalf("first = %q %v %v", id, started, err)
	}
	if _, started, err := prflow.StartPRText(f.EpicDir(), job); err != nil || started {
		t.Fatalf("second = %v %v, want none while one is open", started, err)
	}
	md := servertest.ReadFile(t, ticketFile(t, f, id))
	for _, want := range []string{"type: pr-text", "proj:epic-a", "/wt/epic-a", "Skill: my-skill"} {
		if !strings.Contains(md, want) {
			t.Errorf("ticket lacks %q:\n%s", want, md)
		}
	}
}

func TestStartPRText_NoSkillNamesNothing(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	id, _, err := prflow.StartPRText(f.EpicDir(), prflow.TextJob{Epic: "proj:epic-a", Worktree: "/wt"})
	if err != nil {
		t.Fatal(err)
	}
	if md := servertest.ReadFile(t, ticketFile(t, f, id)); strings.Contains(md, "Skill:") {
		t.Errorf("ticket names a skill:\n%s", md)
	}
}

func ticketFile(t *testing.T, f *rigs.PRFlow, id string) string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(f.EpicDir(), "issues", id+"-*.md"))
	if err != nil || len(m) != 1 {
		t.Fatalf("ticket %s files = %v, %v", id, m, err)
	}
	return m[0]
}

// textRun scripts the pr-text agent: resolutions[i] is what attempt i writes.
type textRun struct {
	mu      sync.Mutex
	prompts []string
	cwds    []string
	models  []string
}

func runPRText(t *testing.T, f *rigs.PRFlow, skill string, resolutions []string) *textRun {
	t.Helper()
	servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "01", "root", servertest.TicketOpts{Status: "done"})
	if _, _, err := prflow.StartPRText(f.EpicDir(), prflow.TextJob{Epic: "proj:epic-a", Worktree: "/wt", Skill: skill}); err != nil {
		t.Fatal(err)
	}
	f.Rescan(t)
	run := &textRun{}
	f.H.OnTurn(func(p *servertest.Turn) {
		_, id, _ := strings.Cut(p.Address, "/")
		run.mu.Lock()
		n := len(run.prompts)
		run.prompts = append(run.prompts, p.Text)
		run.cwds = append(run.cwds, p.Cwd)
		run.mu.Unlock()
		path := ticketFile(t, f, id)
		raw := servertest.ReadFile(t, path)
		if n < len(resolutions) {
			raw = strings.TrimRight(raw, "\n") + "\n\n## Resolution\n\n" + resolutions[n] + "\n"
			if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
				t.Error(err)
			}
		}
		servertest.ReportIteration(t, path, "finished")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := f.H.Client.Queue.Add(ctx, "proj:epic-a/02", "claude"); err != nil || res.Refused {
		t.Fatalf("queue add: %+v, %v", res, err)
	}
	return run
}

func prTextTickets(f *rigs.PRFlow) (all []tickets.Ticket) {
	for _, tk := range tickets.LoadEpicAt(f.EpicDir()).Tickets {
		if tk.Type == string(schema.TypePRText) {
			all = append(all, tk)
		}
	}
	return all
}

func TestPRText_ValidResolutionLeavesADoneTicketAndLatestTextReturnsIt(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	head := servertest.GitOut(t, f.Origin.Local, "rev-parse", "HEAD")
	run := runPRText(t, f, "my-skill", []string{"Title: Ship it\nBody:\nDid the thing."})
	testutil.WaitFor(t, 30*time.Second, "pr-text ticket done", func() bool {
		_, _, ok := prflow.LatestText(f.EpicDir())
		return ok
	})
	title, body, _ := prflow.LatestText(f.EpicDir())
	if title != "Ship it" || body != "Did the thing." {
		t.Errorf("LatestText = %q %q", title, body)
	}
	if got := len(prTextTickets(f)); got != 1 {
		t.Errorf("pr-text tickets = %d, want 1", got)
	}
	if got := servertest.GitOut(t, f.Origin.Local, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD moved from %s to %s: the ticket must commit nothing", head, got)
	}
	if !strings.Contains(run.prompts[0], "gx-pr-text") {
		t.Errorf("prompt %q does not launch gx-pr-text", run.prompts[0])
	}
	if want := filepath.Join("epic-a"); !strings.HasSuffix(run.cwds[0], want) {
		t.Errorf("agent cwd = %q, want the feature worktree ending in %q", run.cwds[0], want)
	}
	if md := servertest.ReadFile(t, ticketFile(t, f, "02")); !strings.Contains(md, "Skill: my-skill") {
		t.Errorf("ticket does not name the skill:\n%s", md)
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); prstate.Of(e, prstate.DeliveryPR) != "awaiting-pr" {
		t.Errorf("epic state = %q, want awaiting-pr once pr-text is done", prstate.Of(e, prstate.DeliveryPR))
	}
}

func TestPRText_InvalidResolutionCancelsAndForksTheNext(t *testing.T) {
	cases := map[string]string{
		"empty body":    "Title: T\nBody:\n",
		"missing title": "Body:\nsomething",
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
			runPRText(t, f, "", []string{bad, "Title: Good\nBody:\nok"})
			testutil.WaitFor(t, 30*time.Second, "second try done", func() bool {
				_, _, ok := prflow.LatestText(f.EpicDir())
				return ok
			})
			all := prTextTickets(f)
			if len(all) != 2 || all[0].Status != string(schema.StatusCancelled) {
				t.Fatalf("pr-text tickets = %+v, want a cancelled one then a done one", all)
			}
			if kind := tickets.LoadEpicTicketMD(f.EpicDir()).MergeBlockKind; kind != "" {
				t.Errorf("land_block_kind = %q, want none", kind)
			}
		})
	}
}

func TestPRText_ThreeInvalidTriesStorePRTextFailedAndForkNoFourth(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	run := runPRText(t, f, "", []string{"Title: T\nBody:\n", "Body:\nx", "Title:\nBody:\ny"})
	testutil.WaitFor(t, 30*time.Second, "pr-text-failed stored", func() bool {
		return tickets.LoadEpicTicketMD(f.EpicDir()).MergeBlockKind == "pr-text-failed"
	})
	e := tickets.LoadEpicTicketMD(f.EpicDir())
	if !strings.Contains(e.MergeBlockReason, "no Title:") {
		t.Errorf("land_block_reason = %q, want the last reason (no Title:)", e.MergeBlockReason)
	}
	if got := len(prTextTickets(f)); got != 3 {
		t.Errorf("pr-text tickets = %d, want 3", got)
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if len(run.prompts) != 3 {
		t.Errorf("agent turns = %d, want 3", len(run.prompts))
	}
	if starts := f.H.Starts(); len(starts) == 0 {
		t.Error("no agent started")
	}
}
