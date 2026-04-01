package stage

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gx/git"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) pickAvailableSection() {
	hasUnstaged := len(m.unstaged.viewLines) > 0
	hasStaged := len(m.staged.viewLines) > 0
	if hasUnstaged && !hasStaged {
		m.section = sectionUnstaged
	}
	if hasStaged && !hasUnstaged {
		m.section = sectionStaged
	}
}

func (m Model) canSwitchSections() bool {
	return len(m.unstaged.viewLines) > 0 && len(m.staged.viewLines) > 0
}

func (m *Model) currentSection() *sectionState {
	if m.section == sectionStaged {
		return &m.staged
	}
	return &m.unstaged
}

func (m *Model) ensureActiveVisible(sec *sectionState) {
	active := m.activeRawLineIndex(*sec)
	if active >= 0 {
		display := active
		if active < len(sec.rawToDisplay) && sec.rawToDisplay[active] >= 0 {
			display = sec.rawToDisplay[active]
		}
		sec.viewport.EnsureVisible(display, 0, 0)
	}
}

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

func cmdGitCommit(worktreeRoot string) tea.Cmd {
	if os.Getenv("TMUX") != "" {
		return func() tea.Msg {
			err := exec.Command("tmux", "split-window", "-v", "-c", worktreeRoot, "git commit").Run()
			return commitFinishedMsg{err: err, tmuxSplit: true}
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
	args := append(parts[1:], target)
	c := exec.Command(parts[0], args...)
	m.setStatus("opening editor...")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editFileFinishedMsg{err: err}
	})
}

func (m Model) selectedStatusEntry() (statusEntry, bool) {
	if m.selected < 0 || m.selected >= len(m.statusEntries) {
		return statusEntry{}, false
	}
	return m.statusEntries[m.selected], true
}

func (m *Model) refresh() {
	preserve := ""
	if entry, ok := m.selectedStatusEntry(); ok {
		preserve = entry.Path
	}
	m.reload(preserve)
	m.syncDiffViewports()
	if m.focus == focusDiff {
		m.ensureActiveVisible(m.currentSection())
	}
}

func (m Model) selectedFile() (git.StageFileStatus, bool) {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryFile {
		return git.StageFileStatus{}, false
	}
	return entry.File, true
}

func (m *Model) toggleDirOnEnter() bool {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryDir {
		return false
	}
	m.collapsedDirs[entry.Path] = entry.Expanded
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)
	if m.selected >= len(m.statusEntries) {
		m.selected = len(m.statusEntries) - 1
	}
	return true
}

func (m *Model) collapseSelectedDir() {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryDir || !entry.Expanded {
		return
	}
	m.collapsedDirs[entry.Path] = true
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)
	if m.selected >= len(m.statusEntries) {
		m.selected = len(m.statusEntries) - 1
	}
}

func (m *Model) focusParentInStatus() bool {
	entry, ok := m.selectedStatusEntry()
	if !ok {
		return false
	}
	parent := path.Dir(entry.Path)
	if parent == "." || parent == "" || parent == entry.Path {
		return false
	}
	for i, candidate := range m.statusEntries {
		if candidate.Kind == statusEntryDir && candidate.Path == parent {
			if m.selected == i {
				return false
			}
			m.selected = i
			m.onStatusSelectionChanged()
			return true
		}
	}
	return false
}

func (m *Model) expandSelectedDir() {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Expanded {
		return
	}
	delete(m.collapsedDirs, entry.Path)
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)
}

func (m *Model) moveToAdjacentFile(delta int) bool {
	if delta == 0 || len(m.statusEntries) == 0 {
		return false
	}
	idx := m.selected
	for {
		idx += delta
		if idx < 0 || idx >= len(m.statusEntries) {
			return false
		}
		if m.statusEntries[idx].Kind == statusEntryFile {
			m.selected = idx
			m.onStatusSelectionChanged()
			m.reloadDiffsForSelection()
			if m.focus == focusDiff {
				m.ensureActiveVisible(m.currentSection())
			}
			return true
		}
	}
}

func (m *Model) toggleStageStatusEntry() {
	entry, ok := m.selectedStatusEntry()
	if !ok {
		return
	}

	path := entry.Path
	stageAll := entry.HasUnstaged
	var err error
	if stageAll {
		err = git.StagePath(m.worktreeRoot, path)
	} else if entry.HasStaged {
		err = git.UnstagePath(m.worktreeRoot, path)
	} else {
		return
	}
	if err != nil {
		m.showGitError(err)
		return
	}
	if stageAll {
		m.setStatus("staged " + path)
	} else {
		m.setStatus("unstaged " + path)
	}
	m.reload(path)
}

