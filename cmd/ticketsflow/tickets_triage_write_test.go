package ticketsflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

// triageAtWrite is a two-row Triage at step write; row 0 carries an answer.
func triageAtWrite(t *testing.T) (repo, project string, s triageShown) {
	t.Helper()
	repo, project = triageProjectDir(t)
	s = triageAtApprove(t, repo, true)
	if _, err := runIn(t, repo, "tickets", "triage", "row", "--row", s.Rows[0].Address,
		"--revision", strconv.Itoa(s.Revision), "--answer", "q1=beta"); err != nil {
		t.Fatal(err)
	}
	s = triageShow(t, repo)
	if _, err := runIn(t, repo, "tickets", "triage", "approve", "--revision", strconv.Itoa(s.Revision)); err != nil {
		t.Fatal(err)
	}
	return repo, project, triageShow(t, repo)
}

type locateOut struct {
	Position     string   `json:"position"`
	Address      string   `json:"address"`
	NewAddresses []string `json:"newAddresses"`
	HasAnswers   bool     `json:"hasAnswers"`
	SeenAgain    bool     `json:"seenAgain"`
}

func triageLocate(t *testing.T, repo string, args ...string) locateOut {
	t.Helper()
	out, err := runIn(t, repo, append([]string{"tickets", "triage", "locate"}, args...)...)
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	var l locateOut
	if err := json.Unmarshal([]byte(out), &l); err != nil {
		t.Fatalf("locate json %q: %v", out, err)
	}
	return l
}

func followUpFile(project string) string {
	return filepath.Join(project, tickets.FollowUpsEpic, "issues", "01-f.md")
}

func ticketText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTriageLocate_ReportsEachPosition(t *testing.T) {
	repo, project, s := triageAtWrite(t)
	row := s.Rows[0].Address
	if got := triageLocate(t, repo, "--row", row); got.Position != "follow-ups" || got.HasAnswers {
		t.Errorf("in follow-ups = %+v", got)
	}

	src := followUpFile(project)
	moved := writeEpicTicket(t, project, "ready-x", "05-f.md",
		"---\nid: \"05\"\nstatus: open\ntype: research\noriginal_address: \""+row+"\"\n---\n\n# Follow-up 01\n")
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	got := triageLocate(t, repo, "--row", row)
	if got.Position != "moved" || !strings.HasSuffix(got.Address, "ready-x/05") {
		t.Errorf("after move = %+v", got)
	}

	archived := writeEpicTicket(t, project, tickets.ArchivedFollowUpsEpic, "01-f.md",
		"---\nid: \"01\"\nstatus: cancelled\ntype: research\n---\n\n# Follow-up 01\n\n## Resolution\n\n- reason: fixed\n"+tickets.OriginalLine(mustAddr(t, row))+"\n")
	if err := os.Remove(moved); err != nil {
		t.Fatal(err)
	}
	if got := triageLocate(t, repo, "--row", row); got.Position != "archived" {
		t.Errorf("after close = %+v", got)
	}
	if err := os.Remove(archived); err != nil {
		t.Fatal(err)
	}
	if got := triageLocate(t, repo, "--row", row); got.Position != "missing" {
		t.Errorf("after delete = %+v", got)
	}
}

func TestTriageLocate_PromotedArchiveReportsNewAddresses(t *testing.T) {
	repo, project, s := triageAtWrite(t)
	row := s.Rows[0].Address
	for _, n := range []string{"05", "06"} {
		writeEpicTicket(t, project, "ready-x", n+"-f.md",
			"---\nid: \""+n+"\"\nstatus: open\ntype: research\noriginal_address: \""+row+"\"\n---\n\n# New "+n+"\n")
	}
	writeEpicTicket(t, project, tickets.ArchivedFollowUpsEpic, "01-f.md",
		"---\nid: \"01\"\nstatus: cancelled\ntype: research\n---\n\n# Follow-up 01\n\n## Resolution\n\n- reason: promoted\n"+tickets.OriginalLine(mustAddr(t, row))+"\n")
	if err := os.Remove(followUpFile(project)); err != nil {
		t.Fatal(err)
	}
	got := triageLocate(t, repo, "--row", row)
	if got.Position != "archived" || !strings.HasSuffix(got.Address, tickets.ArchivedFollowUpsEpic+"/01") ||
		len(got.NewAddresses) != 2 || !strings.HasSuffix(got.NewAddresses[0], "ready-x/05") || !strings.HasSuffix(got.NewAddresses[1], "ready-x/06") {
		t.Errorf("promoted archive = %+v", got)
	}
}

