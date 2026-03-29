package stage

import (
	"fmt"
	"strings"
	"time"

	"gx/git"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type focusPane int

const (
	focusStatus focusPane = iota
	focusDiff
)

type diffSection int

const (
	sectionUnstaged diffSection = iota
	sectionStaged
)

type navMode int

const (
	navHunk navMode = iota
	navLine
)

type sectionState struct {
	rawLines   []string
	viewLines  []string
	parsed     parsedDiff
	activeHunk int
	activeLine int
	viewport   viewport.Model
}

type Model struct {
	worktreeRoot string
	settings     Settings

	width  int
	height int
	ready  bool

	focus   focusPane
	section diffSection
	navMode navMode

	files         []git.StageFileStatus
	statusEntries []statusEntry
	collapsedDirs map[string]bool
	selected      int

	unstaged sectionState
	staged   sectionState

	statusMsg string
	err       error
	flash     flashState
}

type Settings struct {
	DiffContextLines int
}

func DefaultSettings() Settings {
	return Settings{DiffContextLines: 1}
}

type flashState struct {
	active  bool
	section diffSection
	navMode navMode
	hunk    int
	line    int
	frames  int
}

type flashTickMsg struct{}

var (
	catBase0  = lipgloss.Color("#1e1e2e")
	catText   = lipgloss.Color("#cdd6f4")
	catSubtle = lipgloss.Color("#a6adc8")
	catBlue   = lipgloss.Color("#89b4fa")
	catGreen  = lipgloss.Color("#a6e3a1")
	catYellow = lipgloss.Color("#f9e2af")
	catRed    = lipgloss.Color("#f38ba8")
	catOrange = lipgloss.Color("#fab387")
)

func New(worktreeRoot string) Model {
	return NewWithSettings(worktreeRoot, DefaultSettings())
}

func NewWithSettings(worktreeRoot string, settings Settings) Model {
	if settings.DiffContextLines < 0 {
		settings.DiffContextLines = 0
	}
	if settings.DiffContextLines > 20 {
		settings.DiffContextLines = 20
	}
	m := Model{
		worktreeRoot:  worktreeRoot,
		settings:      settings,
		focus:         focusStatus,
		section:       sectionUnstaged,
		navMode:       navHunk,
		collapsedDirs: map[string]bool{},
		selected:      0,
		unstaged:      newSectionState(),
		staged:        newSectionState(),
	}
	m.reload("")
	return m
}

func newSectionState() sectionState {
	vp := viewport.New()
	return sectionState{
		activeHunk: -1,
		activeLine: -1,
		viewport:   vp,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.syncDiffViewports()
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.focus == focusStatus {
			return m.handleStatusKey(msg)
		}
		return m.handleDiffKey(msg)
	case flashTickMsg:
		if m.flash.active {
			m.flash.frames--
			if m.flash.frames <= 0 {
				m.flash.active = false
				return m, nil
			}
			return m, nextFlashCmd()
		}
	}
	return m, nil
}

func (m Model) handleStatusKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.selected < len(m.statusEntries)-1 {
			m.selected++
			m.reloadDiffsForSelection()
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
			m.reloadDiffsForSelection()
		}
	case "h", "left":
		m.collapseSelectedDir()
		m.reloadDiffsForSelection()
	case "l", "right":
		m.expandSelectedDir()
		m.reloadDiffsForSelection()
	case "space":
		m.toggleStageStatusEntry()
		m.reloadDiffsForSelection()
	case "enter":
		if m.toggleDirOnEnter() {
			m.reloadDiffsForSelection()
			return m, nil
		}
		m.focus = focusDiff
		m.pickAvailableSection()
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleDiffKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.focus = focusStatus
		return m, nil
	case "tab":
		if m.canSwitchSections() {
			if m.section == sectionUnstaged {
				m.section = sectionStaged
			} else {
				m.section = sectionUnstaged
			}
			m.ensureActiveVisible(m.currentSection())
		}
	case "a":
		if m.navMode == navHunk {
			m.navMode = navLine
		} else {
			m.navMode = navHunk
		}
		m.ensureActiveVisible(m.currentSection())
	case "j", "down":
		m.moveActive(1)
	case "k", "up":
		m.moveActive(-1)
	case "J":
		sec := m.currentSection()
		sec.viewport.ScrollDown(3)
	case "K":
		sec := m.currentSection()
		sec.viewport.ScrollUp(3)
	case "space":
		cmd := m.applySelection()
		return m, cmd
	}
	return m, nil
}

