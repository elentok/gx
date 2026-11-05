package log

import (
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/commitops"
	"github.com/elentok/gx/ui/gitops"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/splitview"
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
	filter           LogFilter

	width  int
	height int
	ready  bool

	listPanel listPanel
	keys      keys.Manager
	search    search.Model
	err       error

	help help.Model

	branchDiverged bool
	trunkProblem   string // last trunk resolver error shown as a warning

	commitOps commitops.Model
	git       gitops.Model

	commitInfo commitInfoPopup

	pendingFocusSubject string
	pendingFocusRef     string
	flashSubject        string
	flashUntil          time.Time

	refreshing bool

	showGraph bool

	rebase rebaseFlow

	// Split view state (embedded so m.split / m.commitDetail keep working).
	splitPane

	// Worktree status for the pseudo-log-line.
	statusLoaded    bool
	statusStaged    int
	statusUnstaged  int
	statusUntracked int
}

func NewModel(worktreeRoot, startRef string, settings ui.Settings, filter LogFilter, extraKeys keys.Manager) Model {
	m := Model{
		worktreeRoot:     worktreeRoot,
		settings:         settings,
		compiledRefRules: compileRefRules(settings.LogConfig.ImportantRefs),
		compiledHideRefs: compileHideRefs(settings.LogConfig.HideRefs),
		startRef:         normalizedRef(startRef),
		filter:           filter,
		keys:             newLogManager(),
		search:           search.NewModel(),
	}
	m.help = help.NewModel(help.BuildSections(m.keys, m.search.Keys(), extraKeys), settings.About)
	m.commitOps = commitops.New()
	m.git = gitops.New()
	m.listPanel = newListPanel()
	m.commitDetail = commit.NewModel(worktreeRoot, "HEAD", "", settings, keys.Manager{})
	// split must be built after commitDetail (it holds copies of both panels).
	m.split = splitview.New(m.listPanel, m.commitDetail)
	return m
}

func (m Model) KeyManager() keys.Manager {
	return m.keys
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.cmdReload(), m.cmdLoadStatus()) }

// AutoReload is called by the app shell when the tab is stale or carries a
// FocusSubject payload. It preserves selection index (cmdReload) and handles
// the pending-focus path (cmdReloadFocusSubject).
func (m Model) AutoReload() tea.Cmd {
	if m.pendingFocusSubject != "" {
		return m.cmdReloadFocusSubject(m.pendingFocusSubject)
	}
	return m.cmdReload()
}

// NeedsInitialLoad reports whether the page's first data load never landed.
// rows stays nil until a reloadMsg is applied; if the user leaves the tab
// before the in-flight Init reload returns, that result is delivered to the
// then-active page and never reaches this model. The app shell uses this to
// reload on re-activation instead of leaving the page stuck loading.
func (m Model) NeedsInitialLoad() bool {
	return m.listPanel.Rows() == nil
}

// WithPendingFocus sets a subject to focus on when the page next activates.
func (m Model) WithPendingFocus(subject string) Model {
	m.pendingFocusSubject = subject
	return m
}

func (m Model) InputFocused() bool {
	return m.search.Mode() == search.SearchModeInput || m.git.InputFocused() || m.help.InputFocused()
}

func (m Model) ModalOpen() bool {
	return m.commitOps.IsOpen() || m.git.IsOpen() || m.commitInfo.open
}

func normalizedRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "HEAD"
	}
	return ref
}
