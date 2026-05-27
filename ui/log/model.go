package log

import (
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
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

// LogFilter restricts the log view to a file path or a line range within a file.
type LogFilter struct {
	Path      string
	StartLine int // 0 = file-only
	EndLine   int
}

func (f LogFilter) IsActive() bool { return f.Path != "" }

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

type Model struct {
	worktreeRoot     string
	settings         ui.Settings
	compiledRefRules []compiledRefRule
	compiledHideRefs []*regexp.Regexp
	startRef         string
	filter          LogFilter

	width  int
	height int
	ready  bool

	rows []row
	list list.Model
	keys keys.Manager
	search    search.Model
	err       error

	help help.Model

	branchDiverged bool

	amendConfirm amend.Model
	bump         bump.Model
	push         push.Model
	pull         pull.Model
	output       output.Model

	reword reword.Model

	pendingFocusSubject string
	flashSubject        string
	flashUntil          time.Time

	refreshing bool

	rebaseConfirm  rebaseConfirmState // confirm modal for stash-before-rebase and stash-pop-after-rebase
	rebaseDidStash bool               // stash was pushed before rebase; pop prompt fires on FocusMsg (kitty/tmux) or immediately (exec)
}

func NewModel(worktreeRoot, startRef string, settings ui.Settings, filter LogFilter, extraKeys keys.Manager) Model {
	m := Model{
		worktreeRoot:     worktreeRoot,
		settings:         settings,
		compiledRefRules: compileRefRules(settings.LogConfig.ImportantRefs),
		compiledHideRefs: compileHideRefs(settings.LogConfig.HideRefs),
		startRef:         normalizedRef(startRef),
		filter:          filter,
		keys:            newLogManager(),
		search:          search.NewModel(),
	}
	m.help = help.NewModel(help.BuildSections(m.keys, m.search.Keys(), extraKeys))
	m.amendConfirm = amend.New()
	m.bump = bump.New()
	m.push = push.New()
	m.pull = pull.New()
	m.output = output.New()
	m.reword = reword.New()
	m.reload()
	return m
}

func (m Model) KeyManager() keys.Manager {
	return m.keys
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
	return m.search.Mode() == search.SearchModeInput || m.push.InputFocused() || m.pull.InputFocused()
}

func normalizedRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "HEAD"
	}
	return ref
}
