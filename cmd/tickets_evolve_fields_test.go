package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validateTicket(t *testing.T, frontmatter string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-a-ticket.md")
	writeTicketFile(t, path, "---\nid: \"01\"\n"+frontmatter+"---\nBody.\n")
	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	return execute([]string{"tickets", "validate", path}, d)
}

func TestExecute_TicketsValidate_Category(t *testing.T) {
	t.Parallel()
	for _, c := range []string{"bug", "idea"} {
		if err := validateTicket(t, "status: open\ntype: implement\ncategory: "+c+"\n"); err != nil {
			t.Errorf("category %s: %v", c, err)
		}
	}
	err := validateTicket(t, "status: open\ntype: implement\ncategory: chore\n")
	if err == nil || !strings.Contains(err.Error(), "category") {
		t.Errorf("bad category: err = %v, want one naming category", err)
	}
}

func TestExecute_TicketsValidate_EvolveType(t *testing.T) {
	t.Parallel()
	if err := validateTicket(t, "status: open\ntype: evolve\n"); err != nil {
		t.Errorf("evolve ticket: %v", err)
	}
	err := validateTicket(t, "status: open\ntype: evolve\nparent: \"02\"\n")
	if err == nil || !strings.Contains(err.Error(), "parent") {
		t.Errorf("evolve with parent: err = %v, want one naming parent", err)
	}
}

func TestExecute_TicketsValidate_ExportedAt(t *testing.T) {
	t.Parallel()
	if err := validateTicket(t, "status: open\ntype: implement\nexported_at: 2026-01-02T03:04:05Z\n"); err != nil {
		t.Errorf("exported_at: %v", err)
	}
}

func TestExecute_TicketsValidate_EpicEvolveFields(t *testing.T) {
	t.Parallel()
	validate := func(epicFM string) error {
		epicDir := filepath.Join(t.TempDir(), "proj", "epic")
		path := filepath.Join(epicDir, "issues", "01-a.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(epicDir, "ticket.md"), []byte("---\nstatus: open\n"+epicFM+"---\n\n# epic\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
		return execute([]string{"tickets", "validate", path}, d)
	}

	good := "evolved_at: 2026-01-02T03:04:05Z\nevolve_skipped: true\nevolve_ticket: gx:epic/03\ndestination_unconfirmed: false\n"
	if err := validate(good); err != nil {
		t.Errorf("valid epic fields: %v", err)
	}
	for _, field := range []string{"evolved_at", "evolve_skipped", "destination_unconfirmed"} {
		err := validate(field + ": not-a-value\n")
		if err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("bad %s: err = %v, want one naming the field", field, err)
		}
	}
}

func TestExecute_TicketsSet_RefusesExportedAt(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "01-a-ticket.md")
	const content = "---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n"
	writeTicketFile(t, path, content)

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}
	err := execute([]string{"tickets", "set", path, "--exported-at=2026-01-02T03:04:05Z"}, d)
	if err == nil || !strings.Contains(err.Error(), "exported_at") {
		t.Fatalf("err = %v, want a refusal naming exported_at", err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != content {
		t.Errorf("ticket changed: %q", raw)
	}
}
