package ticketsflow

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/elentok/gx/cmd/ticketref"
	ticketscmd "github.com/elentok/gx/cmd/tickets"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

// followUpAddOptions are the flags of `gx tickets follow-up add`.
type followUpAddOptions struct {
	Force bool
	JSON  bool
}

// runTicketsFollowUpAdd files one follow-up into the follow-ups epic at
// epicPath, enforcing the one fixed follow-up format so every skill that
// files one writes the same shape. The ticket is always draft + research; a
// body asking for anything else is refused. Every check runs before anything
// is written.
func runTicketsFollowUpAdd(d Deps, epicPath, body, category string, opts followUpAddOptions, w io.Writer) error {
	if filepath.Base(filepath.Clean(epicPath)) != tickets.FollowUpsEpic {
		return &tickets.AddressError{Code: tickets.CodeNotFollowUpsEpic, Msg: "follow-up add targets a " + tickets.FollowUpsEpic + " epic, not " + filepath.Base(epicPath)}
	}
	// Read before bodyFrontmatter, which would reject a bad status with an
	// uncoded error.
	var asked struct {
		Status string `yaml:"status"`
		Type   string `yaml:"type"`
	}
	if yamlPart, _, ok := schema.SplitFrontmatter(strings.TrimLeft(body, "\n")); ok {
		if err := yaml.Unmarshal([]byte(yamlPart), &asked); err != nil {
			return fmt.Errorf("body frontmatter: %w", err)
		}
	}
	if asked.Status != "" && asked.Status != string(schema.StatusDraft) {
		return &tickets.AddressError{Code: tickets.CodeStatusOverride, Msg: fmt.Sprintf("a follow-up is always filed draft, not %q", asked.Status)}
	}
	if asked.Type != "" && asked.Type != string(schema.TypeResearch) {
		return &tickets.AddressError{Code: tickets.CodeTypeOverride, Msg: fmt.Sprintf("a follow-up is always filed research, not %q", asked.Type)}
	}

	fm, text, err := ticketscmd.BodyFrontmatter(body)
	if err != nil {
		return err
	}
	if category != "" {
		if fm.Category != "" {
			return &tickets.AddressError{Code: tickets.CodeCategoryConflict, Msg: "category is set by both --category and the body frontmatter; use one"}
		}
		if !schema.Category(category).Valid() {
			return fmt.Errorf("invalid category %q, want bug or idea", category)
		}
		fm.Category = category
	}
	if fm.Category == "" {
		return &tickets.AddressError{Code: tickets.CodeCategoryRequired, Msg: "a follow-up needs a category (bug or idea)"}
	}
	required := []string{"Source", "Problem", "Suggested fix"}
	if fm.Category == string(schema.CategoryBug) {
		required = append(required, "Evidence")
	}
	var missing []string
	for _, h := range required {
		if schema.Section(text, h) == "" {
			missing = append(missing, "## "+h)
		}
	}
	if len(missing) > 0 {
		return &tickets.AddressError{Code: tickets.CodeSectionRequired, Msg: "a follow-up needs a non-empty " + strings.Join(missing, ", ") + " section"}
	}

	if !opts.Force {
		if dup, err := openFollowUpWithTitle(epicPath, tickets.BodyTitle(text, "")); err != nil {
			return err
		} else if dup != "" {
			return &tickets.AddressError{Code: tickets.CodeDuplicateFollowUp, Msg: fmt.Sprintf("%s is an open follow-up with this title; add to it instead, or pass --force to file it anyway", dup)}
		}
	}

	fm.Status = string(schema.StatusDraft)
	fm.Type = string(schema.TypeResearch)
	path, _, err := ticketscmd.CreateTicket(epicPath, "", followUpSlug(text), &text, fm)
	if err != nil {
		return err
	}
	ticketscmd.PingServer(d.Env, path)
	return ticketscmd.PrintTicketRef(path, opts.JSON, w)
}

// openFollowUpWithTitle returns the address of an open (not done or
// cancelled) follow-up in the epic at epicPath whose title equals title,
// ignoring case and surrounding space, or "". An empty title never matches,
// and a follow-ups epic that does not exist yet has no duplicates.
func openFollowUpWithTitle(epicPath, title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", nil
	}
	root := filepath.Dir(filepath.Clean(epicPath))
	epics, err := tickets.Load(root)
	if err != nil {
		return "", err
	}
	for _, e := range epics {
		if e.Name != tickets.FollowUpsEpic {
			continue
		}
		for _, s := range e.Tickets {
			if s.Status == string(schema.StatusDone) || s.Status == string(schema.StatusCancelled) {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(s.Title), title) {
				return ticketref.TicketLabel(s.Path), nil
			}
		}
	}
	return "", nil
}

var slugUnsafeRe = regexp.MustCompile(`[^a-z0-9]+`)

// followUpSlug names the file after the first words of the problem, since the
// fixed format has no title to take it from.
func followUpSlug(text string) string {
	words := strings.Fields(strings.ToLower(schema.Section(text, "Problem")))
	if len(words) > 6 {
		words = words[:6]
	}
	slug := strings.Trim(slugUnsafeRe.ReplaceAllString(strings.Join(words, "-"), "-"), "-")
	if slug == "" {
		return "follow-up"
	}
	return slug
}
