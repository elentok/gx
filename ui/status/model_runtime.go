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
	"github.com/elentok/gx/ui/nav"
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

func (m *Model) cmdEditSelectedFile(splitType terminalrun.SplitType) tea.Cmd {
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
	args := ui.EditorLaunchArgs(parts[0], parts[1:], target, line)
	cmd := terminalrun.CommandWithSplit(m.worktreeRoot, m.settings.Terminal, splitType, parts[0], args, func(err error, splitApp string) tea.Msg {
		return editFileFinishedMsg{err: err, splitApp: splitApp}
	})
	return cmd
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
	if cmd == nil {
		return notify.Warning(msg)
	}
	return cmd
}

type autoReloadMsg struct{}

func statusAutoReloadCmd() tea.Cmd {
	return func() tea.Msg { return autoReloadMsg{} }
}

// AutoReload is called by the app shell when this tab is stale (gate epoch
// mismatch). It dispatches autoReloadMsg so the real refresh runs inside
// Update, where the model mutations are properly returned to the caller.
func (m Model) AutoReload() tea.Cmd {
	return statusAutoReloadCmd()
}

// OnPageDeactivated is called by the app shell when the user switches away
// from the status tab — a disrupting event per ADR 0010, since any active
// image-diff overlay would otherwise be left floating over whatever the next
// tab renders. Clearing here is eager and unconditional; the overlay's active
// IDs are left as-is (Model is a value here, so they can't be mutated back into
// the shell's copy) — the next disrupting event will harmlessly re-clear the
// same (already-cleared) IDs before placing anything new.
func (m Model) OnPageDeactivated() tea.Cmd {
	return m.overlay.OnDeactivate()
}

func statusRepoMutatedCmd() tea.Cmd {
	return nav.RepoMutated()
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
