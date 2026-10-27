package cmd

import (
	"bytes"
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
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "epic"))
	testutil.WriteFile(t, issues, "06-hello.md", showTicketFixture)
	return repo, project
}

func TestExecute_TicketsList_IsHiddenAliasOfShow(t *testing.T) {
	repo, _ := setupShowProject(t)

	for _, addr := range []string{"mine:epic", "mine:epic/06"} {
		want, err := runIn(t, repo, "tickets", "show", addr)
		if err != nil {
			t.Fatalf("show %s: %v", addr, err)
		}
		got, err := runIn(t, repo, "tickets", "list", addr)
		if err != nil {
			t.Fatalf("list %s: %v", addr, err)
		}
		if got != want {
			t.Errorf("list %s printed %q, want show output %q", addr, got, want)
		}
	}

	var help bytes.Buffer
	tickets, _, _ := newRootCmd(deps{}).Find([]string{"tickets"})
	tickets.SetOut(&help)
	if err := tickets.Help(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help.String(), "list") {
		t.Errorf("tickets help lists list:\n%s", help.String())
	}
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
	if m := decodeJSON(t, []byte(out)); m["via"] != "direct" || m["actor"] == nil {
		t.Errorf("via/actor = %v/%v, want direct/<actor>", m["via"], m["actor"])
	}
}

func TestExecute_TicketsShow_FollowsArchivedOneOff(t *testing.T) {
	repo, project := setupShowProject(t)
	archived := filepath.Join(project, tickets.ArchivedOneOffsEpic)
	if err := os.MkdirAll(filepath.Join(archived, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, archived)
	testutil.WriteFile(t, filepath.Join(archived, "issues"), "01-one-off.md",
		"---\nid: \"01\"\nstatus: done\ntype: prompt\n---\n\n# 01 — Moved\n\n## Resolution\n\n- original: mine:one-offs/07\n")

	out, err := runIn(t, repo, "tickets", "show", "mine:one-offs/07")
	if err != nil {
		t.Fatalf("show old address: %v", err)
	}
	if !strings.HasPrefix(out, "mine:one-offs-archived/01  ") {
		t.Errorf("printed %q, want the archived address", out)
	}
}

func TestExecute_TicketsShow_RendersWidenedBlocker(t *testing.T) {
	repo, project := setupShowProject(t)
	other := filepath.Join(project, "other", "issues")
	if err := os.MkdirAll(other, 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "other"))
	testutil.WriteFile(t, other, "01-code.md", "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# 01 — Code\n")
	testutil.WriteFile(t, other, "02-notes.md", "---\nid: \"02\"\nstatus: open\ntype: research\n---\n\n# 02 — Notes\n")
	testutil.WriteFile(t, filepath.Join(project, "epic", "issues"), "07-dep.md",
		"---\nid: \"07\"\nstatus: open\ntype: implement\nblocked_by: [06, other/01, other/02]\n---\n\n# 07 — Dep\n")

	out, err := runIn(t, repo, "tickets", "show", "mine:epic/07")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "blocked_by: 06, other (via other/01), other/02\n") {
		t.Errorf("show printed %q", out)
	}
}

