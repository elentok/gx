package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// nonAgentGetwd returns a getwd fake pointed at a plain (non-git) tempdir, so
// git.CurrentBranch fails and checkAgentStatusGuard treats the caller as an
// unrecognised branch — i.e. not an iteration agent. Existing tests use it to
// keep exercising hand-driven-caller behavior unaffected by the guard.
func nonAgentGetwd(t *testing.T) func() (string, error) {
	t.Helper()
	dir := t.TempDir()
	return func() (string, error) { return dir, nil }
}

// agentGetwd returns a getwd fake pointed at a real git repo checked out on
// branch, so git.CurrentBranch resolves to it for checkAgentStatusGuard.
func agentGetwd(t *testing.T, branch string) func() (string, error) {
	t.Helper()
	dir := testutil.TempRepo(t)
	if err := git.CreateBranch(git.Repo{Root: dir}, branch); err != nil {
		t.Fatalf("create branch %s: %v", branch, err)
	}
	return func() (string, error) { return dir, nil }
}

func TestTicketsSchemaText_HasTicketAndEpicSections(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}

	err := execute([]string{"tickets", "set", path, "--status=draft", "--blocked-by=01,03"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}
	if !strings.Contains(stdout.String(), "updated (status=draft, blocked_by=01,03)") {
		t.Errorf("stdout = %q, want it to list the changed fields", stdout.String())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: draft") {
		t.Errorf("ticket file = %q, want status: draft", string(raw))
	}
	if !strings.Contains(string(raw), `blocked_by:`) {
		t.Errorf("ticket file = %q, want blocked_by written", string(raw))
	}
}

func TestExecute_TicketsSet_Parent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	original := "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n"
	writeTicketFile(t, path, original)

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}

	err := execute([]string{"tickets", "set", path, "--commitless=true"}, d)
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

// Seam C: with no server running, the direct verbs still work and the
// orchestration statuses are refused without touching the file.
func TestExecute_TicketsSet_OrchestrationStatusesRefused(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"claimed", "done", "needs-answer", "needs-repair", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "04b-ticket.md")
			original := "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n"
			writeTicketFile(t, path, original)

			d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}
			err := execute([]string{"tickets", "set", path, "--status=" + status}, d)
			if err == nil || !strings.Contains(err.Error(), "orchestration status") {
				t.Fatalf("error = %v, want an orchestration-status refusal", err)
			}
			raw, _ := os.ReadFile(path)
			if string(raw) != original {
				t.Errorf("ticket changed despite refusal: %q", raw)
			}
		})
	}
}

// A map epic's decision tickets are hand-driven: set claims and closes them.
// Statuses outside claimed/done stay refused even there.
func TestExecute_TicketsSet_MapEpicClaimsAndCloses(t *testing.T) {
	t.Parallel()
	epic := filepath.Join(t.TempDir(), "map-epic")
	if err := os.MkdirAll(epic, 0755); err != nil {
		t.Fatal(err)
	}
	writeTicketFile(t, filepath.Join(epic, "ticket.md"), "---\nkind: map\nstatus: open\n---\n# Map\n")
	if err := os.MkdirAll(filepath.Join(epic, "issues"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(epic, "issues", "04-pick.md")
	writeTicketFile(t, path, "---\nid: \"04\"\nstatus: open\ntype: grilling\n---\nBody.\n")
	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}

	for _, args := range [][]string{{"--status=claimed"}, {"--status=done", "--commitless=true"}} {
		if err := execute(append([]string{"tickets", "set", path}, args...), d); err != nil {
			t.Fatalf("set %v: %v", args, err)
		}
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "status: done") || !strings.Contains(string(raw), "commitless: true") {
		t.Errorf("ticket = %q, want done + commitless", raw)
	}

	err := execute([]string{"tickets", "set", path, "--status=needs-repair"}, d)
	if err == nil || !strings.Contains(err.Error(), "orchestration status") {
		t.Errorf("needs-repair in a map epic: error = %v, want refusal", err)
	}
}

func TestExecute_TicketsSet_StatusOpenRefusedWithEmptyBody(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: draft\ntype: implement\n---\n")

	var stdout, stderr bytes.Buffer
	d := deps{stdout: &stdout, stderr: &stderr, getwd: nonAgentGetwd(t)}

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: draft\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: needs-repair\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: draft\ntype: implement\n---\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	if err := execute([]string{"tickets", "validate", path}, d); err != nil {
		t.Fatalf("execute tickets validate: %v, want a body-less draft accepted", err)
	}
}

func TestExecute_TicketsSet_IterationStatusWorkingAccepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: implement\n---\nBody.\n")

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: implement\n---\nBody.\n")

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

func TestExecute_TicketsSet_IterationStatusFinishedBareAccepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--iteration-status=finished"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v, want a bare finished report accepted — the zero-commit check belongs to ralphloop's landing path, not this CLI guard", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "iteration_status: finished") {
		t.Errorf("ticket file = %q, want iteration_status: finished written", string(raw))
	}
}

