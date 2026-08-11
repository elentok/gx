package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTicketsSchemaText_HasTicketAndEpicSections(t *testing.T) {
	if !strings.Contains(ticketsSchemaText, "Ticket frontmatter fields:") {
		t.Error("schema text missing \"Ticket frontmatter fields:\" section")
	}
	if !strings.Contains(ticketsSchemaText, "Epic frontmatter fields:") {
		t.Error("schema text missing \"Epic frontmatter fields:\" section")
	}
	if !strings.Contains(ticketsSchemaText, "started_at") {
		t.Error("schema text missing started_at")
	}
	if !strings.Contains(ticketsSchemaText, "completed_at") {
		t.Error("schema text missing completed_at")
	}
	if !strings.Contains(ticketsSchemaText, "parent") {
		t.Error("schema text missing parent")
	}
	if strings.Contains(ticketsSchemaText, "children") {
		t.Error("schema text still describes the retired children field")
	}
	for _, retired := range []string{"needs-triage", "ready-for-agent", "ready-for-human"} {
		if strings.Contains(ticketsSchemaText, retired) {
			t.Errorf("schema text still lists retired status %q", retired)
		}
	}
	if !strings.Contains(ticketsSchemaText, "code-review") {
		t.Error("schema text missing code-review type")
	}
	if strings.Contains(ticketsSchemaText, "split_from") || strings.Contains(ticketsSchemaText, "--split") || strings.Contains(ticketsSchemaText, "code_review_fixes") {
		t.Error("schema text should no longer describe split/split_from/code_review_fixes as settable")
	}
}

func TestExecute_TicketsSet_MultiFieldSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--status=claimed", "--blocked-by=01,03"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}
	if !strings.Contains(stdout.String(), "updated (status=claimed, blocked_by=01,03)") {
		t.Errorf("stdout = %q, want it to list the changed fields", stdout.String())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: claimed") {
		t.Errorf("ticket file = %q, want status: claimed", string(raw))
	}
	if !strings.Contains(string(raw), `blocked_by:`) {
		t.Errorf("ticket file = %q, want blocked_by written", string(raw))
	}
}

func TestExecute_TicketsSet_Parent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--parent=04"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}
	if !strings.Contains(stdout.String(), "parent=04") {
		t.Errorf("stdout = %q, want it to list parent", stdout.String())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "parent: \"04\"") {
		t.Errorf("ticket file = %q, want parent: \"04\"", string(raw))
	}
}

func TestExecute_TicketsSet_SplitAliasFlagsRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--split-from=04", "--split=04c,04d"}, d)
	if err == nil {
		t.Fatal("expected an unknown-flag error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q, want it to mention unknown flag", err.Error())
	}
}

func TestExecute_TicketsSet_CodeReviewFixesFlagRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--code-review-fixes=none"}, d)
	if err == nil {
		t.Fatal("expected an unknown-flag error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q, want it to mention unknown flag", err.Error())
	}
}

func TestExecute_TicketsSet_RefusedFieldUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--actual-context-window=100"}, d)
	if err == nil {
		t.Fatal("expected an unknown-flag error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q, want it to mention unknown flag", err.Error())
	}
}

func TestExecute_TicketsSet_ValidationFailureWritesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	original := "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n"
	writeTicketFile(t, path, original)

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--status=bogus-status"}, d)
	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error = %q, want it to mention the invalid status field", err.Error())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if string(raw) != original {
		t.Errorf("ticket file changed on validation failure: got %q, want unchanged %q", string(raw), original)
	}
}

func TestExecute_TicketsSet_Commitless(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--status=done", "--commitless=true"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "commitless: true") {
		t.Errorf("ticket file = %q, want commitless: true written", string(raw))
	}
}

func TestExecute_TicketsSet_StatusDoneRefusedWithUnresolvedBlocker(t *testing.T) {
	epicPath := filepath.Join(t.TempDir(), "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	blockerPath := filepath.Join(issuesDir, "01-blocker.md")
	writeTicketFile(t, blockerPath, "---\nid: \"01\"\nstatus: open\ntype: task\n---\nBody.\n")
	targetPath := filepath.Join(issuesDir, "02-target.md")
	writeTicketFile(t, targetPath, "---\nid: \"02\"\nstatus: claimed\nblocked_by: [\"01\"]\ntype: task\n---\nBody.\n")

	var stdout, stderr bytes.Buffer
	d := deps{stdout: &stdout, stderr: &stderr}

	err := execute([]string{"tickets", "set", targetPath, "--status=done"}, d)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "unresolved blocked_by") || !strings.Contains(err.Error(), "01") {
		t.Errorf("error = %q, want it to mention unresolved blocked_by (01)", err.Error())
	}

	raw, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: claimed") {
		t.Errorf("ticket file changed despite refused write: %q", string(raw))
	}
}

