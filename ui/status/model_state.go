package status

import (
	"regexp"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/help"

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

	focus focusPane
	diffArea
	diffContextLines int
	statusPageState
	fileTreeModel filetree.Model[git.StageFileStatus]

	statusMsg      string
	statusUntil    time.Time
	err            error
	errorOpen      bool
	errorVP        viewport.Model
	help           help.Model
	activeFilePath string
	diffReloadSeq  int

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
	files         []git.StageFileStatus
	branchName    string
	branchBaseRef string
	branchSync    git.SyncStatus
	statusEntries []statusEntry
	statusRows    []filetree.Entry[git.StageFileStatus]
	collapsedDirs map[string]bool
	selected      int
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

func NewModel(worktreeRoot string, settings Settings) Model {
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
		help:             help.NewModel(keySections),
		focus:            focusFiletree,
		diffArea:         newDiffArea(),
		statusPageState: statusPageState{
			collapsedDirs: map[string]bool{},
			selected:      0,
		},
		fileTreeModel: filetree.NewModel[git.StageFileStatus](),
	}

	if settings.EnableNavigation {
		m.reloadFileList("")
	} else {
		m.reload("")
	}
	return m
}

func New(worktreeRoot string) Model {
	return NewModel(worktreeRoot, DefaultSettings())
}
