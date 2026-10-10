package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

// followUpDraft is one follow-up of the outbox, with the frontmatter the
// loader's Ticket doesn't carry.
type followUpDraft struct {
	ticket   tickets.Ticket
	category schema.Category
	exported bool
}

// outboxFollowUps lists the draft follow-ups in the `gx` project's follow-ups
// epic, oldest first. A missing project or epic has none.
func outboxFollowUps(env evolveEnv) ([]followUpDraft, error) {
	dir, err := tickets.ProjectDirByName(env.storePath, evolveGxProject)
	if err != nil {
		return nil, err
	}
	epics, err := tickets.Load(dir)
	if err != nil {
		return nil, err
	}
	var out []followUpDraft
	for _, e := range epics {
		if e.Name != followUpsEpic {
			continue
		}
		for _, tk := range e.Tickets {
			if tk.Status != string(schema.StatusDraft) {
				continue
			}
			sc, err := schema.ParseTicket(tk.Path)
			if err != nil {
				return nil, fmt.Errorf("ticket %s: %w", tk.Identifier, err)
			}
			out = append(out, followUpDraft{ticket: tk, category: sc.Category, exported: !sc.ExportedAt.IsZero()})
		}
	}
	return out, nil
}

// evolveReminder tells a contributor how many follow-ups are still unexported.
// Every evolve verb ends with it. It is a hint, so a store it can't read costs
// the hint, not the verb.
func evolveReminder(env evolveEnv, mode string, w io.Writer) error {
	if mode != config.EvolveContributor {
		return nil
	}
	drafts, err := outboxFollowUps(env)
	if err != nil {
		return nil
	}
	n := 0
	for _, d := range drafts {
		if !d.exported {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	_, err = fmt.Fprintf(w, "%d unexported follow-ups; run `gx evolve export`\n", n)
	return err
}

// runEvolveExport writes the unexported follow-ups (all of them with again) as
// one markdown bundle to out, or w when out is empty, then stamps exported_at
// on each. The stamp comes after the write, so a failed write leaves them
// unexported. It never posts anything. The reminder is left out: it would
// always read zero.
func runEvolveExport(env evolveEnv, w, errW io.Writer, out string, again bool) error {
	if err := server.EnsureOutboxProject(env.storePath, env.cfg.Evolve); err != nil {
		return err
	}
	dir, err := tickets.ProjectDirByName(env.storePath, evolveGxProject)
	if err != nil {
		return err
	}
	if _, err := evolveGuard(env, dir); err != nil {
		return err
	}
	drafts, err := outboxFollowUps(env)
	if err != nil {
		return err
	}
	var picked []followUpDraft
	for _, d := range drafts {
		if again || !d.exported {
			picked = append(picked, d)
		}
	}
	if len(picked) == 0 {
		fmt.Fprintln(errW, "no unexported follow-ups")
		return nil
	}

	now := env.now()
	bundle := buildFollowUpBundle(picked, now)
	if out == "" {
		if _, err := io.WriteString(w, bundle); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(out, []byte(bundle), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(errW, "exported %d follow-ups to %s\n", len(picked), out)
	}
	for _, d := range picked {
		err := schema.UpdateTicket(d.ticket.Path, func(t *schema.Ticket) { t.ExportedAt = now.UTC() })
		if err != nil {
			return fmt.Errorf("stamping %s: %w", d.ticket.Identifier, err)
		}
	}
	return nil
}

// buildFollowUpBundle renders the export: a dated title, then per follow-up its
// title, a metadata block and its body with every heading one level deeper so
// the bundle's own headings stay on top.
func buildFollowUpBundle(drafts []followUpDraft, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# gx follow-ups — %s\n", now.Format("2006-01-02"))
	for _, d := range drafts {
		tk := d.ticket
		fmt.Fprintf(&b, "\n## %s\n\n```\ncategory: %s\ntype: %s\nsource: %s\n```\n\n%s\n",
			tk.Title, d.category, tk.Type, sourceProject(tk.Body), strings.TrimSpace(pushHeadingsDown(tk.Body)))
	}
	return b.String()
}

// sourceProject is the project of the address that opens a follow-up's Source
// section. The full section stays in the body.
func sourceProject(body string) string {
	fields := strings.Fields(schema.Section(body, "Source"))
	if len(fields) == 0 {
		return ""
	}
	project, _, _ := strings.Cut(fields[0], ":")
	return project
}

// pushHeadingsDown adds one `#` to every ATX heading outside a fenced block.
func pushHeadingsDown(body string) string {
	lines := strings.Split(body, "\n")
	fenced := false
	for i, line := range lines {
		switch {
		case strings.HasPrefix(strings.TrimSpace(line), "```"):
			fenced = !fenced
		case !fenced && strings.HasPrefix(line, "#"):
			lines[i] = "#" + line
		}
	}
	return strings.Join(lines, "\n")
}

func newEvolveExportCmd(env func() (evolveEnv, error)) *cobra.Command {
	var out string
	var again bool
	c := &cobra.Command{
		Use:   "export [-o <file>] [--again]",
		Short: "bundle the unexported follow-ups into one markdown file",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return runEvolveExport(e, c.OutOrStdout(), c.ErrOrStderr(), out, again)
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "write the bundle to this file instead of stdout")
	c.Flags().BoolVar(&again, "again", false, "include follow-ups that were already exported")
	return c
}
