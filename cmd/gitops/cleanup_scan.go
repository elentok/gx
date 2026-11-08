package gitops

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/elentok/gx/cleanupscan"
	"github.com/elentok/gx/config"
)

// runCleanupScan scans the repo at cwd and prints the result to w either as
// human-readable text or as JSON.
func runCleanupScan(dirs config.Dirs, cwd string, jsonOut bool, w io.Writer) error {
	result, err := cleanupscan.Scan(dirs, cwd)
	if err != nil {
		return err
	}
	if jsonOut {
		return printCleanupScanJSON(w, result)
	}
	printCleanupScanText(w, result)
	return nil
}

func printCleanupScanJSON(w io.Writer, result cleanupscan.Result) error {
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

func printCleanupScanText(w io.Writer, result cleanupscan.Result) {
	fmt.Fprintln(w, "Epics:")
	if len(result.Epics) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, e := range result.Epics {
		codeReview := "no code-review ticket"
		if e.HasCodeReviewTicket {
			codeReview = "code-review done"
			if !e.CodeReviewDone {
				codeReview = "code-review pending"
			}
		}
		fmt.Fprintf(w, "  %s: done=%t merged=%t %s\n", e.Name, e.AllDone, e.MergedToTrunk, codeReview)
	}

	fmt.Fprintln(w, "Worktrees:")
	if len(result.Worktrees) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, ws := range result.Worktrees {
		detail := ws.Kind
		switch ws.Kind {
		case "iteration":
			detail = fmt.Sprintf("iteration epic=%s ticket=%s done=%t merged=%t", ws.Epic, ws.TicketID, ws.TicketDone, ws.Merged)
		case "feature":
			detail = fmt.Sprintf("feature epic=%s merged=%t", ws.Epic, ws.MergedToTrunk)
		case "other":
			detail = fmt.Sprintf("other merged=%t", ws.MergedToTrunk)
		}
		rec := ws.Recommendation
		if rec == "" {
			rec = "-"
		}
		fmt.Fprintf(w, "  %s: %s active=%t recommendation=%s\n", ws.Branch, detail, ws.Active, rec)
	}

	fmt.Fprintln(w, "Attic refs:")
	if len(result.AtticRefs) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, ref := range result.AtticRefs {
		fmt.Fprintf(w, "  %s\n", ref)
	}

	fmt.Fprintln(w, "Housekeeping:")
	if result.Housekeeping.Skipped {
		fmt.Fprintln(w, "  skipped (scratch root is inside a bare git directory)")
		return
	}
	if len(result.Housekeeping.TrackedFiles) == 0 {
		fmt.Fprintln(w, "  no tracked files under .scratch")
	} else {
		fmt.Fprintf(w, "  tracked files under .scratch: %s\n", strings.Join(result.Housekeeping.TrackedFiles, ", "))
	}
	if result.Housekeeping.GitignoreHasScratch {
		fmt.Fprintln(w, "  .scratch/ is gitignored")
	} else {
		fmt.Fprintln(w, "  .scratch/ is NOT gitignored")
	}
}
