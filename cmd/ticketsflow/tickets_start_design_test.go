package ticketsflow

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestStartDesign_RunsAndRefuses(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repo)
	dirs := config.DirsFrom(envFor(t))
	if err := runTicketsEpicAdd(dirs, repo, "mine:backlog", "backlog", "open", "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"01", "02"} {
		writeTicketFile(t, filepath.Join(project, "backlog", "issues", id+"-t.md"),
			"---\nid: \""+id+"\"\nstatus: open\ntype: implement\n---\n\n# "+id+"\n")
	}
	ids := []string{"01", "02"}

	refuse := func(tickets.Address) error { return errors.New("queued") }
	if err := runTicketsStartDesign(dirs, repo, "mine:backlog", "theme", "Theme", ids, refuse, &bytes.Buffer{}); err == nil {
		t.Fatal("want the active refusal")
	}
	if _, err := os.Stat(filepath.Join(project, "theme")); !os.IsNotExist(err) {
		t.Fatalf("map created on a refusal: %v", err)
	}

	var out bytes.Buffer
	ok := func(tickets.Address) error { return nil }
	if err := runTicketsStartDesign(dirs, repo, "mine:backlog", "theme", "Theme", ids, ok, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "mine:theme\n" {
		t.Errorf("printed %q", got)
	}
	raw, err := os.ReadFile(filepath.Join(project, "theme", "ticket.md"))
	if err != nil || !strings.Contains(string(raw), "kind: map") || !strings.Contains(string(raw), "destination_unconfirmed: true") {
		t.Errorf("map ticket.md = %s, %v", raw, err)
	}
	if got := len(tickets.LoadEpic(project, "theme").Tickets); got != 2 {
		t.Errorf("map holds %d tickets, want 2", got)
	}
}