func TestExecute_TicketsShow_EpicAddressPrintsEntryAndTickets(t *testing.T) {
	repo, project := setupShowProject(t)
	testutil.WriteFile(t, filepath.Join(project, "epic", "issues"), "07-second.md",
		"---\nid: \"07\"\nstatus: done\ntype: implement\n---\n\n# 07 — Second\n")
	entry, err := os.ReadFile(filepath.Join(project, "epic", "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}

	for _, addr := range []string{"mine:epic", "epic"} {
		out, err := runIn(t, repo, "tickets", "show", addr)
		if err != nil {
			t.Fatalf("show %s: %v", addr, err)
		}
		want := "mine:epic\npath: " + filepath.Join(project, "epic", "ticket.md") + "\n" + string(entry) +
			"\ntickets:\n06  open  Hello\n07  done  Second\n"
		if out != want {
			t.Errorf("show %s printed\n%s\nwant\n%s", addr, out, want)
		}
	}
}

func TestExecute_TicketsShow_SectionPrintsOnlyMatchingSections(t *testing.T) {
	repo, project := setupShowProject(t)
	issues := filepath.Join(project, "epic", "issues")
	testutil.WriteFile(t, issues, "07-second.md",
		"---\nid: \"07\"\nstatus: done\ntype: implement\n---\n\n# 07 — Second\n\n## What to build\n\nSecret.\n\n## Spec deviations\n\n- narrowed X\n")

	want := "mine:epic/07  Second\n- narrowed X\n\n"
	for _, args := range [][]string{
		{"tickets", "show", "epic", "--section", "Spec deviations"},
		{"tickets", "show", "mine:epic/07", "--section", "Spec deviations"},
	} {
		out, err := runIn(t, repo, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if out != want {
			t.Errorf("%v printed\n%q\nwant\n%q", args, out, want)
		}
	}

	out, err := runIn(t, repo, "tickets", "show", "mine:epic/06", "--section", "Spec deviations")
	if err == nil || !strings.Contains(err.Error(), "no section Spec deviations in mine:epic/06") || out != "" {
		t.Errorf("ticket without the section printed %q, err %v; want a missing-section error", out, err)
	}
	if _, err := runIn(t, repo, "tickets", "show", "mine:epic/07", "--section", "Spec deviations", "--json"); err == nil {
		t.Error("--section with --json should refuse")
	}
}

func TestExecute_TicketsShow_RefusesWithStableCode(t *testing.T) {
	repo, project := setupShowProject(t)

	// A ticket.md one level above the epic dirs must stay unreachable via "..".
	testutil.WriteFile(t, filepath.Dir(project), "ticket.md", "escaped\n")
	if err := os.MkdirAll(filepath.Join(project, ".archive"), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, ".archive"))

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"mine:epic/99"}, tickets.CodeUnknownTicket},
		{[]string{"mine:nope/06"}, tickets.CodeUnknownEpic},
		{[]string{"other:epic/06"}, tickets.CodeUnknownProject},
		{[]string{"06"}, tickets.CodeEpicRequired},
		{[]string{"mine:nope"}, tickets.CodeUnknownEpic},
		{[]string{"nope"}, tickets.CodeUnknownEpic},
		{[]string{"a/b/c"}, tickets.CodeMalformedAddress},
		{[]string{".."}, tickets.CodeUnknownEpic},
		{[]string{".archive"}, tickets.CodeUnknownEpic},
		{[]string{"mine:.archive"}, tickets.CodeUnknownEpic},
		{[]string{"epic", "--json"}, tickets.CodeEpicJSON},
		{[]string{"mine:epic", "--json"}, tickets.CodeEpicJSON},
	} {
		out, err := runIn(t, repo, append([]string{"tickets", "show"}, tc.args...)...)
		var ae *tickets.AddressError
		if !errors.As(err, &ae) || ae.Code != tc.code {
			t.Errorf("show %v err = %v, want code %s", tc.args, err, tc.code)
		}
		if out != "" {
			t.Errorf("show %v stdout = %q, want empty", tc.args, out)
		}
	}
}

func TestExecute_TicketsShow_CrossProject(t *testing.T) {
	repo, project := setupShowProject(t)
	theirs := addProject(t, filepath.Dir(project), "theirs", testutil.TempRepo(t))
	if err := os.MkdirAll(filepath.Join(theirs, "epic", "issues"), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(theirs, "epic"))
	testutil.WriteFile(t, filepath.Join(theirs, "epic", "issues"), "03-remote.md",
		"---\nid: \"03\"\nstatus: open\ntype: implement\n---\n\n# 03 — Remote\n\nremote body\n")

	out, err := runIn(t, repo, "tickets", "show", "theirs:epic/03")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.HasPrefix(out, "theirs:epic/03  Remote\n") || !strings.Contains(out, "remote body") {
		t.Errorf("show printed %q", out)
	}

	out, err = runIn(t, repo, "tickets", "show", "theirs:epic")
	if err != nil {
		t.Fatalf("show theirs:epic: %v", err)
	}
	if !strings.HasPrefix(out, "theirs:epic\npath: "+filepath.Join(theirs, "epic", "ticket.md")) ||
		!strings.HasSuffix(out, "\ntickets:\n03  open  Remote\n") {
		t.Errorf("show theirs:epic printed %q", out)
	}

	// A prefix that is a directory of the store but not a project is unknown too.
	if err := os.MkdirAll(filepath.Join(filepath.Dir(project), "plain", "epic", "issues"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"nope:epic/03", "plain:epic/03"} {
		_, err = runIn(t, repo, "tickets", "show", addr)
		var ae *tickets.AddressError
		if !errors.As(err, &ae) || ae.Code != tickets.CodeUnknownProject {
			t.Errorf("show %s err = %v, want %s", addr, err, tickets.CodeUnknownProject)
		}
	}
}
