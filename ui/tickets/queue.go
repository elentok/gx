package tickets

import (
	"fmt"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

// Keep plan waves aligned with the concurrency used when the plan executes.
const queuePlanMaxParallel = 2

const queueBanner = "This is the execution plan, press Enter to start"

// QueueModel renders a checked selection as dependency-aware epic waves.
type QueueModel struct {
	worktreeRoot string
	settings     ui.Settings
	checked      map[string]bool

	width, height int
	ready         bool
	loaded        bool
	epics         []tickets.Epic
	candidates    map[string]bool

	selected int

	implementAgentMenuOpen bool
	implementAgentMenu     components.MenuState
	runningEpic            string
	runningAgent           ralphloop.AgentKind
}

func NewQueueModel(worktreeRoot string, settings ui.Settings, checked map[string]bool) QueueModel {
	if checked == nil {
		checked = map[string]bool{}
	}
	return QueueModel{
		worktreeRoot:       worktreeRoot,
		settings:           settings,
		checked:            checked,
		implementAgentMenu: newImplementAgentMenu(),
	}
}

func (m QueueModel) Init() tea.Cmd {
	return m.cmdLoadQueue()
}

type queueEpicsLoadedMsg struct {
	epics []tickets.Epic
	err   error
}

func (m QueueModel) cmdLoadQueue() tea.Cmd {
	scratchDir := filepath.Join(m.worktreeRoot, ".scratch")
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		return queueEpicsLoadedMsg{epics: epics, err: err}
	}
}

func (m QueueModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil
	case queueEpicsLoadedMsg:
		m.loaded = true
		m.epics = msg.epics
		m.candidates = make(map[string]bool, len(m.checked))
		for path := range m.checked {
			m.candidates[path] = true
		}
		m.clampSelected()
		return m, nil
	case implementStartedMsg:
		m.runningEpic = msg.epicName
		return m, tea.Batch(cmdPollImplement(msg.epicName), cmdDrainQueueEvents(msg.epicName, msg.events))
	case implementPollMsg:
		if running, epicName, _ := ralphLoopRegistry.snapshot(msg.epicName); running && epicName == msg.epicName {
			return m, cmdPollImplement(msg.epicName)
		}
		m.runningEpic = ""
		return m, implementFinishedNotifyCmd(msg.epicName)
	case implementFailedMsg:
		m.runningEpic = ""
		return m, notify.Error(msg.err.Error())
	case tea.KeyPressMsg:
		return m.handleQueueKey(msg)
	}
	return m, nil
}

func (m QueueModel) handleQueueKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.implementAgentMenuOpen {
		return m.handleQueueAgentMenuKey(msg)
	}
	switch msg.String() {
	case "q", "esc":
		return m, nav.Back()
	case "j", "down":
		m.moveSelection(1)
	case "k", "up":
		m.moveSelection(-1)
	case "R":
		return m, m.cmdLoadQueue()
	case " ", "space":
		m.toggleSelected()
	case "enter":
		if m.runningEpic == "" {
			if _, _, _, ok := m.firstCheckedEpic(); ok {
				m.implementAgentMenu = newImplementAgentMenu()
				m.implementAgentMenuOpen = true
			}
		}
	}
	return m, nil
}

func (m QueueModel) handleQueueAgentMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "l":
		return m.startCheckedEpic(ralphloop.AgentClaude)
	case "o":
		return m.startCheckedEpic(ralphloop.AgentCodex)
	}

	next, decided, accepted, handled := components.UpdateMenu(msg, m.implementAgentMenu)
	if !handled {
		return m, nil
	}
	m.implementAgentMenu = next
	if !decided {
		return m, nil
	}
	if !accepted {
		m.implementAgentMenuOpen = false
		return m, nil
	}
	agent := ralphloop.AgentKind(m.implementAgentMenu.Items[m.implementAgentMenu.Cursor].Value)
	return m.startCheckedEpic(agent)
}

func (m QueueModel) startCheckedEpic(agent ralphloop.AgentKind) (tea.Model, tea.Cmd) {
	epic, ticketIDs, done, ok := m.firstCheckedEpic()
	if !ok {
		m.implementAgentMenuOpen = false
		return m, nil
	}
	m.implementAgentMenuOpen = false
	m.runningAgent = agent
	return m, cmdStartImplement(m.worktreeRoot, epic.Name, agent, done, len(ticketIDs), ticketIDs)
}

func (m QueueModel) firstCheckedEpic() (tickets.Epic, []string, int, bool) {
	for _, epic := range m.epics {
		var ticketIDs []string
		done := 0
		for _, idx := range sortedTicketIndexes(epic) {
			ticket := epic.Tickets[idx]
			if !m.checked[ticket.Path] {
				continue
			}
			ticketIDs = append(ticketIDs, ticket.Identifier)
			if epic.RenderedStatus(ticket) == tickets.StatusDone {
				done++
			}
		}
		if len(ticketIDs) > 0 {
			return epic, ticketIDs, done, true
		}
	}
	return tickets.Epic{}, nil, 0, false
}

type queueEventsDrainedMsg struct{}

func cmdDrainQueueEvents(epicName string, events <-chan ralphloop.LiveEvent) tea.Cmd {
	return func() tea.Msg {
		ticker := time.NewTicker(implementPollInterval)
		defer ticker.Stop()
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return queueEventsDrainedMsg{}
				}
				if event.Kind == ralphloop.LiveEventIterationFinished {
					ralphLoopRegistry.recordTicketFinished(epicName)
				}
			case <-ticker.C:
				running, runningEpic, _ := ralphLoopRegistry.snapshot(epicName)
				if !running || runningEpic != epicName {
					return queueEventsDrainedMsg{}
				}
			}
		}
	}
}

