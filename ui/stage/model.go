package stage

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"gx/git"
	"gx/ui/components"

	"charm.land/bubbles/v2/textinput"
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
	rawLines         []string
	baseLines        []string
	baseDisplayToRaw []int
	viewLines        []string
	displayToRaw     []int
	rawToDisplay     []int
	parsed           parsedDiff
	activeHunk       int
	activeLine       int
	viewport         viewport.Model
}

type Model struct {
	worktreeRoot string
	settings     Settings

	width  int
	height int
	ready  bool

	focus          focusPane
	section        diffSection
	navMode        navMode
	diffFullscreen bool
	wrapSoft       bool

	files         []git.StageFileStatus
	statusEntries []statusEntry
	collapsedDirs map[string]bool
	selected      int

	unstaged sectionState
	staged   sectionState

	statusMsg      string
	statusUntil    time.Time
	err            error
	errorOpen      bool
	errorVP        viewport.Model
	helpOpen       bool
	helpVP         viewport.Model
	activeFilePath string
	diffReloadSeq  int
	searchMode     stageSearchMode
	searchScope    stageSearchScope
	searchQuery    string
	searchMatches  []stageSearchMatch
	searchCursor   int
	searchInput    textinput.Model
	confirmOpen    bool
	confirmTitle   string
	confirmLines   []string
	confirmYes     bool
	confirmAction  stageConfirmAction
	confirmRemote  string
	confirmBranch  string
	runningOpen    bool
	runningTitle   string
	runningVP      viewport.Model
	runningContent string
	runningRunner  *stageActionRunner
	runningDone    bool
	flash          flashState
	keyPrefix      string
}

type Settings struct {
	DiffContextLines int
	UseNerdFontIcons bool
}

