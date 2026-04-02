package stage

import (
	"regexp"
	"strconv"
	"strings"

	"gx/git"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var deltaHunkHeaderRe = regexp.MustCompile(`^\s*[•*]\s+.+:\d+:(?:\s.*)?$`)
var deltaSideBySideLineRe = regexp.MustCompile(`^\s*│\s*([0-9]+)?\s*│.*│\s*([0-9]+)?\s*│`)

type movedTarget struct {
	fromSection diffSection
	navMode     navMode
	hunkHeader  string
	lineText    string
}

func (m *Model) applySelection() tea.Cmd {
	file, ok := m.selectedFile()
	if !ok {
		return nil
	}

	sec := m.currentSection()
	sig := movedTarget{fromSection: m.section, navMode: m.navMode}
	if file.IsUntracked() && m.section == sectionUnstaged {
		if err := git.StageIntentPath(m.worktreeRoot, file.Path); err != nil {
			m.showGitError(err)
			return nil
		}
	}

	if m.navMode == navHunk {
		if sec.activeHunk < 0 || sec.activeHunk >= len(sec.parsed.Hunks) {
			return nil
		}
		sig.hunkHeader = sec.parsed.Hunks[sec.activeHunk].Header
		patch, err := buildHunkPatch(sec.parsed, sec.activeHunk)
		if err != nil {
			m.setStatus(err.Error())
			return nil
		}
		reverse := m.section == sectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, false); err != nil {
			if !isCorruptPatchErr(err) {
				m.showGitError(err)
				return nil
			}
			h := sec.parsed.Hunks[sec.activeHunk]
			if len(h.ChangedLineOffset) == 0 {
				m.showGitError(err)
				return nil
			}
			startChanged := h.ChangedLineOffset[0]
			endChanged := h.ChangedLineOffset[len(h.ChangedLineOffset)-1]
			fallbackPatch, fallbackErr := buildLineRangePatch(sec.parsed, startChanged, endChanged)
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
		if sec.activeLine < 0 || sec.activeLine >= len(sec.parsed.Changed) {
			return nil
		}
		startLine, endLine := sec.activeLine, sec.activeLine
		if sec.visualActive {
			startLine, endLine = visualLineBounds(*sec)
		}
		sig.lineText = sec.parsed.Changed[endLine].Text

		var (
			patch string
			err   error
		)
		if sec.visualActive && endLine > startLine {
			patch, err = buildLineRangePatch(sec.parsed, startLine, endLine)
		} else {
			patch, err = buildSingleLinePatch(sec.parsed, sec.activeLine)
		}
		if err != nil {
			m.setStatus(err.Error())
			return nil
		}
		reverse := m.section == sectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, true); err != nil {
			m.showGitError(err)
			return nil
		}
	}

	sec.visualActive = false
	sec.visualAnchor = sec.activeLine
	m.setStatus("updated " + file.Path)
	from := m.section
	m.reload(file.Path)
	if m.shouldSwitchAfterApply(from) {
		m.focusMovedTarget(sig)
		if m.flash.active {
			return nextFlashCmd()
		}
	} else {
		m.section = from
		m.ensureActiveVisible(m.currentSection())
	}
	return nil
}

func isCorruptPatchErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "corrupt patch")
}

func (m *Model) shouldSwitchAfterApply(from diffSection) bool {
	var sec sectionState
	if from == sectionStaged {
		sec = m.staged
	} else {
		sec = m.unstaged
	}
	return len(sec.parsed.Hunks) == 0
}

func (m *Model) reload(preservePath string) {
	m.reloadBranchState()
	files, err := git.ListStageFiles(m.worktreeRoot)
	if err != nil {
		m.err = err
		m.files = nil
		m.statusEntries = nil
		m.unstaged = newSectionState()
		m.staged = newSectionState()
		return
	}
	m.err = nil
	m.files = files
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)
	if strings.TrimSpace(m.searchQuery) != "" && m.searchScope == searchScopeStatus {
		m.recomputeSearchMatches()
	}

	if len(m.statusEntries) == 0 {
		m.selected = 0
		m.unstaged = newSectionState()
		m.staged = newSectionState()
		m.focus = focusStatus
		return
	}

	if preservePath != "" {
		for i, entry := range m.statusEntries {
			if entry.Path == preservePath {
				m.selected = i
				break
			}
		}
	}
	if m.selected >= len(m.statusEntries) {
		m.selected = len(m.statusEntries) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}

	m.reloadDiffsForSelection()
}

