package schema

import (
	"errors"
	"fmt"
	"strings"
)

// NeedsRepairState is a best-effort snapshot of the iteration a needs-repair
// write happened in. Every field is optional: a fault write only has as much
// state as survived whatever broke, and FormatNeedsRepairBody omits an empty
// field from the rendered block rather than showing it as a placeholder — a
// state block that says "unknown" three times would look like information
// where there is none.
type NeedsRepairState struct {
	Label    string
	Branch   string
	Worktree string
}

// SplitReason splits reason at its first newline into a one-line summary and
// the remaining detail (empty for a single-line reason). Called from one
// place — FormatNeedsRepairBody — rather than leaving each fault path decide
// how to summarise its own error, so the section's opening line is
// guaranteed to be one line no matter which fault path wrote it.
func SplitReason(reason string) (summary, detail string) {
	summary, detail, _ = strings.Cut(reason, "\n")
	return summary, strings.TrimLeft(detail, "\n")
}

// FormatNeedsRepairBody renders the "## Needs Repair" section appended to a
// ticket's body: a summary line, optional detail, and a best-effort state
// block, per gx-local-tracker.md's contract. No "## Handoff" section is ever
// produced here — a handoff describes an agent's own work in progress, and a
// fault write has no live agent to author one.
//
// reason must be non-empty: this is the contract's write-conditional
// validation half. It fires here, on the write, and nowhere near the loader
// — a hand-authored ticket that never calls this still loads fine even
// without a "## Needs Repair" section.
func FormatNeedsRepairBody(reason string, state NeedsRepairState) (string, error) {
	if strings.TrimSpace(reason) == "" {
		return "", errors.New("needs-repair: reason must not be empty")
	}

	summary, detail := SplitReason(reason)

	var b strings.Builder
	fmt.Fprintf(&b, "\n## Needs Repair\n\n%s\n", summary)
	if detail != "" {
		fmt.Fprintf(&b, "\n%s\n", detail)
	}

	var stateLines []string
	if state.Label != "" {
		stateLines = append(stateLines, fmt.Sprintf("- Iteration: %s", state.Label))
	}
	if state.Branch != "" {
		stateLines = append(stateLines, fmt.Sprintf("- Branch: %s", state.Branch))
	}
	if state.Worktree != "" {
		stateLines = append(stateLines, fmt.Sprintf("- Worktree: %s", state.Worktree))
	}
	if len(stateLines) > 0 {
		fmt.Fprintf(&b, "\n%s\n", strings.Join(stateLines, "\n"))
	}

	return b.String(), nil
}
