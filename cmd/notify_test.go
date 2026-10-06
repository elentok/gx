package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestExecute_Notify_Enable_ClearsTransportMuteAndReportsIt(t *testing.T) {
	repo := testutil.TempRepo(t)
	storeProjectFor(t, repo)

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	if err := execute([]string{"notify", "--disable", "telegram"}, d); err != nil {
		t.Fatalf("execute notify --disable: %v", err)
	}
	stdout.Reset()

	if err := execute([]string{"notify", "--enable", "telegram"}, d); err != nil {
		t.Fatalf("execute notify --enable: %v", err)
	}
	if !strings.Contains(stdout.String(), "telegram: active") {
		t.Fatalf("expected active report, got: %q", stdout.String())
	}

	var status bytes.Buffer
	d.stdout = &status
	d.getwd = func() (string, error) { return repo, nil }
	if err := execute([]string{"notify", "--status"}, d); err != nil {
		t.Fatalf("execute notify --status: %v", err)
	}
	if !strings.Contains(status.String(), "telegram: active") {
		t.Fatalf("expected telegram active in status, got: %q", status.String())
	}
}

func TestExecute_Notify_Disable_TripsTransportMuteAndReportsIt(t *testing.T) {
	repo := testutil.TempRepo(t)
	storeProjectFor(t, repo)

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repo, nil },
	}

	if err := execute([]string{"notify", "--disable", "slack"}, d); err != nil {
		t.Fatalf("execute notify --disable: %v", err)
	}
	if !strings.Contains(stdout.String(), "slack: muted") {
		t.Fatalf("expected muted report, got: %q", stdout.String())
	}

	var status bytes.Buffer
	d.stdout = &status
	if err := execute([]string{"notify", "--status"}, d); err != nil {
		t.Fatalf("execute notify --status: %v", err)
	}
	if !strings.Contains(status.String(), "slack: muted (manual-disable)") {
		t.Fatalf("expected slack muted (manual-disable) in status, got: %q", status.String())
	}
	if !strings.Contains(status.String(), "telegram: active") {
		t.Fatalf("expected telegram unaffected, got: %q", status.String())
	}
}

func TestExecute_Notify_EnableDisable_UnknownTransportRejected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	err := execute([]string{"notify", "--enable", "carrier-pigeon"}, d)
	if err == nil || !strings.Contains(err.Error(), "carrier-pigeon") {
		t.Fatalf("expected unknown-transport error, got: %v", err)
	}
}

func TestExecute_Notify_Status_ReportsTicketsWithMutes(t *testing.T) {
	dir := testutil.TempRepo(t)
	project := storeProjectFor(t, dir)
	writeMutedTicket(t, project, "an-epic", "01-work.md", "01", `
  - event_type: gate-tripped
    tripped_at: 2026-08-01T00:00:00Z`)

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return dir, nil },
	}
	if err := execute([]string{"notify", "--status"}, d); err != nil {
		t.Fatalf("execute notify --status: %v", err)
	}
	if !strings.Contains(stdout.String(), "an-epic/01: gate-tripped") {
		t.Fatalf("expected muted ticket line, got: %q", stdout.String())
	}
}

func TestExecute_Notify_MessageWithStatusRejected(t *testing.T) {
	t.Parallel()
	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	err := execute([]string{"notify", "hello", "--status"}, d)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got: %v", err)
	}
}

func TestExecute_Notify_MessageWithEnableRejected(t *testing.T) {
	t.Parallel()
	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	err := execute([]string{"notify", "hello", "--enable", "slack"}, d)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got: %v", err)
	}
}

func writeMutedTicket(t *testing.T, projectDir, epic, filename, id, mutesYAML string) {
	t.Helper()
	path := filepath.Join(projectDir, epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: \"" + id + "\"\nstatus: open\ntype: implement\nmutes:" + mutesYAML + "\n---\nBody.\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
