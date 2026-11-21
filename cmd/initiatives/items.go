package initiatives

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/initiative"
)

// itemEdit changes one line of body and returns the line it stored. view is
// derived from the same body the edit sees.
type itemEdit func(body *initiative.Body, view initiative.View) (string, error)

func newDeferred(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "deferred", Short: "add and change deferred items"}
	cmd.AddCommand(newDeferredAdd(d), newDeferredSet(d))
	return cmd
}

func newAssumption(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "assumption", Short: "add and change Assumptions"}
	cmd.AddCommand(newAssumptionAdd(d), newAssumptionSet(d))
	return cmd
}

// itemCmd wires a verb: it resolves the cwd, then runs build on the parsed
// args and flags to get the edit.
func itemCmd(d Deps, use, short string, nargs int, setup func(*cobra.Command), build func(c *cobra.Command, args []string) (itemEdit, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(nargs),
		RunE: func(c *cobra.Command, args []string) error {
			edit, err := build(c, args)
			if err != nil {
				return err
			}
			cwd, err := d.Cwd()
			if err != nil {
				return err
			}
			return editInitiative(d.Dirs(), cwd, args[0], c.OutOrStdout(), edit)
		},
	}
	setup(cmd)
	return cmd
}

func newDeferredAdd(d Deps) *cobra.Command {
	return itemCmd(d, "add <project:initiative> <item>", "add an open deferred item", 2,
		func(c *cobra.Command) {
			c.Flags().String("to", "", "Milestone slug, or unplaced (required)")
			c.Flags().String("why", "", "why it is deferred (required)")
			c.Flags().String("from", "", "the Design it came from (required)")
			for _, f := range []string{"to", "why", "from"} {
				_ = c.MarkFlagRequired(f)
			}
		},
		func(c *cobra.Command, args []string) (itemEdit, error) {
			to, _ := c.Flags().GetString("to")
			why, _ := c.Flags().GetString("why")
			from, _ := c.Flags().GetString("from")
			return func(b *initiative.Body, v initiative.View) (string, error) {
				if err := checkPlacement(v, to); err != nil {
					return "", err
				}
				for _, e := range b.Deferred {
					if !e.Unparsed() && e.Value.Item == args[1] {
						return "", fmt.Errorf("deferred item %q already exists", args[1])
					}
				}
				item := initiative.DeferredItem{Item: args[1], To: to, Why: why, From: from, State: initiative.DeferredOpen}
				line, err := initiative.RenderDeferred(item)
				if err != nil {
					return "", err
				}
				b.Deferred = append(b.Deferred, initiative.Parsed(item))
				return line, nil
			}, nil
		})
}

func newDeferredSet(d Deps) *cobra.Command {
	return itemCmd(d, "set <project:initiative> <item>", "change a deferred item's state or placement", 2,
		func(c *cobra.Command) {
			c.Flags().String("state", "", "open, taken or dropped")
			c.Flags().String("to", "", "Milestone slug, or unplaced")
			c.Flags().String("why", "", "new reason")
		},
		func(c *cobra.Command, args []string) (itemEdit, error) {
			if err := requireChange(c, "state", "to", "why"); err != nil {
				return nil, err
			}
			return func(b *initiative.Body, v initiative.View) (string, error) {
				idx := -1
				for i, e := range b.Deferred {
					if e.Unparsed() || e.Value.Item != args[1] {
						continue
					}
					if idx >= 0 {
						return "", fmt.Errorf("deferred item %q is ambiguous: it appears more than once", args[1])
					}
					idx = i
				}
				if idx < 0 {
					return "", fmt.Errorf("no deferred item %q", args[1])
				}
				item := b.Deferred[idx].Value
				if c.Flags().Changed("state") {
					item.State, _ = c.Flags().GetString("state")
				}
				if c.Flags().Changed("to") {
					item.To, _ = c.Flags().GetString("to")
					if err := checkPlacement(v, item.To); err != nil {
						return "", err
					}
				}
				if c.Flags().Changed("why") {
					item.Why, _ = c.Flags().GetString("why")
				}
				line, err := initiative.RenderDeferred(item)
				if err != nil {
					return "", err
				}
				b.Deferred[idx] = initiative.Parsed(item)
				return line, nil
			}, nil
		})
}

func newAssumptionAdd(d Deps) *cobra.Command {
	return itemCmd(d, "add <project:initiative> <text>", "add an unknown Assumption", 2,
		func(c *cobra.Command) {
			c.Flags().String("tested-by", "", "slug of the Milestone that tests it (required)")
			_ = c.MarkFlagRequired("tested-by")
		},
		func(c *cobra.Command, args []string) (itemEdit, error) {
			testedBy, _ := c.Flags().GetString("tested-by")
			return func(b *initiative.Body, v initiative.View) (string, error) {
				if err := checkMilestone(v, testedBy); err != nil {
					return "", err
				}
				a := initiative.NewAssumption(initiative.NextAssumptionNumber(b.Assumptions), args[1], testedBy)
				line, err := initiative.RenderAssumption(a)
				if err != nil {
					return "", err
				}
				b.Assumptions = append(b.Assumptions, initiative.Parsed(a))
				return line, nil
			}, nil
		})
}