func (m *Model) reloadDiffsForSelection() {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind == statusEntryDir {
		m.activeFilePath = ""
		m.unstaged = newSectionState()
		m.staged = newSectionState()
		m.syncDiffViewports()
		if strings.TrimSpace(m.searchQuery) != "" && (m.searchScope == searchScopeUnstaged || m.searchScope == searchScopeStaged) {
			m.recomputeSearchMatches()
		}
		return
	}

	file := entry.File
	if file.Path != m.activeFilePath {
		m.section = sectionUnstaged
		m.activeFilePath = file.Path
	}
	if file.IsUntracked() {
		renderWidth := m.deltaRenderWidth()
		raw, err := git.DiffUntrackedPath(m.worktreeRoot, file.Path, false, false, 0, m.settings.DiffContextLines)
		if err != nil {
			m.showGitError(err)
			raw = ""
		}
		col, err := git.DiffUntrackedPath(m.worktreeRoot, file.Path, true, m.renderMode == renderSideBySide, renderWidth, m.settings.DiffContextLines)
		if err != nil {
			col = raw
		}
		m.unstaged = buildSectionState(raw, col, m.unstaged, m.renderMode == renderSideBySide)
		m.staged = newSectionState()
		m.section = sectionUnstaged
		m.syncDiffViewports()
		return
	}

	unstagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, false, m.settings.DiffContextLines)
	if err != nil {
		m.showGitError(err)
		unstagedRaw = ""
	}
	renderWidth := m.deltaRenderWidth()
	unstagedColor, err := git.DiffPathWithDelta(m.worktreeRoot, file.Path, false, m.renderMode == renderSideBySide, renderWidth, m.settings.DiffContextLines)
	if err != nil {
		unstagedColor = unstagedRaw
	}

	stagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, true, m.settings.DiffContextLines)
	if err != nil {
		m.showGitError(err)
		stagedRaw = ""
	}
	stagedColor, err := git.DiffPathWithDelta(m.worktreeRoot, file.Path, true, m.renderMode == renderSideBySide, renderWidth, m.settings.DiffContextLines)
	if err != nil {
		stagedColor = stagedRaw
	}

	m.unstaged = buildSectionState(unstagedRaw, unstagedColor, m.unstaged, m.renderMode == renderSideBySide)
	m.staged = buildSectionState(stagedRaw, stagedColor, m.staged, m.renderMode == renderSideBySide)
	m.pickAvailableSection()
	m.syncDiffViewports()
	if strings.TrimSpace(m.searchQuery) != "" && (m.searchScope == searchScopeUnstaged || m.searchScope == searchScopeStaged) {
		m.recomputeSearchMatches()
	}
}

func (m *Model) enterDiffFromStatus(resetSection bool) {
	if _, ok := m.selectedFile(); !ok {
		return
	}
	m.diffReloadSeq++
	m.reloadDiffsForSelection()
	m.focus = focusDiff
	if resetSection {
		m.section = sectionUnstaged
	}
	m.pickAvailableSection()
	m.syncDiffViewports()
	m.ensureActiveVisible(m.currentSection())
}

