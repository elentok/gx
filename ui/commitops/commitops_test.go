package commitops

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/reword"
)

func notice(t *testing.T, cmd tea.Cmd) notify.NotifyMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg, ok := cmd().(notify.NotifyMsg)
	if !ok {
		t.Fatalf("expected NotifyMsg, got %T", cmd())
	}
	return msg
}

func headHash(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func stubClipboard(t *testing.T, err error) *[]string {
	t.Helper()
	var got []string
	prev := ClipboardWrite
	ClipboardWrite = func(s string) error {
		got = append(got, s)
		return err
	}
	t.Cleanup(func() { ClipboardWrite = prev })
	return &got
}

func TestYanks(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() tea.Cmd
		want string
		note string
	}{
		{"body", func() tea.Cmd { return YankBody("b1\nb2") }, "b1\nb2", "yanked commit body"},
		{"hash", func() tea.Cmd { return YankHash("abc123") }, "abc123", "yanked commit hash"},
		{"subject", func() tea.Cmd { return YankSubject("subj") }, "subj", "yanked commit subject"},
		{"message", func() tea.Cmd { return YankMessage("subj", "body") }, "subj\n\nbody", "yanked commit message"},
		{"message without body", func() tea.Cmd { return YankMessage("subj", "") }, "subj", "yanked commit message"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stubClipboard(t, nil)
			msg := notice(t, tt.cmd())
			if len(*got) != 1 || (*got)[0] != tt.want {
				t.Fatalf("clipboard = %q, want %q", *got, tt.want)
			}
			if msg.Kind != notify.KindInfo || msg.Message != tt.note {
				t.Fatalf("notice = %+v", msg)
			}
		})
	}
}

func TestYanksWarnWhenEmpty(t *testing.T) {
	got := stubClipboard(t, nil)
	for name, cmd := range map[string]tea.Cmd{
		"body":    YankBody(""),
		"hash":    YankHash(""),
		"subject": YankSubject(""),
		"message": YankMessage("", "body"),
	} {
		if msg := notice(t, cmd); msg.Kind != notify.KindWarning {
			t.Errorf("%s: kind = %v, want warning", name, msg.Kind)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("clipboard written: %q", *got)
	}
}

func TestYankReportsClipboardError(t *testing.T) {
	stubClipboard(t, errors.New("boom"))
	msg := notice(t, YankHash("abc"))
	if msg.Kind != notify.KindError || msg.Message != "clipboard copy failed: boom" {
		t.Fatalf("notice = %+v", msg)
	}
}

func TestOpenWithoutCommit(t *testing.T) {
	m := New()
	if err := m.OpenAmend("/nowhere", "", "s"); err == nil {
		t.Fatal("expected error for empty hash")
	}
	if msg := notice(t, m.OpenReword("/nowhere", "", "s", "b")); msg.Kind != notify.KindWarning {
		t.Fatalf("notice = %+v", msg)
	}
	if m.IsOpen() {
		t.Fatal("nothing should be open")
	}
}

func TestAmendOpensAndDeclineFinishes(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "two\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	hash := headHash(t, repo)

	m := New()
	if err := m.OpenAmend(repo, hash, "base"); err != nil {
		t.Fatal(err)
	}
	if !m.AmendOpen() || !m.IsOpen() || m.RewordOpen() {
		t.Fatal("amend confirm should be the only open modal")
	}
	page := strings.TrimRight(strings.Repeat(strings.Repeat(".", 80)+"\n", 24), "\n")
	if m.Overlay(page, 80, 24) == page {
		t.Fatal("overlay should draw the confirm")
	}

	next, _, res := m.Update(repo, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !res.Done || res.Kind != KindAmend || res.Err != nil {
		t.Fatalf("result = %+v", res)
	}
	if next.IsOpen() {
		t.Fatal("amend should be closed after declining")
	}
}

func TestAmendOpenFailsWithoutStagedChanges(t *testing.T) {
	repo := testutil.TempRepo(t)
	hash := headHash(t, repo)
	m := New()
	if err := m.OpenAmend(repo, hash, "s"); err == nil {
		t.Fatal("expected error with nothing staged")
	}
}

func TestEditorFailureNotifiesWithoutFinishing(t *testing.T) {
	m := New()
	next, cmd, res := m.Update("/nowhere", reword.EditorFinishedMsg{Err: errors.New("crashed")})
	msg := notice(t, cmd)
	if msg.Kind != notify.KindError || msg.Message != "reword: editor failed: crashed" {
		t.Fatalf("notice = %+v", msg)
	}
	if res.Done || next.IsOpen() {
		t.Fatalf("result = %+v, open = %v", res, next.IsOpen())
	}
}

func TestEditorUnreadableResultNotifies(t *testing.T) {
	m := New()
	_, cmd, res := m.Update("/nowhere", reword.EditorFinishedMsg{})
	if msg := notice(t, cmd); msg.Kind != notify.KindError {
		t.Fatalf("notice = %+v", msg)
	}
	if res.Done {
		t.Fatal("must not finish")
	}
}

func TestEditorWithoutChangesNotifies(t *testing.T) {
	repo := testutil.TempRepo(t)
	hash := headHash(t, repo)
	t.Setenv("EDITOR", "true")
	m := New()
	if cmd := m.OpenReword(repo, hash, "subject", "body"); cmd == nil {
		t.Fatal("expected the editor command")
	}
	next, cmd, res := m.Update(repo, reword.EditorFinishedMsg{})
	if msg := notice(t, cmd); msg.Kind != notify.KindInfo || msg.Message != "reword: no changes" {
		t.Fatalf("notice = %+v", msg)
	}
	if res.Done || next.RewordOpen() {
		t.Fatalf("result = %+v", res)
	}
}

func TestIdleUpdateIsNoop(t *testing.T) {
	m := New()
	_, cmd, res := m.Update("/nowhere", tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil || res != (Result{}) {
		t.Fatalf("cmd = %v, result = %+v", cmd, res)
	}
}