func newAssumptionSet(d Deps) *cobra.Command {
	return itemCmd(d, "set <project:initiative> <A<n>>", "change an Assumption's state, evidence or Milestone", 2,
		func(c *cobra.Command) {
			c.Flags().String("state", "", "unknown, confirmed or broken")
			c.Flags().String("evidence", "", "evidence text")
			c.Flags().String("tested-by", "", "move to another Milestone (only at a Gate)")
		},
		func(c *cobra.Command, args []string) (itemEdit, error) {
			if err := requireChange(c, "state", "evidence", "tested-by"); err != nil {
				return nil, err
			}
			return func(b *initiative.Body, v initiative.View) (string, error) {
				idx := -1
				for i, e := range b.Assumptions {
					if !e.Unparsed() && fmt.Sprintf("A%d", e.Value.N) == args[1] {
						idx = i
						break
					}
				}
				if idx < 0 {
					return "", fmt.Errorf("no Assumption %q", args[1])
				}
				a := b.Assumptions[idx].Value
				if c.Flags().Changed("state") {
					a.State, _ = c.Flags().GetString("state")
				}
				if c.Flags().Changed("evidence") {
					a.Evidence, _ = c.Flags().GetString("evidence")
				}
				if c.Flags().Changed("tested-by") {
					a.TestedBy, _ = c.Flags().GetString("tested-by")
					if err := checkMilestone(v, a.TestedBy); err != nil {
						return "", err
					}
					if err := checkAtGate(v); err != nil {
						return "", err
					}
				}
				line, err := initiative.RenderAssumption(a)
				if err != nil {
					return "", err
				}
				b.Assumptions[idx] = initiative.Parsed(a)
				return line, nil
			}, nil
		})
}

func requireChange(c *cobra.Command, flags ...string) error {
	for _, f := range flags {
		if c.Flags().Changed(f) {
			return nil
		}
	}
	return fmt.Errorf("nothing to change: pass one of --%s", strings.Join(flags, ", --"))
}

func checkMilestone(v initiative.View, slug string) error {
	if slices.ContainsFunc(v.Milestones, func(m initiative.MilestoneView) bool { return m.Slug == slug }) {
		return nil
	}
	return fmt.Errorf("unknown Milestone %q", slug)
}

// checkPlacement accepts a Milestone slug or unplaced.
func checkPlacement(v initiative.View, to string) error {
	if to == initiative.Unplaced {
		return nil
	}
	return checkMilestone(v, to)
}

// checkAtGate refuses unless the latest Milestone with a Design is merged and
// has no gate note: the moment a Gate happens.
func checkAtGate(v initiative.View) error {
	var latest *initiative.MilestoneView
	for i := range v.Milestones {
		if v.Milestones[i].Design != "" {
			latest = &v.Milestones[i]
		}
	}
	switch {
	case latest == nil:
		return fmt.Errorf("tested by can only change at a Gate, and no Milestone has a Design yet")
	case latest.State == initiative.MilestoneGated:
		return fmt.Errorf("tested by can only change at a Gate, and Milestone %q already has a gate note", latest.Slug)
	case latest.State != initiative.MilestoneMerged:
		return fmt.Errorf("tested by can only change at a Gate, and Milestone %q is %s, not merged", latest.Slug, latest.State)
	}
	return nil
}

// editInitiative applies edit to the Initiative named by arg under the epic
// lock, then prints the line it stored. Nothing is written when edit fails.
func editInitiative(cfgDirs config.Dirs, cwd, arg string, w io.Writer, edit itemEdit) error {
	return editInitiativeIn(cfgDirs, cwd, arg, w, func(b *initiative.Body, v initiative.View, _ epicSet) (string, error) {
		return edit(b, v)
	})
}

// epicSet answers whether an epic exists in the project's store, live or
// archived.
type epicSet func(name string) bool

// storeEdit is an itemEdit that also sees which epics exist.
type storeEdit func(body *initiative.Body, view initiative.View, exists epicSet) (string, error)

func editInitiativeIn(cfgDirs config.Dirs, cwd, arg string, w io.Writer, edit storeEdit) error {
	projectDir, _, slug, err := tickets.ResolveEpicTarget(cfgDirs, cwd, arg)
	if err != nil {
		return err
	}
	epicPath := filepath.Join(projectDir, slug)
	ticketPath := filepath.Join(epicPath, "ticket.md")
	if _, err := os.Stat(ticketPath); err != nil {
		return fmt.Errorf("no epic %q", slug)
	}
	if epic := tickets.LoadEpicTicketMD(epicPath); !epic.IsInitiative {
		return fmt.Errorf("epic %q is %s, not an initiative", slug, epicKind(epic))
	}
	live, err := tickets.Load(projectDir)
	if err != nil {
		return err
	}
	archived, err := tickets.LoadArchived(projectDir)
	if err != nil {
		return err
	}
	exists := func(name string) bool {
		return slices.ContainsFunc(append(slices.Clone(live), archived...), func(e tickets.Epic) bool { return e.Name == name })
	}
	var line string
	err = tickets.EditEpicBody(ticketPath, func(text string) (string, error) {
		body := initiative.ParseBody(text)
		view := initiative.Read(tickets.Epic{Name: slug, Body: text}, live, archived)
		var editErr error
		if line, editErr = edit(&body, view, exists); editErr != nil {
			return "", editErr
		}
		return body.Render()
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, line)
	return err
}

func epicKind(e tickets.Epic) string {
	switch {
	case e.IsMap:
		return "a " + tickets.KindMap
	case e.IsBacklog:
		return "a " + tickets.KindBacklog
	}
	return "a plain epic"
}
