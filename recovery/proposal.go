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
	Reason  string
}

// String is the call as one line: "verb address [reason]".
func (c Call) String() string {
	return strings.TrimSpace(c.Verb + " " + c.Address + " " + c.Reason)
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
			c.Reason = parts[2]
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

// Apply makes the call through v.
func (c Call) Apply(v Verbs) (Result, error) {
	switch c.Verb {
	case "park":
		return v.Park(c.Address, c.Reason)
	case "relaunch":
		return v.Relaunch(c.Address)
	case "commitless-done":
		return v.CommitlessDone(c.Address)
	}
	return Result{}, fmt.Errorf("unknown proposed verb %q", c.Verb)
}

// recorder is Verbs that only remember what was asked.
type recorder struct{ calls []Call }

func (r *recorder) Park(address, reason string) (Result, error) {
	r.calls = append(r.calls, Call{Verb: "park", Address: address, Reason: reason})
	return Result{}, nil
}

func (r *recorder) Relaunch(address string) (Result, error) {
	r.calls = append(r.calls, Call{Verb: "relaunch", Address: address})
	return Result{}, nil
}

func (r *recorder) CommitlessDone(address string) (Result, error) {
	r.calls = append(r.calls, Call{Verb: "commitless-done", Address: address})
	return Result{}, nil
}

// Proposable reports whether the matched entry is a high-authority rule: it
// is never applied unattended, but its remedy says what it would do.
func (e Entry) Proposable(f Failure) bool {
	return e.Executor == ExecutorRule && e.Authority == AuthorityHigh && e.Remedy != nil && !f.DiagnosisOnly()
}

// Propose runs the entry's remedy against a recorder and returns the verbs it
// would have called.
func (e Entry) Propose(f Failure) ([]Call, error) {
	var r recorder
	if err := e.Remedy(f, &r); err != nil {
		return nil, err
	}
	return r.calls, nil
}
