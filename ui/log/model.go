package log

import (
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	humanize "github.com/dustin/go-humanize"
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
}

type searchMode int

const (
	searchModeNone searchMode = iota
	searchModeInput
)

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

	rows         []row
	cursor       int
	statusMsg    string
	keyPrefix    string
	searchMode   searchMode
	searchInput  textinput.Model
	searchQuery  string
	searchMatch  []int
	searchCursor int
	err          error
}

func New(worktreeRoot, startRef string) Model {
	return NewWithSettings(worktreeRoot, startRef, Settings{UseNerdFontIcons: true})
}

func NewWithSettings(worktreeRoot, startRef string, settings Settings) Model {
	m := Model{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		startRef:     normalizedRef(startRef),
		cursor:       0,
	}
	m.reload()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func normalizedRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "HEAD"
	}
	return ref
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return humanize.Time(t)
}
