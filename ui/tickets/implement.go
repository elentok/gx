package tickets

import (
	"fmt"
	"sort"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

// implementPollInterval is how often a tickets.Model polls ralphLoopRegistry
// for completion. Polling (rather than a done-channel the launching Model
// alone waits on) is what lets *any* tickets.Model instance — including one
// rebuilt by a tab switch away and back, see OnPageActivated — pick up an
// in-flight run's state: the app shell only routes tea.Msgs to the active
// page, so a message sent to a backgrounded page's model is silently
// dropped, and the done-channel-blocking Cmd would rebind to a fresh model
// value each time OnPageActivated fires without an independent way to check
// "is it still running" first.
const implementPollInterval = 300 * time.Millisecond

// implementStartedMsg reports that a ralph-loop launch was accepted and its
// background goroutine is now running.
type implementStartedMsg struct {
	epicName string
}

// implementPollMsg drives the poll loop started by implementStartedMsg/
// OnPageActivated: on each tick it re-checks ralphLoopRegistry for epicName.
type implementPollMsg struct {
	epicName string
}

// implementSyncMsg reports every epic ralphLoopRegistry has running, as
// observed when this tab (re)gained focus (see OnPageActivated), so a Model
// that missed completion messages for one or more epics while another tab was
// active can catch up on all of them at once.
type implementSyncMsg struct {
	runningEpics []string
}

// implementFailedMsg reports that a launch never made it to a background
// goroutine at all (already running, or couldn't resolve the repo).
type implementFailedMsg struct {
	err error
}

func newImplementAgentMenu() components.MenuState {
	return components.MenuState{
		Items: []components.MenuItem{
			{Label: "l  Claude", Value: string(ralphloop.AgentClaude)},
			{Label: "o  Codex", Value: string(ralphloop.AgentCodex)},
		},
		Cursor: 0,
	}
}

// handleReplaceQueueKey applies bugs-05/03's "r" ("Replace queue") action:
// "r" is blocked only when the epic under the cursor itself has a live run —
// a live run on some other epic no longer stops it, mirroring "a"'s
// per-epic scoping (handleAddToQueueKey) instead of the old process-wide
// IsLoopRunning() check. With no epic under the cursor there's nothing to
// scope the guard to, so the check is simply skipped. Once past the guard,
// "r" opens the same confirmation step "a" already goes through
// (openImplementConfirm) before touching the queue, naming what's about to
// happen; accepting it runs replaceQueuedSelection via
// handleReplaceQueueConfirmed and switches to the Queue tab.
func (m Model) handleReplaceQueueKey() (tea.Model, tea.Cmd) {
	if r, ok := m.selectedRow(); ok {
		epic := m.epics[r.epicIdx]
		if ralphLoopRegistry.isRunningEpic(epic.Name) {
			return m, notify.Info("Can't replace a live queue")
		}
	}
	if len(m.checked) == 0 {
		return m, notify.Info("check at least one ticket to build an execution plan")
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    "Replace the queue with the checked selection?",
		AcceptCmd: cmdConfirmReplaceQueue(m.worktreeRoot),
	})
	return m, nil
}

// replaceQueueConfirmedMsg carries "r"'s confirmation acceptance: worktreeRoot
// is captured when the modal opened (mirroring checkAddConfirmedMsg's same
// capture-at-open-time approach in checked.go) since the actual queue
// mutation must run against the live Model, not the value m.confirm.Open
// closed over.
type replaceQueueConfirmedMsg struct {
	worktreeRoot string
}

func cmdConfirmReplaceQueue(worktreeRoot string) tea.Cmd {
	return func() tea.Msg {
		return replaceQueueConfirmedMsg{worktreeRoot: worktreeRoot}
	}
}

// handleReplaceQueueConfirmed applies replaceQueueConfirmedMsg: the queue's
// not-yet-started entries are replaced with the checked selection, then the
// app switches to the Queue tab.
func (m Model) handleReplaceQueueConfirmed(msg replaceQueueConfirmedMsg) (tea.Model, tea.Cmd) {
	if err := m.replaceQueuedSelection(); err != nil {
		return m, notify.Error("save queue: " + err.Error())
	}
	return m, cmdOpenQueueTab(msg.worktreeRoot)
}