func TestTriageLocate_MovedMapEpicFollowUpStaysMoved(t *testing.T) {
	repo, project, s := triageAtWrite(t)
	row := s.Rows[0].Address
	writeEpicTicket(t, project, "map-x", "05-f.md",
		"---\nid: \"05\"\nstatus: open\ntype: research\noriginal_address: \""+row+"\"\n---\n\n# Moved\n")
	if err := os.Remove(followUpFile(project)); err != nil {
		t.Fatal(err)
	}
	got := triageLocate(t, repo, "--row", row)
	if got.Position != "moved" || len(got.NewAddresses) != 0 {
		t.Errorf("moved map-epic follow-up = %+v", got)
	}
}

func mustAddr(t *testing.T, s string) tickets.Address {
	t.Helper()
	a, err := tickets.ParseAddress(s, tickets.AddressContext{})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestTriageAnswers_RunTwiceWritesOnce(t *testing.T) {
	repo, project, s := triageAtWrite(t)
	row := s.Rows[0].Address
	for i := range 2 {
		if _, err := runIn(t, repo, "tickets", "triage", "answers", "--row", row); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	text := ticketText(t, followUpFile(project))
	if n := strings.Count(text, "- which?: beta"); n != 1 {
		t.Errorf("answer appears %d times in:\n%s", n, text)
	}
	if got := triageLocate(t, repo, "--row", row); !got.HasAnswers {
		t.Errorf("locate after answers = %+v", got)
	}
}

func TestTriageSeenAgain_RunTwiceAddsOneEntry(t *testing.T) {
	repo, project, s := triageAtWrite(t)
	keep, from := s.Rows[0].Address, s.Rows[1].Address
	if got := triageLocate(t, repo, "--row", keep, "--seen-from", from); got.SeenAgain {
		t.Fatalf("seen before write: %+v", got)
	}
	for i := range 2 {
		if _, err := runIn(t, repo, "tickets", "triage", "seen-again", "--row", keep, "--from", from); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	text := ticketText(t, followUpFile(project))
	if n := strings.Count(text, "seen again"); n != 1 {
		t.Errorf("seen-again entries = %d in:\n%s", n, text)
	}
	if got := triageLocate(t, repo, "--row", keep, "--seen-from", from); !got.SeenAgain {
		t.Errorf("locate after write = %+v", got)
	}
}

// A crash between a verb and its row save reruns the whole row: the answers and merge note skip,
// and the second move finds nothing to move instead of writing another ticket.
func TestTriageRerunAfterCrash_WritesNoSecondMoveNoteOrAnswers(t *testing.T) {
	repo, project, s := triageAtWrite(t)
	row, from := s.Rows[0].Address, s.Rows[1].Address
	writeEpicTicket(t, project, "ready-x", "01-seed.md", "---\nid: \"01\"\nstatus: open\ntype: research\n---\n\n# Seed\n")
	// Tickets made by `gx tickets add` carry original_address, which locate follows through a move.
	src := followUpFile(project)
	if err := os.WriteFile(src, []byte(strings.Replace(ticketText(t, src), "---\n", "---\noriginal_address: \""+row+"\"\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	pass := func() error {
		for _, args := range [][]string{
			{"tickets", "triage", "answers", "--row", row},
			{"tickets", "triage", "seen-again", "--row", row, "--from", from},
			{"tickets", "move", tickets.FollowUpsEpic + "/01", "ready-x"},
		} {
			if _, err := runIn(t, repo, args...); err != nil {
				return err
			}
		}
		return nil
	}
	if err := pass(); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	got := triageLocate(t, repo, "--row", row)
	if got.Position != "moved" {
		t.Fatalf("after move = %+v", got)
	}
	// The rerun starts from locate: a moved row is not moved again.
	if _, err := runIn(t, repo, "tickets", "triage", "answers", "--row", row); err != nil {
		t.Fatalf("answers rerun: %v", err)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "seen-again", "--row", row, "--from", from); err != nil {
		t.Fatalf("seen-again rerun: %v", err)
	}
	_, moveErr := runIn(t, repo, "tickets", "move", tickets.FollowUpsEpic+"/01", "ready-x")
	files, err := filepath.Glob(filepath.Join(project, "ready-x", "issues", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("ready-x holds %d tickets after the rerun (move err %v), want 2: %v", len(files), moveErr, files)
	}
	for _, f := range files {
		text := ticketText(t, f)
		if !strings.Contains(text, row) {
			continue
		}
		if n := strings.Count(text, "- which?: beta"); n != 1 {
			t.Errorf("answer appears %d times in:\n%s", n, text)
		}
		if n := strings.Count(text, "seen again"); n != 1 {
			t.Errorf("seen-again entries = %d in:\n%s", n, text)
		}
		return
	}
	t.Errorf("no ticket in ready-x carries %s: %v", row, files)
}

func TestTriageWriteState_FailedKeepsRefusalAndRerunClearsIt(t *testing.T) {
	repo, _, s := triageAtWrite(t)
	row := s.Rows[0].Address
	if _, err := runIn(t, repo, "tickets", "triage", "write-state", "--row", row, "--state", "failed", "--refusal", "server-not-running"); err != nil {
		t.Fatal(err)
	}
	if got := rowWrite(t, repo, row); got.Write != "failed" || got.Refusal != "server-not-running" {
		t.Fatalf("failed row = %+v", got)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "write-state", "--row", row, "--state", "done", "--new-address", "p:ready-x/05,p:ready-x/06"); err != nil {
		t.Fatal(err)
	}
	if got := rowWrite(t, repo, row); got.Write != "done" || got.Refusal != "" || strings.Join(got.NewAddresses, " ") != "p:ready-x/05 p:ready-x/06" {
		t.Errorf("rerun row = %+v", got)
	}
}

type rowState struct {
	Address      string   `json:"address"`
	Write        string   `json:"write"`
	Refusal      string   `json:"refusal"`
	NewAddresses []string `json:"newAddresses"`
}

func rowWrite(t *testing.T, repo, address string) rowState {
	t.Helper()
	out, err := runIn(t, repo, "tickets", "triage", "show", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		State string     `json:"state"`
		Rows  []rowState `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	for _, r := range v.Rows {
		if r.Address == address {
			return r
		}
	}
	t.Fatalf("no row %s", address)
	return rowState{}
}

func triageState(t *testing.T, repo string) string {
	t.Helper()
	out, err := runIn(t, repo, "tickets", "triage", "show", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	return v.State
}

func TestTriageLastPass_DoneNeedsTheFlagAndEveryRowSettled(t *testing.T) {
	repo, _, s := triageAtWrite(t)
	a, b := s.Rows[0].Address, s.Rows[1].Address
	state := func(row, st string) {
		t.Helper()
		if _, err := runIn(t, repo, "tickets", "triage", "write-state", "--row", row, "--state", st); err != nil {
			t.Fatal(err)
		}
	}
	state(a, "done")
	state(b, "gone") // a gone row does not block done
	// Every row settled but the flag unset: a crash between the last row save and the last pass.
	if got := triageState(t, repo); got != "live" {
		t.Fatalf("flag unset: state = %q, want live", got)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "last-pass"); err != nil {
		t.Fatal(err)
	}
	if got := triageState(t, repo); got != "done" {
		t.Errorf("flag set: state = %q, want done", got)
	}
}

func TestTriageLastPass_RowStillPendingKeepsItLive(t *testing.T) {
	repo, _, s := triageAtWrite(t)
	if _, err := runIn(t, repo, "tickets", "triage", "write-state", "--row", s.Rows[0].Address, "--state", "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "last-pass"); err != nil {
		t.Fatal(err)
	}
	if got := triageState(t, repo); got != "live" {
		t.Errorf("state = %q, want live", got)
	}
}

func TestTriageEpics_RecordedBeforeEpicAddSoARerunIsKnown(t *testing.T) {
	repo, _, _ := triageAtWrite(t)
	out, err := runIn(t, repo, "tickets", "triage", "epics", "--ready", "ready-x", "--map", "m1")
	if err != nil || out != "ready-x new\nm1 new\n" {
		t.Fatalf("first = %q, %v", out, err)
	}
	out, err = runIn(t, repo, "tickets", "triage", "epics", "--ready", "ready-x", "--map", "m1")
	if err != nil || out != "ready-x known\nm1 known\n" {
		t.Fatalf("rerun = %q, %v", out, err)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "epics", "--ready", "other"); err == nil {
		t.Error("a second ready epic was accepted")
	}
}
