package history

import (
	"errors"
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

func withFakeEditorEnv(t *testing.T, editor string) {
	t.Helper()
	prevGetenv := historyGetenv
	historyGetenv = func(key string) string {
		if key == "EDITOR" {
			return editor
		}
		return ""
	}
	t.Cleanup(func() { historyGetenv = prevGetenv })
}

func TestResolveEditor_PrefersEnvVar(t *testing.T) {
	withFakeEditorEnv(t, "code --wait")
	if got := resolveEditor(); got != "code --wait" {
		t.Fatalf("resolveEditor() = %q, want %q", got, "code --wait")
	}
}

func TestResolveEditor_FallsBackToNvimThenVi(t *testing.T) {
	withFakeEditorEnv(t, "")
	prevLookPath := historyLookPath
	t.Cleanup(func() { historyLookPath = prevLookPath })

	historyLookPath = func(file string) (string, error) {
		if file == "nvim" {
			return "/usr/bin/nvim", nil
		}
		return "", errors.New("not found")
	}
	if got := resolveEditor(); got != "nvim" {
		t.Fatalf("resolveEditor() = %q, want nvim", got)
	}

	historyLookPath = func(string) (string, error) { return "", errors.New("not found") }
	if got := resolveEditor(); got != "vi" {
		t.Fatalf("resolveEditor() = %q, want vi fallback", got)
	}
}

func TestCmdExportAndEdit_NoConversations_Warns(t *testing.T) {
	m := newTestModel()
	cmd := m.cmdExportAndEdit()
	if cmd == nil {
		t.Fatal("expected a warning cmd for empty conversations list")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Kind != notify.KindWarning {
		t.Fatalf("expected a warning NotifyMsg, got %#v", cmd())
	}
}

func TestCmdExportAndEdit_BuildsTempFileAndEditorArgv(t *testing.T) {
	withFakeEditorEnv(t, "myeditor")
	prevExport := historyExportMarkdown
	historyExportMarkdown = func(path string) (string, error) {
		return "## exported content", nil
	}
	t.Cleanup(func() { historyExportMarkdown = prevExport })

	m := enterConversationsFixture(t)
	cmd := m.cmdExportAndEdit()
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	msg := cmd()
	exported, ok := msg.(convExportedMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want convExportedMsg", msg)
	}
	if exported.err != nil {
		t.Fatalf("unexpected error: %v", exported.err)
	}
	if exported.editorBin != "myeditor" {
		t.Fatalf("editorBin = %q, want myeditor", exported.editorBin)
	}
	if len(exported.args) != 1 {
		t.Fatalf("args = %#v, want a single temp-file path", exported.args)
	}
	content, err := os.ReadFile(exported.args[0])
	if err != nil {
		t.Fatalf("reading temp file: %v", err)
	}
	if string(content) != "## exported content" {
		t.Fatalf("temp file content = %q, want the exported markdown", content)
	}
	os.Remove(exported.args[0])
}

func TestCmdResumeConversation_NoConversations_Warns(t *testing.T) {
	m := newTestModel()
	cmd := m.cmdResumeConversation()
	if cmd == nil {
		t.Fatal("expected a warning cmd for empty conversations list")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Kind != notify.KindWarning {
		t.Fatalf("expected a warning NotifyMsg, got %#v", cmd())
	}
}

func TestCmdResumeConversation_NoSessionID_Warns(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = update(m, conversationsLoadedMsg{conversations: []claudehistory.Conversation{
		{SessionID: "", Title: "no session"},
	}})
	cmd := m.cmdResumeConversation()
	if cmd == nil {
		t.Fatal("expected a warning cmd when selected conversation has no session id")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Kind != notify.KindWarning {
		t.Fatalf("expected a warning NotifyMsg, got %#v", cmd())
	}
}

func TestCmdResumeConversation_BuildsSplitLaunch(t *testing.T) {
	m := enterConversationsFixture(t)
	m.terminal = ui.TerminalTmux
	m.convProjectCwd = "/dev/gx/main"
	cmd := m.cmdResumeConversation()
	if cmd == nil {
		t.Fatal("expected a non-nil split-launch cmd")
	}
}

func TestResumeCommandArgs(t *testing.T) {
	program, args := resumeCommandArgs("sess-101")
	if program != "claude" {
		t.Fatalf("program = %q, want claude", program)
	}
	want := []string{"--resume", "sess-101"}
	if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestCmdYankSessionID_NoConversations_Warns(t *testing.T) {
	m := newTestModel()
	cmd := m.cmdYankSessionID()
	if cmd == nil {
		t.Fatal("expected a warning cmd for empty conversations list")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Kind != notify.KindWarning {
		t.Fatalf("expected a warning NotifyMsg, got %#v", cmd())
	}
}

func TestCmdYankSessionID_WritesToClipboard(t *testing.T) {
	prev := historyClipboardWrite
	var written string
	historyClipboardWrite = func(s string) error {
		written = s
		return nil
	}
	t.Cleanup(func() { historyClipboardWrite = prev })

	m := enterConversationsFixture(t)
	cmd := m.cmdYankSessionID()
	if cmd == nil {
		t.Fatal("expected a non-nil cmd")
	}
	cmd()
	if written != "sess-101" {
		t.Fatalf("clipboard write = %q, want sess-101", written)
	}
}

func TestCmdYankSessionID_ClipboardError_ReportsError(t *testing.T) {
	prev := historyClipboardWrite
	historyClipboardWrite = func(string) error { return errors.New("no clipboard") }
	t.Cleanup(func() { historyClipboardWrite = prev })

	m := enterConversationsFixture(t)
	cmd := m.cmdYankSessionID()
	if cmd == nil {
		t.Fatal("expected a non-nil cmd")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Kind != notify.KindError {
		t.Fatalf("expected an error NotifyMsg, got %#v", cmd())
	}
}