// handleAddToQueueKey applies ticket 10's "a" ("Add to queue") action: the
// epic under the cursor must already have a live run, and the checked
// tickets belonging to it are added to that run's scope after confirmation —
// widening a frozen scope via ralphloop.RunScope.Add (ticket 09) so each
// becomes claimable on the run's next iteration. Unlike "r", "a" always
// confirms first, naming the count about to be added.
func (m Model) handleAddToQueueKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	epic := m.epics[r.epicIdx]
	if !ralphLoopRegistry.isRunningEpic(epic.Name) {
		return m, notify.Info(fmt.Sprintf("epic %q isn't running", epic.Name))
	}
	ticketIDs := checkedTicketIDsForEpic(epic, m.checked)
	if len(ticketIDs) == 0 {
		return m, notify.Info("check at least one ticket in the epic to add")
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    fmt.Sprintf("Add %d ticket(s) to the live queue?", len(ticketIDs)),
		AcceptCmd: cmdAddToLiveQueue(epic.Name, ticketIDs),
	})
	return m, nil
}

// checkedTicketIDsForEpic collects DisplayNumber identifiers (the form
// ralphloop.RunScope.Add expects) for epic's checked, not-yet-done tickets —
// a done ticket has nothing left to add to a live run's scope.
func checkedTicketIDsForEpic(epic tickets.Epic, checked map[string]bool) []string {
	var ids []string
	for _, t := range epic.Tickets {
		if !checked[t.Path] || epic.RenderedStatus(t) == tickets.StatusDone {
			continue
		}
		ids = append(ids, t.DisplayNumber())
	}
	return ids
}

// cmdAddToLiveQueue widens epicName's live RunScope to include ticketIDs
// once the "a" confirmation modal is accepted.
func cmdAddToLiveQueue(epicName string, ticketIDs []string) tea.Cmd {
	return func() tea.Msg {
		scope, ok := ralphLoopRegistry.scopeFor(epicName)
		if !ok {
			return notify.Error(fmt.Sprintf("epic %q is no longer running", epicName))()
		}
		scope.Add(ticketIDs...)
		return notify.Info(fmt.Sprintf("added %d ticket(s) to epic %q", len(ticketIDs), epicName))()
	}
}

// replaceQueuedSelection applies ticket 10's "r" replace logic, per bugs-06/03's
// fix to the domain glossary's "Replace queue" entry: every pending
// (not-yet-started) and done (already-finished) queue entry is dropped and
// replaced by the current checked selection. Running/errored entries are left
// exactly as they are, whether or not they're still checked, since a live
// run's own state isn't something Replace should silently discard. Ticket
// 15's EnqueueAndClearChecked also clears every just-queued path from the
// Tickets tab's independent checked set in the same atomic write, so the
// checkboxes visually reset the moment their tickets are queued.
func (m *Model) replaceQueuedSelection() error {
	snapshot := m.queueStore.Snapshot()
	next := make(map[string]queueItemStatus, len(snapshot.Status))
	order := make(map[string]uint64, len(snapshot.Order))
	for path, status := range snapshot.Status {
		if status == queueStatusPending || status == queueStatusDone {
			continue
		}
		next[path] = status
		order[path] = snapshot.Order[path]
	}
	clearedPaths := make([]string, 0, len(m.checked))
	for path := range m.checked {
		clearedPaths = append(clearedPaths, path)
		if m.isTicketDone(path) {
			continue
		}
		if _, exists := next[path]; exists {
			continue
		}
		next[path] = queueStatusPending
		order[path] = m.checkOrder[path]
	}
	if err := m.queueStore.EnqueueAndClearChecked(next, order, clearedPaths); err != nil {
		return err
	}
	m.refreshQueueSnapshot()
	return nil
}

// isTicketDone reports whether path's ticket is already tickets.StatusDone
// within m.epics — a done ticket has nothing left to implement, so
// replaceQueuedSelection excludes it from the checked selection it enqueues.
func (m *Model) isTicketDone(path string) bool {
	for _, epic := range m.epics {
		for _, t := range epic.Tickets {
			if t.Path == path {
				return epic.RenderedStatus(t) == tickets.StatusDone
			}
		}
	}
	return false
}

func cmdOpenQueueTab(worktreeRoot string) tea.Cmd {
	return nav.Switch(nav.ViewState{Tab: nav.TabQueue, WorktreeRoot: worktreeRoot})
}

func (m Model) handleImplementAgentMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "l":
		return m.openImplementConfirm(ralphloop.AgentClaude)
	case "o":
		return m.openImplementConfirm(ralphloop.AgentCodex)
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
	return m.openImplementConfirm(agent)
}

func (m Model) openImplementConfirm(agent ralphloop.AgentKind) (tea.Model, tea.Cmd) {
	m.implementAgentMenuOpen = false
	r, ok := m.selectedRow()
	if !ok || !r.isEpic() {
		return m, nil
	}
	epic := m.epics[r.epicIdx]
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    fmt.Sprintf("Start implementing epic %q with %s?", epic.Name, agentDisplayName(agent)),
		AcceptCmd: m.cmdStartImplement(epic.Name, agent, epic.DoneCount(), epic.TotalCount()),
	})
	return m, nil
}