func (m *Model) moveActive(delta int) {
	sec := m.currentSection()
	if m.navMode == navHunk {
		if len(sec.parsed.Hunks) == 0 {
			return
		}
		sec.activeHunk += delta
		if sec.activeHunk < 0 {
			sec.activeHunk = 0
		}
		if sec.activeHunk >= len(sec.parsed.Hunks) {
			sec.activeHunk = len(sec.parsed.Hunks) - 1
		}
	} else {
		if len(sec.parsed.Changed) == 0 {
			return
		}
		sec.activeLine += delta
		if sec.activeLine < 0 {
			sec.activeLine = 0
		}
		if sec.activeLine >= len(sec.parsed.Changed) {
			sec.activeLine = len(sec.parsed.Changed) - 1
		}
	}
	m.ensureActiveVisible(sec)
}

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

	if file.IsUntracked() && m.section == sectionUnstaged {
		if err := git.StagePath(m.worktreeRoot, file.Path); err != nil {
			m.statusMsg = err.Error()
			return nil
		}
		m.statusMsg = "staged " + file.Path
		m.reload(file.Path)
		return nil
	}

	sec := m.currentSection()
	sig := movedTarget{fromSection: m.section, navMode: m.navMode}

	if m.navMode == navHunk {
		if sec.activeHunk < 0 || sec.activeHunk >= len(sec.parsed.Hunks) {
			return nil
		}
		sig.hunkHeader = sec.parsed.Hunks[sec.activeHunk].Header
		patch, err := buildHunkPatch(sec.parsed, sec.activeHunk)
		if err != nil {
			m.statusMsg = err.Error()
			return nil
		}
		reverse := m.section == sectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, false); err != nil {
			m.statusMsg = err.Error()
			return nil
		}
	} else {
		if sec.activeLine < 0 || sec.activeLine >= len(sec.parsed.Changed) {
			return nil
		}
		sig.lineText = sec.parsed.Changed[sec.activeLine].Text
		patch, err := buildSingleLinePatch(sec.parsed, sec.activeLine)
		if err != nil {
			m.statusMsg = err.Error()
			return nil
		}
		reverse := m.section == sectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, true); err != nil {
			m.statusMsg = err.Error()
			return nil
		}
	}

	m.statusMsg = "updated " + file.Path
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
	files, err := git.ListStageFiles(m.worktreeRoot)
	if err != nil {
		m.err = err
		m.files = nil
		m.statusEntries = nil
		m.unstaged = sectionState{activeHunk: -1, activeLine: -1}
		m.staged = sectionState{activeHunk: -1, activeLine: -1}
		return
	}
	m.err = nil
	m.files = files
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)

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
		m.unstaged = newSectionState()
		m.staged = newSectionState()
		m.syncDiffViewports()
		return
	}

	file := entry.File
	if file.IsUntracked() {
		raw, err := git.DiffUntrackedPath(m.worktreeRoot, file.Path, false, m.settings.DiffContextLines)
		if err != nil {
			m.statusMsg = err.Error()
			raw = ""
		}
		col, err := git.DiffUntrackedPath(m.worktreeRoot, file.Path, true, m.settings.DiffContextLines)
		if err != nil {
			col = raw
		}
		m.unstaged = buildSectionState(raw, col, m.unstaged)
		m.staged = newSectionState()
		m.section = sectionUnstaged
		m.syncDiffViewports()
		return
	}

	unstagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, false, m.settings.DiffContextLines)
	if err != nil {
		m.statusMsg = err.Error()
		unstagedRaw = ""
	}
	unstagedColor, err := git.DiffPathWithDelta(m.worktreeRoot, file.Path, false, m.settings.DiffContextLines)
	if err != nil {
		unstagedColor = unstagedRaw
	}

	stagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, true, m.settings.DiffContextLines)
	if err != nil {
		m.statusMsg = err.Error()
		stagedRaw = ""
	}
	stagedColor, err := git.DiffPathWithDelta(m.worktreeRoot, file.Path, true, m.settings.DiffContextLines)
	if err != nil {
		stagedColor = stagedRaw
	}

	m.unstaged = buildSectionState(unstagedRaw, unstagedColor, m.unstaged)
	m.staged = buildSectionState(stagedRaw, stagedColor, m.staged)
	m.pickAvailableSection()
	m.syncDiffViewports()
}