func TestExecute_TicketsSet_IterationStatusFinishedWithCommitlessAccepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: implement\n---\nBody.\n")

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

func TestExecute_TicketsSet_IterationStatusFinishedAlreadyCommitlessOnDiskAccepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ncommitless: true\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--iteration-status=finished"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v, want already-commitless-on-disk to satisfy the guard", err)
	}
}

func TestExecute_TicketsSet_IterationStatusNeedsAnswerBareAccepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: claimed\ntype: implement\n---\nBody.\n")

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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\nblocked_by: [\"01\", \"03\"]\ntype: implement\n---\nBody.\n")

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

func TestExecute_TicketsSet_AgentBranch_NonPromotionStatusRefused(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"draft"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "04b-ticket.md")
			writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

			var stdout, stderr bytes.Buffer
			d := deps{stdout: &stdout, stderr: &stderr, getwd: agentGetwd(t, "ralph-loop/widget-item-04b")}

			err := execute([]string{"tickets", "set", path, "--status=" + status}, d)
			if err == nil {
				t.Fatalf("expected --status=%s to be refused on a ralph-loop/* branch, got nil", status)
			}
			if !strings.Contains(err.Error(), "ralph-loop/") {
				t.Errorf("error = %q, want it to name the ralph-loop/* branch", err.Error())
			}
			if !strings.Contains(err.Error(), status) {
				t.Errorf("error = %q, want it to name the refused status %q", err.Error(), status)
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading ticket back: %v", err)
			}
			if !strings.Contains(string(raw), "status: open") {
				t.Errorf("ticket file changed despite refused write: %q", string(raw))
			}
		})
	}
}

func TestExecute_TicketsSet_AgentBranch_DraftToOpenAccepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: draft\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil), getwd: agentGetwd(t, "ralph-loop/widget-item-04b")}

	err := execute([]string{"tickets", "set", path, "--status=open"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v, want draft -> open accepted on a ralph-loop/* branch", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: open") {
		t.Errorf("ticket file = %q, want status: open", string(raw))
	}
}

func TestExecute_TicketsSet_AgentBranch_AlreadyOpenToOpenRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

	var stdout, stderr bytes.Buffer
	d := deps{stdout: &stdout, stderr: &stderr, getwd: agentGetwd(t, "ralph-loop/widget-item-04b")}

	err := execute([]string{"tickets", "set", path, "--status=open"}, d)
	if err == nil {
		t.Fatal("expected --status=open on an already-open ticket to be refused (not a draft promotion), got nil")
	}
}

func TestExecute_TicketsSet_AgentBranch_NeedsAnswerAndNeedsRepairRefused(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"needs-answer", "needs-repair"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "04b-ticket.md")
			writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

			var stdout, stderr bytes.Buffer
			d := deps{stdout: &stdout, stderr: &stderr, getwd: agentGetwd(t, "ralph-loop/widget-item-04b")}

			err := execute([]string{"tickets", "set", path, "--status=" + status}, d)
			if err == nil {
				t.Fatalf("expected --status=%s to be refused, got nil", status)
			}
		})
	}
}

func TestExecute_TicketsSet_UnrecognisedBranch_StatusAccepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	// "main" isn't a ralph-loop/* branch, so a hand-driven caller working on
	// it must never be caught by the guard, even for a status the guard would
	// otherwise refuse for an iteration agent.
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil), getwd: agentGetwd(t, "widget-hand-driven")}

	err := execute([]string{"tickets", "set", path, "--status=draft"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v, want an unrecognised branch to never be refused", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: draft") {
		t.Errorf("ticket file = %q, want status: draft", string(raw))
	}
}
