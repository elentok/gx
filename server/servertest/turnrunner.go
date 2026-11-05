package servertest

import (
	"slices"
	"strings"
	"sync"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/testutil/runnerfake"
)

// TurnRunner runs turn once per session, in its cwd, before the prompt
// settles: the agent's work.
type TurnRunner struct {
	*runnerfake.Runner
	turn func(label, cwd string)

	mu     sync.Mutex
	cwds   map[string]string
	labels []string
	turned map[string]bool
}

func NewTurnRunner(turn func(label, cwd string)) *TurnRunner {
	r := &TurnRunner{Runner: runnerfake.NewRunner(), turn: turn, cwds: map[string]string{}, turned: map[string]bool{}}
	r.PromptState = agentrunner.StateIdle
	return r
}

func (r *TurnRunner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	r.mu.Lock()
	r.cwds[opts.Label] = opts.Cwd
	r.labels = append(r.labels, opts.Label)
	r.mu.Unlock()
	return r.Runner.Start(opts)
}

func (r *TurnRunner) Prompt(s agentrunner.Session, text string) error {
	r.mu.Lock()
	cwd, first := r.cwds[s.Label], !r.turned[s.Label]
	r.turned[s.Label] = true
	r.mu.Unlock()
	if first {
		r.turn(s.Label, cwd)
	}
	return r.Runner.Prompt(s, text)
}

// Started returns the first session label that starts with prefix.
func (r *TurnRunner) Started(prefix string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.IndexFunc(r.labels, func(l string) bool { return strings.HasPrefix(l, prefix) })
	if i < 0 {
		return "", false
	}
	return r.labels[i], true
}
