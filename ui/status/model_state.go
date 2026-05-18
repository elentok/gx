package status

import (
	"regexp"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/bump"
	"github.com/elentok/gx/ui/output"
	"github.com/elentok/gx/ui/pull"
	"github.com/elentok/gx/ui/push"
	"github.com/elentok/gx/ui/status/diffarea"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
)

type Model struct {
	worktreeRoot string
	settings     ui.Settings
	initialPath  string

	width  int
	height int
	ready  bool

	focus            focusPane
	diffarea         diffarea.Model
	diffContextLines int
	statusData       statusData
	fileTreeModel    filetree.Model[git.StageFileStatus]

	err            error
	errorOpen      bool
	errorVP        viewport.Model
	help           help.Model
	activeFilePath string
	diffReloadSeq  int

	bump bump.Model
	push push.Model
	pull pull.Model

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
	credentialOpen          bool
	credentialPrompt        string
	credentialInput         textinput.Model
	credentialSecret        bool
	output output.Model
	keys   keys.Manager
}

type statusData struct {
	files         []git.StageFileStatus
	branchName    string
	branchBaseRef string
	branchSync    git.SyncStatus
	statusEntries []statusEntry
	statusRows    []filetree.Entry[git.StageFileStatus]
	listState     list.Model
}

func DefaultSettings() ui.Settings {
	return ui.Settings{UseNerdFontIcons: true, Terminal: ui.TerminalPlain, DiffContextLines: 1}
}

type flashTickMsg struct{}
type renderTickMsg struct{}
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
type editCommentFinishedMsg struct {
	err      error
	splitApp string
}

const statusDiffReloadDebounce = 100 * time.Millisecond

var (
	ansiCSIRe = regexp.MustCompile(`\x1b\[[0-9:;<=>?]*[ -/]*[@-~]`)
	ansiOSCRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`) // OSC ... BEL/ST
)

func NewModel(worktreeRoot string, settings ui.Settings, initialPath string, extraKeys keys.Manager) Model {
	if settings.DiffContextLines < 0 {
		settings.DiffContextLines = 0
	}
	if settings.DiffContextLines > 20 {
		settings.DiffContextLines = 20
	}
	statusKeys := newStatusManager()
	diffarreaModel := diffarea.NewModel()
	fileTreeModel := filetree.NewModel[git.StageFileStatus]()
	m := Model{
		worktreeRoot:     worktreeRoot,
		settings:         settings,
		initialPath:      initialPath,
		diffContextLines: settings.DiffContextLines,
		help:             help.NewModel(help.BuildSections(statusKeys, *diffarreaModel.Keys(), *fileTreeModel.Keys(), extraKeys)),
		keys:             statusKeys,
		focus:            focusFiletree,
		diffarea:         diffarreaModel,
		statusData:       statusData{},
		fileTreeModel:    fileTreeModel,
		output:           output.New(),
		bump:             bump.New(),
		push:             push.New(),
		pull:             pull.New(),
	}

	if settings.EnableNavigation {
		m.reloadFileList("")
	} else {
		m.reload("")
	}
	return m
}


func (m Model) KeyManager() keys.Manager {
	return m.keys
}
