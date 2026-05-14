package log

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
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
	cursor    int
	statusMsg string
	keys      keys.Manager
	search    search.Model
	err       error

	help help.Model

	branchDiverged bool
}

func NewModel(worktreeRoot, startRef string, settings Settings) Model {
	m := Model{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		startRef:     normalizedRef(startRef),
		cursor:       0,
		keys:         newLogManager(),
		search:       search.NewModel(),
	}
	m.help = help.NewModel(buildKeySections(m.keys))
	m.reload()
	return m
}

func (m Model) Init() tea.Cmd { return m.cmdReload() }

// OnPageActivated is called by the app shell when switching to the log page.
func (m Model) OnPageActivated() tea.Cmd { return m.cmdReload() }

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
