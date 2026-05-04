package status

import (
	"regexp"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

type Model struct {
	worktreeRoot string
	settings     Settings
	initialPath  string

	width  int
	height int
	ready  bool

	explorerState
	diffContextLines int
	statusPageState

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
	help                    help.Model
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
	credentialOpen          bool
	credentialPrompt        string
	credentialInput         textinput.Model
	credentialSecret        bool
	outputOpen              bool
	outputTitle             string
	outputContent           string
	outputViewport          viewport.Model
	pendingActionOutput     string
	keyPrefix               string
}

type statusPageState struct {
	files          []git.StageFileStatus
	branchName     string
	branchBaseRef  string
	branchSync     git.SyncStatus
	statusEntries  []statusEntry
	collapsedDirs  map[string]bool
	selected       int
	activeFilePath string
}

type Settings struct {
	DiffContextLines int
	UseNerdFontIcons bool
	InitialPath      string
	Terminal         ui.Terminal
	InputModalBottom config.InputModalBottom
	EnableNavigation bool
}

func DefaultSettings() Settings {
	return Settings{DiffContextLines: 1, UseNerdFontIcons: true, Terminal: ui.TerminalPlain}
}

type flashTickMsg struct{}
type statusTickMsg struct{}
type actionPollMsg struct{}
type diffReloadMsg struct{ seq int }
type statusStartupLoadMsg struct{}

type commitFinishedMsg struct {
	err      error
	splitApp string // "tmux", "kitty", or "" for foreground
}

type lazygitLogFinishedMsg struct{ err error }
type editFileFinishedMsg struct{ err error }

var (
	catBase0   = lipgloss.Color("#1e1e2e")
	catDeepBg  = lipgloss.Color("#11111a")
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
		worktreeRoot:     worktreeRoot,
		settings:         settings,
		initialPath:      settings.InitialPath,
		diffContextLines: settings.DiffContextLines,
		explorerState: explorerState{
			focus:      focusStatus,
			section:    sectionUnstaged,
			navMode:    navHunk,
			renderMode: renderUnified,
			wrapSoft:   true,
			unstaged:   newSectionState(),
			staged:     newSectionState(),
		},
		statusPageState: statusPageState{
			collapsedDirs: map[string]bool{},
			selected:      0,
		},
		help: newStageHelpModel(),
	}
	if settings.EnableNavigation {
		m.reloadFileList("")
	} else {
		m.reload("")
	}
	return m
}