func (m *Model) openDiscardStatusConfirm() {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryFile {
		return
	}

	title := fmt.Sprintf("Discard changes in %s?", entry.Path)
	paths := []string{entry.Path}
	lines := []string{}

	switch {
	case entry.File.IsUntracked():
		lines = append(lines, "This will delete the untracked file.")
	case entry.File.IsRenamed() && entry.File.RenameFrom != "":
		lines = append(lines,
			"This will undo the rename.",
			entry.File.RenameFrom+" -> "+entry.File.Path,
		)
		paths = []string{entry.File.RenameFrom, entry.File.Path}
	case entry.File.IndexStatus == 'A' || entry.File.WorktreeCode == 'A':
		lines = append(lines, "This will delete the new file.")
	case entry.File.IndexStatus == 'D' || entry.File.WorktreeCode == 'D':
		lines = append(lines, "This will restore the deleted file from HEAD.")
	default:
		lines = append(lines, "This will undo all changes in this file.")
	}

	m.openConfirm(title, lines, confirmDiscardStatus, "", "")
	m.confirmDiscardUntracked = entry.File.IsUntracked()
	m.confirmPaths = uniqueNonEmpty(paths)
}

func (m *Model) openDiscardDiffConfirm() {
	if m.section != sectionUnstaged {
		return
	}
	file, ok := m.selectedFile()
	if !ok {
		return
	}
	sec := m.currentSection()

	var (
		title       string
		lines       []string
		patch       string
		unidiffZero bool
		err         error
	)

	if m.navMode == navHunk {
		if sec.activeHunk < 0 || sec.activeHunk >= len(sec.parsed.Hunks) {
			return
		}
		patch, err = buildHunkPatch(sec.parsed, sec.activeHunk)
		title = "Discard selected hunk?"
		lines = []string{"This will discard the selected hunk from your working tree."}
	} else {
		if sec.activeLine < 0 || sec.activeLine >= len(sec.parsed.Changed) {
			return
		}
		startLine, endLine := sec.activeLine, sec.activeLine
		if sec.visualActive {
			startLine, endLine = visualLineBounds(*sec)
		}
		if sec.visualActive && endLine > startLine {
			patch, err = buildLineRangePatch(sec.parsed, startLine, endLine)
			title = "Discard selected lines?"
			lines = []string{"This will discard the selected lines from your working tree."}
		} else {
			patch, err = buildSingleLinePatch(sec.parsed, sec.activeLine)
			title = "Discard selected line?"
			lines = []string{"This will discard the selected line from your working tree."}
		}
		unidiffZero = true
	}

	if err != nil {
		m.setStatus(err.Error())
		return
	}

	m.openConfirm(title, lines, confirmDiscardUnstaged, "", "")
	m.confirmPaths = []string{file.Path}
	m.confirmPatch = patch
	m.confirmPatchUnidiffZero = unidiffZero
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

func (m *Model) focusMovedTarget(sig movedTarget) {
	if sig.fromSection == sectionUnstaged {
		m.section = sectionStaged
	} else {
		m.section = sectionUnstaged
	}
	var sec *sectionState
	if m.section == sectionStaged {
		sec = &m.staged
	} else {
		sec = &m.unstaged
	}

	if sig.navMode == navHunk {
		for i := range sec.parsed.Hunks {
			if sec.parsed.Hunks[i].Header == sig.hunkHeader {
				sec.activeHunk = i
				m.ensureActiveVisible(sec)
				m.flash = flashState{active: true, section: m.section, navMode: navHunk, hunk: i, frames: 4}
				return
			}
		}
		if len(sec.parsed.Hunks) > 0 {
			sec.activeHunk = 0
			m.ensureActiveVisible(sec)
			m.flash = flashState{active: true, section: m.section, navMode: navHunk, hunk: 0, frames: 4}
		}
		return
	}

	for i := range sec.parsed.Changed {
		if sec.parsed.Changed[i].Text == sig.lineText {
			sec.activeLine = i
			m.ensureActiveVisible(sec)
			m.flash = flashState{active: true, section: m.section, navMode: navLine, line: i, frames: 4}
			return
		}
	}
	if len(sec.parsed.Changed) > 0 {
		sec.activeLine = 0
		m.ensureActiveVisible(sec)
		m.flash = flashState{active: true, section: m.section, navMode: navLine, line: 0, frames: 4}
	}
}
