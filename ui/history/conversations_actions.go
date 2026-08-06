package history

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

// resumeSplitType is the split kind ctrl+r opens `claude --resume` into.
const resumeSplitType = terminalrun.HSplit

// Seams so tests can fake the editor/clipboard/export side effects without
// touching a real filesystem editor, clipboard, or terminal multiplexer.
var (
	historyGetenv         = os.Getenv
	historyLookPath       = exec.LookPath
	historyExportMarkdown = claudehistory.ExportMarkdown
	historyCreateTempFile = os.CreateTemp
	historyClipboardWrite = clipboard.WriteAll
)

type convExportedMsg struct {
	err       error
	editorBin string
	args      []string
}

type editorFinishedMsg struct{ err error }

type resumeFinishedMsg struct {
	err      error
	splitApp string
}

// selectedConversation returns the currently highlighted row in the filtered
// conversations list, or false if the list is empty (nothing to act on).
func (m Model) selectedConversation() (claudehistory.Conversation, bool) {
	items := m.filteredConversations()
	sel := m.convList.Selected()
	if len(items) == 0 || sel < 0 || sel >= len(items) {
		return claudehistory.Conversation{}, false
	}
	return items[sel], true
}

// resolveEditor picks the editor to open exported markdown in: $EDITOR, then
// the first of nvim/vi found on PATH, then vi unconditionally — same
// resolution order as blf's internal/claudehistory.
func resolveEditor() string {
	if e := strings.TrimSpace(historyGetenv("EDITOR")); e != "" {
		return e
	}
	for _, e := range []string{"nvim", "vi"} {
		if _, err := historyLookPath(e); err == nil {
			return e
		}
	}
	return "vi"
}

// cmdExportAndEdit exports the selected conversation to a temp markdown file
// and, on success, opens it in the resolved editor (suspending the TUI in
// place, same as blf — unlike ctrl+r's resume, this isn't split into a new
// terminal pane).
func (m Model) cmdExportAndEdit() tea.Cmd {
	conv, ok := m.selectedConversation()
	if !ok {
		return notify.Warning("no conversation selected")
	}
	editor := resolveEditor()
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return notify.Warning("$EDITOR is empty")
	}
	editorBin, editorArgs := parts[0], parts[1:]
	path := conv.Path

	return func() tea.Msg {
		md, err := historyExportMarkdown(path)
		if err != nil {
			return convExportedMsg{err: err}
		}
		f, err := historyCreateTempFile("", "claude-history-*.md")
		if err != nil {
			return convExportedMsg{err: err}
		}
		if _, err := f.WriteString(md); err != nil {
			f.Close()
			return convExportedMsg{err: err}
		}
		f.Close()
		args := ui.EditorLaunchArgs(editorBin, editorArgs, f.Name(), 0)
		return convExportedMsg{editorBin: editorBin, args: args}
	}
}

func (m Model) handleConvExported(msg convExportedMsg, notifyCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, tea.Batch(notifyCmd, notify.Error("export failed: "+msg.err.Error()))
	}
	editCmd := tea.ExecProcess(exec.Command(msg.editorBin, msg.args...), func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
	return m, tea.Batch(notifyCmd, editCmd)
}

func (m Model) handleEditorFinished(msg editorFinishedMsg, notifyCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, tea.Batch(notifyCmd, notify.Error("editor exited with error: "+msg.err.Error()))
	}
	return m, notifyCmd
}

// resumeCommandArgs builds the argv `claude --resume <sessionID>` split-launches.
func resumeCommandArgs(sessionID string) (program string, args []string) {
	return "claude", []string{"--resume", sessionID}
}

// cmdResumeConversation opens `claude --resume <session-id>` for the selected
// conversation in a new terminal split (ui.DetectTerminal's mechanism), so the
// history TUI keeps running in its own pane — unlike blf's original in-place
// tea.ExecProcess suspend.
func (m Model) cmdResumeConversation() tea.Cmd {
	conv, ok := m.selectedConversation()
	if !ok {
		return notify.Warning("no conversation selected")
	}
	if conv.SessionID == "" {
		return notify.Warning("conversation has no session id to resume")
	}
	program, args := resumeCommandArgs(conv.SessionID)
	return terminalrun.CommandWithSplitBare(m.convProjectCwd, m.terminal, resumeSplitType, program, args, func(err error, splitApp string) tea.Msg {
		return resumeFinishedMsg{err: err, splitApp: splitApp}
	})
}

func (m Model) handleResumeFinished(msg resumeFinishedMsg, notifyCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, tea.Batch(notifyCmd, notify.Error("resume failed: "+msg.err.Error()))
	}
	if msg.splitApp != "" {
		return m, tea.Batch(notifyCmd, notify.Info("resumed in "+msg.splitApp+" split"))
	}
	return m, notifyCmd
}

// cmdYankSessionID copies the selected conversation's session id to the
// clipboard, following ui/log/model_yank.go's clipboard-seam pattern.
func (m Model) cmdYankSessionID() tea.Cmd {
	conv, ok := m.selectedConversation()
	if !ok {
		return notify.Warning("no conversation selected")
	}
	if conv.SessionID == "" {
		return notify.Warning("conversation has no session id to copy")
	}
	if err := historyClipboardWrite(conv.SessionID); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("copied session id: " + conv.SessionID)
}