func DefaultSettings() Settings {
	return Settings{DiffContextLines: 1, UseNerdFontIcons: true}
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
type statusTickMsg struct{}
type actionPollMsg struct{}
type diffReloadMsg struct{ seq int }

type commitFinishedMsg struct {
	err       error
	tmuxSplit bool
}

var (
	catBase0   = lipgloss.Color("#1e1e2e")
	catText    = lipgloss.Color("#cdd6f4")
	catSubtle  = lipgloss.Color("#a6adc8")
	catBlue    = lipgloss.Color("#89b4fa")
	catGreen   = lipgloss.Color("#a6e3a1")
	catYellow  = lipgloss.Color("#f9e2af")
	catRed     = lipgloss.Color("#f38ba8")
	catOrange  = lipgloss.Color("#fab387")
	catSurface = lipgloss.Color("#313244")
)

const ansiReset = "\x1b[0m"

const statusMessageTTL = 5 * time.Second
const statusDiffReloadDebounce = 100 * time.Millisecond

var (
	ansiCSIRe = regexp.MustCompile(`\x1b\[[0-9:;<=>?]*[ -/]*[@-~]`)
	ansiOSCRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`) // OSC ... BEL/ST
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
		wrapSoft:      true,
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
	return statusTickCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.syncDiffViewports()
		return m, nil
	case tea.FocusMsg:
		m.refresh()
		return m, nil
	case statusTickMsg:
		if m.statusMsg != "" && !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
			m.clearStatus()
		}
		return m, statusTickCmd()
	case actionPollMsg:
		if m.runningRunner != nil {
			if chunk := m.runningRunner.Consume(); chunk != "" {
				m.appendRunningOutput(chunk)
			}
			if !m.runningDone {
				if res, done := m.runningRunner.Result(); done {
					m.runningDone = true
					m.handleActionResult(res)
				}
			}
		}
		if m.runningOpen && !m.runningDone {
			return m, actionPollCmd()
		}
		return m, nil
	case diffReloadMsg:
		if msg.seq == m.diffReloadSeq && m.focus == focusStatus {
			m.reloadDiffsForSelection()
		}
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.runningOpen && !m.runningDone && m.runningRunner != nil {
				m.runningRunner.Cancel()
				m.setStatus("cancel requested")
				return m, nil
			}
			return m, tea.Quit
		}
		if msg.String() == "q" {
			return m, tea.Quit
		}
		if m.runningOpen {
			return m.handleRunningKey(msg)
		}
		if m.confirmOpen {
			return m.handleConfirmKey(msg)
		}
		if m.errorOpen {
			return m.handleErrorKey(msg)
		}
		if m.helpOpen {
			return m.handleHelpKey(msg)
		}
		if m.searchMode != searchModeNone {
			return m.handleSearchKey(msg)
		}
		if cmd, handled := m.handleSearchNavigateKey(msg); handled {
			return m, cmd
		}
		if msg.String() == "/" {
			m.enterSearchMode()
			return m, nil
		}
		if handledModel, cmd, handled := m.handleChordKey(msg); handled {
			return handledModel, cmd
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
	case commitFinishedMsg:
		if msg.err != nil {
			m.setStatus("commit failed: " + msg.err.Error())
			return m, nil
		}
		if msg.tmuxSplit {
			m.setStatus("opened tmux split: git commit")
			return m, nil
		}
		m.setStatus("git commit finished")
		m.refresh()
		return m, nil
	}
	return m, nil
}

func (m Model) handleErrorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.errorOpen = false
		return m, nil
	}
	var cmd tea.Cmd
	m.errorVP, cmd = m.errorVP.Update(msg)
	return m, cmd
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "enter":
		m.helpOpen = false
		return m, nil
	}
	var cmd tea.Cmd
	m.helpVP, cmd = m.helpVP.Update(msg)
	return m, cmd
}

func (m Model) handleChordKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	shiftG := (msg.Mod&tea.ModShift) != 0 && (msg.Code == 'g' || msg.Code == 'G' || msg.Text == "g" || msg.Text == "G")
	isUpperG := key == "G" || key == "shift+g" || msg.Text == "G" || msg.ShiftedCode == 'G' || shiftG
	isLowerG := key == "g" && !isUpperG && (msg.Mod&tea.ModShift) == 0
	if m.keyPrefix == "c" {
		m.keyPrefix = ""
		if key == "c" {
			m.setStatus("opening git commit...")
			return m, cmdGitCommit(m.worktreeRoot), true
		}
		if key == "esc" {
			m.clearStatus()
			return m, nil, true
		}
	}
	if m.keyPrefix == "g" {
		m.keyPrefix = ""
		if isLowerG {
			m.jumpToTop()
			if m.focus == focusStatus {
				return m, m.scheduleDiffReload(), true
			}
			return m, nil, true
		}
		if isUpperG {
			m.jumpToBottom()
			if m.focus == focusStatus {
				return m, m.scheduleDiffReload(), true
			}
			return m, nil, true
		}
		if key == "esc" {
			m.clearStatus()
			return m, nil, true
		}
	}
	if key == "c" {
		m.keyPrefix = "c"
		m.setStatus("cc: git commit")
		return m, nil, true
	}
	if isLowerG {
		m.keyPrefix = "g"
		m.setStatus("gg: jump to top")
		return m, nil, true
	}
	if isUpperG {
		m.keyPrefix = ""
		m.jumpToBottom()
		if m.focus == focusStatus {
			return m, m.scheduleDiffReload(), true
		}
		return m, nil, true
	}
	m.keyPrefix = ""
	return m, nil, false
}

func (m Model) handleStatusKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.selected < len(m.statusEntries)-1 {
			m.selected++
			m.onStatusSelectionChanged()
			return m, m.scheduleDiffReload()
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
			m.onStatusSelectionChanged()
			return m, m.scheduleDiffReload()
		}
	case "h", "left":
		if m.focusParentInStatus() {
			return m, m.scheduleDiffReload()
		}
		m.collapseSelectedDir()
		m.reloadDiffsForSelection()
	case "l", "right":
		entry, ok := m.selectedStatusEntry()
		if ok && entry.Kind == statusEntryFile {
			m.enterDiffFromStatus(false)
			return m, nil
		}
		m.expandSelectedDir()
		m.reloadDiffsForSelection()
	case "r":
		m.refresh()
	case "p":
		m.startPullAction()
		return m, actionPollCmd()
	case "P":
		if err := m.preparePushConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case "b":
		if err := m.prepareRebaseConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case "A":
		if err := m.openAmendConfirm(); err != nil {
			m.showGitError(err)
		}
	case "ctrl+d":
		if m.scrollStatusPage(1) {
			return m, m.scheduleDiffReload()
		}
	case "ctrl+u":
		if m.scrollStatusPage(-1) {
			return m, m.scheduleDiffReload()
		}
	case "space", " ":
		m.toggleStageStatusEntry()
		m.reloadDiffsForSelection()
	case "enter":
		if m.toggleDirOnEnter() {
			m.reloadDiffsForSelection()
			return m, nil
		}
		m.enterDiffFromStatus(false)
	case "?":
		m.showHelpOverlay()
	}
	return m, nil
}

func (m Model) handleDiffKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.focus = focusStatus
		return m, nil
	case "h", "left":
		m.focus = focusStatus
		return m, nil
	case "tab":
		if m.canSwitchSections() {
			if m.section == sectionUnstaged {
				m.section = sectionStaged
			} else {
				m.section = sectionUnstaged
			}
			m.syncDiffViewports()
			m.ensureActiveVisible(m.currentSection())
		}
	case "a":
		if m.navMode == navHunk {
			m.navMode = navLine
		} else {
			m.navMode = navHunk
		}
		m.ensureActiveVisible(m.currentSection())
	case "f":
		m.diffFullscreen = !m.diffFullscreen
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
	case "w":
		m.wrapSoft = !m.wrapSoft
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
	case "r":
		m.refresh()
	case "p":
		m.startPullAction()
		return m, actionPollCmd()
	case "P":
		if err := m.preparePushConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case "b":
		if err := m.prepareRebaseConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case "A":
		if err := m.openAmendConfirm(); err != nil {
			m.showGitError(err)
		}
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
	case "ctrl+d":
		m.scrollDiffPage(1)
	case "ctrl+u":
		m.scrollDiffPage(-1)
	case "space", " ":
		cmd := m.applySelection()
		return m, cmd
	case "?":
		m.showHelpOverlay()
	}
	return m, nil
}

func (m *Model) moveActive(delta int) {
	sec := m.currentSection()
	if m.navMode == navHunk {
		if len(sec.parsed.Hunks) == 0 {
			return
		}
		old := sec.activeHunk
		if sec.activeHunk >= 0 && sec.activeHunk < len(sec.parsed.Hunks) {
			if start, end, ok := hunkDisplayBounds(*sec, sec.activeHunk); ok {
				visible := sec.viewport.VisibleLineCount()
				y := sec.viewport.YOffset()
				if visible > 0 {
					last := y + visible - 1
					if delta > 0 && end > last {
						sec.viewport.ScrollDown(1)
						return
					}
					if delta < 0 && start < y {
						sec.viewport.ScrollUp(1)
						return
					}
				}
			}
		}
		sec.activeHunk += delta
		if sec.activeHunk < 0 {
			sec.activeHunk = 0
		}
		if sec.activeHunk >= len(sec.parsed.Hunks) {
			sec.activeHunk = len(sec.parsed.Hunks) - 1
		}
		if sec.activeHunk == old {
			return
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
	m.syncSearchCursorFromDiffFocus()
	m.ensureActiveVisible(sec)
}

func (m *Model) scrollStatusPage(direction int) bool {
	if len(m.statusEntries) == 0 {
		return false
	}
	old := m.selected
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	statusH, _ := m.splitHeight(mainH)
	visible := maxInt(1, (statusH-2)/2)
	if direction > 0 {
		m.selected += visible
	} else {
		m.selected -= visible
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.statusEntries) {
		m.selected = len(m.statusEntries) - 1
	}
	if m.selected == old {
		return false
	}
	m.onStatusSelectionChanged()
	return true
}

func (m *Model) scrollDiffPage(direction int) {
	sec := m.currentSection()
	visible := sec.viewport.VisibleLineCount()
	if visible <= 0 {
		return
	}
	step := maxInt(1, visible/2)
	if direction > 0 {
		sec.viewport.ScrollDown(step)
	} else {
		sec.viewport.ScrollUp(step)
	}
}

func (m *Model) jumpToTop() {
	if m.focus == focusStatus {
		if len(m.statusEntries) == 0 {
			return
		}
		if m.selected == 0 {
			return
		}
		m.selected = 0
		m.onStatusSelectionChanged()
		return
	}
	sec := m.currentSection()
	sec.viewport.SetYOffset(0)
	if m.navMode == navHunk {
		if len(sec.parsed.Hunks) == 0 {
			return
		}
		sec.activeHunk = 0
	} else {
		if len(sec.parsed.Changed) == 0 {
			return
		}
		sec.activeLine = 0
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpToBottom() {
	if m.focus == focusStatus {
		if len(m.statusEntries) == 0 {
			return
		}
		if m.selected == len(m.statusEntries)-1 {
			return
		}
		m.selected = len(m.statusEntries) - 1
		m.onStatusSelectionChanged()
		return
	}
	sec := m.currentSection()
	maxOffset := sec.viewport.TotalLineCount() - sec.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	sec.viewport.SetYOffset(maxOffset)
	if m.navMode == navHunk {
		if len(sec.parsed.Hunks) == 0 {
			return
		}
		sec.activeHunk = len(sec.parsed.Hunks) - 1
	} else {
		if len(sec.parsed.Changed) == 0 {
			return
		}
		sec.activeLine = len(sec.parsed.Changed) - 1
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) scheduleDiffReload() tea.Cmd {
	m.diffReloadSeq++
	seq := m.diffReloadSeq
	return tea.Tick(statusDiffReloadDebounce, func(time.Time) tea.Msg {
		return diffReloadMsg{seq: seq}
	})
}

func (m *Model) onStatusSelectionChanged() {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind == statusEntryDir {
		m.section = sectionUnstaged
		return
	}
	if entry.File.Path != m.activeFilePath {
		m.section = sectionUnstaged
	}
}

func hunkDisplayBounds(sec sectionState, hunkIdx int) (start int, end int, ok bool) {
	if hunkIdx < 0 || hunkIdx >= len(sec.parsed.Hunks) {
		return 0, 0, false
	}
	h := sec.parsed.Hunks[hunkIdx]
	start = -1
	end = -1
	for displayIdx, rawIdx := range sec.displayToRaw {
		if rawIdx < h.StartLine || rawIdx > h.EndLine {
			continue
		}
		if start < 0 {
			start = displayIdx
		}
		end = displayIdx
	}
	if start < 0 || end < 0 {
		return 0, 0, false
	}
	return start, end, true
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
			m.showGitError(err)
			return nil
		}
	} else {
		if sec.activeLine < 0 || sec.activeLine >= len(sec.parsed.Changed) {
			return nil
		}
		sig.lineText = sec.parsed.Changed[sec.activeLine].Text
		patch, err := buildSingleLinePatch(sec.parsed, sec.activeLine)
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
		raw, err := git.DiffUntrackedPath(m.worktreeRoot, file.Path, false, m.settings.DiffContextLines)
		if err != nil {
			m.showGitError(err)
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
		m.showGitError(err)
		unstagedRaw = ""
	}
	unstagedColor, err := git.DiffPathWithDelta(m.worktreeRoot, file.Path, false, m.settings.DiffContextLines)
	if err != nil {
		unstagedColor = unstagedRaw
	}

	stagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, true, m.settings.DiffContextLines)
	if err != nil {
		m.showGitError(err)
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

func buildSectionState(raw, color string, prev sectionState) sectionState {
	state := sectionState{activeHunk: prev.activeHunk, activeLine: prev.activeLine, viewport: prev.viewport}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		state.activeHunk = -1
		state.activeLine = -1
		state.baseLines = nil
		state.baseDisplayToRaw = nil
		state.viewLines = nil
		state.displayToRaw = nil
		state.rawToDisplay = nil
		state.viewport.SetContent("")
		state.viewport.SetYOffset(0)
		return state
	}

	state.parsed = parseUnifiedDiff(raw)
	state.rawLines = append([]string{}, state.parsed.Lines...)

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
	if m.runningOpen {
		out = overlayModal(out, m.runningModalView(), m.width, m.height)
	} else if m.confirmOpen {
		out = overlayModal(out, m.confirmModalView(), m.width, m.height)
	} else if m.errorOpen {
		out = overlayModal(out, m.errorModalView(), m.width, m.height)
	} else if m.helpOpen {
		out = overlayModal(out, m.helpModalView(), m.width, m.height)
	}
	v := tea.NewView(out)
	v.AltScreen = true
	v.ReportFocus = true
	return v
}

func (m *Model) showGitError(err error) {
	if err == nil {
		return
	}
	m.setStatus("git command failed")
	vpW := m.width * 2 / 3
	if vpW < 44 {
		vpW = 44
	}
	if vpW > 96 {
		vpW = 96
	}
	vpH := m.height/2 - 6
	if vpH < 4 {
		vpH = 4
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(err.Error())
	m.errorVP = vp
	m.errorOpen = true
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
	innerH := maxInt(1, height-2)
	lines := make([]string, 0, innerH)

	bodyH := innerH
	if bodyH < 0 {
		bodyH = 0
	}

	if len(m.statusEntries) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(catSubtle).Render("clean working tree"))
	} else {
		icons := statusPaneIconsFor(m.settings.UseNerdFontIcons)
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
			mark := " "
			if i == m.selected {
				mark = lipgloss.NewStyle().Foreground(catOrange).Render("▌")
			}
			indent := strings.Repeat("  ", entry.Depth)
			statusColor := statusEntryColor(entry)
			deleted := entry.Kind == statusEntryFile && isDeletedFileStatus(entry.File)
			metaRaw := statusEntryMeta(entry, m.settings.UseNerdFontIcons, icons)
			metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
			if deleted {
				metaStyle = metaStyle.Faint(true)
			}
			meta := metaStyle.Render(metaRaw)
			name := entry.DisplayName
			if entry.Kind == statusEntryDir {
				symbol := icons.folderOpen
				if !entry.Expanded {
					symbol = icons.folderClosed
				}
				name = symbol + " " + name + "/"
			} else {
				if entry.File.IsRenamed() && entry.File.RenameFrom != "" {
					name = entry.File.RenameFrom + " -> " + entry.File.Path
				}
				name = statusFileIcon(entry.File, icons) + " " + name
			}
			if m.searchMatchStatusIndex(i) {
				name = highlightMatchText(name, m.searchQuery)
			}
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
			if deleted {
				nameStyle = nameStyle.Faint(true)
			}
			name = nameStyle.Render(name)
			sep := " "
			if strings.TrimSpace(metaRaw) == "" {
				sep = ""
			}
			line := fmt.Sprintf("%s%s%s%s%s", mark, indent, meta, sep, name)
			if i == m.selected && !deleted {
				line = lipgloss.NewStyle().Bold(true).Render(line)
			}
			lines = append(lines, line)
		}
	}

	for len(lines) < innerH {
		lines = append(lines, "")
	}

	return m.renderPanelWithBorderTitle(width, height, "Status", "", lines, m.focus == focusStatus, sectionUnstaged)
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
	if m.diffFullscreen {
		if m.section == sectionStaged {
			return m.renderSectionPane(width, height, "Staged", &m.staged, sectionStaged)
		}
		return m.renderSectionPane(width, height, "Unstaged", &m.unstaged, sectionUnstaged)
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
	accent := catOrange
	if section == sectionStaged {
		accent = catGreen
	}
	hunkStart, hunkEnd := -1, -1
	if m.navMode == navHunk && sec.activeHunk >= 0 && sec.activeHunk < len(sec.parsed.Hunks) {
		hunkStart = sec.parsed.Hunks[sec.activeHunk].StartLine
		hunkEnd = sec.parsed.Hunks[sec.activeHunk].EndLine
	}
	sec.viewport.SetHeight(maxInt(0, bodyH))
	sec.viewport.SetWidth(innerW)

	titleText := title
	if file, ok := m.selectedFile(); ok && file.IsRenamed() && file.RenameFrom != "" {
		titleText += " [moved: " + file.RenameFrom + " -> " + file.Path + "]"
	}
	if m.diffFullscreen {
		titleText += " [fullscreen]"
	}
	rightTitleText := ""
	if sec.viewport.TotalLineCount() > sec.viewport.VisibleLineCount() && sec.viewport.VisibleLineCount() > 0 {
		pct := int(sec.viewport.ScrollPercent()*100 + 0.5)
		rightTitleText = fmt.Sprintf("%d%%", pct)
	}

	overflowTopDisplay := -1
	overflowBottomDisplay := -1
	if m.navMode == navHunk && activeSection && sec.activeHunk >= 0 {
		if start, end, ok := hunkDisplayBounds(*sec, sec.activeHunk); ok && sec.viewport.VisibleLineCount() > 0 {
			vpTop := sec.viewport.YOffset()
			vpBottom := vpTop + sec.viewport.VisibleLineCount() - 1
			if start < vpTop {
				overflowTopDisplay = vpTop
			}
			if end > vpBottom {
				overflowBottomDisplay = vpBottom
			}
		}
	}
	overflowTopMark, overflowBottomMark, overflowBothMark := m.hunkOverflowMarkers()

	lines := make([]string, 0, bodyH)

	for i := 0; i < bodyH; i++ {
		displayIdx := sec.viewport.YOffset() + i
		if displayIdx >= len(sec.viewLines) {
			lines = append(lines, "")
			continue
		}
		rawIdx := -1
		if displayIdx >= 0 && displayIdx < len(sec.displayToRaw) {
			rawIdx = sec.displayToRaw[displayIdx]
		}
		mark := "  "
		inActiveHunk := rawIdx >= 0 && m.navMode == navHunk && rawIdx >= hunkStart && rawIdx <= hunkEnd
		if inActiveHunk && activeSection {
			mark = lipgloss.NewStyle().Foreground(accent).Render("▌ ")
		}
		if rawIdx >= 0 && rawIdx == active && activeSection {
			mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("▌ ")
		}
		if inActiveHunk {
			if displayIdx == overflowTopDisplay && displayIdx == overflowBottomDisplay {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowBothMark)
			} else if displayIdx == overflowTopDisplay {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowTopMark)
			} else if displayIdx == overflowBottomDisplay {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowBottomMark)
			}
		}
		if rawIdx >= 0 && m.flashMarker(section, rawIdx, sec) {
			mark = lipgloss.NewStyle().Foreground(catGreen).Bold(true).Render("◆ ")
		}

		indicator := "  "
		if matched, current := m.searchMatchDiffDisplay(section, displayIdx); matched {
			icon := "* "
			if m.settings.UseNerdFontIcons {
				icon = "󰍉 "
			}
			style := lipgloss.NewStyle().Foreground(catYellow).Bold(true)
			if current {
				style = style.Foreground(catGreen)
			}
			indicator = style.Render(icon)
		}

		markW := ansi.StringWidth(mark)
		indicatorW := ansi.StringWidth(indicator)
		bodyW := innerW - markW - indicatorW
		if bodyW < 0 {
			bodyW = 0
		}
		body := ansi.Truncate(sec.viewLines[displayIdx], bodyW, "")
		body += strings.Repeat(" ", maxInt(0, bodyW-ansi.StringWidth(body)))
		lines = append(lines, mark+body+indicator)
	}
	return m.renderPanelWithBorderTitle(width, height, titleText, rightTitleText, lines, activeSection, section)
}

func (m Model) hunkOverflowMarkers() (top, bottom, both string) {
	if m.settings.UseNerdFontIcons {
		return " ", " ", "↕ "
	}
	return "↑ ", "↓ ", "↕ "
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
	wrapWidth := maxInt(1, vpW-2)
	reflowSectionLines(&m.unstaged, wrapWidth, m.wrapSoft)
	reflowSectionLines(&m.staged, wrapWidth, m.wrapSoft)

	hasUnstaged := len(m.unstaged.viewLines) > 0
	hasStaged := len(m.staged.viewLines) > 0
	if m.diffFullscreen {
		if m.section == sectionUnstaged {
			m.unstaged.viewport.SetHeight(maxInt(0, diffH-3))
			m.staged.viewport.SetHeight(0)
		} else {
			m.staged.viewport.SetHeight(maxInt(0, diffH-3))
			m.unstaged.viewport.SetHeight(0)
		}
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetWidth(vpW)
		m.unstaged.viewport.SetContentLines(m.unstaged.viewLines)
		m.staged.viewport.SetContentLines(m.staged.viewLines)
		return
	}

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

func reflowSectionLines(sec *sectionState, wrapWidth int, wrapSoft bool) {
	if len(sec.baseLines) == 0 {
		sec.viewLines = nil
		sec.displayToRaw = nil
		sec.rawToDisplay = buildRawToDisplayMap(sec.parsed, nil)
		sec.viewport.SetContent("")
		sec.viewport.SetYOffset(0)
		return
	}

	prevOffset := sec.viewport.YOffset()
	view := make([]string, 0, len(sec.baseLines))
	mapRaw := make([]int, 0, len(sec.baseDisplayToRaw))

	for i, line := range sec.baseLines {
		rawIdx := -1
		if i < len(sec.baseDisplayToRaw) {
			rawIdx = sec.baseDisplayToRaw[i]
		}
		if !wrapSoft || rawIdx < 0 {
			view = append(view, line)
			mapRaw = append(mapRaw, rawIdx)
			continue
		}
		parts := wrapANSI(line, wrapWidth)
		for _, p := range parts {
			view = append(view, p)
			mapRaw = append(mapRaw, rawIdx)
		}
	}

	sec.viewLines = view
	sec.displayToRaw = mapRaw
	sec.rawToDisplay = buildRawToDisplayMap(sec.parsed, sec.displayToRaw)
	sec.viewport.SetContentLines(sec.viewLines)
	sec.viewport.SetYOffset(prevOffset)
}

func wrapANSI(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	total := ansi.StringWidth(s)
	if total <= width {
		return []string{s}
	}
	out := make([]string, 0, total/width+1)
	for start := 0; start < total; start += width {
		end := start + width
		if end > total {
			end = total
		}
		part := ansi.Cut(s, start, end)
		if part == "" {
			break
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
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

func statusTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return statusTickMsg{}
	})
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
	if m.searchMode != searchModeNone {
		line := lipgloss.NewStyle().Foreground(catSubtle).Render("  " + m.searchFooterText())
		if m.width > 0 {
			line = ansi.Truncate(line, m.width, "")
		}
		return line
	}
	if m.focus == focusStatus {
		hint := "status · ? help"
		if s := m.searchCounterLabel(); s != "" {
			hint = s + " · " + hint
		}
		return m.renderFooterLine(hint)
	}
	modeLabel := "hunk"
	if m.navMode == navLine {
		modeLabel = "line"
	}
	wrapLabel := "off"
	if m.wrapSoft {
		wrapLabel = "on"
	}
	hint := "diff: mode:" + modeLabel + " · wrap:" + wrapLabel + " · ? help"
	if s := m.searchCounterLabel(); s != "" {
		hint = s + " · " + hint
	}
	return m.renderFooterLine(hint)
}

func (m Model) searchCounterLabel() string {
	if strings.TrimSpace(m.searchQuery) == "" || len(m.searchMatches) == 0 {
		return ""
	}
	idx := m.searchCursor + 1
	if idx < 1 {
		idx = 1
	}
	if idx > len(m.searchMatches) {
		idx = len(m.searchMatches)
	}
	icon := "*"
	if m.settings.UseNerdFontIcons {
		icon = "󰍉"
	}
	return fmt.Sprintf("%s %d/%d", icon, idx, len(m.searchMatches))
}

func (m Model) renderFooterLine(hint string) string {
	hintText := "· " + hint
	hintStyled := lipgloss.NewStyle().Foreground(catSubtle).Render(hintText)
	lineW := m.width
	if lineW <= 0 {
		if m.statusMsg == "" {
			return hintStyled
		}
		return m.statusMsg + "  " + hintStyled
	}

	hintW := ansi.StringWidth(hintText)
	if m.statusMsg == "" {
		if hintW >= lineW {
			return ansi.Truncate(hintStyled, lineW, "")
		}
		return strings.Repeat(" ", lineW-hintW) + hintStyled
	}

	sep := "  "
	sepW := ansi.StringWidth(sep)
	statusMax := lineW - hintW - sepW
	if statusMax <= 0 {
		if hintW >= lineW {
			return ansi.Truncate(hintStyled, lineW, "")
		}
		return strings.Repeat(" ", lineW-hintW) + hintStyled
	}

	status := ansi.Truncate(m.statusMsg, statusMax, "...")
	left := status + sep
	leftW := ansi.StringWidth(left)
	if leftW+hintW >= lineW {
		return left + hintStyled
	}
	return left + strings.Repeat(" ", lineW-leftW-hintW) + hintStyled
}

func (m *Model) showHelpOverlay() {
	vpW := m.width * 2 / 3
	if vpW < 56 {
		vpW = 56
	}
	if vpW > 104 {
		vpW = 104
	}
	vpH := m.height/2 - 4
	if vpH < 8 {
		vpH = 8
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(stageHelpText())
	m.helpVP = vp
	m.helpOpen = true
}

func stageHelpText() string {
	return strings.Join([]string{
		"Global",
		"  ?       toggle this help",
		"  q       quit",
		"  cc      open git commit",
		"  p/P     pull / push",
		"  b       rebase on origin/master",
		"  A       amend last commit (confirm)",
		"",
		"Status Focus",
		"  j / k   move selection",
		"  gg / G  jump top / bottom",
		"  ctrl+u/d scroll half page",
		"  h       collapse open directory",
		"  l       expand directory / open diff on file",
		"  space   stage/unstage file",
		"  enter   open diff view",
		"  r       refresh",
		"",
		"Diff Focus",
		"  esc/h   return to status",
		"  gg / G  jump top / bottom",
		"  ctrl+u/d scroll half page",
		"  tab     switch unstaged/staged section",
		"  a       toggle hunk/line mode",
		"  j / k   move active hunk/line",
		"  J / K   scroll diff viewport",
		"  space   stage/unstage active hunk/line",
		"  f       toggle fullscreen diff",
		"  w       toggle soft wrap",
		"  r       refresh",
	}, "\n")
}

func (m Model) errorModalView() string {
	return components.RenderOutputModal(
		"Error",
		m.errorVP.View(),
		"esc / enter dismiss · j/k scroll",
		catRed,
		catRed,
		catSubtle,
		m.errorVP.Width(),
	)
}

func (m Model) helpModalView() string {
	titleStyle := lipgloss.NewStyle().Foreground(catBlue).Bold(true)
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(catBlue).
		Padding(0, 1).
		Width(m.helpVP.Width())

	hint := lipgloss.NewStyle().Foreground(catSubtle).Render("? / esc / enter dismiss · j/k scroll")
	inner := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Keyboard Help"),
		"",
		m.helpVP.View(),
		"",
		hint,
	)
	return borderStyle.Render(inner)
}

func overlayModal(bg, modal string, screenW, screenH int) string {
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	x := (screenW - modalW) / 2
	y := (screenH - modalH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return placeOverlay(bg, modal, x, y)
}

func placeOverlay(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		bgY := y + i
		if bgY < 0 || bgY >= len(bgLines) {
			continue
		}
		bgLine := bgLines[bgY]
		fgW := ansi.StringWidth(fgLine)

		left := ansi.Truncate(bgLine, x, "")
		if leftW := ansi.StringWidth(left); leftW < x {
			left += strings.Repeat(" ", x-leftW)
		}
		right := ansi.TruncateLeft(bgLine, x+fgW, "")
		bgLines[bgY] = left + fgLine + right
	}

	return strings.Join(bgLines, "\n")
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

func (m Model) renderPanelWithBorderTitle(width, height int, title, rightTitle string, lines []string, active bool, section diffSection) string {
	if width < 2 || height < 2 {
		return ""
	}
	innerW := width - 2
	innerH := height - 2

	borderColor := catSubtle
	titleStyle := lipgloss.NewStyle().Foreground(catBlue)
	if section == sectionStaged {
		borderColor = catGreen
		titleStyle = lipgloss.NewStyle().Foreground(catGreen)
		if active {
			titleStyle = titleStyle.Bold(true)
		}
	} else if active {
		borderColor = catOrange
		titleStyle = lipgloss.NewStyle().Foreground(catOrange).Bold(true)
	}
	border := lipgloss.NewStyle().Foreground(borderColor)

	titleSeg := titleStyle.Render(" " + title + " ")
	rightSeg := ""
	if rightTitle != "" {
		rightSeg = titleStyle.Render(" " + rightTitle + " ")
	}
	titleW := ansi.StringWidth(titleSeg)
	rightW := ansi.StringWidth(rightSeg)
	topInner := ""
	if rightW >= innerW {
		topInner = ansi.Truncate(rightSeg, innerW, "")
	} else if titleW+rightW >= innerW {
		titleSeg = ansi.Truncate(titleSeg, innerW-rightW, "")
		titleW = ansi.StringWidth(titleSeg)
		topInner = titleSeg + rightSeg
	} else if titleW >= innerW {
		topInner = ansi.Truncate(titleSeg, innerW, "")
		titleW = ansi.StringWidth(topInner)
	} else {
		topInner = titleSeg + border.Render(strings.Repeat("─", innerW-titleW-rightW)) + rightSeg
	}
	if titleW+rightW < innerW && !strings.Contains(topInner, "─") {
		topInner += border.Render(strings.Repeat("─", innerW-titleW-rightW))
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
		body = append(body, border.Render("│")+line+ansiReset+border.Render("│"))
	}

	bottom := border.Render("╰" + strings.Repeat("─", innerW) + "╯")
	top := border.Render("╭") + topInner + border.Render("╮")
	return strings.Join(append([]string{top}, append(body, bottom)...), "\n")
}

type statusPaneIcons struct {
	folderClosed string
	folderOpen   string
	fileModified string
	fileNew      string
	fileDeleted  string
	fileRenamed  string
	partial      string
	staged       string
}

func statusPaneIconsFor(useNerdFontIcons bool) statusPaneIcons {
	if !useNerdFontIcons {
		return statusPaneIcons{
			folderClosed: "▸",
			folderOpen:   "▾",
			fileModified: "M",
			fileNew:      "N",
			fileDeleted:  "D",
			fileRenamed:  "R",
			partial:      "+",
			staged:       "✓",
		}
	}
	return statusPaneIcons{
		folderClosed: "",
		folderOpen:   "",
		fileModified: "",
		fileNew:      "",
		fileDeleted:  "",
		fileRenamed:  "󰁔",
		partial:      "",
		staged:       "",
	}
}

func statusEntryColor(entry statusEntry) string {
	if entry.Kind == statusEntryFile && isDeletedFileStatus(entry.File) {
		return "#a6adc8"
	}
	if entry.Kind == statusEntryFile && entry.File.IsRenamed() {
		return "#89b4fa"
	}
	if entry.HasStaged && entry.HasUnstaged {
		return "#fab387"
	}
	if entry.HasStaged {
		return "#a6e3a1"
	}
	return "#cdd6f4"
}

func statusEntryMeta(entry statusEntry, useNerdFontIcons bool, icons statusPaneIcons) string {
	if entry.HasStaged && entry.HasUnstaged {
		return icons.partial
	}
	if entry.HasStaged {
		return icons.staged
	}
	if useNerdFontIcons {
		return "  "
	}
	if entry.Kind == statusEntryDir {
		return "-"
	}
	return entry.File.XY()
}

func statusFileIcon(file git.StageFileStatus, icons statusPaneIcons) string {
	if isDeletedFileStatus(file) {
		return icons.fileDeleted
	}
	if file.IsRenamed() {
		return icons.fileRenamed
	}
	if file.IsUntracked() || file.IndexStatus == 'A' {
		return icons.fileNew
	}
	return icons.fileModified
}

func isDeletedFileStatus(file git.StageFileStatus) bool {
	return file.IndexStatus == 'D' || file.WorktreeCode == 'D'
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
