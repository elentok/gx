package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestExecute_TicketsSet_AddressWritesSameFileAsPath(t *testing.T) {
	repo, project := setupShowProject(t)
	path := filepath.Join(project, "epic", "issues", "06-hello.md")

	out, err := runIn(t, repo, "tickets", "set", "mine:epic/06", "--status", "draft")
	if err != nil {
		t.Fatalf("set by address: %v", err)
	}
	if want := "mine:epic/06: updated (status=draft)\n"; out != want {
		t.Errorf("set by address printed %q, want %q", out, want)
	}
	byAddr, _ := os.ReadFile(path)

	if err := os.WriteFile(path, []byte(showTicketFixture), 0644); err != nil {
		t.Fatal(err)
	}
	out, err = runIn(t, repo, "tickets", "set", path, "--status", "draft")
	if err != nil {
		t.Fatalf("set by path: %v", err)
	}
	if want := "mine:epic/06: updated (status=draft)\n"; out != want {
		t.Errorf("set by path printed %q, want %q", out, want)
	}
	byPath, _ := os.ReadFile(path)
	if string(byAddr) != string(byPath) {
		t.Errorf("address write differs from path write:\n%s\n---\n%s", byAddr, byPath)
	}
}

func TestExecute_TicketsSet_UnpaddedIDResolvesPaddedTicket(t *testing.T) {
	repo, _ := setupShowProject(t)

	out, err := runIn(t, repo, "tickets", "set", "mine:epic/6", "--status", "draft")
	if err != nil {
		t.Fatalf("set unpadded: %v", err)
	}
	if want := "mine:epic/06: updated (status=draft)\n"; out != want {
		t.Errorf("set printed %q, want %q", out, want)
	}
}

func TestExecute_TicketsSet_UnknownAddressRefused(t *testing.T) {
	repo, _ := setupShowProject(t)

	_, err := runIn(t, repo, "tickets", "set", "mine:epic/99", "--status", "draft")
	if err == nil || !strings.Contains(err.Error(), "unknown-ticket") {
		t.Errorf("err = %v, want unknown-ticket", err)
	}
}

// setupOtherProject registers a second project "theirs" holding theirs:epic/03.
func setupOtherProject(t *testing.T, project string) string {
	t.Helper()
	theirs := addProject(t, filepath.Dir(project), "theirs", testutil.TempRepo(t))
	issues := filepath.Join(theirs, "epic", "issues")
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(theirs, "epic"))
	testutil.WriteFile(t, issues, "03-remote.md",
		"---\nid: \"03\"\nstatus: open\ntype: implement\n---\n\n# 03 — Remote\n\nremote body\n")
	return filepath.Join(issues, "03-remote.md")
}

func TestExecute_TicketsSet_CrossProject(t *testing.T) {
	repo, project := setupShowProject(t)
	path := setupOtherProject(t, project)

	out, err := runIn(t, repo, "tickets", "set", "theirs:epic/03", "--status", "draft")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if want := "theirs:epic/03: updated (status=draft)\n"; out != want {
		t.Errorf("set printed %q, want %q", out, want)
	}
	if got, _ := os.ReadFile(path); !strings.Contains(string(got), "status: draft") {
		t.Errorf("remote ticket not updated:\n%s", got)
	}
}

func TestExecute_TicketsSection_CrossProject(t *testing.T) {
	repo, project := setupShowProject(t)
	path := setupOtherProject(t, project)

	if _, err := runIn(t, repo, "tickets", "section", "theirs:epic/03", "Notes", "seen again"); err != nil {
		t.Fatalf("section: %v", err)
	}
	if got, _ := os.ReadFile(path); !strings.Contains(string(got), "## Notes\n\nseen again") {
		t.Errorf("remote ticket has no section:\n%s", got)
	}
}

func TestExecute_TicketsSection_ContentStartingWithDash(t *testing.T) {
	repo, project := setupShowProject(t)
	path := setupOtherProject(t, project)

	if _, err := runIn(t, repo, "tickets", "section", "theirs:epic/03", "Notes", "- first\n- second"); err != nil {
		t.Fatalf("section: %v", err)
	}
	if got, _ := os.ReadFile(path); !strings.Contains(string(got), "## Notes\n\n- first\n- second") {
		t.Errorf("list content not written:\n%s", got)
	}
}

func TestExecute_TicketsSetAndSection_UnknownProjectRefusedWritesNothing(t *testing.T) {
	repo, project := setupShowProject(t)
	path := setupOtherProject(t, project)
	before, _ := os.ReadFile(path)

	for _, args := range [][]string{
		{"tickets", "set", "nope:epic/03", "--status", "draft"},
		{"tickets", "section", "nope:epic/03", "Notes", "x"},
	} {
		_, err := runIn(t, repo, args...)
		var ae *tickets.AddressError
		if !errors.As(err, &ae) || ae.Code != tickets.CodeUnknownProject {
			t.Errorf("%v err = %v, want %s", args[1], err, tickets.CodeUnknownProject)
		}
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("refused command wrote:\n%s", after)
	}
}

func TestExecute_TicketsValidate_AcceptsAddressAndPrintsIt(t *testing.T) {
	repo, _ := setupShowProject(t)

	out, err := runIn(t, repo, "tickets", "validate", "epic/06")
	if err != nil || !strings.HasPrefix(out, "mine:epic/06: valid ticket") {
		t.Errorf("validate = %q, %v", out, err)
	}
}
