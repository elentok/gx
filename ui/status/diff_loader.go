package status

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/status/diffarea"
)

// diffLoader loads the selected file's diffs into the diff sections and applies
// the selection to the index. It owns the debounce sequence and the path of the
// file whose diffs are loaded.
type diffLoader struct {
	activeFilePath string
	reloadSeq      int
}

// diffHost is what the loader needs from the Model while it works.
type diffHost interface {
	showGitError(err error)
	reload(preservePath string) tea.Cmd
	syncDiffViewports()
	focusDiffPane()
}

// diffLoadEnv carries the Model state a load reads.
type diffLoadEnv struct {
	host         diffHost
	diffarea     *diffarea.Model
	worktreeRoot string
	home         string
	file         statusDiffFileSelection
	hasFile      bool
	contextLines int
	renderWidth  int
}

type movedTarget struct {
	fromSection diffarea.Section
	navMode     diffview.NavMode
	hunkHeader  string
	lineText    string
}

func (diffLoader) colorizeDiffsSync(e diffLoadEnv, filePath, unstagedRaw, stagedRaw string, sideBySide bool) (unstagedColor, stagedColor string) {
	unstagedColor, _ = stage.ColorizeDiff(e.worktreeRoot, e.home, filePath, unstagedRaw, false, sideBySide, e.renderWidth, e.contextLines)
	stagedColor, _ = stage.ColorizeDiff(e.worktreeRoot, e.home, filePath, stagedRaw, true, sideBySide, e.renderWidth, e.contextLines)
	return unstagedColor, stagedColor
}

func (diffLoader) colorizeUntrackedSync(e diffLoadEnv, filePath, rawDiff string, sideBySide bool) string {
	color, _ := stage.ColorizeUntrackedDiff(e.worktreeRoot, e.home, filePath, rawDiff, sideBySide, e.renderWidth, e.contextLines)
	return color
}

func (l *diffLoader) applySelection(e diffLoadEnv) tea.Cmd {
	if !e.hasFile {
		return nil
	}
	file := e.file
	da := e.diffarea

	diffviewModel := da.ActiveSectionModel()
	sig := movedTarget{fromSection: da.ActiveSection, navMode: da.NavMode()}
	if file.Untracked && da.ActiveSection == diffarea.SectionUnstaged {
		if err := stage.StageIntentPath(e.worktreeRoot, file.Path); err != nil {
			e.host.showGitError(err)
			return nil
		}
	}

	if da.NavMode() == diffview.NavModeHunk {
		if diffviewModel.DataRef().ActiveHunk < 0 || diffviewModel.DataRef().ActiveHunk >= len(diffviewModel.DataRef().Parsed.Hunks) {
			return nil
		}
		sig.hunkHeader = diffviewModel.DataRef().Parsed.Hunks[diffviewModel.DataRef().ActiveHunk].Header
		patch, err := diffcore.BuildHunkPatch(diffviewModel.DataRef().Parsed, diffviewModel.DataRef().ActiveHunk)
		if err != nil {
			return notify.Error(err.Error())
		}
		reverse := da.ActiveSection == diffarea.SectionStaged
		if err := stage.ApplyPatchToIndex(e.worktreeRoot, patch, reverse, false); err != nil {
			if !isCorruptPatchErr(err) {
				e.host.showGitError(err)
				return nil
			}
			h := diffviewModel.DataRef().Parsed.Hunks[diffviewModel.DataRef().ActiveHunk]
			if len(h.ChangedLineOffset) == 0 {
				e.host.showGitError(err)
				return nil
			}
			startChanged := h.ChangedLineOffset[0]
			endChanged := h.ChangedLineOffset[len(h.ChangedLineOffset)-1]
			fallbackPatch, fallbackErr := diffcore.BuildLineRangePatch(diffviewModel.DataRef().Parsed, startChanged, endChanged)
			if fallbackErr != nil {
				e.host.showGitError(err)
				return nil
			}
			if fallbackApplyErr := stage.ApplyPatchToIndex(e.worktreeRoot, fallbackPatch, reverse, true); fallbackApplyErr != nil {
				e.host.showGitError(err)
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
		reverse := da.ActiveSection == diffarea.SectionStaged
		if err := stage.ApplyPatchToIndex(e.worktreeRoot, patch, reverse, true); err != nil {
			e.host.showGitError(err)
			return nil
		}
	}

	diffviewModel.DataRef().VisualActive = false
	diffviewModel.DataRef().VisualAnchor = diffviewModel.DataRef().ActiveLine
	from := da.ActiveSection
	reloadCmd := e.host.reload(file.Path)
	da.ActiveSection = from
	da.ActiveSectionModel().EnsureActiveVisible(da.NavMode())
	l.markMovedTarget(da, sig)
	mutatedCmd := statusRepoMutatedCmd()
	if da.Flash.Active {
		return tea.Batch(reloadCmd, nextFlashCmd(), mutatedCmd)
	}
	return tea.Batch(reloadCmd, mutatedCmd)
}

func isCorruptPatchErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "corrupt patch")
}

