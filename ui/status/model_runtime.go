package status

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/explorer"

	tea "charm.land/bubbletea/v2"
)

func nextFlashCmd() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg {
		return flashTickMsg{}
	})
}

func statusTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return statusTickMsg{}
	})
}

func cmdGitCommit(worktreeRoot string, terminal ui.Terminal) tea.Cmd {
	if terminal == ui.TerminalTmux {
		return func() tea.Msg {
			err := exec.Command("tmux", "split-window", "-v", "-c", worktreeRoot, "git commit").Run()
			return commitFinishedMsg{err: err, splitApp: "tmux"}
		}
	}
	if terminal == ui.TerminalKittyRemote {
		return func() tea.Msg {
			args := []string{"@", "launch", "--copy-env", "--location=hsplit", "--cwd=" + worktreeRoot}
			args = append(args, "git", "commit")
			out, err := exec.Command("kitty", args...).CombinedOutput()
			if err != nil {
				err = fmt.Errorf("$ kitty %s\n\n%w\n\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
			return commitFinishedMsg{err: err, splitApp: "kitty"}
		}
	}
	c := exec.Command("git", "commit")
	c.Dir = worktreeRoot
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return commitFinishedMsg{err: err}
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
		m.setStatus("no file selected")
		return nil
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		m.setStatus("$EDITOR is not set")
		return nil
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		m.setStatus("$EDITOR is empty")
		return nil
	}
	target := filepath.Join(m.worktreeRoot, file.Path)
	line := m.editorLineForCurrentSelection()
	args := editorLaunchArgs(parts[0], parts[1:], target, line)
	c := exec.Command(parts[0], args...)
	m.setStatus(ui.MessageOpening("editor"))
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editFileFinishedMsg{err: err}
	})
}

func (m *Model) refresh() {
	m.refreshWithBehavior(false)
}

func (m *Model) refreshPreserveScroll() {
	m.refreshWithBehavior(true)
}

func (m *Model) refreshWithBehavior(preserveScroll bool) {
	preserve := ""
	if entry, ok := m.selectedStatusEntry(); ok {
		preserve = entry.Path
	}
	unstagedOffset := m.unstaged.viewport.YOffset()
	stagedOffset := m.staged.viewport.YOffset()
	m.reload(preserve)
	m.syncDiffViewports()
	if preserveScroll {
		explorer.RestoreViewportYOffset(&m.unstaged.viewport, unstagedOffset)
		explorer.RestoreViewportYOffset(&m.staged.viewport, stagedOffset)
		return
	}
	if m.focus == focusDiff {
		m.ensureActiveVisible(m.currentSection())
	}
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

func (m *Model) setStatus(msg string) {
	m.statusMsg = msg
	if msg == "" {
		m.statusUntil = time.Time{}
		return
	}
	m.statusUntil = time.Now().Add(statusMessageTTL)
}

func (m *Model) clearStatus() {
	m.statusMsg = ""
	m.statusUntil = time.Time{}
}
