package ralphloop

import (
	"fmt"
	"io"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// RunOptions configures a single `gx ralph-loop {epic-name}` invocation.
type RunOptions struct {
	EpicName   string
	Skill      string // slash-command skill each iteration invokes, e.g. "implement"
	ScratchDir string // defaults to ".scratch"
	RepoDir    string // repo root passed as the herdr workspace/worktree cwd
}

// Run drives every unblocked ticket in the named epic to completion, one
// iteration worktree at a time: create the iteration worktree, launch
// claude, send the initial "/{skill} <ticket-path>" prompt, wait for it to
// finish, cherry-pick its commits onto the feature branch, mark the ticket
// done, and remove the iteration worktree. It exits once every ticket in the
// epic reaches a done-family status, or immediately if the epic has none to
// run.
func Run(opts RunOptions, d Deps, out io.Writer) error {
	scratchDir := opts.ScratchDir
	if scratchDir == "" {
		scratchDir = ".scratch"
	}

	initial, err := loadNamedEpic(scratchDir, opts.EpicName)
	if err != nil {
		return err
	}
	if initial == nil || len(initial.Tickets) == 0 {
		fmt.Fprintf(out, "no tickets found for epic %q; nothing to do\n", opts.EpicName)
		return nil
	}
	if initial.AllDone() {
		fmt.Fprintf(out, "epic %q is already complete (%d/%d done)\n", opts.EpicName, initial.DoneCount(), initial.TotalCount())
		return nil
	}

	workspaceID, err := d.FindOrCreateWorkspace(opts.EpicName, opts.RepoDir)
	if err != nil {
		return fmt.Errorf("finding/creating herdr workspace %q: %w", opts.EpicName, err)
	}

	featureWT, err := d.WorktreeCreate(herdr.WorktreeCreateOptions{
		WorkspaceID: workspaceID,
		Cwd:         opts.RepoDir,
		Branch:      opts.EpicName,
		Label:       opts.EpicName,
		Focus:       true,
	})
	if err != nil {
		return fmt.Errorf("creating feature worktree for branch %q: %w", opts.EpicName, err)
	}

	completed := 0
	for {
		epic, err := loadNamedEpic(scratchDir, opts.EpicName)
		if err != nil {
			return err
		}
		if epic.AllDone() {
			break
		}

		frontier := Frontier(*epic)
		if len(frontier) == 0 {
			return fmt.Errorf("epic %q has no unblocked tickets left but isn't all done; check for a stuck ticket", opts.EpicName)
		}
		ticket := frontier[0]

		if err := Claim(ticket.Path); err != nil {
			return fmt.Errorf("claiming ticket %d: %w", ticket.Number, err)
		}

		if err := runIteration(d, iterationParams{
			WorkspaceID:     workspaceID,
			RepoDir:         opts.RepoDir,
			FeatureWorktree: featureWT.Path,
			FeatureBranch:   opts.EpicName,
			Skill:           opts.Skill,
			Ticket:          ticket,
		}); err != nil {
			return fmt.Errorf("ticket %02d: %w", ticket.Number, err)
		}

		fmt.Fprintf(out, "ticket %02d %q landed on %s\n", ticket.Number, ticket.Title, opts.EpicName)
		completed++
	}

	fmt.Fprintf(out, "ralph-loop %q complete: %d ticket(s) landed on %s\n", opts.EpicName, completed, opts.EpicName)
	return nil
}

// iterationParams are the per-ticket inputs to runIteration.
type iterationParams struct {
	WorkspaceID     string
	RepoDir         string
	FeatureWorktree string
	FeatureBranch   string
	Skill           string
	Ticket          tickets.Ticket
}

// runIteration drives one ticket through the full iteration lifecycle:
// create its worktree, launch and prompt the agent, wait for it to finish,
// cherry-pick its commits onto the feature branch, mark the ticket done, and
// remove the iteration worktree.
func runIteration(d Deps, p iterationParams) error {
	label := iterLabel(p.Ticket.Number)
	branch := iterBranch(p.Ticket.Number)

	base, err := d.RevParse(p.FeatureWorktree, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s tip: %w", p.FeatureBranch, err)
	}

	iterWT, err := d.WorktreeCreate(herdr.WorktreeCreateOptions{
		WorkspaceID: p.WorkspaceID,
		Cwd:         p.RepoDir,
		Branch:      branch,
		Base:        p.FeatureBranch,
		Label:       label,
	})
	if err != nil {
		return fmt.Errorf("creating iteration worktree: %w", err)
	}

	if _, err := d.AgentStart(herdr.AgentStartOptions{
		Name: label,
		Kind: "claude",
		Pane: iterWT.PaneID,
	}); err != nil {
		return fmt.Errorf("launching claude: %w", err)
	}

	if _, err := d.AgentWait(herdr.AgentWaitOptions{
		Target: iterWT.PaneID,
		Until:  []string{"idle"},
	}); err != nil {
		return fmt.Errorf("waiting for claude to reach idle after launch: %w", err)
	}

	prompt := fmt.Sprintf("/%s %s", p.Skill, p.Ticket.Path)
	if _, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target: iterWT.PaneID,
		Text:   prompt,
		Wait:   true,
		Until:  []string{"working"},
	}); err != nil {
		return fmt.Errorf("sending initial prompt: %w", err)
	}

	if _, err := d.AgentWait(herdr.AgentWaitOptions{
		Target: iterWT.PaneID,
		Until:  []string{"idle", "done"},
	}); err != nil {
		return fmt.Errorf("waiting for agent to finish: %w", err)
	}

	if err := d.CherryPickRange(p.FeatureWorktree, base, branch); err != nil {
		return fmt.Errorf("cherry-picking onto %s: %w", p.FeatureBranch, err)
	}

	if err := MarkDone(p.Ticket.Path); err != nil {
		return fmt.Errorf("marking ticket done: %w", err)
	}

	if err := d.WorktreeRemove(iterWT.WorkspaceID, true); err != nil {
		return fmt.Errorf("removing iteration worktree: %w", err)
	}

	return nil
}

func iterLabel(ticketNumber int) string {
	return fmt.Sprintf("iter-%02d", ticketNumber)
}

func iterBranch(ticketNumber int) string {
	return "ralph-loop/" + iterLabel(ticketNumber)
}

// loadNamedEpic loads scratchDir and returns the epic named name, or nil if
// no such epic exists.
func loadNamedEpic(scratchDir, name string) (*tickets.Epic, error) {
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		return nil, err
	}
	for i := range epics {
		if epics[i].Name == name {
			return &epics[i], nil
		}
	}
	return nil, nil
}
