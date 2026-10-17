package tickets

import (
	"fmt"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

// actionResume is the server's unpark, under the one "Resume" label every
// menu shares. The "m" menu offers it for the park kind the server
// can't safely auto-clear — an announce-and-stop (zero-commit) park's
// live-but-idle pane looks identical to an answered gate park, so a person
// confirms it explicitly here instead.
const (
	actionResume = serverVerbUnpark
	resumeLabel  = "Resume"
)

// actionInvestigate opens a new herdr tab and launches a fresh
// gx-investigate session for the ticket (see cmdLaunchInvestigate), for any
// status signaling a problem (see suggestedActionItems).
const actionInvestigate = "investigate"

// investigateSkill is the skill cmdLaunchInvestigate's prompt invokes.
const investigateSkill = "gx-investigate"

// actionUnmuteReopen is offered on any ticket carrying a non-empty Mutes
// field, independent of status — a mute that tripped with no parkTicket
// callback (see ralphloop.muteSource) never moves the ticket to
// needs-repair, so gating on status alone would leave that mute invisible
// and unclearable from the Tickets tab.
const actionUnmuteReopen = serverVerbUnmute

// actionAnswer opens $EDITOR at the ticket's "## Needs Answer" heading (see
// cmdAnswerThen).
const actionAnswer = "answer"

// suggestedActionItems returns ticket's suggested-action menu items, given
// its rendered status. The answer items only apply to needs-answer;
// investigate is whitelisted to the statuses that signal an
// actual problem (needs-answer, needs-repair, error) rather than excluding
// the healthy ones — Open/Claimed/Done are healthy, but so are Blocked
// (waiting on a dependency), Draft (not yet offered to an agent) and
// WaitingForChildren (own work already done), none of which have a live or
// stalled session worth investigating. Whitelisting also fails closed for
// any status added to the enum later, instead of showing the badge by
// default. herdrDown (ui.Settings.HerdrUnavailable) drops Investigate, whose
// launch opens a herdr tab.
//
// Watch and Transcript close every list; they are not status actions, so
// they never count toward the badge (whose per-row liveness lookups would be
// wasteful). Watch is listed only while the agent is live: nativeLive for a
// native runner, paneLive for a herdr pane, which herdrDown hides.
func suggestedActionItems(status tickets.RenderedStatus, ticket tickets.Ticket, paneLive, nativeLive, herdrDown bool) []components.MenuItem {
	items := statusActionItems(status, ticket, herdrDown)
	if nativeLive || (paneLive && !herdrDown) {
		items = append(items, components.MenuItem{Label: "Watch", Value: actionWatch})
	}
	return append(items, components.MenuItem{Label: "Transcript", Value: actionTranscript})
}

func statusActionItems(status tickets.RenderedStatus, ticket tickets.Ticket, herdrDown bool) []components.MenuItem {
	var items []components.MenuItem
	if answerSuggested(status) {
		items = append(items,
			components.MenuItem{Label: "Answer…", Value: actionAnswer},
			components.MenuItem{Label: resumeLabel, Value: actionResume})
	}
	if !herdrDown && investigateSuggested(status) {
		items = append(items, components.MenuItem{Label: "Investigate", Value: actionInvestigate})
	}
	if unmuteSuggested(ticket) {
		items = append(items, components.MenuItem{Label: "Unmute & Reopen", Value: actionUnmuteReopen})
	}
	return items
}

// answerSuggested, investigateSuggested and unmuteSuggested decide the
// Suggested actions for both menus and the row badge, so they cannot drift.
func answerSuggested(status tickets.RenderedStatus) bool {
	return status == tickets.StatusNeedsAnswer
}

func investigateSuggested(status tickets.RenderedStatus) bool {
	return status == tickets.StatusNeedsAnswer || status == tickets.StatusNeedsRepair || status == tickets.StatusError
}

func unmuteSuggested(ticket tickets.Ticket) bool {
	return len(ticket.Mutes) > 0
}

// ticketHasSuggestedActions reports whether ticket's row should carry the
// "m" suggested-actions badge (ui.IconSet.SuggestedAction).
func ticketHasSuggestedActions(status tickets.RenderedStatus, ticket tickets.Ticket, herdrDown bool) bool {
	return len(statusActionItems(status, ticket, herdrDown)) > 0
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
