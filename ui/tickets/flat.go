package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/terminalrun"
)

// flatRefreshInterval is how often FlatModel re-reads .scratch/ from disk to
// pick up Status: line changes made by a concurrently running `ralphloop.Run`
// (or by hand) — this ticket doesn't yet wire in live orchestrator state
// (spinners, paused badges), so disk polling is the only signal it has.
const flatRefreshInterval = 2 * time.Second

type flatFocus int

const (
	flatFocusList flatFocus = iota
	flatFocusPreview
)

// FlatModel is the standalone `gx ralph-loop {epic-name}` TUI: a flat list of
// one epic's tickets (no epic-of-epics nesting like Model's sidebar) paired
// with a preview panel, launched via its own tea.NewProgram (see
// cmd/ralphloop.go) rather than registered as a ui/app tab — so unlike Model
// it owns its own quit/notify handling instead of delegating to nav/app.
type FlatModel struct {
	worktreeRoot string
	epicName     string
	settings     ui.Settings
	keys         keys.Manager
	notify       notify.Model

	width  int
	height int
	ready  bool

	loaded  bool
	found   bool
	epic    tickets.Epic
	ordered []tickets.Ticket // epic.Tickets sorted by rendered-status group then number

	selected int

	focus         flatFocus
	previewVP     viewport.Model
	previewSelKey string
	previewSearch search.Model
}

// NewFlatModel builds the flat ralph-loop TUI model scoped to a single named
// epic under worktreeRoot's `.scratch/`.
func NewFlatModel(worktreeRoot, epicName string, settings ui.Settings) FlatModel {
	return FlatModel{
		worktreeRoot:  worktreeRoot,
		epicName:      epicName,
		settings:      settings,
		keys:          newFlatTicketsManager(),
		notify:        notify.New(settings.UseNerdFontIcons),
		previewSearch: search.NewModel(),
		previewVP:     viewport.New(),
	}
}

func (m FlatModel) KeyManager() keys.Manager { return m.keys }

func (m FlatModel) Init() tea.Cmd {
	return tea.Batch(m.cmdLoad(), m.cmdTick())
}

type flatEpicLoadedMsg struct {
	epic  tickets.Epic
	found bool
	err   error
}

type flatTickMsg struct{}

func (m FlatModel) scratchDir() string {
	return filepath.Join(m.worktreeRoot, ".scratch")
}

func (m FlatModel) cmdLoad() tea.Cmd {
	scratchDir := m.scratchDir()
	epicName := m.epicName
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		if err != nil {
			return flatEpicLoadedMsg{err: err}
		}
		for _, epic := range epics {
			if epic.Name == epicName {
				return flatEpicLoadedMsg{epic: epic, found: true}
			}
		}
		return flatEpicLoadedMsg{found: false}
	}
}

// cmdRefresh reloads .scratch/ on demand (the "R" binding), matching every
// other tab's manual refresh convention with a success toast.
func (m FlatModel) cmdRefresh() tea.Cmd {
	return tea.Batch(notify.Success("refreshed"), m.cmdLoad())
}

func (m FlatModel) cmdTick() tea.Cmd {
	return tea.Tick(flatRefreshInterval, func(time.Time) tea.Msg {
		return flatTickMsg{}
	})
}

// sortedTickets orders epic.Tickets the same way ui/tickets' sidebar does:
// by rendered-status group (unblocked → blocked → needs-info → done →
// error), ticket number ascending within each group.
func sortedTickets(epic tickets.Epic) []tickets.Ticket {
	ordered := make([]tickets.Ticket, len(epic.Tickets))
	copy(ordered, epic.Tickets)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		groupA, groupB := tickets.GroupOrder(epic.RenderedStatus(a)), tickets.GroupOrder(epic.RenderedStatus(b))
		if groupA != groupB {
			return groupA < groupB
		}
		return a.Number < b.Number
	})
	return ordered
}

func (m *FlatModel) clampSelected() {
	n := len(m.ordered)
	switch {
	case n == 0:
		m.selected = 0
	case m.selected >= n:
		m.selected = n - 1
	case m.selected < 0:
		m.selected = 0
	}
}

func (m FlatModel) selectedTicket() (tickets.Ticket, bool) {
	if m.selected < 0 || m.selected >= len(m.ordered) {
		return tickets.Ticket{}, false
	}
	return m.ordered[m.selected], true
}

func (m FlatModel) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}

func (m FlatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.updateInner(msg)
	nm := next.(FlatModel)
	nm.syncPreviewViewport()
	return nm, cmd
}

func (m FlatModel) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	var notifyCmd tea.Cmd
	m.notify, notifyCmd = m.notify.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, notifyCmd

	case flatEpicLoadedMsg:
		m.loaded = true
		m.found = msg.found
		m.epic = msg.epic
		m.ordered = sortedTickets(msg.epic)
		m.clampSelected()
		if msg.err != nil {
			return m, tea.Batch(notifyCmd, notify.Error("load .scratch/: "+msg.err.Error()))
		}
		return m, notifyCmd

	case flatTickMsg:
		return m, tea.Batch(notifyCmd, m.cmdLoad(), m.cmdTick())

	case editFileFinishedMsg:
		next, cmd := m.handleEditFileFinished(msg)
		return next, tea.Batch(notifyCmd, cmd)

	case tea.KeyPressMsg:
		next, cmd := m.handleFlatKey(msg)
		return next, tea.Batch(notifyCmd, cmd)
	}
	return m, notifyCmd
}

func (m FlatModel) handleEditFileFinished(msg editFileFinishedMsg) (FlatModel, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return m, notify.Info("opened " + msg.splitApp + " split: editor")
	}
	return m, nil
}

func (m FlatModel) selectedEditTarget() (path string, ok bool) {
	t, ok := m.selectedTicket()
	if !ok {
		return "", false
	}
	return t.Path, true
}

// cmdEditSelectedFile opens the selected ticket's file for editing, mirroring
// Model.cmdEditSelectedFile (model_runtime.go) minus the epic/map.md case
// this flat, single-epic list has no use for.
func (m FlatModel) cmdEditSelectedFile(splitType terminalrun.SplitType) tea.Cmd {
	target, ok := m.selectedEditTarget()
	if !ok {
		return notify.Warning("nothing selected")
	}

	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return notify.Warning("$EDITOR is not set")
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return notify.Warning("$EDITOR is empty")
	}

	args := ui.EditorLaunchArgs(parts[0], parts[1:], target, 0)
	return terminalrun.CommandWithSplit(m.worktreeRoot, m.settings.Terminal, splitType, parts[0], args, func(err error, splitApp string) tea.Msg {
		return editFileFinishedMsg{err: err, splitApp: splitApp}
	})
}

func (m FlatModel) flatSplitWidth() (listW, previewW int) {
	if m.flatUseStackedLayout() {
		return m.width, m.width
	}
	width := m.width - 1
	listW = int(float64(width) * 0.55)
	previewW = width - listW
	return
}

func (m FlatModel) flatUseStackedLayout() bool {
	return m.width <= 100
}

func (m FlatModel) flatContentHeight() int {
	h := m.height
	if m.flatUseStackedLayout() {
		h -= 1
	}
	if h < 4 {
		return 4
	}
	return h
}

func (m FlatModel) titleLine() string {
	return fmt.Sprintf("%s (%d/%d)", m.epicName, m.epic.DoneCount(), m.epic.TotalCount())
}
