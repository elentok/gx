package status

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) colorizeDiffsSync(filePath, unstagedRaw, stagedRaw string, sideBySide bool, renderWidth int) (unstagedColor, stagedColor string) {
	contextLines := m.currentDiffContextLines()
	unstagedColor, _ = git.ColorizeDiff(m.worktreeRoot, filePath, unstagedRaw, false, sideBySide, renderWidth, contextLines)
	stagedColor, _ = git.ColorizeDiff(m.worktreeRoot, filePath, stagedRaw, true, sideBySide, renderWidth, contextLines)
	return unstagedColor, stagedColor
}

func (m *Model) colorizeUntrackedSync(filePath, rawDiff string, sideBySide bool, renderWidth int) string {
	contextLines := m.currentDiffContextLines()
	color, _ := git.ColorizeUntrackedDiff(m.worktreeRoot, filePath, rawDiff, sideBySide, renderWidth, contextLines)
	return color
}

type movedTarget struct {
	fromSection diffSection
	navMode     diffview.NavMode
	hunkHeader  string
	lineText    string
}

func (m *Model) applySelection() tea.Cmd {
	file, ok := m.selectedStatusFile()
	if !ok {
		return nil
	}

	sec := m.diff.ActiveSectionModel()
	sig := movedTarget{fromSection: m.diff.ActiveSection, navMode: m.diff.NavMode()}
	if file.Untracked && m.diff.ActiveSection == sectionUnstaged {
		if err := git.StageIntentPath(m.worktreeRoot, file.Path); err != nil {
			m.showGitError(err)
			return nil
		}
	}

	if m.diff.NavMode() == diffview.NavModeHunk {
		if sec.DataRef().ActiveHunk < 0 || sec.DataRef().ActiveHunk >= len(sec.DataRef().Parsed.Hunks) {
			return nil
		}
		sig.hunkHeader = sec.DataRef().Parsed.Hunks[sec.DataRef().ActiveHunk].Header
		patch, err := diffcore.BuildHunkPatch(sec.DataRef().Parsed, sec.DataRef().ActiveHunk)
		if err != nil {
			m.setStatus(err.Error())
			return nil
		}
		reverse := m.diff.ActiveSection == sectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, false); err != nil {
			if !isCorruptPatchErr(err) {
				m.showGitError(err)
				return nil
			}
			h := sec.DataRef().Parsed.Hunks[sec.DataRef().ActiveHunk]
			if len(h.ChangedLineOffset) == 0 {
				m.showGitError(err)
				return nil
			}
			startChanged := h.ChangedLineOffset[0]
			endChanged := h.ChangedLineOffset[len(h.ChangedLineOffset)-1]
			fallbackPatch, fallbackErr := diffcore.BuildLineRangePatch(sec.DataRef().Parsed, startChanged, endChanged)
			if fallbackErr != nil {
				m.showGitError(err)
				return nil
			}
			if fallbackApplyErr := git.ApplyPatchToIndex(m.worktreeRoot, fallbackPatch, reverse, true); fallbackApplyErr != nil {
				m.showGitError(err)
				return nil
			}
		}
	} else {
		if sec.DataRef().ActiveLine < 0 || sec.DataRef().ActiveLine >= len(sec.DataRef().Parsed.Changed) {
			return nil
		}
		startLine, endLine := sec.DataRef().ActiveLine, sec.DataRef().ActiveLine
		if sec.DataRef().VisualActive {
			startLine, endLine = visualLineBounds(sec.Data())
		}
		sig.lineText = sec.DataRef().Parsed.Changed[endLine].Text

		var (
			patch string
			err   error
		)
		if sec.DataRef().VisualActive && endLine > startLine {
			patch, err = diffcore.BuildLineRangePatch(sec.DataRef().Parsed, startLine, endLine)
		} else {
			patch, err = diffcore.BuildSingleLinePatch(sec.DataRef().Parsed, sec.DataRef().ActiveLine)
		}
		if err != nil {
			m.setStatus(err.Error())
			return nil
		}
		reverse := m.diff.ActiveSection == sectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, true); err != nil {
			m.showGitError(err)
			return nil
		}
	}

	sec.DataRef().VisualActive = false
	sec.DataRef().VisualAnchor = sec.DataRef().ActiveLine
	m.setStatus("updated " + file.Path)
	from := m.diff.ActiveSection
	reloadCmd := m.reload(file.Path)
	m.diff.ActiveSection = from
	m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
	m.markMovedTarget(sig)
	if m.diff.Flash.Active {
		return tea.Batch(reloadCmd, nextFlashCmd())
	}
	return reloadCmd
}

func isCorruptPatchErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "corrupt patch")
}