func buildSectionState(raw, color string, prev sectionState, sideBySide bool) sectionState {
	state := sectionState{activeHunk: prev.activeHunk, activeLine: prev.activeLine, visualActive: prev.visualActive, visualAnchor: prev.visualAnchor, viewport: prev.viewport}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		state.activeHunk = -1
		state.activeLine = -1
		state.baseLines = nil
		state.baseDisplayToRaw = nil
		state.viewLines = nil
		state.displayToRaw = nil
		state.rawToDisplay = nil
		state.hunkDisplayRange = nil
		state.changedDisplay = nil
		state.visualActive = false
		state.visualAnchor = -1
		state.viewport.SetContent("")
		state.viewport.SetYOffset(0)
		return state
	}

	state.parsed = parseUnifiedDiff(raw)
	state.rawLines = append([]string{}, state.parsed.Lines...)
	if sideBySide {
		initSideBySideSectionState(&state, color)
		return state
	}

	colorLines := splitLines(color)
	if len(colorLines) == 0 {
		colorLines = append([]string{}, state.rawLines...)
	} else if len(colorLines) < len(state.rawLines) {
		colorLines = append(colorLines, state.rawLines[len(colorLines):]...)
	} else if len(colorLines) > len(state.rawLines) {
		colorLines = colorLines[:len(state.rawLines)]
	}
	state.baseLines, state.baseDisplayToRaw = buildDisplayBaseLines(state.parsed, colorLines)
	state.viewLines = append([]string{}, state.baseLines...)
	state.displayToRaw = append([]int{}, state.baseDisplayToRaw...)
	state.rawToDisplay = buildRawToDisplayMap(state.parsed, state.displayToRaw)
	state.hunkDisplayRange = nil
	state.changedDisplay = nil
	prevOffset := state.viewport.YOffset()
	state.viewport.SetContentLines(state.viewLines)
	state.viewport.SetYOffset(prevOffset)

	if len(state.parsed.Hunks) == 0 {
		state.activeHunk = -1
	} else {
		if state.activeHunk < 0 {
			state.activeHunk = 0
		}
		if state.activeHunk >= len(state.parsed.Hunks) {
			state.activeHunk = len(state.parsed.Hunks) - 1
		}
	}

	if len(state.parsed.Changed) == 0 {
		state.activeLine = -1
		state.visualActive = false
		state.visualAnchor = -1
	} else {
		if state.activeLine < 0 {
			state.activeLine = 0
		}
		if state.activeLine >= len(state.parsed.Changed) {
			state.activeLine = len(state.parsed.Changed) - 1
		}
		if state.visualAnchor < 0 || state.visualAnchor >= len(state.parsed.Changed) {
			state.visualAnchor = state.activeLine
		}
	}

	return state
}

func initSideBySideSectionState(state *sectionState, color string) {
	state.viewLines = splitLines(color)
	if len(state.viewLines) == 0 {
		state.viewLines = append([]string{}, state.rawLines...)
	}
	state.baseLines = append([]string{}, state.viewLines...)
	state.baseDisplayToRaw = make([]int, len(state.baseLines))
	for i := range state.baseDisplayToRaw {
		state.baseDisplayToRaw[i] = -1
	}
	state.displayToRaw = append([]int{}, state.baseDisplayToRaw...)
	state.changedDisplay = make([]int, len(state.parsed.Changed))
	for i := range state.changedDisplay {
		state.changedDisplay[i] = -1
	}
	mapSideBySideDisplayLinesToChanged(state)
	state.hunkDisplayRange = sideBySideHunkDisplayRanges(state.viewLines, len(state.parsed.Hunks))
	prevOffset := state.viewport.YOffset()
	state.viewport.SetContentLines(state.viewLines)
	state.viewport.SetYOffset(prevOffset)

	if len(state.parsed.Hunks) == 0 {
		state.activeHunk = -1
	} else {
		if state.activeHunk < 0 {
			state.activeHunk = 0
		}
		if state.activeHunk >= len(state.parsed.Hunks) {
			state.activeHunk = len(state.parsed.Hunks) - 1
		}
	}

	if len(state.parsed.Changed) == 0 {
		state.activeLine = -1
		state.visualActive = false
		state.visualAnchor = -1
	} else {
		if state.activeLine < 0 {
			state.activeLine = 0
		}
		if state.activeLine >= len(state.parsed.Changed) {
			state.activeLine = len(state.parsed.Changed) - 1
		}
		if state.visualAnchor < 0 || state.visualAnchor >= len(state.parsed.Changed) {
			state.visualAnchor = state.activeLine
		}
	}
}

func mapSideBySideDisplayLinesToChanged(state *sectionState) {
	oldByLine := map[int][]int{}
	newByLine := map[int][]int{}
	for i, cl := range state.parsed.Changed {
		if cl.Prefix == '-' {
			oldByLine[cl.OldLine] = append(oldByLine[cl.OldLine], i)
		}
		if cl.Prefix == '+' {
			newByLine[cl.NewLine] = append(newByLine[cl.NewLine], i)
		}
	}

	for displayIdx, line := range state.viewLines {
		plain := ansi.Strip(line)
		m := deltaSideBySideLineRe.FindStringSubmatch(plain)
		if m == nil {
			continue
		}
		left := parseOptionalLineNumber(m[1])
		right := parseOptionalLineNumber(m[2])

		if left > 0 {
			if queue := oldByLine[left]; len(queue) > 0 {
				idx := queue[0]
				oldByLine[left] = queue[1:]
				state.changedDisplay[idx] = displayIdx
				state.displayToRaw[displayIdx] = state.parsed.Changed[idx].LineIndex
			}
		}
		if right > 0 {
			if queue := newByLine[right]; len(queue) > 0 {
				idx := queue[0]
				newByLine[right] = queue[1:]
				state.changedDisplay[idx] = displayIdx
				if state.displayToRaw[displayIdx] < 0 {
					state.displayToRaw[displayIdx] = state.parsed.Changed[idx].LineIndex
				}
			}
		}
	}
	state.rawToDisplay = buildRawToDisplayMap(state.parsed, state.displayToRaw)
}

