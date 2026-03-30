package stage

import (
	"regexp"
	"time"

	"gx/git"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	visualActive     bool
	visualAnchor     int
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

	statusMsg               string
	statusUntil             time.Time
	err                     error
	errorOpen               bool
	errorVP                 viewport.Model
	helpOpen                bool
	helpVP                  viewport.Model
	activeFilePath          string
	diffReloadSeq           int
	searchMode              stageSearchMode
	searchScope             stageSearchScope
	searchQuery             string
	searchMatches           []stageSearchMatch
	searchCursor            int
	searchInput             textinput.Model
	confirmOpen             bool
	confirmTitle            string
	confirmLines            []string
	confirmYes              bool
	confirmAction           stageConfirmAction
	confirmRemote           string
	confirmBranch           string
	confirmPaths            []string
	confirmPatch            string
	confirmPatchUnidiffZero bool
	confirmDiscardUntracked bool
	runningOpen             bool
	runningTitle            string
	runningVP               viewport.Model
	runningContent          string
	runningRunner           *stageActionRunner
	runningDone             bool
	flash                   flashState
	keyPrefix               string
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
		activeHunk:   -1,
		activeLine:   -1,
		visualAnchor: -1,
		viewport:     vp,
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
	if m.keyPrefix == "y" {
		m.keyPrefix = ""
		switch key {
		case "c":
			m.yankContextForAI()
			return m, nil, true
		case "f":
			m.yankFilename()
			return m, nil, true
		case "esc":
			m.clearStatus()
			return m, nil, true
		}
	}
	if key == "c" {
		m.keyPrefix = "c"
		m.setStatus("cc: git commit")
		return m, nil, true
	}
	if key == "y" {
		m.keyPrefix = "y"
		m.setStatus("yc: yank AI context · yf: yank filename")
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
	case "d":
		m.openDiscardStatusConfirm()
	}
	return m, nil
}

func (m Model) handleDiffKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		sec := m.currentSection()
		if sec.visualActive {
			sec.visualActive = false
			sec.visualAnchor = sec.activeLine
			return m, nil
		}
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
		sec := m.currentSection()
		sec.visualActive = false
		if m.navMode == navHunk {
			m.navMode = navLine
		} else {
			m.navMode = navHunk
		}
		m.ensureActiveVisible(m.currentSection())
	case "v":
		sec := m.currentSection()
		if m.navMode == navHunk {
			m.navMode = navLine
		}
		if len(sec.parsed.Changed) == 0 {
			return m, nil
		}
		if !sec.visualActive {
			sec.visualActive = true
			sec.visualAnchor = sec.activeLine
		} else {
			sec.visualActive = false
			sec.visualAnchor = sec.activeLine
		}
		m.ensureActiveVisible(sec)
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
	case "d":
		if m.section == sectionStaged {
			cmd := m.applySelection()
			return m, cmd
		}
		m.openDiscardDiffConfirm()
		return m, nil
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

func visualLineBounds(sec sectionState) (start, end int) {
	start = sec.visualAnchor
	end = sec.activeLine
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if end >= len(sec.parsed.Changed) {
		end = len(sec.parsed.Changed) - 1
	}
	if start >= len(sec.parsed.Changed) {
		start = len(sec.parsed.Changed) - 1
	}
	if start < 0 {
		start = 0
	}
	return start, end
}
