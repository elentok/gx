package status

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/status/diffarea"
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
	fromSection diffarea.Section
	navMode     diffview.NavMode
	hunkHeader  string
	lineText    string
}

func (m *Model) applySelection() tea.Cmd {
	file, ok := m.selectedStatusFile()
	if !ok {
		return nil
	}

	diffviewModel := m.diffarea.ActiveSectionModel()
	sig := movedTarget{fromSection: m.diffarea.ActiveSection, navMode: m.diffarea.NavMode()}
	if file.Untracked && m.diffarea.ActiveSection == diffarea.SectionUnstaged {
		if err := git.StageIntentPath(m.worktreeRoot, file.Path); err != nil {
			m.showGitError(err)
			return nil
		}
	}

	if m.diffarea.NavMode() == diffview.NavModeHunk {
		if diffviewModel.DataRef().ActiveHunk < 0 || diffviewModel.DataRef().ActiveHunk >= len(diffviewModel.DataRef().Parsed.Hunks) {
			return nil
		}
		sig.hunkHeader = diffviewModel.DataRef().Parsed.Hunks[diffviewModel.DataRef().ActiveHunk].Header
		patch, err := diffcore.BuildHunkPatch(diffviewModel.DataRef().Parsed, diffviewModel.DataRef().ActiveHunk)
		if err != nil {
			return notify.Error(err.Error())
		}
		reverse := m.diffarea.ActiveSection == diffarea.SectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, false); err != nil {
			if !isCorruptPatchErr(err) {
				m.showGitError(err)
				return nil
			}
			h := diffviewModel.DataRef().Parsed.Hunks[diffviewModel.DataRef().ActiveHunk]
			if len(h.ChangedLineOffset) == 0 {
				m.showGitError(err)
				return nil
			}
			startChanged := h.ChangedLineOffset[0]
			endChanged := h.ChangedLineOffset[len(h.ChangedLineOffset)-1]
			fallbackPatch, fallbackErr := diffcore.BuildLineRangePatch(diffviewModel.DataRef().Parsed, startChanged, endChanged)
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
		if diffviewModel.DataRef().ActiveLine < 0 || diffviewModel.DataRef().ActiveLine >= len(diffviewModel.DataRef().Parsed.Changed) {
			return nil
		}
		startLine, endLine := diffviewModel.DataRef().ActiveLine, diffviewModel.DataRef().ActiveLine
		if diffviewModel.DataRef().VisualActive {
			startLine, endLine = diffviewModel.Data().VisualLineBounds()
		}
		sig.lineText = diffviewModel.DataRef().Parsed.Changed[endLine].Text

		var (
			patch string
			err   error
		)
		if diffviewModel.DataRef().VisualActive && endLine > startLine {
			patch, err = diffcore.BuildLineRangePatch(diffviewModel.DataRef().Parsed, startLine, endLine)
		} else {
			patch, err = diffcore.BuildSingleLinePatch(diffviewModel.DataRef().Parsed, diffviewModel.DataRef().ActiveLine)
		}
		if err != nil {
			return notify.Error(err.Error())
		}
		reverse := m.diffarea.ActiveSection == diffarea.SectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, true); err != nil {
			m.showGitError(err)
			return nil
		}
	}

	diffviewModel.DataRef().VisualActive = false
	diffviewModel.DataRef().VisualAnchor = diffviewModel.DataRef().ActiveLine
	from := m.diffarea.ActiveSection
	reloadCmd := m.reload(file.Path)
	m.diffarea.ActiveSection = from
	m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	m.markMovedTarget(sig)
	if m.diffarea.Flash.Active {
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

	sideBySide := m.diffarea.RenderMode() == diffview.RenderModeSideBySide
	renderWidth := m.deltaRenderWidth()

	sel, ok := m.selectedStatusDiff()
	if !ok {
		m.activeFilePath = ""
		m.diffarea.ResetSections()
		m.syncDiffViewports()
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
		m.diffarea.SectionModel(diffarea.SectionUnstaged).BuildFromRaw(raw, color)
		m.diffarea.SectionModel(diffarea.SectionStaged).BuildFromRaw("", "")
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
	m.diffarea.SectionModel(diffarea.SectionUnstaged).BuildFromRaw(unstagedRaw, unstagedColor)
	m.diffarea.SectionModel(diffarea.SectionStaged).BuildFromRaw(stagedRaw, stagedColor)
	m.syncDiffViewports()
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
		m.diffarea.ActiveSection = diffarea.SectionUnstaged
	}
	m.syncDiffViewports()
	m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	return cmd
}

