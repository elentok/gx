package tickets

import (
	"fmt"
	"slices"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

// actionResume is the server's resume.
const actionResume = serverVerbResume

// actionInvestigate opens a new herdr tab and launches a fresh
// gx-investigate session for the ticket (see cmdLaunchInvestigate), for any
// status signaling a problem (see statusActionItems).
const actionInvestigate = "investigate"

// investigateSkill is the skill cmdLaunchInvestigate's prompt invokes.
const investigateSkill = "gx-investigate"

// actionUnmuteReopen is offered on any ticket carrying a non-empty Mutes
// field, independent of status — a mute that tripped with no pausing-a-ticket
// callback (see ralphloop.muteSource) never moves the ticket to
// paused (repair), so gating on status alone would leave that mute invisible
// and unclearable from the Tickets tab.
const actionUnmuteReopen = serverVerbUnmute

// resumeSuggested, investigateSuggested and unmuteSuggested decide which
// ticket Context menu items move to Suggested.
func resumeSuggested(status tickets.RenderedStatus) bool {
	return status == tickets.StatusPausedAnswer
}

// investigateSuggested whitelists the statuses that signal an actual problem
// rather than excluding the healthy ones: Blocked, Draft and
// WaitingForChildren are healthy too, with no live or stalled session worth
// investigating, and a status added to the enum later fails closed.
func investigateSuggested(status tickets.RenderedStatus) bool {
	return status == tickets.StatusPausedAnswer || status == tickets.StatusPausedRepair || status == tickets.StatusError
}

func unmuteSuggested(ticket tickets.Ticket) bool {
	return len(ticket.Mutes) > 0
}

// ticketHasSuggestedActions reports whether a row carries the "m" badge
// (ui.IconSet.SuggestedAction): its Context menu has an enabled Suggested
// item.
func ticketHasSuggestedActions(in ticketMenuInput) bool {
	// The row has no detail yet, so an answer pause's recovery items (suggested
	// once the menu opens) are counted here.
	if resumeSuggested(in.rendered) && in.serverReason == "" && !in.archived {
		return true
	}
	return slices.ContainsFunc(ticketMenuItems(in), func(it contextmenu.Item[ticketTarget]) bool {
		return it.Group == contextmenu.Suggested && it.Disabled == ""
	})
}

// cmdLaunchInvestigate opens a new herdr tab in the epic's workspace and
// starts claude there in auto-permission mode with a /gx-investigate prompt
// for ticketID, mirroring ralphloop's TabCreate -> AgentStart -> AgentPrompt
// launch sequence (see ralphloop/iteration.go, ralphloop/launch.go) without
// ralph-loop's iteration lifecycle (worktree creation, merge, finish) — this
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
