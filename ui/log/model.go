package log

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/amend"
	"github.com/elentok/gx/ui/bump"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/output"
	"github.com/elentok/gx/ui/pull"
	"github.com/elentok/gx/ui/push"
	"github.com/elentok/gx/ui/reword"
	"github.com/elentok/gx/ui/search"
)

const maxLogEntries = 250

type rowKind int

const (
	rowCommit rowKind = iota
	rowPseudoStatus
)

type row struct {
	kind   rowKind
	commit git.LogEntry
	label  string
	detail string
	class  git.BranchHistoryClass
}

type Settings struct {
	UseNerdFontIcons bool
	InputModalBottom config.InputModalBottom
	EnableNavigation bool
}

type Model struct {
	worktreeRoot string
	settings     Settings
	startRef     string

	width  int
	height int
	ready  bool

	rows      []row
	list      list.Model
	statusMsg string
	keys      keys.Manager
	search    search.Model
	err       error

	help help.Model

	branchDiverged bool

	amendConfirm amend.Model
	bump         bump.Model
	push         push.Model
	pull         pull.Model
	output       output.Model

	reword           reword.Model
	rewordTmpFile    string
	rewordOrigMsg    string
	rewordHash       string
	rewordSubject    string
	rewordNewSubject string

	pendingFocusSubject string
	flashSubject        string
	flashUntil          time.Time

	refreshing bool
}

func NewModel(worktreeRoot, startRef string, settings Settings) Model {
	m := Model{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		startRef:     normalizedRef(startRef),
		keys:         newLogManager(),
		search:       search.NewModel(),
	}
	m.help = help.NewModel(buildKeySections(m.keys))
	m.amendConfirm = amend.New()
	m.bump = bump.New()
	m.push = push.New()
	m.pull = pull.New()
	m.output = output.New()
	m.reword = reword.New()
	m.reload()
	return m
}

func (m Model) Init() tea.Cmd { return m.cmdReload() }

// OnPageActivated is called by the app shell when switching to the log page.
func (m Model) OnPageActivated() tea.Cmd {
	if m.pendingFocusSubject != "" {
		return m.cmdReloadFocusSubject(m.pendingFocusSubject)
	}
	return m.cmdReload()
}

// WithPendingFocus sets a subject to focus on when the page next activates.
func (m Model) WithPendingFocus(subject string) Model {
	m.pendingFocusSubject = subject
	return m
}

func (m Model) InputFocused() bool {
	return m.search.Mode() == search.SearchModeInput
}

func normalizedRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "HEAD"
	}
	return ref
}
