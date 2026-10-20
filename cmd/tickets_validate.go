package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"unicode/utf8"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runTicketsValidate parses path via schema.ParseTicket and reports whether
// it's well-formed. A parse/validation failure is returned as an error (for
// a non-zero exit code via cobra's RunE); success prints a confirmation to w.
//
// A ticket in the tracker's <project>/<epic>/issues/ layout is validated
// against its whole project, so an error in a sibling ticket or epic fails it
// too; an ad-hoc file is validated on its own frontmatter alone.
func runTicketsValidate(path string, w, stderr io.Writer) error {
	ticket, err := schema.ParseTicket(path)
	if err != nil {
		return err
	}
	if projectDir, ok := projectDirOfTicket(path); ok {
		if err := tickets.ValidateProject(projectDir); err != nil {
			return err
		}
		if err := printTicketWarnings(projectDir, path, stderr); err != nil {
			return err
		}
	}
	if err := validateTicketLog(path, ticket, stderr); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: valid ticket (id=%s, status=%s)\n", ticketLabel(path), ticket.ID, ticket.Status)
	return nil
}

// validateTicketLog checks the ticket's log: a malformed middle line is an
// error (ReadLog skips a torn last line), a needs-answer ticket must have an
// open latest question, and an over-long question summary only warns. This
// lives here rather than in schema.Validate, which stays free of log checks.
func validateTicketLog(path string, ticket schema.Ticket, stderr io.Writer) error {
	events, err := tickets.ReadLog(path)
	if err != nil {
		return err
	}
	if ticket.Status == schema.StatusNeedsAnswer && tickets.OpenQuestion(events) == nil {
		return fmt.Errorf("%s: status is needs-answer but the log has no open question", ticketLabel(path))
	}
	for _, ev := range events {
		if ev.Type != tickets.EventParkQuestion {
			continue
		}
		if n := utf8.RuneCountInString(ev.Summary); n > tickets.MaxLogSummaryChars {
			fmt.Fprintf(stderr, "warning: %s: question %d summary is %d characters, max %d\n",
				ticketLabel(path), ev.Seq, n, tickets.MaxLogSummaryChars)
		}
	}
	return nil
}

// runTicketsValidateAll validates every project in the ticket store, reporting
// the errors of all of them at once.
func runTicketsValidateAll(storePath string, w, stderr io.Writer) error {
	dirs, err := tickets.ProjectDirs(storePath)
	if err != nil {
		return err
	}
	var errs []error
	for _, dir := range dirs {
		if err := tickets.ValidateProject(dir); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := printProjectWarnings(dir, stderr); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	fmt.Fprintf(w, "%d projects valid\n", len(dirs))
	return nil
}

// printProjectWarnings writes projectDir's non-fatal findings to w.
func printProjectWarnings(projectDir string, w io.Writer) error {
	warnings, err := tickets.ProjectWarnings(projectDir)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}
	return nil
}

// printTicketWarnings writes the findings for the ticket at path to w, and
// folds every other ticket's into one count: an agent validating its own
// ticket shouldn't wade through the whole project's.
func printTicketWarnings(projectDir, path string, w io.Writer) error {
	warnings, err := tickets.ProjectWarnings(projectDir)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	others := map[string]bool{}
	for _, warning := range warnings {
		if filepath.Clean(warning.Path) == abs {
			fmt.Fprintf(w, "warning: %s\n", warning)
		} else {
			others[warning.Path] = true
		}
	}
	switch len(others) {
	case 0:
	case 1:
		fmt.Fprintln(w, "1 other ticket has warnings; run `gx tickets validate --all`")
	default:
		fmt.Fprintf(w, "%d other tickets have warnings; run `gx tickets validate --all`\n", len(others))
	}
	return nil
}

// projectDirOfTicket is the project directory above path when it sits in the
// <project>/<epic>/issues/<file>.md layout.
func projectDirOfTicket(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	issuesDir := filepath.Dir(abs)
	if filepath.Base(issuesDir) != "issues" {
		return "", false
	}
	return filepath.Dir(filepath.Dir(issuesDir)), true
}
