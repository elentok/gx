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
// gx-investigate session for the ticket (see cmdLaunchInvestigate). Unlike
// actionResumeAnswered it applies to every ticket regardless of status, so
// applySuggestedAction never handles it — dispatch happens in
// handleActionsMenuKey/handleQueueActionsMenuKey, which have the epic name
// and ticket identifier the herdr launch needs and applySuggestedAction's
// path-only signature doesn't carry.
const actionInvestigate = "investigate"

// investigateSkill is the skill cmdLaunchInvestigate's prompt invokes.
const investigateSkill = "gx-investigate"

// suggestedActionItems returns status's suggested-action menu items.
// resume-answered only applies to needs-answer; investigate applies to any
// status signaling a problem — everything except open/claimed (still
// healthy, unclaimed or in-progress work) and done (finished successfully).
func suggestedActionItems(status tickets.RenderedStatus) []components.MenuItem {
	items := []components.MenuItem{}
	if status == tickets.StatusNeedsAnswer {
		items = append(items, components.MenuItem{Label: "Resume (I answered)", Value: actionResumeAnswered})
	}
	if status != tickets.StatusOpen && status != tickets.StatusClaimed && status != tickets.StatusDone {
		items = append(items, components.MenuItem{Label: "Investigate", Value: actionInvestigate})
	}
	return items
}

// ticketHasSuggestedActions reports whether status's row should carry the "m"
// suggested-actions badge (ui.IconSet.SuggestedAction).
func ticketHasSuggestedActions(status tickets.RenderedStatus) bool {
	return len(suggestedActionItems(status)) > 0
}

// applySuggestedAction performs action's write against the ticket at path.
func applySuggestedAction(path, action string, now time.Time) error {
	switch action {
	case actionResumeAnswered:
		return ralphloop.UnparkTicket(path, now)
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