func (m Model) implementAgentMenuView() string {
	prompt := "Choose the agent for this ralph-loop:"
	if r, ok := m.selectedRow(); ok && r.isEpic() {
		prompt = fmt.Sprintf("Choose the agent for epic %q:", m.epics[r.epicIdx].Name)
	}
	return renderImplementAgentMenu(prompt, m.implementAgentMenu)
}

func renderImplementAgentMenu(prompt string, menu components.MenuState) string {
	return components.RenderMenuModal(
		"Implement Epic",
		prompt,
		menu,
		"",
		ui.ColorBorder,
		ui.ColorBlue,
		ui.ColorSubtle,
		ui.ColorText,
		48,
	)
}

func agentDisplayName(agent ralphloop.AgentKind) string {
	if agent == ralphloop.AgentCodex {
		return "Codex"
	}
	return "Claude"
}

func (m Model) handleConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.Update(msg)
	m.confirm = next
	return m, cmd
}

func (m Model) handleConfirmMouseUpdate(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.UpdateMouse(msg, m.width, m.width, m.height)
	m.confirm = next
	return m, cmd
}

func (m Model) handleImplementStarted(msg implementStartedMsg) (tea.Model, tea.Cmd) {
	m.implementEpic = msg.epicName
	closeCmd := m.syncRunSnapshot(msg.epicName)
	return m, tea.Batch(m.implementSpinner.Tick, cmdPollImplement(msg.epicName), closeCmd)
}

// handleImplementPoll projects active registry state and reloads disk state
// after completion. Errors remain in the registry for every observer.
func (m Model) handleImplementPoll(msg implementPollMsg) (tea.Model, tea.Cmd) {
	if ralphLoopRegistry.isRunningEpic(msg.epicName) {
		closeCmd := m.syncRunSnapshot(msg.epicName)
		return m, tea.Batch(cmdPollImplement(msg.epicName), closeCmd)
	}
	m.clearLiveTrackingFor(msg.epicName)
	return m, tea.Batch(implementFinishedNotifyCmd(msg.epicName), m.cmdLoad())
}

// implementFinishedNotifyCmd reports epicName's just-finished run: an error
// toast if ralphloop.Run returned one, otherwise the plain completion toast.
func implementFinishedNotifyCmd(epicName string) tea.Cmd {
	if err := ralphLoopRegistry.lastError(epicName); err != nil {
		return notify.Error(fmt.Sprintf("ralph-loop failed for epic %q: %v", epicName, err))
	}
	return notify.Info(fmt.Sprintf("ralph-loop finished for epic %q", epicName))
}

// handleImplementSync answers OnPageActivated's resync Cmd: it reconciles
// every epic this Model was tracking against ralphLoopRegistry's live state,
// which may have changed while this tab was in the background (a plain
// tea.Msg sent to a backgrounded page is dropped by the app shell, so the
// model that launched a run can't rely on ever seeing its own completion
// message if the user switched away in the meantime) — and it also starts
// tracking any epic that's running but that this Model instance never saw
// start, e.g. one launched before this Model was rebuilt by a tab switch.
func (m Model) handleImplementSync(msg implementSyncMsg) (tea.Model, tea.Cmd) {
	running := make(map[string]bool, len(msg.runningEpics))
	var cmds []tea.Cmd
	for _, epicName := range msg.runningEpics {
		running[epicName] = true
		cmds = append(cmds, m.syncRunSnapshot(epicName), cmdPollImplement(epicName))
	}
	if len(msg.runningEpics) > 0 {
		cmds = append(cmds, m.implementSpinner.Tick)
	}

	finished := make([]string, 0, len(m.implementingEpics))
	for epicName := range m.implementingEpics {
		if !running[epicName] {
			finished = append(finished, epicName)
		}
	}
	sort.Strings(finished)
	for _, epicName := range finished {
		m.clearLiveTrackingFor(epicName)
		cmds = append(cmds, implementFinishedNotifyCmd(epicName))
	}
	if len(finished) > 0 {
		cmds = append(cmds, m.cmdLoad())
	}

	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleImplementSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if len(m.implementingEpics) == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.implementSpinner, cmd = m.implementSpinner.Update(msg)
	return m, cmd
}

// OnPageActivated implements the app shell's pageActivationAware duck-type
// (see ui/app/model_tabs.go), firing every time this tab (re)gains focus —
// including the very first time. It fires an implementSyncMsg listing every
// epic ralphLoopRegistry currently has running, so this Model can recover
// every running epic's live state deterministically — including a completion
// it missed, or an epic it never even saw start — without opening an event
// reader of its own or depending on messages that arrived while another tab
// was active.
func (m Model) OnPageActivated() tea.Cmd {
	syncCmd := func() tea.Msg {
		return implementSyncMsg{runningEpics: ralphLoopRegistry.runningEpicNames()}
	}
	return tea.Batch(syncCmd, m.cmdReattachScan())
}

