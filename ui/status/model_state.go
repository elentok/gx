package status

import (
	"regexp"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
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
	colorizeSeq             int
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

type diffColorizeMsg struct {
	seq           int
	filePath      string
	unstagedRaw   string
	unstagedColor string
	stagedRaw     string
	stagedColor   string
}

type branchSyncLoadedMsg struct {
	branchName string
	sync       git.SyncStatus
}

type commitFinishedMsg struct {
	err      error
	splitApp string // "tmux", "kitty", or "" for foreground
}

type lazygitLogFinishedMsg struct{ err error }
type editFileFinishedMsg struct{ err error }

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
	}
	if settings.EnableNavigation {
		m.reloadFileList("")
	} else {
		m.reload("")
	}
	return m
}
