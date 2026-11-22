package tickets

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestExecute_TicketsList_IsHiddenAliasOfShow(t *testing.T) {
	repo, _ := cmdtest.SetupShowProject(t)

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
	tk := New(testDeps(t, nil, nil, nil))
	tk.SetOut(&help)
	if err := tk.Help(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help.String(), "list") {
		t.Errorf("tickets help lists list:\n%s", help.String())
	}
}

func TestExecute_TicketsShow_BriefListsTicketsOnly(t *testing.T) {
	repo, _ := cmdtest.SetupShowProject(t)

	full, err := runIn(t, repo, "tickets", "show", "mine:epic")
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"show", "list"} {
		out, err := runIn(t, repo, "tickets", cmd, "mine:epic", "--brief")
		if err != nil {
			t.Fatalf("%s --brief: %v", cmd, err)
		}
		if strings.Contains(out, "tickets:") || strings.Contains(out, "path:") {
			t.Errorf("%s --brief printed the epic body:\n%s", cmd, out)
		}
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		if got, want := len(lines), strings.Count(full[strings.Index(full, "tickets:\n"):], "\n")-1; got != want {
			t.Errorf("%s --brief printed %d lines, want %d:\n%s", cmd, got, want, out)
		}
		if !strings.Contains(out, "mine:epic/06  open  implement  Hello\n") {
			t.Errorf("%s --brief output = %q", cmd, out)
		}
		for _, l := range lines {
			if !strings.HasPrefix(l, "mine:epic/") {
				t.Errorf("non-ticket line %q", l)
			}
		}
	}

	for _, args := range [][]string{
		{"tickets", "show", "mine:epic", "--brief", "--json"},
		{"tickets", "show", "mine:epic", "--brief", "--section", "x"},
		{"tickets", "show", "mine:epic/06", "--brief"},
	} {
		if _, err := runIn(t, repo, args...); err == nil {
			t.Errorf("%v: want an error", args)
		}
	}
}

func TestExecute_TicketsShow_PrintsAgentWhenSet(t *testing.T) {
	repo, project := cmdtest.SetupShowProject(t)
	testutil.WriteFile(t, filepath.Join(project, "epic", "issues"), "07-pinned.md",
		"---\nid: \"07\"\nstatus: open\ntype: implement\nagent: codex\n---\n\n# 07 — Pinned\n\nbody\n")

	out, err := runIn(t, repo, "tickets", "show", "mine:epic/07")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\nagent: codex\n") {
		t.Errorf("text output has no agent line:\n%s", out)
	}
	out, err = runIn(t, repo, "tickets", "show", "mine:epic/07", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if m := cmdtest.DecodeJSON(t, []byte(out)); m["agent"] != "codex" {
		t.Errorf("json agent = %v, want codex", m["agent"])
	}

	out, err = runIn(t, repo, "tickets", "show", "mine:epic/06")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "agent:") {
		t.Errorf("ticket without agent printed an agent line:\n%s", out)
	}
}

func TestExecute_TicketsShow_FullAndShortAddressesPrintCanonical(t *testing.T) {
	repo, _ := cmdtest.SetupShowProject(t)

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
	if m := cmdtest.DecodeJSON(t, []byte(out)); m["via"] != "direct" || m["actor"] == nil {
		t.Errorf("via/actor = %v/%v, want direct/<actor>", m["via"], m["actor"])
	}
}

func TestExecute_TicketsShow_UnpaddedSubIDResolvesPaddedTicket(t *testing.T) {
	repo, project := cmdtest.SetupShowProject(t)
	testutil.WriteFile(t, filepath.Join(project, "epic", "issues"), "06a-sub.md",
		strings.Replace(showTicketFixture, `id: "06"`, `id: "06a"`, 1))

	out, err := runIn(t, repo, "tickets", "show", "epic/6a")
	if err != nil {
		t.Fatalf("show epic/6a: %v", err)
	}
	if !strings.HasPrefix(out, "mine:epic/06a  ") {
		t.Errorf("show epic/6a printed %q", out)
	}
}