func (m *Model) openDiscardDiffConfirm() tea.Cmd {
	if m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		return nil
	}
	file, ok := m.selectedStatusFile()
	if !ok {
		return nil
	}
	diffviewModel := m.diffarea.ActiveSectionModel()

	var (
		title       string
		lines       []string
		patch       string
		unidiffZero bool
		err         error
	)

	if m.diffarea.NavMode() == diffview.NavModeHunk {
		if diffviewModel.DataRef().ActiveHunk < 0 || diffviewModel.DataRef().ActiveHunk >= len(diffviewModel.DataRef().Parsed.Hunks) {
			return nil
		}
		patch, err = diffcore.BuildHunkPatch(diffviewModel.DataRef().Parsed, diffviewModel.DataRef().ActiveHunk)
		title = "Discard selected hunk?"
		lines = []string{"This will discard the selected hunk from your working tree."}
	} else {
		if diffviewModel.DataRef().ActiveLine < 0 || diffviewModel.DataRef().ActiveLine >= len(diffviewModel.DataRef().Parsed.Changed) {
			return nil
		}
		startLine, endLine := diffviewModel.DataRef().ActiveLine, diffviewModel.DataRef().ActiveLine
		if diffviewModel.DataRef().VisualActive {
			startLine, endLine = diffviewModel.Data().VisualLineBounds()
		}
		if diffviewModel.DataRef().VisualActive && endLine > startLine {
			patch, err = diffcore.BuildLineRangePatch(diffviewModel.DataRef().Parsed, startLine, endLine)
			title = "Discard selected lines?"
			lines = []string{"This will discard the selected lines from your working tree."}
		} else {
			patch, err = diffcore.BuildSingleLinePatch(diffviewModel.DataRef().Parsed, diffviewModel.DataRef().ActiveLine)
			title = "Discard selected line?"
			lines = []string{"This will discard the selected line from your working tree."}
		}
		unidiffZero = true
	}

	if err != nil {
		return notify.Error(err.Error())
	}

	m.openConfirm(title, lines, confirmDiscardUnstaged, "", "")
	m.confirmPaths = []string{file.Path}
	m.confirmPatch = patch
	m.confirmPatchUnidiffZero = unidiffZero
	return nil
}

func (m *Model) markMovedTarget(sig movedTarget) {
	target := diffarea.SectionUnstaged
	if sig.fromSection == diffarea.SectionUnstaged {
		target = diffarea.SectionStaged
	}
	diffviewModel := m.diffarea.SectionModel(target)
	m.diffarea.Flash = diffarea.FlashState{Active: true, Section: target, NavMode: sig.navMode, Hunk: -1, Line: -1, Frames: 4}

	if sig.navMode == diffview.NavModeHunk {
		for i := range diffviewModel.DataRef().Parsed.Hunks {
			if diffviewModel.DataRef().Parsed.Hunks[i].Header == sig.hunkHeader {
				m.diffarea.Flash.Hunk = i
				return
			}
		}
		if len(diffviewModel.DataRef().Parsed.Hunks) > 0 {
			m.diffarea.Flash.Hunk = 0
		}
		return
	}

	for i := range diffviewModel.DataRef().Parsed.Changed {
		if diffviewModel.DataRef().Parsed.Changed[i].Text == sig.lineText {
			m.diffarea.Flash.Line = i
			return
		}
	}
	if len(diffviewModel.DataRef().Parsed.Changed) > 0 {
		m.diffarea.Flash.Line = 0
	}
}
