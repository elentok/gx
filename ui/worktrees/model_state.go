package worktrees

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
	keymgr "github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/pull"
	"github.com/elentok/gx/ui/search"

	"github.com/elentok/gx/ui/help"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
)

type mode int

const (
	modeNormal mode = iota
	modeRename
	modeClone
	modeNew
	modeNewAndOpen
	modeYank
	modePaste
	modeSearch
	modeError
	modeLogs
	modePushDiverged
	modeCredentialPrompt
	modeHelp
	modeTerminalMenu
	modeDeleteProgress
)

type promptableJobKind int

const (
	promptableJobPushFetch promptableJobKind = iota
	promptableJobPush
	promptableJobForcePush
)

type dirtyState struct {
	hasModified  bool
	hasUntracked bool
}

// Model is the BubbleTea model for the worktrees page.
type Model struct {
	repo               git.Repo
	activeWorktreePath string // path of the worktree the user launched from
	settings           ui.Settings

	worktrees         []git.Worktree
	selectedWorktrees map[string]bool // worktree names tagged for bulk operations
	statuses          map[string]git.SyncStatus
	dirties           map[string]dirtyState
	baseStatus        map[string]*bool // keyed by branch; nil=loading, &true=rebased, &false=needs rebase

	table    table.Model
	viewport viewport.Model

	previewUpstream      string // empty if no remote tracking branch found
	previewHeadCommit    git.Commit
	previewAheadCommits  []git.Commit
	previewBehindCommits []git.Commit
	previewChanges       []git.Change
	previewLoading       bool

	pull   pull.Model
	pullWT *git.Worktree // worktree being pulled, set when pull.Open is called

	mode                 mode
	textInput            textinput.Model // shared by rename and clone modes
	credentialPromptText string
	errorViewport        viewport.Model
	helpModel            help.Model
	jobRunner            *components.CommandRunner
	jobKind              promptableJobKind
	jobWorktree          *git.Worktree
	jobLog               *ui.CommandOutputLog
	jobStashed           bool

	lastJobLog   string
	lastJobLabel string
	logsViewport viewport.Model

	confirm confirm.Model

	deleteQueue    []git.Worktree    // worktrees pending deletion in the current batch
	deleteInFlight int               // number of concurrent deletes in progress
	deleteSteps    []components.Step // one per worktree in the batch, for the progress modal
	deleteResults  []deleteResultMsg // accumulated results from the batch

	pushDivergence   *git.PushDivergence
	pushDivergenceWT *git.Worktree
	pushMenu         components.MenuState

	yankLoading   bool
	yankSource    git.Worktree
	yankChecklist components.Checklist
	clipboard     *clipboardState

	keyManager keymgr.Manager
	search     search.Model

	openTargetName string
	openTargetPath string
	terminalMenu   components.MenuState

	help help.Model

	spinner       spinner.Model
	spinnerActive bool
	spinnerLabel  string

	width  int
	height int
	ready  bool // true once we've received the first WindowSizeMsg

	loading    bool
	refreshing bool
	err        error
}

// New creates a new worktrees page model. activeWorktreePath is the path of the
// worktree the user is currently in (empty if launched from the bare repo root).
func New(repo git.Repo, activeWorktreePath string) Model {
	return NewWithSettings(repo, activeWorktreePath, ui.Settings{})
}

// NewWithSettings creates a new worktrees page model with explicit settings.
func NewWithSettings(repo git.Repo, activeWorktreePath string, settings ui.Settings) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	km := newWorktreesManager()

	return Model{
		repo:               repo,
		activeWorktreePath: activeWorktreePath,
		settings:           settings,
		statuses:           make(map[string]git.SyncStatus),
		dirties:            make(map[string]dirtyState),
		baseStatus:         make(map[string]*bool),
		selectedWorktrees:  make(map[string]bool),
		table:              newTable(),
		loading:            true,
		confirm:            confirm.New(),
		pull:               pull.New(),
		helpModel:          help.NewModel(help.BuildSections(km)),
		keyManager:         km,
		search:             search.NewModel(),
		spinner:            sp,
	}
}

func dirtyStateFromChanges(changes []git.Change) dirtyState {
	var out dirtyState
	for _, ch := range changes {
		if ch.Kind == git.ChangeUntracked {
			out.hasUntracked = true
		} else {
			out.hasModified = true
		}
	}
	return out
}

func (m Model) KeyManager() keymgr.Manager {
	return m.keyManager
}

// cursorWorktree returns a pointer to the currently highlighted worktree, or nil.
func (m Model) cursorWorktree() *git.Worktree {
	if len(m.worktrees) == 0 {
		return nil
	}
	w := m.worktrees[m.table.Cursor()]
	return &w
}
