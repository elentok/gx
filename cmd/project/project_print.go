package project

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/elentok/gx/server/wire"
)

func printProjects(w io.Writer, projects []wire.ProjectInfo) error {
	for _, p := range projects {
		counts := make([]string, 0, len(p.Tickets))
		for status, n := range p.Tickets {
			counts = append(counts, fmt.Sprintf("%s=%d", status, n))
		}
		sort.Strings(counts)
		if _, err := fmt.Fprintf(w, "%s\t%s\n", p.Name, strings.Join(counts, " ")); err != nil {
			return err
		}
	}
	return nil
}

func printProjectWorktrees(w io.Writer, projects []wire.ProjectWorktrees) error {
	for _, p := range projects {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", p.Project, p.Root); err != nil {
			return err
		}
		for _, wt := range p.Worktrees {
			if _, err := fmt.Fprintf(w, "%s\t%s\n", p.Project, wt); err != nil {
				return err
			}
		}
	}
	return nil
}
