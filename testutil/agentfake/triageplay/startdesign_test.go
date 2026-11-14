package triageplay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/triage"
)

// scriptGx is a gx binary that logs every call and answers the verbs the two
// players read from the files named in the environment.
const scriptGx = `#!/bin/sh
echo "$@" >> "$LOG"
case "$*" in
  *"tickets root"*) echo "$ROOT" ;;
  *"epic next-name"*) echo "p:retry-work" ;;
  *"tickets start-design"*) echo "p:retry-work" ;;
  *"server web-url"*) echo '{"url":"http://web.test/"}' ;;
  *"triage oldepics"*) cat "$OLDEPICS" ;;
  *"triage show"*) cat "$SHOW" ;;
esac
`

// scriptEnv is a player's scratch: the gx stub, its log and a project dir with
// an area of n open tickets.
type scriptEnv struct {
	gx, dir, log string
	env          []string
}

func newScript(t *testing.T, area string, open int) *scriptEnv {
	t.Helper()
	dir := t.TempDir()
	s := &scriptEnv{gx: filepath.Join(dir, "gx"), dir: dir, log: filepath.Join(dir, "log")}
	if err := os.WriteFile(s.gx, []byte(scriptGx), 0o755); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(dir, "store")
	issues := filepath.Join(store, "p", area, "issues")
	if err := os.MkdirAll(issues, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "p", area, "ticket.md"), []byte("---\nstatus: open\nkind: area\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= open; i++ {
		id := fmt.Sprintf("%02d", i)
		body := "---\nid: \"" + id + "\"\nstatus: open\ntype: implement\n---\n\n# t-" + id + "\n"
		if err := os.WriteFile(filepath.Join(issues, id+"-t-"+id+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.env = []string{"LOG=" + s.log, "ROOT=" + filepath.Join(store, "p")}
	return s
}

func (s *scriptEnv) calls(t *testing.T) string {
	t.Helper()
	raw, _ := os.ReadFile(s.log)
	return string(raw)
}

func TestDesignPlayer_ProposesAllOfASmallAreaAndTheThemeOfALargeOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ open, want int }{{5, 5}, {11, 11}, {20, maxDesignTickets}} {
		s := newScript(t, "sync", tc.open)
		p := DesignPlayer{Gx: s.gx, Env: s.env, Dir: s.dir}
		card, text, err := p.Propose(StartDesignPromptPrefix + "p:sync")
		if err != nil {
			t.Fatal(err)
		}
		if len(card.Tickets) != tc.want {
			t.Errorf("%d open: card has %d tickets, want %d", tc.open, len(card.Tickets), tc.want)
		}
		if !strings.Contains(text, "Start this Design?") || !strings.Contains(text, "type: yes-no") || !strings.Contains(text, "Map: retry-work") {
			t.Errorf("card text = %q, want the question, its yes-no block and the map name", text)
		}
	}
}

func TestDesignPlayer_AnEditReprintsOnceAndYesCreatesAndLinks(t *testing.T) {
	t.Parallel()
	s := newScript(t, "sync", 5)
	p := DesignPlayer{Gx: s.gx, Env: s.env, Dir: s.dir}
	card, _, err := p.Propose(StartDesignPromptPrefix + "p:sync")
	if err != nil {
		t.Fatal(err)
	}
	text, err := p.React(card, "drop 2")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(text, "Start this Design?"); got != 1 || len(card.Tickets) != 4 {
		t.Errorf("after the edit: %d cards, %d tickets, want one card of 4", got, len(card.Tickets))
	}
	if strings.Contains(s.calls(t), "start-design") {
		t.Error("an edit ran the create verb")
	}
	text, err = p.React(card, "yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.calls(t), "tickets start-design p:sync --map retry-work --title Tidy the sync area --ticket 01,03,04,05") {
		t.Errorf("calls = %q, want the create verb with the four ids", s.calls(t))
	}
	if want := "http://web.test/#/d/p:retry-work"; !strings.Contains(text, want) {
		t.Errorf("reply = %q, want the link %q", text, want)
	}
}

func TestTriagePlayer_RouteListsTheOldEpicMoveAndTheWaitingEpic(t *testing.T) {
	t.Parallel()
	s := newScript(t, "tooling", 0)
	oldEpics := filepath.Join(s.dir, "oldepics.json")
	body := `[{"epic":"test-flakes","area":"test-health","move":["p:test-flakes/01"],"claimed":[]},` +
		`{"epic":"stuck","area":"","move":[],"waiting":true,"claimed":["p:stuck/01"]}]`
	if err := os.WriteFile(oldEpics, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	show := filepath.Join(s.dir, "show.json")
	if err := os.WriteFile(show, []byte(`{"next":"questions","step":"route"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Player{Gx: s.gx, Env: append(s.env, "OLDEPICS="+oldEpics, "SHOW="+show), Project: "p", Dir: s.dir}
	lines, err := p.oldEpicMoves(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"move test-flakes -> area test-health: 1 tickets", "waiting: stuck holds claimed p:stuck/01"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("lines = %q, want the move and the waiting line", lines)
	}
	calls := s.calls(t)
	if !strings.Contains(calls, "triage area --project p --name test-health --source "+triage.AreaTable) || !strings.Contains(calls, "triage moves --project p") {
		t.Errorf("calls = %q, want the table area saved and the move unit", calls)
	}
}
