package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

const showTicketFixture = "---\nid: \"06\"\nstatus: open\ntype: implement\n---\n\n# 06 — Hello\n\nbody text\n"

func setupShowProject(t *testing.T) (repo, project string) {
	t.Helper()
	store := isolateTicketStore(t)
	repo = testutil.TempRepo(t)
	project = addProject(t, store, "mine", repo)
	issues := filepath.Join(project, "epic", "issues")
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, issues, "06-hello.md", showTicketFixture)
	return repo, project
}

func TestExecute_TicketsShow_FullAndShortAddressesPrintCanonical(t *testing.T) {
	repo, _ := setupShowProject(t)

	if out, err := exec.Command("git", "-C", repo, "checkout", "-q", "-b", "ralph-loop/epic-item-07").CombinedOutput(); err != nil {
		t.Fatalf("checkout: %v\n%s", err, out)
	}

	for _, tc := range []struct{ cwd, addr string }{
		{repo, "mine:epic/06"},
		{repo, "epic/06"},
		{repo, "06"},
	} {
		out, err := runIn(t, tc.cwd, "tickets", "show", tc.addr)
		if err != nil {
			t.Fatalf("show %s: %v", tc.addr, err)
		}
		if !strings.HasPrefix(out, "mine:epic/06  Hello\n") || !strings.Contains(out, "body text") {
			t.Errorf("show %s printed %q", tc.addr, out)
		}
	}

	out, err := runIn(t, repo, "tickets", "show", "epic/06", "--json")
	if err != nil {
		t.Fatalf("show --json: %v", err)
	}
	var got ticketShowJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if got.Address != "mine:epic/06" || got.Status != "open" || got.Type != "implement" {
		t.Errorf("json = %+v", got)
	}
}

func TestExecute_TicketsShow_RefusesWithStableCode(t *testing.T) {
	repo, _ := setupShowProject(t)

	for addr, code := range map[string]string{
		"mine:epic/99":  tickets.CodeUnknownTicket,
		"mine:nope/06":  tickets.CodeUnknownEpic,
		"other:epic/06": tickets.CodeUnknownProject,
		"06":            tickets.CodeEpicRequired,
		"garbage":       tickets.CodeMalformedAddress,
	} {
		out, err := runIn(t, repo, "tickets", "show", addr)
		var ae *tickets.AddressError
		if !errors.As(err, &ae) || ae.Code != code {
			t.Errorf("show %s err = %v, want code %s", addr, err, code)
		}
		if out != "" {
			t.Errorf("show %s stdout = %q, want empty", addr, out)
		}
	}
}
