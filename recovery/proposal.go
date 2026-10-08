package recovery

import (
	"fmt"
	"strings"
)

// Call is one server verb a remedy wants to make. A proposal is the calls a
// high-authority remedy would have made, written down instead of run.
type Call struct {
	Verb    string
	Address string
	// Text is a park's reason or a nudge's text.
	Text string
	// Parent is set-parent's new parent.
	Parent string
}

// arg is the call's one argument, whichever field the verb takes it in.
func (c Call) arg() string {
	if c.Verb == VerbSetParent {
		return c.Parent
	}
	return c.Text
}

// String is the call as one line: "verb address [arg]".
func (c Call) String() string {
	return strings.TrimSpace(c.Verb + " " + c.Address + " " + c.arg())
}

// ParseCalls reads the lines String wrote.
func ParseCalls(text string) ([]Call, error) {
	var calls []Call
	for line := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("bad proposed verb %q", line)
		}
		c := Call{Verb: parts[0], Address: parts[1]}
		if len(parts) == 3 {
			if c.Verb == VerbSetParent {
				c.Parent = parts[2]
			} else {
				c.Text = parts[2]
			}
		}
		calls = append(calls, c)
	}
	return calls, nil
}

// FormatCalls is ParseCalls' inverse.
func FormatCalls(calls []Call) string {
	lines := make([]string, len(calls))
	for i, c := range calls {
		lines[i] = c.String()
	}
	return strings.Join(lines, "\n")
}

// recorder is Verbs that only remember what was asked.
type recorder struct{ calls []Call }

func (r *recorder) Do(c Call) (Result, error) {
	r.calls = append(r.calls, c)
	return Result{}, nil
}

func (r *recorder) LaunchPrompt(string) (string, error) { return "", nil }

// Proposable reports whether the matched entry is a high-authority rule: it
// is never applied unattended, but its remedy says what it would do.
func (e Entry) Proposable(f Failure) bool {
	return e.Executor == ExecutorRule && e.Authority == AuthorityHigh && e.Remedy != nil && !f.DiagnosisOnly()
}

// Propose runs the entry's remedy against a recorder and returns the verbs it
// would have called. A verb outside the entry's Verbs fails the proposal.
func (e Entry) Propose(f Failure) ([]Call, error) {
	var r recorder
	if err := e.Apply(f, &r); err != nil {
		return nil, err
	}
	return r.calls, nil
}
