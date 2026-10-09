package tickets

import (
	"fmt"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

// actionResumeAnswered reopens a needs-answer ticket and demotes its
// "## Needs Answer" stub, the same write unparkAnswered performs
// automatically for a gate park (ralphloop.UnparkTicket). This menu exists
// for the park kind that pre-pass can't safely auto-clear — an
// announce-and-stop (zero-commit) park's live-but-idle pane looks identical
// to an answered gate park, so a person confirms it explicitly here instead.
const actionResumeAnswered = "resume-answered"

// actionInvestigate opens a new herdr tab and launches a fresh
// gx-investigate session for the ticket (see cmdLaunchInvestigate), for any
// status signaling a problem (see suggestedActionItems). applySuggestedAction
// never handles it — dispatch happens in
// handleActionsMenuKey/handleQueueActionsMenuKey, which have the epic name
// and ticket identifier the herdr launch needs and applySuggestedAction's
// path-only signature doesn't carry.
const actionInvestigate = "investigate"

// investigateSkill is the skill cmdLaunchInvestigate's prompt invokes.
const investigateSkill = "gx-investigate"

// actionUnmuteReopen is offered on any ticket carrying a non-empty Mutes
// field, independent of status — a mute that tripped with no parkTicket
// callback (see ralphloop.muteSource) never moves the ticket to
// needs-repair, so gating on status alone would leave that mute invisible
// and unclearable from the Tickets tab.
const actionUnmuteReopen = "unmute-reopen"

// actionAnswerInPane focuses the parked ticket's still-live iteration tab;
// answering there lets the existing auto-unpark clear the park. Handled by
// handleActionsMenuKey/handleQueueActionsMenuKey (it needs the epic name and
// ticket id), never applySuggestedAction.
const actionAnswerInPane = "answer-in-pane"

// actionAnswer opens $EDITOR at the ticket's "## Needs Answer" heading, for a
// park whose pane is gone. Dispatched like actionAnswerInPane (see cmdAnswer).
const actionAnswer = "answer"

// suggestedActionItems returns ticket's suggested-action menu items, empty
// when none apply, given its rendered status. The answer items and
// resume-answered only apply to needs-answer, paneLive picking between
// "Answer in pane" and "Answer…" (the badge in the row lists pass false: it
// only needs to know the list is non-empty, and a herdr lookup per rendered
// row would be wasteful); investigate is whitelisted to the statuses that signal an
// actual problem (needs-answer, needs-repair, error) rather than excluding
// the healthy ones — Open/Claimed/Done are healthy, but so are Blocked
// (waiting on a dependency), Draft (not yet offered to an agent) and
// WaitingForChildren (own work already done), none of which have a live or
// stalled session worth investigating. Whitelisting also fails closed for
// any status added to the enum later, instead of showing the badge by
// default. handleSuggestedActionsKey/handleQueueSuggestedActionsKey toast
// "no suggested actions" rather than opening an empty menu.
//
// herdrDown (ui.Settings.HerdrUnavailable) drops the herdr-only items:
// Investigate, whose launch opens a herdr tab, and Answer in pane.
func suggestedActionItems(status tickets.RenderedStatus, ticket tickets.Ticket, paneLive, herdrDown bool) []components.MenuItem {
	var items []components.MenuItem
	if status == tickets.StatusNeedsAnswer {
		if paneLive && !herdrDown {
			items = append(items, components.MenuItem{Label: "Answer in pane", Value: actionAnswerInPane})
		} else {
			items = append(items, components.MenuItem{Label: "Answer…", Value: actionAnswer})
		}
		items = append(items, components.MenuItem{Label: "Resume (I answered)", Value: actionResumeAnswered})
	}
	if !herdrDown && (status == tickets.StatusNeedsAnswer || status == tickets.StatusNeedsRepair || status == tickets.StatusError) {
		items = append(items, components.MenuItem{Label: "Investigate", Value: actionInvestigate})
	}
	if len(ticket.Mutes) > 0 {
		items = append(items, components.MenuItem{Label: "Unmute & Reopen", Value: actionUnmuteReopen})
	}
	return items
}

// ticketHasSuggestedActions reports whether ticket's row should carry the
// "m" suggested-actions badge (ui.IconSet.SuggestedAction).
func ticketHasSuggestedActions(status tickets.RenderedStatus, ticket tickets.Ticket, herdrDown bool) bool {
	return len(suggestedActionItems(status, ticket, false, herdrDown)) > 0
}

// applySuggestedAction performs action's write against the ticket at path.
func applySuggestedAction(path, action string, now time.Time) error {
	switch action {
	case actionResumeAnswered:
		return ralphloop.UnparkTicket(path, now)
	case actionUnmuteReopen:
		return ralphloop.UnmuteTicket(path, now)
	}
	return nil
}

// cmdLaunchInvestigate opens a new herdr tab in the epic's workspace and
// starts claude there in auto-permission mode with a /gx-investigate prompt
// for ticketID, mirroring ralphloop's TabCreate -> AgentStart -> AgentPrompt
// launch sequence (see ralphloop/iteration.go, ralphloop/launch.go) without
// ralph-loop's iteration lifecycle (worktree creation, land, finish) — this
// is a one-shot investigation session, not a tracked iteration.
func cmdLaunchInvestigate(worktreeRoot, epicName, ticketID string) tea.Cmd {
	return func() tea.Msg {
		workspaceID, err := herdr.FindOrCreateWorkspace(epicName, worktreeRoot)
		if err != nil {
			return notify.Error(fmt.Sprintf("investigate: %s", err))()
		}
		tab, err := herdr.TabCreate(herdr.TabCreateOptions{
			WorkspaceID: workspaceID,
			Cwd:         worktreeRoot,
			Label:       "investigate-" + ticketID,
			Focus:       true,
		})
		if err != nil {
			return notify.Error(fmt.Sprintf("investigate: %s", err))()
		}
		if _, err := herdr.AgentStart(herdr.AgentStartOptions{
			Name:      "investigate-" + ticketID,
			Kind:      "claude",
			Pane:      tab.RootPaneID,
			AgentArgs: []string{"--permission-mode", "auto"},
		}); err != nil {
			return notify.Error(fmt.Sprintf("investigate: %s", err))()
		}
		if _, err := herdr.AgentWait(herdr.AgentWaitOptions{
			Target: tab.RootPaneID,
			Until:  []string{"idle"},
		}); err != nil {
			return notify.Error(fmt.Sprintf("investigate: %s", err))()
		}
		prompt := fmt.Sprintf("/%s epic %s %s", investigateSkill, epicName, ticketID)
		if _, err := herdr.AgentPrompt(herdr.AgentPromptOptions{
			Target: tab.RootPaneID,
			Text:   prompt,
			Wait:   true,
			Until:  []string{"working"},
		}); err != nil {
			return notify.Error(fmt.Sprintf("investigate: %s", err))()
		}
		return notify.Info(fmt.Sprintf("investigate launched for %s", ticketID))()
	}
}