func TestExecute_TicketsShow_FollowsArchivedOneOff(t *testing.T) {
	repo, project := cmdtest.SetupShowProject(t)
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
	repo, project := cmdtest.SetupShowProject(t)
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
	repo, project := cmdtest.SetupShowProject(t)
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
	repo, project := cmdtest.SetupShowProject(t)
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

func TestExecute_TicketsShow_OptionalSectionMissingPrintsNothing(t *testing.T) {
	repo, project := cmdtest.SetupShowProject(t)
	testutil.WriteFile(t, filepath.Join(project, "epic", "issues"), "07-second.md",
		"---\nid: \"07\"\nstatus: done\ntype: implement\n---\n\n# 07 — Second\n\n## Iteration summaries\n\n- did X\n")

	out, err := runIn(t, repo, "tickets", "show", "mine:epic/06", "--section", "Iteration summaries")
	if err != nil || out != "" {
		t.Errorf("missing optional section printed %q, err %v; want nothing and no error", out, err)
	}
	out, err = runIn(t, repo, "tickets", "show", "mine:epic/07", "--section", "Iteration summaries")
	if err != nil || out != "mine:epic/07  Second\n- did X\n\n" {
		t.Errorf("present optional section printed %q, err %v", out, err)
	}
}

func TestExecute_TicketsShowAndSection_EpicOwnSection(t *testing.T) {
	repo, project := cmdtest.SetupShowProject(t)
	testutil.WriteFile(t, filepath.Join(project, "epic"), "ticket.md",
		"---\nstatus: open\n---\n\n# Epic\n\n## Deferred\n\n- later\n\n## Other\n\nx\n")
	testutil.WriteFile(t, filepath.Join(project, "epic", "issues"), "07-second.md",
		"---\nid: \"07\"\nstatus: done\ntype: implement\n---\n\n# 07 — Second\n\n## Deferred\n\n- ticket-level\n")

	for _, verb := range [][]string{{"show", "epic", "--section", "Deferred"}, {"section", "epic", "Deferred"}} {
		out, err := runIn(t, repo, append([]string{"tickets"}, verb...)...)
		want := "- later\n\nmine:epic/07  Second\n- ticket-level\n\n"
		if err != nil || out != want {
			t.Errorf("%v printed %q, err %v; want %q", verb, out, err, want)
		}
	}

	for _, verb := range [][]string{{"show", "epic", "--section", "Nope"}, {"section", "epic", "Nope"}} {
		out, err := runIn(t, repo, append([]string{"tickets"}, verb...)...)
		if err == nil || !strings.Contains(err.Error(), "no section Nope in epic epic") || out != "" {
			t.Errorf("%v printed %q, err %v; want a missing-section error", verb, out, err)
		}
	}

	out, err := runIn(t, repo, "tickets", "show", "epic", "--section", "Iteration summaries")
	if err != nil || out != "" {
		t.Errorf("missing optional section printed %q, err %v; want nothing and no error", out, err)
	}
}

func TestExecute_TicketsShow_SectionTicketsFilterNarrowsEpic(t *testing.T) {
	repo, project := cmdtest.SetupShowProject(t)
	issues := filepath.Join(project, "epic", "issues")
	for _, id := range []string{"07", "08", "09", "10"} {
		testutil.WriteFile(t, issues, id+"-t"+id+".md",
			"---\nid: \""+id+"\"\nstatus: done\ntype: implement\n---\n\n# "+id+" — T"+id+"\n\n## Spec deviations\n\n- note "+id+"\n")
	}

	out, err := runIn(t, repo, "tickets", "show", "epic", "--section", "Spec deviations", "--tickets", "8-9,10")
	if err != nil {
		t.Fatal(err)
	}
	want := "mine:epic/08  T08\n- note 08\n\nmine:epic/09  T09\n- note 09\n\nmine:epic/10  T10\n- note 10\n\n"
	if out != want {
		t.Errorf("printed\n%q\nwant\n%q", out, want)
	}

	for _, bad := range []string{"x", "9-8", "1-"} {
		if _, err := runIn(t, repo, "tickets", "show", "epic", "--section", "Spec deviations", "--tickets", bad); err == nil {
			t.Errorf("--tickets %q should refuse", bad)
		}
	}
	if _, err := runIn(t, repo, "tickets", "show", "epic", "--tickets", "8"); err == nil {
		t.Error("--tickets without --section should refuse")
	}
}

func TestExecute_TicketsShow_ExistsReportsASectionByExitCode(t *testing.T) {
	repo, project := cmdtest.SetupShowProject(t)
	issues := filepath.Join(project, "epic", "issues")
	for _, id := range []string{"07", "08"} {
		testutil.WriteFile(t, issues, id+"-t"+id+".md",
			"---\nid: \""+id+"\"\nstatus: done\ntype: implement\n---\n\n# "+id+" — T"+id+"\n\n## Spec deviations\n\n- note "+id+"\n")
	}

	// exit is the code `gx tickets show` would end with: 0 for nil, the ExitError code, -1 for any other error.
	exit := func(args ...string) (int, string) {
		out, err := runIn(t, repo, append([]string{"tickets", "show"}, args...)...)
		var ee *cmdbase.ExitError
		switch {
		case err == nil:
			return 0, out
		case errors.As(err, &ee):
			return ee.Code, out
		}
		return -1, out
	}
	for _, c := range []struct {
		name string
		args []string
		want int
	}{
		{"ticket with the section", []string{"mine:epic/07", "--section", "Spec deviations", "--exists"}, 0},
		{"ticket without the section", []string{"mine:epic/06", "--section", "Spec deviations", "--exists"}, 1},
		{"epic with the section somewhere", []string{"epic", "--section", "Spec deviations", "--exists"}, 0},
		{"epic without the section anywhere", []string{"epic", "--section", "Nope", "--exists"}, 1},
		{"epic, filter hits a ticket with it", []string{"epic", "--section", "Spec deviations", "--tickets", "8", "--exists"}, 0},
		{"epic, filter misses every ticket with it", []string{"epic", "--section", "Spec deviations", "--tickets", "6", "--exists"}, 1},
		{"no --section", []string{"mine:epic/07", "--exists"}, -1},
		{"with --json", []string{"mine:epic/07", "--section", "Spec deviations", "--exists", "--json"}, -1},
		{"with --brief", []string{"epic", "--section", "Spec deviations", "--exists", "--brief"}, -1},
		{"bad address", []string{"mine:epic/99", "--section", "Spec deviations", "--exists"}, -1},
	} {
		if got, out := exit(c.args...); got != c.want || (c.want >= 0 && out != "") {
			t.Errorf("%s: exit %d, output %q; want exit %d and no output", c.name, got, out, c.want)
		}
	}
}

func TestExecute_TicketsShow_RefusesWithStableCode(t *testing.T) {
	repo, project := cmdtest.SetupShowProject(t)

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
	repo, project := cmdtest.SetupShowProject(t)
	theirs := cmdtest.AddProject(t, filepath.Dir(project), "theirs", testutil.TempRepo(t))
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