func buildSectionState(raw, color string, prev sectionState) sectionState {
	state := sectionState{activeHunk: prev.activeHunk, activeLine: prev.activeLine, viewport: prev.viewport}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		state.activeHunk = -1
		state.activeLine = -1
		state.viewport.SetContent("")
		state.viewport.SetYOffset(0)
		return state
	}

	state.parsed = parseUnifiedDiff(raw)
	state.rawLines = append([]string{}, state.parsed.Lines...)

	colorLines := splitLines(color)
	if len(colorLines) != len(state.rawLines) {
		colorLines = append([]string{}, state.rawLines...)
	}
	state.viewLines = colorLines
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
	} else {
		if state.activeLine < 0 {
			state.activeLine = 0
		}
		if state.activeLine >= len(state.parsed.Changed) {
			state.activeLine = len(state.parsed.Changed) - 1
		}
	}

	return state
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

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

func (m Model) View() tea.View {
	if !m.ready {
		v := tea.NewView("\n  Loading stage UI…")
		v.AltScreen = true
		return v
	}

	if m.err != nil {
		v := tea.NewView("\n  Error: " + m.err.Error())
		v.AltScreen = true
		return v
	}

	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}

	statusW, diffW := m.splitWidth()
	statusH, diffH := m.splitHeight(mainH)

	statusPanel := m.renderStatusPane(statusW, statusH)
	diffPanel := m.renderDiffPane(diffW, diffH)

	var body string
	if m.useStackedLayout() {
		body = lipgloss.JoinVertical(lipgloss.Left, statusPanel, diffPanel)
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, statusPanel, diffPanel)
	}

	footer := m.helpLine()
	out := lipgloss.JoinVertical(lipgloss.Left, body, footer)
	v := tea.NewView(out)
	v.AltScreen = true
	return v
}

func (m Model) splitWidth() (statusW, diffW int) {
	if m.useStackedLayout() {
		return m.width, m.width
	}
	statusW = int(float64(m.width) * 0.30)
	if statusW < 20 {
		statusW = 20
	}
	diffW = m.width - statusW
	if diffW < 20 {
		diffW = 20
		statusW = m.width - diffW
	}
	return statusW, diffW
}

func (m Model) splitHeight(total int) (statusH, diffH int) {
	if !m.useStackedLayout() {
		return total, total
	}
	statusH = int(float64(total) * 0.30)
	if statusH < 5 {
		statusH = 5
	}
	diffH = total - statusH
	if diffH < 5 {
		diffH = 5
		statusH = total - diffH
	}
	return statusH, diffH
}

func (m Model) useStackedLayout() bool {
	return m.width <= 100
}

