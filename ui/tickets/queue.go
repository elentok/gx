package tickets

import (
	"fmt"
	"path/filepath"
	"sort"
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
	executionStartedAt   time.Time
	executionCompletedAt time.Time
	executionTickets     map[string]bool
	liveContextTokens    map[string]int
	completedTickets     map[string]bool
	now                  func() time.Time
	worktreeRoot         string
	settings             ui.Settings
	checked              map[string]bool
	checkOrder           map[string]uint64
	queueStore           *QueueStore

	width, height int
	ready         bool
	loaded        bool
	epics         []tickets.Epic
	candidates    map[string]bool

	selected int

	implementAgentMenuOpen bool
	implementAgentMenu     components.MenuState
	pendingEpics           []checkedEpicPlan
	runningEpics           map[string]bool
	runningAgent           ralphloop.AgentKind
	paused                 bool
}

func NewQueueModel(worktreeRoot string, settings ui.Settings, checked map[string]bool, orders ...map[string]uint64) QueueModel {
	if checked == nil {
		checked = map[string]bool{}
	}
	checkOrder := map[string]uint64{}
	if len(orders) > 0 && orders[0] != nil {
		checkOrder = orders[0]
	}
	return QueueModel{
		executionTickets:   map[string]bool{},
		liveContextTokens:  map[string]int{},
		completedTickets:   map[string]bool{},
		now:                time.Now,
		worktreeRoot:       worktreeRoot,
		settings:           settings,
		checked:            checked,
		checkOrder:         checkOrder,
		implementAgentMenu: newImplementAgentMenu(),
		runningEpics:       map[string]bool{},
		paused:             ralphLoopRegistry.isPaused(),
	}
}

func NewQueueModelWithStore(worktreeRoot string, settings ui.Settings, store *QueueStore) QueueModel {
	snapshot := store.Snapshot()
	m := NewQueueModel(worktreeRoot, settings, snapshot.Checked, snapshot.Order)
	m.queueStore = store
	return m
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
	if m.queueStore != nil {
		snapshot := m.queueStore.Snapshot()
		m.checked = snapshot.Checked
		m.checkOrder = snapshot.Order
	}
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
		if m.executionStartedAt.IsZero() {
			m.executionStartedAt = m.now()
		}
		m.runningEpics[msg.epicName] = true
		m.syncRunSnapshot(msg.epicName)
		return m, cmdPollImplement(msg.epicName)
	case implementPollMsg:
		m.syncRunSnapshot(msg.epicName)
		if running, epicName := ralphLoopRegistry.snapshot(msg.epicName); running && epicName == msg.epicName {
			return m, cmdPollImplement(msg.epicName)
		}
		delete(m.runningEpics, msg.epicName)
		executionComplete := !m.executionStartedAt.IsZero() && len(m.runningEpics) == 0 && len(m.pendingEpics) == 0
		startCmd := m.startAvailableEpics()
		if executionComplete {
			m.executionCompletedAt = m.now()
			return m, tea.Batch(implementFinishedNotifyCmd(msg.epicName), startCmd, m.cmdLoadQueue())
		}
		return m, tea.Batch(implementFinishedNotifyCmd(msg.epicName), startCmd)
	case implementFailedMsg:
		return m, notify.Error(msg.err.Error())
	case tea.KeyPressMsg:
		return m.handleQueueKey(msg)
	}
	return m, nil
}

func (m *QueueModel) syncRunSnapshot(epicName string) {
	snapshot, ok := ralphLoopRegistry.runSnapshot(epicName)
	if !ok {
		return
	}
	for identifier, ticket := range snapshot.Tickets {
		key := epicName + "/" + identifier
		m.liveContextTokens[key] = ticket.ContextTokens
		if ticket.Completed {
			m.completedTickets[key] = true
		}
	}
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
	case "p":
		if m.paused {
			ralphLoopRegistry.resume()
			m.paused = false
			return m, tea.Batch(notify.Success("queue resumed"), m.startAvailableEpics())
		}
		ralphLoopRegistry.pause()
		m.paused = true
		return m, notify.Info("queue paused")
	case " ", "space":
		m.toggleSelected()
	case "enter":
		if len(m.runningEpics) == 0 && len(m.pendingEpics) == 0 {
			if len(m.checkedEpicPlans()) > 0 {
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
	m.pendingEpics = m.checkedEpicPlans()
	if len(m.pendingEpics) == 0 {
		m.implementAgentMenuOpen = false
		return m, nil
	}
	m.implementAgentMenuOpen = false
	m.executionStartedAt = time.Time{}
	m.executionCompletedAt = time.Time{}
	m.executionTickets = map[string]bool{}
	for _, plan := range m.pendingEpics {
		for _, ticketID := range plan.ticketIDs {
			m.executionTickets[plan.epic.Name+"/"+ticketID] = true
		}
	}
	m.liveContextTokens = map[string]int{}
	m.completedTickets = map[string]bool{}
	m.runningAgent = agent
	return m, m.startAvailableEpics()
}

type checkedEpicPlan struct {
	epic      tickets.Epic
	ticketIDs []string
	done      int
	ordinal   uint64
	ordered   bool
}

func (m QueueModel) checkedEpicPlans() []checkedEpicPlan {
	plans := make([]checkedEpicPlan, 0, len(m.epics))
	for _, epic := range m.epics {
		var ticketIDs []string
		done := 0
		var ordinal uint64
		ordered := false
		for _, idx := range sortedTicketIndexes(epic) {
			ticket := epic.Tickets[idx]
			if !m.checked[ticket.Path] {
				continue
			}
			ticketIDs = append(ticketIDs, ticket.Identifier)
			if epic.RenderedStatus(ticket) == tickets.StatusDone {
				done++
			}
			if checkedAt, ok := m.checkOrder[ticket.Path]; ok && (!ordered || checkedAt < ordinal) {
				ordinal, ordered = checkedAt, true
			}
		}
		if len(ticketIDs) > 0 {
			plans = append(plans, checkedEpicPlan{epic: epic, ticketIDs: ticketIDs, done: done, ordinal: ordinal, ordered: ordered})
		}
	}
	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].ordered != plans[j].ordered {
			return plans[i].ordered
		}
		if plans[i].ordered && plans[i].ordinal != plans[j].ordinal {
			return plans[i].ordinal < plans[j].ordinal
		}
		return plans[i].epic.Name < plans[j].epic.Name
	})
	return plans
}

