package initiatives

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/tickets/initiative"
)

// findMilestone returns the body index and the view of the Milestone slug.
func findMilestone(b *initiative.Body, v initiative.View, slug string) (int, initiative.MilestoneView, error) {
	idx := slices.IndexFunc(b.Milestones, func(e initiative.Entry[initiative.Milestone]) bool {
		return !e.Unparsed() && e.Value.Slug == slug
	})
	vi := slices.IndexFunc(v.Milestones, func(m initiative.MilestoneView) bool { return m.Slug == slug })
	if idx < 0 || vi < 0 {
		return 0, initiative.MilestoneView{}, fmt.Errorf("unknown Milestone %q", slug)
	}
	return idx, v.Milestones[vi], nil
}

// lockedBy says why a Milestone can no longer be removed or moved, or "".
func lockedBy(m initiative.MilestoneView) string {
	switch {
	case m.State == initiative.MilestoneGated:
		return "a gate note"
	case m.Design != "":
		return "a Design link"
	}
	return ""
}

// checkPosition accepts a 1-based position within max that is after every
// Milestone with a Design link or a gate note.
func checkPosition(v initiative.View, pos, max int) error {
	if pos < 1 || pos > max {
		return fmt.Errorf("position %d is out of range: use 1 to %d", pos, max)
	}
	for _, m := range v.Milestones {
		if lockedBy(m) != "" && pos <= m.Number {
			return fmt.Errorf("position %d is not after Milestone %q, which has %s: only sketches after the current Milestone can be reordered", pos, m.Slug, lockedBy(m))
		}
	}
	return nil
}

// insertMilestone puts e at the 1-based position pos among the parsed
// Milestones. Unparsed lines keep their text.
func insertMilestone(es []initiative.Entry[initiative.Milestone], pos int, e initiative.Entry[initiative.Milestone]) []initiative.Entry[initiative.Milestone] {
	at := len(es)
	seen := 0
	for i, x := range es {
		if x.Unparsed() {
			continue
		}
		seen++
		if seen == pos {
			at = i
			break
		}
	}
	return slices.Insert(es, at, e)
}

func newMilestoneRemove(d Deps) *cobra.Command {
	return storeCmd(d, "remove <project:initiative> <slug>", "remove a sketch Milestone", func(*cobra.Command) {},
		func(c *cobra.Command, args []string) (storeEdit, error) {
			return func(b *initiative.Body, v initiative.View, _ epicSet) (string, error) {
				idx, mv, err := findMilestone(b, v, args[1])
				if err != nil {
					return "", err
				}
				if why := lockedBy(mv); why != "" {
					return "", fmt.Errorf("Milestone %q has %s, so it cannot be removed", args[1], why)
				}
				var names []string
				for _, e := range b.Assumptions {
					if !e.Unparsed() && e.Value.TestedBy == args[1] {
						names = append(names, fmt.Sprintf("A%d", e.Value.N))
					}
				}
				if len(names) > 0 {
					return "", fmt.Errorf("Milestone %q is still tested by %s: move them to another Milestone first", args[1], strings.Join(names, ", "))
				}
				line, err := initiative.RenderMilestone(b.Milestones[idx].Value)
				if err != nil {
					return "", err
				}
				b.Milestones = slices.Delete(b.Milestones, idx, idx+1)
				for i, e := range b.Deferred {
					if !e.Unparsed() && e.Value.To == args[1] && e.Value.State == initiative.DeferredOpen {
						b.Deferred[i].Value.To = initiative.Unplaced
					}
				}
				return line, nil
			}, nil
		})
}

func newMilestoneMove(d Deps) *cobra.Command {
	return storeCmd(d, "move <project:initiative> <slug>", "move a sketch Milestone to another position", func(c *cobra.Command) {
		c.Flags().Int("to", 0, "new position, 1-based (required)")
		_ = c.MarkFlagRequired("to")
	}, func(c *cobra.Command, args []string) (storeEdit, error) {
		pos, _ := c.Flags().GetInt("to")
		return func(b *initiative.Body, v initiative.View, _ epicSet) (string, error) {
			idx, mv, err := findMilestone(b, v, args[1])
			if err != nil {
				return "", err
			}
			if why := lockedBy(mv); why != "" {
				return "", fmt.Errorf("Milestone %q has %s, so it cannot be moved", args[1], why)
			}
			if err := checkPosition(v, pos, len(v.Milestones)); err != nil {
				return "", err
			}
			e := b.Milestones[idx]
			line, err := initiative.RenderMilestone(e.Value)
			if err != nil {
				return "", err
			}
			b.Milestones = insertMilestone(slices.Delete(b.Milestones, idx, idx+1), pos, e)
			return line, nil
		}, nil
	})
}

func newDestination(d Deps) *cobra.Command {
	return storeCmd(d, "destination <project:initiative> <text>", "replace the Destination text", func(*cobra.Command) {},
		func(c *cobra.Command, args []string) (storeEdit, error) {
			text := strings.TrimSpace(args[1])
			if text == "" {
				return nil, fmt.Errorf("the Destination is empty")
			}
			return func(b *initiative.Body, _ initiative.View, _ epicSet) (string, error) {
				b.SetDestination(text)
				return text, nil
			}, nil
		})
}