func (m Model) renderStatusPane(width, height int) string {
	innerW := maxInt(1, width-2)
	innerH := maxInt(1, height-2)
	lines := make([]string, 0, innerH)
	title := lipgloss.NewStyle().Foreground(catBlue).Bold(true).Render(" Status")
	if m.focus == focusStatus {
		title = lipgloss.NewStyle().Foreground(catOrange).Bold(true).Render(" Status *")
	}
	lines = append(lines, ansi.Truncate(title, innerW, ""))

	bodyH := innerH - 1
	if bodyH < 0 {
		bodyH = 0
	}

	if len(m.statusEntries) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(catSubtle).Render("clean working tree"))
	} else {
		start := m.selected - bodyH/2
		if start < 0 {
			start = 0
		}
		if start > len(m.statusEntries)-bodyH {
			start = len(m.statusEntries) - bodyH
		}
		if start < 0 {
			start = 0
		}
		end := start + bodyH
		if end > len(m.statusEntries) {
			end = len(m.statusEntries)
		}
		for i := start; i < end; i++ {
			entry := m.statusEntries[i]
			mark := "  "
			if i == m.selected {
				mark = lipgloss.NewStyle().Foreground(catOrange).Render("▌ ")
			}
			indent := strings.Repeat("  ", entry.Depth)
			meta := statusEntryMeta(entry)
			name := entry.DisplayName
			if entry.Kind == statusEntryDir {
				symbol := "▾"
				if !entry.Expanded {
					symbol = "▸"
				}
				name = symbol + " " + name + "/"
				name = lipgloss.NewStyle().Foreground(catBlue).Bold(true).Render(name)
			} else if entry.HasOnlyUntracked {
				name = lipgloss.NewStyle().Foreground(catGreen).Render(name)
			}
			line := fmt.Sprintf("%s%s%s %s", mark, indent, meta, name)
			if i == m.selected {
				line = lipgloss.NewStyle().Bold(true).Foreground(catText).Render(line)
			}
			lines = append(lines, ansi.Truncate(line, innerW, ""))
		}
	}

	for len(lines) < innerH {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	return m.panelStyle(m.focus == focusStatus).
		Width(width).
		Height(height).
		Render(content)
}