func (m *Model) reloadDiffsForSelection() tea.Cmd {

	sideBySide := m.diff.RenderMode() == diffview.RenderModeSideBySide
	renderWidth := m.deltaRenderWidth()

	sel, ok := m.selectedStatusDiff()
	if !ok {
		m.activeFilePath = ""
		m.diff.ResetSections()
		m.syncDiffViewports()
		if m.diffSearchActiveInFocus() {
			m.recomputeSearchMatches()
		}
		return nil
	}

	file := sel.file
	if file.Path != m.activeFilePath {
		m.activeFilePath = file.Path
	}
	if file.Untracked {
		raw, err := git.DiffUntrackedPath(m.worktreeRoot, file.Path, false, false, 0, m.currentDiffContextLines())
		if err != nil {
			m.showGitError(err)
			raw = ""
		}
		color := m.colorizeUntrackedSync(file.Path, raw, sideBySide, renderWidth)
		if color == "" {
			color = raw
		}
		m.diff.SectionModel(sectionUnstaged).BuildFromRaw(raw, color)
		m.diff.SectionModel(sectionStaged).BuildFromRaw("", "")
		m.syncDiffViewports()
		return nil
	}

	unstagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, false, m.currentDiffContextLines())
	if err != nil {
		m.showGitError(err)
		unstagedRaw = ""
	}
	stagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, true, m.currentDiffContextLines())
	if err != nil {
		m.showGitError(err)
		stagedRaw = ""
	}

	unstagedColor, stagedColor := m.colorizeDiffsSync(file.Path, unstagedRaw, stagedRaw, sideBySide, renderWidth)
	if unstagedColor == "" {
		unstagedColor = unstagedRaw
	}
	if stagedColor == "" {
		stagedColor = stagedRaw
	}
	m.diff.SectionModel(sectionUnstaged).BuildFromRaw(unstagedRaw, unstagedColor)
	m.diff.SectionModel(sectionStaged).BuildFromRaw(stagedRaw, stagedColor)
	m.syncDiffViewports()
	if m.diffSearchActiveInFocus() {
		m.recomputeSearchMatches()
	}
	return nil
}

func (m *Model) enterDiffFromStatus(resetSection bool) tea.Cmd {
	if _, ok := m.selectedStatusFile(); !ok {
		return nil
	}
	m.diffReloadSeq++
	cmd := m.reloadDiffsForSelection()
	m.focus = focusDiff
	if resetSection {
		m.diff.ActiveSection = sectionUnstaged
	}
	m.syncDiffViewports()
	m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
	return cmd
}

func (m *Model) openDiscardDiffConfirm() {
	if m.diff.ActiveSection != sectionUnstaged {
		return
	}
	file, ok := m.selectedStatusFile()
	if !ok {
		return
	}
	sec := m.diff.ActiveSectionModel()

	var (
		title       string
		lines       []string
		patch       string
		unidiffZero bool
		err         error
	)

	if m.diff.NavMode() == diffview.NavModeHunk {
		if sec.DataRef().ActiveHunk < 0 || sec.DataRef().ActiveHunk >= len(sec.DataRef().Parsed.Hunks) {
			return
		}
		patch, err = diffcore.BuildHunkPatch(sec.DataRef().Parsed, sec.DataRef().ActiveHunk)
		title = "Discard selected hunk?"
		lines = []string{"This will discard the selected hunk from your working tree."}
	} else {
		if sec.DataRef().ActiveLine < 0 || sec.DataRef().ActiveLine >= len(sec.DataRef().Parsed.Changed) {
			return
		}
		startLine, endLine := sec.DataRef().ActiveLine, sec.DataRef().ActiveLine
		if sec.DataRef().VisualActive {
			startLine, endLine = visualLineBounds(sec.Data())
		}
		if sec.DataRef().VisualActive && endLine > startLine {
			patch, err = diffcore.BuildLineRangePatch(sec.DataRef().Parsed, startLine, endLine)
			title = "Discard selected lines?"
			lines = []string{"This will discard the selected lines from your working tree."}
		} else {
			patch, err = diffcore.BuildSingleLinePatch(sec.DataRef().Parsed, sec.DataRef().ActiveLine)
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

func (m *Model) markMovedTarget(sig movedTarget) {
	target := sectionUnstaged
	if sig.fromSection == sectionUnstaged {
		target = sectionStaged
	}
	sec := m.diff.SectionModel(target)
	m.diff.Flash = diffarea.FlashState{Active: true, Section: target, NavMode: sig.navMode, Hunk: -1, Line: -1, Frames: 4}

	if sig.navMode == diffview.NavModeHunk {
		for i := range sec.DataRef().Parsed.Hunks {
			if sec.DataRef().Parsed.Hunks[i].Header == sig.hunkHeader {
				m.diff.Flash.Hunk = i
				return
			}
		}
		if len(sec.DataRef().Parsed.Hunks) > 0 {
			m.diff.Flash.Hunk = 0
		}
		return
	}

	for i := range sec.DataRef().Parsed.Changed {
		if sec.DataRef().Parsed.Changed[i].Text == sig.lineText {
			m.diff.Flash.Line = i
			return
		}
	}
	if len(sec.DataRef().Parsed.Changed) > 0 {
		m.diff.Flash.Line = 0
	}
}

func isDeltaSectionDivider(plain string) bool {
	if plain == "" {
		return false
	}
	for _, r := range plain {
		if r != '─' && r != '-' {
			return false
		}
	}
	return true
}
