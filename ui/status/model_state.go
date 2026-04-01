package stage

import (
	"regexp"
	"time"

	"gx/git"
	"gx/ui/components"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
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
	branchName    string
	branchBaseRef string
	branchSync    git.SyncStatus
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
	confirmUpstream         string
	confirmBranch           string
	confirmPaths            []string
	confirmPatch            string
	confirmPatchUnidiffZero bool
	confirmDiscardUntracked bool
	confirmMenu             components.MenuState
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
type pushPreflightMsg struct {
	err        error
	branch     string
	remote     string
	divergence *git.PushDivergence
}

type commitFinishedMsg struct {
	err       error
	tmuxSplit bool
}

type lazygitLogFinishedMsg struct{ err error }
type editFileFinishedMsg struct{ err error }

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