func parseOptionalLineNumber(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

func sideBySideHunkDisplayRanges(lines []string, hunkCount int) [][2]int {
	if hunkCount <= 0 || len(lines) == 0 {
		return nil
	}
	headers := make([]int, 0, hunkCount)
	for i, line := range lines {
		plain := strings.TrimSpace(ansi.Strip(line))
		if deltaHunkHeaderRe.MatchString(plain) {
			headers = append(headers, i)
		}
	}
	if len(headers) != hunkCount {
		return nil
	}
	ranges := make([][2]int, 0, hunkCount)
	for i, start := range headers {
		end := len(lines) - 1
		if i+1 < len(headers) {
			end = headers[i+1] - 1
		}
		for end >= start {
			plain := strings.TrimSpace(ansi.Strip(lines[end]))
			if plain == "" || isDeltaSectionDivider(plain) {
				end--
				continue
			}
			break
		}
		for start <= end {
			plain := strings.TrimSpace(ansi.Strip(lines[start]))
			if isDeltaSectionDivider(plain) {
				start++
				continue
			}
			break
		}
		if end < start {
			end = start
		}
		ranges = append(ranges, [2]int{start, end})
	}
	return ranges
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

func buildDisplayBaseLines(parsed parsedDiff, colorLines []string) (lines []string, displayToRaw []int) {
	if len(parsed.Lines) == 0 {
		return nil, nil
	}

	hdrStyle := lipgloss.NewStyle().Background(catSurface).Foreground(catText).Bold(true)
	for hi, h := range parsed.Hunks {
		if hi > 0 {
			lines = append(lines, "")
			displayToRaw = append(displayToRaw, -1)
		}

		header := cleanHunkHeader(parsed.Lines[h.StartLine])
		lines = append(lines, hdrStyle.Render(" "+header+" "))
		displayToRaw = append(displayToRaw, h.StartLine)

		for rawIdx := h.StartLine + 1; rawIdx <= h.EndLine && rawIdx < len(parsed.Lines); rawIdx++ {
			line := parsed.Lines[rawIdx]
			if rawIdx < len(colorLines) {
				line = sanitizeANSIInline(colorLines[rawIdx])
			}
			lines = append(lines, line)
			displayToRaw = append(displayToRaw, rawIdx)
		}
	}
	return lines, displayToRaw
}

func buildRawToDisplayMap(parsed parsedDiff, displayToRaw []int) []int {
	rawToDisplay := make([]int, len(parsed.Lines))
	for i := range rawToDisplay {
		rawToDisplay[i] = -1
	}
	for i, rawIdx := range displayToRaw {
		if rawIdx >= 0 && rawIdx < len(rawToDisplay) && rawToDisplay[rawIdx] < 0 {
			rawToDisplay[rawIdx] = i
		}
	}
	return rawToDisplay
}

func sanitizeANSIInline(s string) string {
	s = ansiOSCRe.ReplaceAllString(s, "")
	s = ansiCSIRe.ReplaceAllStringFunc(s, func(seq string) string {
		if strings.HasSuffix(seq, "m") {
			return seq
		}
		return ""
	})
	// Tabs can visually overflow panel width depending on terminal tab stops,
	// causing border glyphs to appear missing. Normalize to spaces.
	s = strings.ReplaceAll(s, "\t", "    ")
	// Drop residual C0 control chars that can affect cursor position/erase.
	b := make([]rune, 0, len(s))
	for _, r := range s {
		if (r < 0x20 && r != 0x1b) || r == 0x7f {
			continue
		}
		b = append(b, r)
	}
	return string(b)
}

func cleanHunkHeader(line string) string {
	first := strings.Index(line, "@@")
	if first == -1 {
		return strings.TrimSpace(line)
	}
	second := strings.Index(line[first+2:], "@@")
	if second == -1 {
		return strings.TrimSpace(line)
	}
	second = first + 2 + second
	tail := strings.TrimSpace(line[second+2:])
	if tail == "" {
		return "hunk"
	}
	return tail
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
