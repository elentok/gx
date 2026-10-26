package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// verifyContextWindow is larger than the code-review stub's: verify drives a
// running gx and reads the spec.
const verifyContextWindow = 70000

// runTicketsEnsureVerify is runTicketsEnsureCodeReview for the epic's `type:
// verify` ticket: a no-op when one exists, else it stamps out an open stub.
func runTicketsEnsureVerify(epicPath string, w io.Writer) error {
	// bodyFormat goes through Sprintf, so the spec line escapes its percents.
	specLine := strings.ReplaceAll(verifySpecLine(epicPath), "%", "%%")
	return ensureTypedTicket(epicPath, w, typedTicketSpec{
		ticketType:    schema.TypeVerify,
		name:          "verify",
		contextWindow: verifyContextWindow,
		bodyFormat:    "\n# %s — Verify: %s\n\n## What to verify\n\n" + specLine + "\n## Test seams\n\nnone — verify ticket, opens child tickets (`parent:` pointing back here) if it finds anything.\n\n## Acceptance criteria\n\n- [ ] The finished epic checked in a running gx\n- [ ] Any findings opened as child tickets\n",
	})
}

// verifySpecLine names the epic's spec, or is empty when there is none. A repo
// spec is named repo-relative, the epic's own spec.md by its store path.
func verifySpecLine(epicPath string) string {
	epicPath = filepath.Clean(epicPath)
	epic := filepath.Base(epicPath)
	projectDir := filepath.Dir(epicPath)
	store := filepath.Dir(projectDir)

	if checkout := tickets.TrunkCheckout(store, filepath.Base(projectDir)); checkout != "" {
		slugs := []string{epic}
		if name, ok := strings.CutSuffix(epic, tickets.ImplEpicSuffix); ok {
			slugs = append(slugs, name)
		}
		for _, slug := range slugs {
			rel := filepath.Join("docs", "specs", slug+".md")
			if fileExists(filepath.Join(checkout, rel)) {
				return "Spec: `" + rel + "`\n\n"
			}
		}
	}
	if own := filepath.Join(epicPath, "spec.md"); fileExists(own) {
		return "Spec: `" + own + "`\n\n"
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