func (m *QueueModel) moveSelection(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(rows) {
		m.selected = len(rows) - 1
	}
}

func (m *QueueModel) clampSelected() {
	n := len(m.rows())
	switch {
	case n == 0:
		m.selected = 0
	case m.selected >= n:
		m.selected = n - 1
	case m.selected < 0:
		m.selected = 0
	}
}

func (m *QueueModel) toggleSelected() {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return
	}
	path := rows[m.selected].ticket.Path
	if m.checked[path] {
		delete(m.checked, path)
		return
	}
	m.checked[path] = true
}

type queueRow struct {
	epicName string
	ticket   tickets.Ticket
	wave     int
	waveSize int
}

func (m QueueModel) rows() []queueRow {
	for path := range m.checked {
		m.candidates[path] = true
	}
	var out []queueRow
	for _, epic := range m.epics {
		for waveIndex, wave := range buildEpicWaves(epic, m.candidates, queuePlanMaxParallel) {
			for _, t := range wave {
				out = append(out, queueRow{epicName: epic.Name, ticket: t, wave: waveIndex, waveSize: len(wave)})
			}
		}
	}
	return out
}

// buildEpicWaves orders selected tickets while ignoring dependencies outside the plan.
func buildEpicWaves(epic tickets.Epic, checked map[string]bool, maxParallel int) [][]tickets.Ticket {
	var pending []tickets.Ticket
	for _, idx := range sortedTicketIndexes(epic) {
		t := epic.Tickets[idx]
		if checked[t.Path] {
			pending = append(pending, t)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	assigned := make(map[string]bool, len(pending))
	var waves [][]tickets.Ticket
	for len(pending) > 0 {
		var wave, next []tickets.Ticket
		for _, t := range pending {
			if len(wave) < maxParallel && blockersSatisfied(epic, t, checked, assigned) {
				wave = append(wave, t)
			} else {
				next = append(next, t)
			}
		}
		if len(wave) == 0 {
			// Malformed cycles remain visible without implying parallel safety.
			wave, next = pending[:1], pending[1:]
		}
		waves = append(waves, wave)
		for _, t := range wave {
			assigned[t.Path] = true
		}
		pending = next
	}
	return waves
}

func blockersSatisfied(epic tickets.Epic, t tickets.Ticket, checked, assigned map[string]bool) bool {
	for _, b := range epic.BlockingTickets(t) {
		if checked[b.Path] && !assigned[b.Path] {
			return false
		}
	}
	return true
}

func (m QueueModel) View() tea.View {
	if !m.ready {
		return ui.NewMainView("\n  Initializing…")
	}
	if m.implementAgentMenuOpen {
		epic, _, _, _ := m.firstCheckedEpic()
		return ui.NewMainView(renderImplementAgentMenu(
			fmt.Sprintf("Choose the agent for epic %q:", epic.Name),
			m.implementAgentMenu,
		))
	}
	lines := m.queueLines()
	height := max(m.height-1, 1)
	return ui.NewMainView(ui.RenderPanel(ui.PanelOptionsFor(
		m.width, height, "Queue", "", lines, true, ui.ColorBlue, nil, false,
	)))
}

func (m QueueModel) queueLines() []string {
	if !m.loaded {
		return []string{ui.StyleDim.Render("  loading…")}
	}

	banner := queueBanner
	if m.runningEpic != "" {
		banner = fmt.Sprintf("Running %s with %s", m.runningEpic, agentDisplayName(m.runningAgent))
	}
	lines := []string{"  " + ui.StyleHint.Render(banner), ""}

	rows := m.rows()
	if len(rows) == 0 {
		lines = append(lines, ui.StyleMuted.Render("  no tickets checked — check tickets in the Tickets tab to build a plan"))
		return lines
	}

	i := 0
	epicName := ""
	previousWave := -1
	for i < len(rows) {
		r := rows[i]
		if r.epicName != epicName {
			epicName = r.epicName
			previousWave = -1
			lines = append(lines, "", sectionHeaderStyle.Render("── "+epicName+" ──"))
		}
		if previousWave >= 0 && r.wave != previousWave {
			lines = append(lines, ui.StyleDim.Render("    then"))
		}
		if r.waveSize > 1 && r.wave != previousWave {
			lines = append(lines, fmt.Sprintf("    %s (%d):", statusClaimedStyle.Render("parallel"), r.waveSize))
		}
		indent := "    "
		if r.waveSize > 1 {
			indent = "      "
		}
		selected := i == m.selected
		line := fmt.Sprintf("%s%s %s %s", indent, m.checkboxGlyph(m.checked[r.ticket.Path]), r.ticket.DisplayNumber(), r.ticket.Title)
		if selected {
			line = ui.RenderRowHighlight(line)
		}
		lines = append(lines, line)
		previousWave = r.wave
		i++
	}
	return lines
}

func (m QueueModel) checkboxGlyph(checked bool) string {
	icons := ui.Icons(m.settings.UseNerdFontIcons)
	if checked {
		return checkedGlyphStyle.Render(icons.CheckboxChecked)
	}
	return uncheckedGlyphStyle.Render(icons.CheckboxUnchecked)
}
