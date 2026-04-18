package worktrees

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type mode int

const (
	modeNormal mode = iota
	modeConfirm
	modeRename
	modeClone
	modeNew
	modeNewTmuxSession
	modeNewTmuxWindow
	modeYank
	modePaste
	modeSearch
	modeError
	modeLogs
	modePushDiverged
	modeCredentialPrompt
)

type promptableJobKind int

const (
	promptableJobPull promptableJobKind = iota
	promptableJobPushFetch
	promptableJobPush
	promptableJobForcePush
)

type dirtyState struct {
	hasModified  bool
	hasUntracked bool
}

// Settings controls optional rendering behavior for the worktrees UI.
type Settings struct {
	UseNerdFontIcons bool
}

// Model is the BubbleTea model for the worktrees page.
type Model struct {
	repo               git.Repo
	activeWorktreePath string // path of the worktree the user launched from
	settings           Settings

	worktrees  []git.Worktree
	statuses   map[string]git.SyncStatus
	dirties    map[string]dirtyState
	baseStatus map[string]*bool // keyed by branch; nil=loading, &true=rebased, &false=needs rebase

	table    table.Model
	viewport viewport.Model

	sidebarUpstream      string // empty if no remote tracking branch found
	sidebarHeadCommit    git.Commit
	sidebarAheadCommits  []git.Commit
	sidebarBehindCommits []git.Commit
	sidebarChanges       []git.Change
	sidebarLoading       bool

	mode          mode
	textInput     textinput.Model // shared by rename and clone modes
	statusMsg     string
	statusGen     int // incremented each time statusMsg is set, used to expire old ticks
	errorViewport viewport.Model
	jobRunner     *components.CommandRunner
	jobKind       promptableJobKind
	jobWorktree   *git.Worktree
	jobLog        *ui.CommandOutputLog
	jobStashed    bool

	lastJobLog   string
	lastJobLabel string
	logsViewport viewport.Model

	confirmPrompt       string
	confirmYes          bool
	confirmCmd          tea.Cmd // executed when the user confirms
	confirmSpinnerLabel string  // if non-empty, spinner is started on confirm
	confirmCancelMsg    string  // if non-empty, shown as statusMsg when user cancels

	pushDivergence   *git.PushDivergence
	pushDivergenceWT *git.Worktree
	pushMenu         components.MenuState

	yankLoading   bool
	yankSource    git.Worktree
	yankChecklist components.Checklist
	clipboard     *clipboardState

	searchQuery   string
	searchMatches []int
	searchCursor  int
	keyPrefix     string

	help help.Model

	spinner       spinner.Model
	spinnerActive bool
	spinnerLabel  string

	width  int
	height int
	ready  bool // true once we've received the first WindowSizeMsg

	loading bool
	err     error
}

// New creates a new worktrees page model. activeWorktreePath is the path of the
// worktree the user is currently in (empty if launched from the bare repo root).
func New(repo git.Repo, activeWorktreePath string) Model {
	return NewWithSettings(repo, activeWorktreePath, Settings{})
}

// NewWithSettings creates a new worktrees page model with explicit settings.
func NewWithSettings(repo git.Repo, activeWorktreePath string, settings Settings) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		repo:               repo,
		activeWorktreePath: activeWorktreePath,
		settings:           settings,
		statuses:           make(map[string]git.SyncStatus),
		dirties:            make(map[string]dirtyState),
		baseStatus:         make(map[string]*bool),
		table:              newTable(),
		loading:            true,
		help:               help.New(),
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

// selectedWorktree returns a pointer to the currently highlighted worktree, or nil.
func (m Model) selectedWorktree() *git.Worktree {
	if len(m.worktrees) == 0 {
		return nil
	}
	w := m.worktrees[m.table.Cursor()]
	return &w
}