func (m *Model) renderDiffPane(width, height int) string {
	hasUnstaged := len(m.unstaged.viewLines) > 0
	hasStaged := len(m.staged.viewLines) > 0

	if !hasUnstaged && !hasStaged {
		content := lipgloss.NewStyle().Foreground(catSubtle).Render("No file selected")
		return m.panelStyle(m.focus == focusDiff).
			Width(width).
			Height(height).
			Render(content)
	}

	if hasUnstaged && !hasStaged {
		return m.renderSectionPane(width, height, "Unstaged", &m.unstaged, sectionUnstaged)
	}
	if hasStaged && !hasUnstaged {
		return m.renderSectionPane(width, height, "Staged", &m.staged, sectionStaged)
	}

	topH := height / 2
	if topH < 5 {
		topH = 5
	}
	bottomH := height - topH
	if bottomH < 5 {
		bottomH = 5
		topH = height - bottomH
	}

	top := m.renderSectionPane(width, topH, "Unstaged", &m.unstaged, sectionUnstaged)
	bottom := m.renderSectionPane(width, bottomH, "Staged", &m.staged, sectionStaged)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (m *Model) renderSectionPane(width, height int, title string, sec *sectionState, section diffSection) string {
	innerW := maxInt(1, width-2)
	innerH := maxInt(1, height-2)

	activeSection := m.focus == focusDiff && m.section == section

	bodyH := innerH
	if bodyH < 0 {
		bodyH = 0
	}

	active := m.activeRawLineIndex(*sec)
	hunkStart, hunkEnd := -1, -1
	if m.navMode == navHunk && sec.activeHunk >= 0 && sec.activeHunk < len(sec.parsed.Hunks) {
		hunkStart = sec.parsed.Hunks[sec.activeHunk].StartLine
		hunkEnd = sec.parsed.Hunks[sec.activeHunk].EndLine
	}
	sec.viewport.SetHeight(maxInt(0, bodyH))
	sec.viewport.SetWidth(innerW)

	titleText := title
	if sec.viewport.TotalLineCount() > sec.viewport.VisibleLineCount() && sec.viewport.VisibleLineCount() > 0 {
		pct := int(sec.viewport.ScrollPercent()*100 + 0.5)
		titleText += fmt.Sprintf(" %d%%", pct)
	}

	lines := make([]string, 0, bodyH)

	for i := 0; i < bodyH; i++ {
		rawIdx := sec.viewport.YOffset() + i
		if rawIdx >= len(sec.viewLines) {
			lines = append(lines, "")
			continue
		}
		mark := "  "
		inActiveHunk := m.navMode == navHunk && rawIdx >= hunkStart && rawIdx <= hunkEnd
		if inActiveHunk && activeSection {
			mark = lipgloss.NewStyle().Foreground(catOrange).Render("▌ ")
		}
		if rawIdx == active && activeSection {
			mark = lipgloss.NewStyle().Foreground(catOrange).Bold(true).Render("▌ ")
		}
		if m.flashMarker(section, rawIdx, sec) {
			mark = lipgloss.NewStyle().Foreground(catGreen).Bold(true).Render("◆ ")
		}
		line := mark + sec.viewLines[rawIdx]
		lines = append(lines, ansi.Truncate(line, innerW, ""))
	}
	return m.renderPanelWithBorderTitle(width, height, titleText, lines, activeSection)
}

func (m Model) activeRawLineIndex(sec sectionState) int {
	if m.navMode == navHunk {
		if sec.activeHunk >= 0 && sec.activeHunk < len(sec.parsed.Hunks) {
			return sec.parsed.Hunks[sec.activeHunk].StartLine
		}
		return -1
	}
	if sec.activeLine >= 0 && sec.activeLine < len(sec.parsed.Changed) {
		return sec.parsed.Changed[sec.activeLine].LineIndex
	}
	return -1
}

func (m *Model) syncDiffViewports() {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	_, diffW := m.splitWidth()
	_, diffH := m.splitHeight(mainH)
	vpW := maxInt(1, diffW-4)

	hasUnstaged := len(m.unstaged.viewLines) > 0
	hasStaged := len(m.staged.viewLines) > 0

	if hasUnstaged && hasStaged {
		topH := diffH / 2
		if topH < 5 {
			topH = 5
		}
		bottomH := diffH - topH
		if bottomH < 5 {
			bottomH = 5
			topH = diffH - bottomH
		}
		m.unstaged.viewport.SetHeight(maxInt(0, topH-3))
		m.staged.viewport.SetHeight(maxInt(0, bottomH-3))
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetWidth(vpW)
	} else if hasUnstaged {
		m.unstaged.viewport.SetHeight(maxInt(0, diffH-3))
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetHeight(0)
		m.staged.viewport.SetWidth(vpW)
	} else if hasStaged {
		m.staged.viewport.SetHeight(maxInt(0, diffH-3))
		m.staged.viewport.SetWidth(vpW)
		m.unstaged.viewport.SetHeight(0)
		m.unstaged.viewport.SetWidth(vpW)
	} else {
		m.unstaged.viewport.SetHeight(0)
		m.staged.viewport.SetHeight(0)
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetWidth(vpW)
	}
	// Ensure content is set and clamped.
	m.unstaged.viewport.SetContentLines(m.unstaged.viewLines)
	m.staged.viewport.SetContentLines(m.staged.viewLines)
}

func (m *Model) ensureActiveVisible(sec *sectionState) {
	active := m.activeRawLineIndex(*sec)
	if active >= 0 {
		sec.viewport.EnsureVisible(active, 0, 0)
	}
}

func nextFlashCmd() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg {
		return flashTickMsg{}
	})
}

func (m Model) selectedStatusEntry() (statusEntry, bool) {
	if m.selected < 0 || m.selected >= len(m.statusEntries) {
		return statusEntry{}, false
	}
	return m.statusEntries[m.selected], true
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

func (m *Model) expandSelectedDir() {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Expanded {
		return
	}
	delete(m.collapsedDirs, entry.Path)
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)
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
		m.statusMsg = err.Error()
		return
	}
	if stageAll {
		m.statusMsg = "staged " + path
	} else {
		m.statusMsg = "unstaged " + path
	}
	m.reload(path)
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

func (m Model) helpLine() string {
	if m.focus == focusStatus {
		if m.statusMsg != "" {
			return "  " + m.statusMsg
		}
		return lipgloss.NewStyle().Foreground(catSubtle).Render("  status: j/k move · h/l collapse-expand · space toggle file/dir · enter diff · q quit")
	}
	if m.statusMsg != "" {
		return "  " + m.statusMsg
	}
	modeLabel := "hunk"
	if m.navMode == navLine {
		modeLabel = "line"
	}
	return lipgloss.NewStyle().Foreground(catSubtle).Render("  diff: mode:" + modeLabel + " · tab section · j/k move · J/K scroll(3) · space stage/unstage · esc/q back")
}

