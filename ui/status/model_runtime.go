package status

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/comments"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

func nextFlashCmd() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg {
		return flashTickMsg{}
	})
}

func statusStartupLoadCmd() tea.Cmd {
	return func() tea.Msg {
		return statusStartupLoadMsg{}
	}
}

func cmdGitCommit(worktreeRoot string, terminal ui.Terminal) tea.Cmd {
	return terminalrun.Command(worktreeRoot, terminal, "git", []string{"commit"}, func(err error, splitApp string) tea.Msg {
		return commitFinishedMsg{err: err, splitApp: splitApp}
	})
}

func cmdLazygitLog(worktreeRoot string) tea.Cmd {
	c := exec.Command("lazygit", "-p", worktreeRoot, "log")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return lazygitLogFinishedMsg{err: err}
	})
}

func (m *Model) cmdEditSelectedFile() tea.Cmd {
	file, ok := m.selectedFile()
	if !ok {
		return notify.Warning("no file selected")
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return notify.Warning("$EDITOR is not set")
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return notify.Warning("$EDITOR is empty")
	}
	target := filepath.Join(m.worktreeRoot, file.Path)
	line := m.editorLineForCurrentSelection()
	args := editorLaunchArgs(parts[0], parts[1:], target, line)
	c := exec.Command(parts[0], args...)
	cmd := tea.ExecProcess(c, func(err error) tea.Msg {
		return editFileFinishedMsg{err: err}
	})
	return tea.Batch(notify.Info(ui.MessageOpening("editor")), cmd)
}

func (m *Model) cmdCreateCommentFromDiff() tea.Cmd {
	file, ok := m.selectedStatusFile()
	if !ok {
		return notify.Warning("no file context for comment")
	}
	loc, body, errMsg, ok := m.commentLocationAndBody()
	if !ok {
		return notify.Warning(errMsg)
	}
	cmd, msg := comments.CmdOpenEditor(file.Path, loc, body, m.worktreeRoot, m.settings.Terminal, func(err error, splitApp string) tea.Msg {
		return editCommentFinishedMsg{err: err, splitApp: splitApp}
	})
	return tea.Batch(notify.Info(msg), cmd)
}

func (m *Model) refresh() tea.Cmd {
	return m.refreshWithBehavior(false)
}

func (m *Model) refreshPreserveScroll() tea.Cmd {
	return m.refreshWithBehavior(true)
}

func (m *Model) refreshWithBehavior(preserveScroll bool) tea.Cmd {
	preserve := ""
	if entry, ok := m.selectedFiletreeEntry(); ok {
		preserve = entry.Path
	}
	unstagedOffset := m.diffarea.Unstaged.Viewport().YOffset()
	stagedOffset := m.diffarea.Staged.Viewport().YOffset()
	cmd := m.reload(preserve)
	m.syncDiffViewports()
	if preserveScroll {
		m.diffarea.Unstaged.RestoreViewportYOffset(unstagedOffset)
		m.diffarea.Staged.RestoreViewportYOffset(stagedOffset)
		return cmd
	}
	if m.focus == focusDiff {
		m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	}
	return cmd
}

func uniqueNonEmpty(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
