package server

import "github.com/elentok/gx/harness"

// Manager creates, stops, removes and watches sessions, and reads and writes
// their meta. It owns the watch goroutines (watches).
//
// srv is the rest of the root; the move into server/sessions replaces it with
// the small interfaces below and the fields the methods actually use.
type Manager struct {
	srv     *Server
	watches *sessionWatches
	// prompter is the prompt, turn and auto-advance side the watch loop calls.
	prompter Prompter
	// designs is the design cleanup the remove path calls back.
	designs designCleaner
	// rollouts answers whether Codex still has a rollout for a session.
	rollouts codexRollouts
}

func newManager(s *Server) *Manager {
	return &Manager{
		srv:      s,
		watches:  newSessionWatches(),
		prompter: serverPrompter{s},
		designs:  s,
		rollouts: s,
	}
}

// Resumer brings sessions back: resume, restore, auto-resume, handoff, link,
// step and the fix agent.
type Resumer struct {
	srv *Server
}

// Prompter is what the session watch loop needs from the prompt, turn and
// auto-advance code. The root implements it; the move into server/sessions/prompt
// keeps the hook.
type Prompter interface {
	newFirstPrompt(dir, id string, run int) FirstPrompt
	markTurn(dir, id string) (turnMark, bool)
	settleTurn(dir, id string, mark turnMark, design designRef, cancel <-chan struct{})
	detectDesignActionable(dir, id string, design designRef)
	sendAction(dir, label string, action []string)
	cancelAutoAdvance(id, why string) bool
}

// FirstPrompt is the first-prompt tracker of one watch.
type FirstPrompt interface {
	observe(state string, ready harness.ReadySignal) string
	noteState(state string)
	personInput()
	start(signal, prompt string)
	stop()
}

// designCleaner is the designs back-call of the remove path.
type designCleaner interface {
	cleanupDesign(project, epic string)
}

// codexRollouts is the sched back-call of the resume path.
type codexRollouts interface {
	codexRolloutExists(cwd, id string) bool
}

// serverPrompter is the root's Prompter.
type serverPrompter struct{ s *Server }

func (p serverPrompter) newFirstPrompt(dir, id string, run int) FirstPrompt {
	return p.s.newFirstPrompt(dir, id, run)
}

func (p serverPrompter) markTurn(dir, id string) (turnMark, bool) { return p.s.markTurn(dir, id) }

func (p serverPrompter) settleTurn(dir, id string, mark turnMark, design designRef, cancel <-chan struct{}) {
	p.s.settleTurn(dir, id, mark, design, cancel)
}

func (p serverPrompter) detectDesignActionable(dir, id string, design designRef) {
	p.s.detectDesignActionable(dir, id, design)
}

func (p serverPrompter) sendAction(dir, label string, action []string) {
	p.s.sendAction(dir, label, action)
}

func (p serverPrompter) cancelAutoAdvance(id, why string) bool {
	return p.s.cancelAutoAdvance(id, why)
}