// cmdStartImplement launches the producer while the registry owns the sole
// event consumer and publishes durable snapshots to presentation models.
func (m Model) cmdStartImplement(epicName string, agent ralphloop.AgentKind, done, total int) tea.Cmd {
	return cmdStartImplement(
		m.worktreeRoot, epicName, agent, done, total,
		m.settings.MaxConcurrentTicketsPerEpic(), nil, m.settings.Notifications, m.settings.ImplementSkill(),
	)
}

func cmdStartImplement(
	worktreeRoot string,
	epicName string,
	agent ralphloop.AgentKind,
	done, total int,
	maxParallel int,
	ticketIDs []string,
	notifications config.NotificationsConfig,
	skill string,
) tea.Cmd {
	return func() tea.Msg {
		sink, ok := ralphLoopRegistry.tryStart(epicName, done, total, scratchDirFor(worktreeRoot))
		if !ok {
			if attachErr := ralphLoopRegistry.takeAttachError(); attachErr != nil {
				return implementFailedMsg{err: attachErr}
			}
			return implementFailedMsg{err: fmt.Errorf("a ralph-loop is already running")}
		}
		scratchDir := scratchDirFor(worktreeRoot)
		if notifications.Telegram.BotToken != "" || notifications.Slack.WebhookURL != "" {
			reporter := ralphloop.NewEpicFailureReporter(scratchDir)
			if notifications.Telegram.BotToken != "" {
				reporter.AddTelegram(notifications.Telegram.BotToken, notifications.Telegram.ChatID)
			}
			if notifications.Slack.WebhookURL != "" {
				reporter.AddSlack(notifications.Slack.WebhookURL)
			}
			ralphLoopRegistry.setFailureNotifier(epicName, reporter)
		}
		opts, err := buildImplementRunOptionsForTickets(worktreeRoot, epicName, agent, maxParallel, ticketIDs, skill)
		if err != nil {
			ralphLoopRegistry.finish(epicName, err)
			return implementFailedMsg{err: err}
		}
		opts.Gate = ralphLoopRegistry.gateFor(epicName)
		opts.Permit = ralphLoopRegistry.permitFor(epicName)
		opts.OnScopeResolved = func(scope ralphloop.RunScope) {
			ralphLoopRegistry.setScope(epicName, scope)
		}
		_ = ralphloop.LogNotificationsConfigured(
			opts.ScratchDir, epicName,
			notifications.Telegram.BotToken != "", notifications.Slack.WebhookURL != "",
		)
		var runSink ralphloop.EventSink = sink
		if notifications.Telegram.BotToken != "" {
			runSink = ralphloop.NewTelegramEventSink(runSink, notifications.Telegram.BotToken, notifications.Telegram.ChatID, opts.ScratchDir, epicName)
		}
		if notifications.Slack.WebhookURL != "" {
			runSink = ralphloop.NewSlackEventSink(runSink, notifications.Slack.WebhookURL, opts.ScratchDir, epicName)
		}
		go func() {
			err := runRalphLoop(opts, ralphloop.DefaultDeps(), runSink)
			ralphLoopRegistry.finish(epicName, err)
		}()
		return implementStartedMsg{epicName: epicName}
	}
}

// cmdPollImplement re-checks ralphLoopRegistry after implementPollInterval;
// see handleImplementPoll.
func cmdPollImplement(epicName string) tea.Cmd {
	return tea.Tick(implementPollInterval, func(time.Time) tea.Msg {
		return implementPollMsg{epicName: epicName}
	})
}

func buildImplementRunOptions(worktreeRoot, epicName string, agent ralphloop.AgentKind) (ralphloop.RunOptions, error) {
	settings := ui.Settings{}
	return buildImplementRunOptionsForTickets(
		worktreeRoot, epicName, agent,
		settings.MaxConcurrentTicketsPerEpic(), nil, settings.ImplementSkill(),
	)
}

func buildImplementRunOptionsForTickets(
	worktreeRoot, epicName string,
	agent ralphloop.AgentKind,
	maxParallel int,
	ticketIDs []string,
	skill string,
) (ralphloop.RunOptions, error) {
	repo, err := git.FindRepo(worktreeRoot)
	if err != nil {
		return ralphloop.RunOptions{}, err
	}
	return ralphloop.RunOptions{
		EpicName:    epicName,
		Agent:       agent,
		Skill:       skill,
		RepoDir:     repo.Root,
		ScratchDir:  repo.ScratchRoot(),
		MaxParallel: max(maxParallel, 1),
		TicketIDs:   ticketIDs,
	}, nil
}