func (m *QueueModel) startAvailableEpics() tea.Cmd {
	count := min(ralphLoopRegistry.availableSlots(), len(m.pendingEpics))
	cmds := make([]tea.Cmd, 0, count)
	for _, plan := range m.pendingEpics[:count] {
		cmds = append(cmds, cmdStartImplement(
			m.worktreeRoot, plan.epic.Name, m.runningAgent, plan.done, len(plan.ticketIDs), plan.ticketIDs,
		))
	}
	m.pendingEpics = m.pendingEpics[count:]
	if len(cmds) == 1 {
		return cmds[0]
	}
	return func() tea.Msg {
		messages := make(tea.BatchMsg, 0, len(cmds))
		for _, cmd := range cmds {
			msg := cmd()
			messages = append(messages, func() tea.Msg { return msg })
		}
		return messages
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
		markUnchecked(m.checked, m.checkOrder, path)
		return
	}
	markChecked(m.checked, m.checkOrder, path)
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
	if m.queueStore != nil {
		snapshot := m.queueStore.Snapshot()
		m.checked = snapshot.Checked
		m.checkOrder = snapshot.Order
	}
	if !m.ready {
		return ui.NewMainView("\n  Initializing…")
	}
	if m.implementAgentMenuOpen {
		plans := m.checkedEpicPlans()
		return ui.NewMainView(renderImplementAgentMenu(
			fmt.Sprintf("Choose the agent for %d checked epic(s):", len(plans)),
			m.implementAgentMenu,
		))
	}
	lines := m.queueLines()
	height := max(m.height-1, 1)
	return ui.NewMainView(ui.RenderPanel(ui.PanelOptionsFor(
		m.width, height, "Queue", "", lines, true, ui.ColorBlue, nil, false,
	)))
}

func (m QueueModel) executionBanner() string {
	if !m.executionCompletedAt.IsZero() {
		done, totalTickets := m.completedExecutionProgress()
		if totalTickets > 0 && done == totalTickets {
			total, average, maximum := m.completedContextMetrics()
			elapsed := int(m.executionCompletedAt.Sub(m.executionStartedAt).Seconds())
			return fmt.Sprintf(
				"status: done, took %s, context windows: total %s, avg %s, max %s",
				formatElapsed(elapsed), formatTokenCount(total), formatTokenCount(average), formatTokenCount(maximum),
			)
		}
	}
	if m.paused {
		return "Queue paused — in-flight iterations will finish"
	}
	if len(m.runningEpics) == 0 {
		return queueBanner
	}

	done, total := m.checkedProgress()
	elapsed := int(m.now().Sub(m.executionStartedAt).Seconds())
	tokens := 0
	for _, current := range m.liveContextTokens {
		tokens += current
	}
	return fmt.Sprintf(
		"status: implementing (%d of %d done), elapsed: %s, context windows: %s",
		done, total, formatElapsed(elapsed), formatTokenCount(tokens),
	)
}

func (m QueueModel) completedContextMetrics() (total, average, maximum int) {
	count := 0
	for _, epic := range m.epics {
		for _, ticket := range epic.Tickets {
			if !m.executionTickets[epic.Name+"/"+ticket.Identifier] {
				continue
			}
			count++
			total += ticket.ActualContextWindow
			maximum = max(maximum, ticket.ActualContextWindow)
		}
	}
	if count > 0 {
		average = total / count
	}
	return total, average, maximum
}

func (m QueueModel) completedExecutionProgress() (done, total int) {
	for _, epic := range m.epics {
		for _, ticket := range epic.Tickets {
			if !m.executionTickets[epic.Name+"/"+ticket.Identifier] {
				continue
			}
			total++
			if epic.RenderedStatus(ticket) == tickets.StatusDone {
				done++
			}
		}
	}
	return done, total
}

func (m QueueModel) checkedProgress() (int, int) {
	done := 0
	total := 0
	for _, epic := range m.epics {
		for _, ticket := range epic.Tickets {
			if !m.checked[ticket.Path] {
				continue
			}
			total++
			key := epic.Name + "/" + ticket.Identifier
			if epic.RenderedStatus(ticket) == tickets.StatusDone || m.completedTickets[key] {
				done++
			}
		}
	}
	return done, total
}

func (m QueueModel) queueLines() []string {
	if !m.loaded {
		return []string{ui.StyleDim.Render("  loading…")}
	}

	banner := m.executionBanner()
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
