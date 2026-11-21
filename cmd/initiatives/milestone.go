package initiatives

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/tickets/initiative"
)

func newMilestone(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "milestone", Short: "add and change Milestones"}
	cmd.AddCommand(newMilestoneAdd(d), newMilestoneSet(d))
	return cmd
}

func storeCmd(d Deps, use, short string, setup func(*cobra.Command), build func(c *cobra.Command, args []string) (storeEdit, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			edit, err := build(c, args)
			if err != nil {
				return err
			}
			cwd, err := d.Cwd()
			if err != nil {
				return err
			}
			return editInitiativeIn(d.Dirs(), cwd, args[0], c.OutOrStdout(), edit)
		},
	}
	setup(cmd)
	return cmd
}

func newMilestoneAdd(d Deps) *cobra.Command {
	return storeCmd(d, "add <project:initiative> <slug>", "append a sketch Milestone", func(c *cobra.Command) {
		c.Flags().String("name", "", "Milestone name (required)")
		c.Flags().String("goal", "", "one-line goal (required)")
		_ = c.MarkFlagRequired("name")
		_ = c.MarkFlagRequired("goal")
	}, func(c *cobra.Command, args []string) (storeEdit, error) {
		name, _ := c.Flags().GetString("name")
		goal, _ := c.Flags().GetString("goal")
		return func(b *initiative.Body, v initiative.View, exists epicSet) (string, error) {
			m := initiative.Milestone{Slug: args[1], Name: name, Goal: goal}
			line, err := initiative.RenderMilestone(m)
			if err != nil {
				return "", err
			}
			if err := checkSlugFree(b, v, exists, m.Slug); err != nil {
				return "", err
			}
			b.Milestones = append(b.Milestones, initiative.Parsed(m))
			return line, nil
		}, nil
	})
}

func newMilestoneSet(d Deps) *cobra.Command {
	flags := []string{"name", "goal", "design", "clear-design", "slug"}
	return storeCmd(d, "set <project:initiative> <slug>", "rename, link a Design, or change the slug of a Milestone", func(c *cobra.Command) {
		c.Flags().String("name", "", "new name")
		c.Flags().String("goal", "", "new one-line goal")
		c.Flags().String("design", "", "link the Design epic that runs it")
		c.Flags().Bool("clear-design", false, "unlink the Design: back to a sketch")
		c.Flags().String("slug", "", "new slug (only while nothing names it)")
		c.MarkFlagsMutuallyExclusive("design", "clear-design")
	}, func(c *cobra.Command, args []string) (storeEdit, error) {
		if err := requireChange(c, flags...); err != nil {
			return nil, err
		}
		return func(b *initiative.Body, v initiative.View, exists epicSet) (string, error) {
			idx := -1
			for i, e := range b.Milestones {
				if !e.Unparsed() && e.Value.Slug == args[1] {
					idx = i
					break
				}
			}
			if idx < 0 {
				return "", fmt.Errorf("unknown Milestone %q", args[1])
			}
			var mv initiative.MilestoneView
			for _, x := range v.Milestones {
				if x.Slug == args[1] {
					mv = x
					break
				}
			}
			m := b.Milestones[idx].Value
			oldSlug := m.Slug
			if c.Flags().Changed("name") {
				m.Name, _ = c.Flags().GetString("name")
			}
			if c.Flags().Changed("goal") {
				m.Goal, _ = c.Flags().GetString("goal")
			}
			if c.Flags().Changed("design") {
				design, _ := c.Flags().GetString("design")
				if !exists(design) {
					return "", fmt.Errorf("Design %q does not exist in the store", design)
				}
				m.Design = design
			}
			if clear, _ := c.Flags().GetBool("clear-design"); clear {
				if mv.State == initiative.MilestoneGated {
					return "", fmt.Errorf("Milestone %q has a gate note, so its Design link stays", m.Slug)
				}
				m.Design = ""
			}
			if c.Flags().Changed("slug") {
				m.Slug, _ = c.Flags().GetString("slug")
				if mv.NamesSlug() {
					return "", fmt.Errorf("the slug of Milestone %q cannot change: %s names it; rename only changes the name and goal", oldSlug, namedBy(mv))
				}
				if m.Slug != oldSlug {
					if err := checkSlugFree(b, v, exists, m.Slug); err != nil {
						return "", err
					}
				}
			}
			line, err := initiative.RenderMilestone(m)
			if err != nil {
				return "", err
			}
			b.Milestones[idx] = initiative.Parsed(m)
			if m.Slug != oldSlug {
				renameSlugRefs(b, oldSlug, m.Slug)
			}
			return line, nil
		}, nil
	})
}

func namedBy(m initiative.MilestoneView) string {
	if m.State == initiative.MilestoneGated {
		return "its gate note"
	}
	return "its Design link"
}

// checkSlugFree refuses a slug that is already a Milestone or whose epic
// names exist, live or archived.
func checkSlugFree(b *initiative.Body, v initiative.View, exists epicSet, slug string) error {
	for _, e := range b.Milestones {
		if !e.Unparsed() && e.Value.Slug == slug {
			return fmt.Errorf("Milestone %q already exists", slug)
		}
	}
	for _, name := range []string{v.Slug + "-" + slug, v.Slug + "-" + slug + "-design", v.Slug + "-" + slug + "-impl"} {
		if exists(name) {
			return fmt.Errorf("epic %q already exists", name)
		}
	}
	return nil
}

// renameSlugRefs points every `tested by:` and `to:` line of oldSlug at
// newSlug. Unparsed lines stay as they are.
func renameSlugRefs(b *initiative.Body, oldSlug, newSlug string) {
	for i, e := range b.Assumptions {
		if !e.Unparsed() && e.Value.TestedBy == oldSlug {
			b.Assumptions[i].Value.TestedBy = newSlug
		}
	}
	for i, e := range b.Deferred {
		if !e.Unparsed() && e.Value.To == oldSlug {
			b.Deferred[i].Value.To = newSlug
		}
	}
}
