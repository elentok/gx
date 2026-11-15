package ticketmove

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/tickettest"
)

func startDesignFixture(t *testing.T, statuses ...string) StartDesignOpts {
	t.Helper()
	proj := filepath.Join(t.TempDir(), "proj")
	tickettest.WriteFile(t, filepath.Join(proj, "project.json"), "{}")
	tickettest.WriteFile(t, filepath.Join(proj, "backlog", "ticket.md"), "---\nstatus: open\nkind: backlog\n---\n\n# backlog\n")
	var ids []string
	for i, st := range statuses {
		id := fmt.Sprintf("%02d", i+1)
		ids = append(ids, id)
		tickettest.WriteFile(t, filepath.Join(proj, "backlog", "issues", id+"-t.md"),
			"---\nid: \""+id+"\"\nstatus: "+st+"\ntype: implement\noriginal_address: p:follow-ups/"+id+"\n---\n\n# "+id+"\n")
	}
	return StartDesignOpts{
		ConversationsDir: t.TempDir(), ProjectDir: proj, Project: "proj", Backlog: "backlog",
		Map: "theme", Title: "Theme", IDs: ids,
		Active: func(tickets.Address) error { return nil },
	}
}

func TestStartDesign_RefusalWritesNothing(t *testing.T) {
	o := startDesignFixture(t, "open", "claimed", "open")
	_, err := StartDesign(o)
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeMoveStatus {
		t.Fatalf("err = %v, want %s", err, tickets.CodeMoveStatus)
	}
	if _, err := os.Stat(filepath.Join(o.ProjectDir, "theme")); !os.IsNotExist(err) {
		t.Errorf("map was created: %v", err)
	}
	if got := len(tickets.LoadEpic(o.ProjectDir, "backlog").Tickets); got != 3 {
		t.Errorf("backlog holds %d tickets, want 3", got)
	}
}

func TestStartDesign_CreatesChartingMap(t *testing.T) {
	o := startDesignFixture(t, "open", "open", "open")
	o.IDs = o.IDs[:2]
	if _, err := StartDesign(o); err != nil {
		t.Fatal(err)
	}
	m := tickets.LoadEpic(o.ProjectDir, "theme")
	if !m.IsMap || !m.DestinationUnconfirmed || m.DesignStep != "" || len(m.Tickets) != 2 {
		t.Fatalf("map = %+v", m)
	}
	if strings.Contains(m.Body, "Triaged from follow-ups") {
		t.Errorf("notes carry the old-epic phrase: %s", m.Body)
	}
	backlog, ids, ok := ParseStartDesignNotes(m.Body)
	if !ok || backlog != "backlog" || strings.Join(ids, ",") != "01,02" {
		t.Errorf("notes parsed to %q %v %v", backlog, ids, ok)
	}
	b, err := os.ReadFile(m.Tickets[0].Path)
	if err != nil || !strings.Contains(string(b), "original_address: p:follow-ups/01") {
		t.Errorf("original_address lost: %s, %v", b, err)
	}
	if left := tickets.LoadEpic(o.ProjectDir, "backlog").Tickets; len(left) != 1 {
		t.Errorf("backlog holds %d tickets, want 1", len(left))
	}
}

func TestStartDesign_RerunAfterFailure(t *testing.T) {
	for _, withList := range []bool{true, false} {
		t.Run(fmt.Sprintf("list=%v", withList), func(t *testing.T) {
			o := startDesignFixture(t, "open", "open", "open")
			all := o.IDs
			fail := true
			o.Active = func(a tickets.Address) error {
				if _, err := os.Stat(filepath.Join(o.ProjectDir, "theme")); fail && err == nil && a.ID == "02" {
					return errors.New("boom")
				}
				return nil
			}
			if _, err := StartDesign(o); err == nil {
				t.Fatal("want the forced failure")
			}
			if got := len(tickets.LoadEpic(o.ProjectDir, "theme").Tickets); got != 1 {
				t.Fatalf("map holds %d tickets after the failure, want 1", got)
			}
			fail = false
			o.IDs = nil
			if withList {
				o.IDs = all
			}
			if _, err := StartDesign(o); err != nil {
				t.Fatal(err)
			}
			if got := len(tickets.LoadEpic(o.ProjectDir, "theme").Tickets); got != 3 {
				t.Errorf("map holds %d tickets, want 3", got)
			}
			if got := len(tickets.LoadEpic(o.ProjectDir, "backlog").Tickets); got != 0 {
				t.Errorf("backlog holds %d tickets, want 0", got)
			}
		})
	}
}

func TestStartDesign_RefusesForeignExistingEpic(t *testing.T) {
	o := startDesignFixture(t, "open")
	tickettest.WriteFile(t, filepath.Join(o.ProjectDir, "theme", "ticket.md"), "---\nstatus: open\nkind: map\n---\n\n# theme\n")
	if _, err := StartDesign(o); err == nil {
		t.Fatal("want a refusal: the map does not name this backlog")
	}
}

func TestParseStartDesignNotes(t *testing.T) {
	body := "# m\n\n## Notes\n\n" + StartDesignNotes("p", "a", []string{"03", "04a"})
	backlog, ids, ok := ParseStartDesignNotes(body)
	if !ok || backlog != "a" || strings.Join(ids, ",") != "03,04a" {
		t.Errorf("got %q %v %v", backlog, ids, ok)
	}
	if _, _, ok := ParseStartDesignNotes("# m\n\n## Notes\n\nnothing\n"); ok {
		t.Error("notes without a backlog line parsed")
	}
}
