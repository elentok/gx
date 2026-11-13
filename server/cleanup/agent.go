package cleanup

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/tickets"
)

func (c *Cleanup) agentWrite(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(w, r, func(req ops.CleanupWriteRequest) (wire.OneOffResult, error) {
		return c.StartAgent(req.Project)
	})
}

// StartAgent starts a one-off /gx-cleanup naming only the agent group: the
// branches and epics no button can decide. A repeat press while one is live
// is refused by the one-off's Unique key.
func (c *Cleanup) StartAgent(project string) (wire.OneOffResult, error) {
	rep, err := c.Report(project)
	if err != nil {
		return wire.OneOffResult{}, err
	}
	var branches, epics []string
	for _, wt := range rep.Worktrees {
		if wt.Action == ops.CleanupActionAgent {
			branches = append(branches, wt.Branch)
		}
	}
	for _, e := range rep.Epics {
		if e.Action == ops.CleanupActionAgent {
			epics = append(epics, e.Name)
		}
	}
	if len(branches) == 0 && len(epics) == 0 {
		return wire.OneOffResult{Refused: true, Reason: ops.ReasonCleanupNothingForAgent, Message: "no branch or epic needs the cleanup agent"}, nil
	}
	return c.d.SubmitOneOff(wire.OneOffRequest{
		Prompt:  agentPrompt(project, branches, epics),
		Project: project,
		Name:    tickets.Slug("cleanup-agent"),
		Type:    "prompt",
		Unique:  "cleanup-agent:" + project,
		Notify:  true,
	})
}

func agentPrompt(project string, branches, epics []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Run /gx-cleanup for project %s, and decide only the items listed here. Leave every other branch and epic alone: the web page's buttons handle them.\n", project)
	writeList(&b, "Unresolved branches (idle, not cleanly merged):", branches)
	writeList(&b, "Done epics that have not landed on trunk:", epics)
	b.WriteString("\nWhere the skill would stop at its single confirm, ask the person with `gx tickets ask` instead.\n")
	return b.String()
}

func writeList(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s\n", title)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
}
