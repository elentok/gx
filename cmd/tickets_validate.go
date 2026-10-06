package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

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
		if err := printProjectWarnings(projectDir, stderr); err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "%s: valid ticket (id=%s, status=%s)\n", ticketLabel(path), ticket.ID, ticket.Status)
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