func (m Model) panelStyle(active bool) lipgloss.Style {
	borderColor := catSubtle
	if active {
		borderColor = catOrange
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(catBase0)
}

func (m Model) renderPanelWithBorderTitle(width, height int, title string, lines []string, active bool) string {
	if width < 2 || height < 2 {
		return ""
	}
	innerW := width - 2
	innerH := height - 2

	borderColor := catSubtle
	titleStyle := lipgloss.NewStyle().Foreground(catBlue)
	if active {
		borderColor = catOrange
		titleStyle = lipgloss.NewStyle().Foreground(catOrange).Bold(true)
	}
	border := lipgloss.NewStyle().Foreground(borderColor)

	titleSeg := titleStyle.Render(" " + title + " ")
	titleW := ansi.StringWidth(titleSeg)
	topInner := ""
	if titleW >= innerW {
		topInner = ansi.Truncate(titleSeg, innerW, "")
		titleW = ansi.StringWidth(topInner)
	} else {
		topInner = titleSeg + border.Render(strings.Repeat("─", innerW-titleW))
	}
	if titleW < innerW && !strings.Contains(topInner, "─") {
		topInner += border.Render(strings.Repeat("─", innerW-titleW))
	}

	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	body := make([]string, 0, innerH)
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], innerW, "")
		}
		line = line + strings.Repeat(" ", maxInt(0, innerW-ansi.StringWidth(line)))
		body = append(body, border.Render("│")+line+border.Render("│"))
	}

	bottom := border.Render("╰" + strings.Repeat("─", innerW) + "╯")
	top := border.Render("╭") + topInner + border.Render("╮")
	return strings.Join(append([]string{top}, append(body, bottom)...), "\n")
}

func statusEntryMeta(entry statusEntry) string {
	if entry.Kind == statusEntryDir {
		switch {
		case entry.HasStaged && entry.HasUnstaged:
			return lipgloss.NewStyle().Foreground(catYellow).Render("M+")
		case entry.HasUnstaged:
			if entry.HasOnlyUntracked {
				return lipgloss.NewStyle().Foreground(catGreen).Render("??")
			}
			return lipgloss.NewStyle().Foreground(catYellow).Render(" M")
		case entry.HasStaged:
			return lipgloss.NewStyle().Foreground(catBlue).Render("M ")
		default:
			return lipgloss.NewStyle().Foreground(catSubtle).Render("--")
		}
	}

	xy := entry.File.XY()
	if entry.File.IsUntracked() {
		return lipgloss.NewStyle().Foreground(catGreen).Render(xy)
	}
	if entry.File.HasStagedChanges() && entry.File.HasUnstagedChanges() {
		return lipgloss.NewStyle().Foreground(catYellow).Render(xy)
	}
	if entry.File.HasStagedChanges() {
		return lipgloss.NewStyle().Foreground(catBlue).Render(xy)
	}
	if entry.File.HasUnstagedChanges() {
		return lipgloss.NewStyle().Foreground(catYellow).Render(xy)
	}
	return lipgloss.NewStyle().Foreground(catSubtle).Render(xy)
}

func (m Model) flashMarker(section diffSection, rawIdx int, sec *sectionState) bool {
	if !m.flash.active || m.flash.section != section {
		return false
	}
	if m.flash.navMode == navHunk {
		if m.flash.hunk < 0 || m.flash.hunk >= len(sec.parsed.Hunks) {
			return false
		}
		h := sec.parsed.Hunks[m.flash.hunk]
		return rawIdx >= h.StartLine && rawIdx <= h.EndLine
	}
	if m.flash.line < 0 || m.flash.line >= len(sec.parsed.Changed) {
		return false
	}
	return sec.parsed.Changed[m.flash.line].LineIndex == rawIdx
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
