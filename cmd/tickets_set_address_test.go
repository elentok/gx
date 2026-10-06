package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestExecute_TicketsSet_UnknownAddressRefused(t *testing.T) {
	repo, _ := setupShowProject(t)

	_, err := runIn(t, repo, "tickets", "set", "mine:epic/99", "--status", "draft")
	if err == nil || !strings.Contains(err.Error(), "unknown-ticket") {
		t.Errorf("err = %v, want unknown-ticket", err)
	}
}

func TestExecute_TicketsValidate_AcceptsAddressAndPrintsIt(t *testing.T) {
	repo, _ := setupShowProject(t)

	out, err := runIn(t, repo, "tickets", "validate", "epic/06")
	if err != nil || !strings.HasPrefix(out, "mine:epic/06: valid ticket") {
		t.Errorf("validate = %q, %v", out, err)
	}
}