func TestExecute_TicketsSet_StatusDoneForcedWithUnresolvedBlockerWarns(t *testing.T) {
	epicPath := filepath.Join(t.TempDir(), "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	blockerPath := filepath.Join(issuesDir, "01-blocker.md")
	writeTicketFile(t, blockerPath, "---\nid: \"01\"\nstatus: open\ntype: task\n---\nBody.\n")
	targetPath := filepath.Join(issuesDir, "02-target.md")
	writeTicketFile(t, targetPath, "---\nid: \"02\"\nstatus: claimed\nblocked_by: [\"01\"]\ntype: task\n---\nBody.\n")

	var stdout, stderr bytes.Buffer
	d := deps{stdout: &stdout, stderr: &stderr}

	err := execute([]string{"tickets", "set", targetPath, "--status=done", "--force"}, d)
	if err != nil {
		t.Fatalf("execute tickets set --force: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning") || !strings.Contains(stderr.String(), "01") {
		t.Errorf("stderr = %q, want a warning mentioning the unresolved blocker (01)", stderr.String())
	}

	raw, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket file = %q, want status: done written despite forced unresolved blocker", string(raw))
	}
}

func TestExecute_TicketsSet_StatusDoneUnaffectedByBlockerOutsideIssuesLayout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\nblocked_by: [\"01\"]\ntype: task\n---\nBody.\n")

	var stdout, stderr bytes.Buffer
	d := deps{stdout: &stdout, stderr: &stderr}

	err := execute([]string{"tickets", "set", path, "--status=done"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v, want no gate outside the <epic>/issues/ layout", err)
	}
}

func TestExecute_TicketsSet_StatusOpenRefusedWithEmptyBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: draft\ntype: task\n---\n")

	var stdout, stderr bytes.Buffer
	d := deps{stdout: &stdout, stderr: &stderr}

	err := execute([]string{"tickets", "set", path, "--status=open"}, d)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "empty body") {
		t.Errorf("error = %q, want it to mention the empty body", err.Error())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: draft") {
		t.Errorf("ticket file changed despite refused write: %q", string(raw))
	}
}

func TestExecute_TicketsSet_StatusOpenAcceptedWithBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: draft\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--status=open"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: open") {
		t.Errorf("ticket file = %q, want status: open", string(raw))
	}
}

func TestExecute_TicketsSet_NeedsRepairToOpenAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: needs-repair\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--status=open"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v, want needs-repair -> open to be accepted", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: open") {
		t.Errorf("ticket file = %q, want status: open", string(raw))
	}
}

func TestExecute_TicketsValidate_AcceptsBodylessDraft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: draft\ntype: task\n---\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	if err := execute([]string{"tickets", "validate", path}, d); err != nil {
		t.Fatalf("execute tickets validate: %v, want a body-less draft accepted", err)
	}
}

func TestExecute_TicketsSet_IterationStatusWorkingAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--iteration-status=working"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "iteration_status: working") {
		t.Errorf("ticket file = %q, want iteration_status: working written", string(raw))
	}
}

func TestExecute_TicketsSet_IterationStatusInvalidRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--iteration-status=bogus"}, d)
	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
	if !strings.Contains(err.Error(), "iteration-status") {
		t.Errorf("error = %q, want it to mention --iteration-status", err.Error())
	}
}

func TestExecute_TicketsSet_IterationStatusFinishedBareRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	original := "---\nid: \"04b\"\nstatus: claimed\ntype: task\n---\nBody.\n"
	writeTicketFile(t, path, original)

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--iteration-status=finished"}, d)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "--commitless=true") || !strings.Contains(err.Error(), "--status=done") {
		t.Errorf("error = %q, want it to mention --commitless=true and --status=done", err.Error())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if string(raw) != original {
		t.Errorf("ticket file changed on validation failure: got %q, want unchanged %q", string(raw), original)
	}
}

func TestExecute_TicketsSet_IterationStatusFinishedWithCommitlessAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--iteration-status=finished", "--commitless=true"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "iteration_status: finished") {
		t.Errorf("ticket file = %q, want iteration_status: finished written", string(raw))
	}
}

func TestExecute_TicketsSet_IterationStatusFinishedWithStatusDoneAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--iteration-status=finished", "--status=done"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "iteration_status: finished") {
		t.Errorf("ticket file = %q, want iteration_status: finished written", string(raw))
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket file = %q, want status: done written", string(raw))
	}
}

func TestExecute_TicketsSet_IterationStatusFinishedAlreadyCommitlessOnDiskAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ncommitless: true\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--iteration-status=finished"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v, want already-commitless-on-disk to satisfy the guard", err)
	}
}

func TestExecute_TicketsSet_IterationStatusNeedsAnswerBareAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--iteration-status=needs-answer"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v, want needs-answer to never trip the finished-only guard", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "iteration_status: needs-answer") {
		t.Errorf("ticket file = %q, want iteration_status: needs-answer written", string(raw))
	}
}

func TestExecute_TicketsSet_ClearingListField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\nblocked_by: [\"01\", \"03\"]\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--blocked-by="}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if strings.Contains(string(raw), "blocked_by") {
		t.Errorf("ticket file = %q, want blocked_by cleared entirely (omitempty)", string(raw))
	}
}