func (l *diffLoader) reloadDiffsForSelection(e diffLoadEnv) tea.Cmd {
	da := e.diffarea
	sideBySide := da.RenderMode() == diffview.RenderModeSideBySide

	if !e.hasFile {
		l.activeFilePath = ""
		da.ResetSections()
		e.host.syncDiffViewports()
		return nil
	}

	file := e.file
	if file.Path != l.activeFilePath {
		l.activeFilePath = file.Path
	}
	if file.Untracked {
		raw, err := stage.DiffUntrackedPath(e.worktreeRoot, e.home, file.Path, false, false, 0, e.contextLines)
		if err != nil {
			e.host.showGitError(err)
			raw = ""
		}
		color := l.colorizeUntrackedSync(e, file.Path, raw, sideBySide)
		if color == "" {
			color = raw
		}
		da.SectionModel(diffarea.SectionUnstaged).BuildFromRaw(raw, color)
		da.SectionModel(diffarea.SectionStaged).BuildFromRaw("", "")
		e.host.syncDiffViewports()
		return nil
	}

	unstagedRaw, err := stage.DiffPath(e.worktreeRoot, file.Path, false, e.contextLines)
	if err != nil {
		e.host.showGitError(err)
		unstagedRaw = ""
	}
	stagedRaw, err := stage.DiffPath(e.worktreeRoot, file.Path, true, e.contextLines)
	if err != nil {
		e.host.showGitError(err)
		stagedRaw = ""
	}

	unstagedColor, stagedColor := l.colorizeDiffsSync(e, file.Path, unstagedRaw, stagedRaw, sideBySide)
	if unstagedColor == "" {
		unstagedColor = unstagedRaw
	}
	if stagedColor == "" {
		stagedColor = stagedRaw
	}
	da.SectionModel(diffarea.SectionUnstaged).BuildFromRaw(unstagedRaw, unstagedColor)
	da.SectionModel(diffarea.SectionStaged).BuildFromRaw(stagedRaw, stagedColor)
	e.host.syncDiffViewports()
	return nil
}

func (l *diffLoader) enterDiffFromStatus(e diffLoadEnv, resetSection bool) tea.Cmd {
	if !e.hasFile {
		return nil
	}
	l.reloadSeq++
	cmd := l.reloadDiffsForSelection(e)
	e.host.focusDiffPane()
	if resetSection {
		e.diffarea.ActiveSection = diffarea.SectionUnstaged
	}
	e.host.syncDiffViewports()
	e.diffarea.ActiveSectionModel().EnsureActiveVisible(e.diffarea.NavMode())
	return cmd
}

func (l *diffLoader) scheduleDiffReload() tea.Cmd {
	l.reloadSeq++
	seq := l.reloadSeq
	return tea.Tick(statusDiffReloadDebounce, func(time.Time) tea.Msg {
		return diffReloadMsg{seq: seq}
	})
}

// isCurrentReload reports whether seq is the latest scheduled reload.
func (l diffLoader) isCurrentReload(seq int) bool {
	return seq == l.reloadSeq
}

func (diffLoader) markMovedTarget(da *diffarea.Model, sig movedTarget) {
	target := diffarea.SectionUnstaged
	if sig.fromSection == diffarea.SectionUnstaged {
		target = diffarea.SectionStaged
	}
	diffviewModel := da.SectionModel(target)
	da.Flash = diffarea.FlashState{Active: true, Section: target, NavMode: sig.navMode, Hunk: -1, Line: -1, Frames: 4}

	if sig.navMode == diffview.NavModeHunk {
		for i := range diffviewModel.DataRef().Parsed.Hunks {
			if diffviewModel.DataRef().Parsed.Hunks[i].Header == sig.hunkHeader {
				da.Flash.Hunk = i
				return
			}
		}
		if len(diffviewModel.DataRef().Parsed.Hunks) > 0 {
			da.Flash.Hunk = 0
		}
		return
	}

	for i := range diffviewModel.DataRef().Parsed.Changed {
		if diffviewModel.DataRef().Parsed.Changed[i].Text == sig.lineText {
			da.Flash.Line = i
			return
		}
	}
	if len(diffviewModel.DataRef().Parsed.Changed) > 0 {
		da.Flash.Line = 0
	}
}
