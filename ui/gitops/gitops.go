// Package gitops owns the five git-action modals (bump, push, pull, stash and
// command output) as one value, so each page that shows them shares one
// Update, one overlay order and one set of notifications.
package gitops

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/bump"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/output"
	"github.com/elentok/gx/ui/pull"
	"github.com/elentok/gx/ui/push"
	"github.com/elentok/gx/ui/stash"
)

// Kind names the action an Update result came from.
type Kind int

const (
	KindNone Kind = iota
	KindBump
	KindPush
	KindPull
	KindStash
)

// Result tells the parent what happened so it can decide what to reload.
// Only meaningful when Done is true.
type Result struct {
	Kind       Kind
	Done       bool
	Err        error
	Aborted    bool   // push/pull: user cancelled before completion
	NewTag     string // bump: tag created
	Outcome    stash.Outcome
	StagedOnly bool // stash: only staged changes were stashed
}

// Mutated reports whether the action finished and may have changed the repo.
func (r Result) Mutated() bool {
	if !r.Done {
		return false
	}
	switch r.Kind {
	case KindPush, KindPull:
		return true
	case KindStash:
		return r.Err == nil && r.Outcome == stash.OutcomeStashed
	}
	return false
}

// Model holds the five modals.
type Model struct {
	bump   bump.Model
	push   push.Model
	pull   pull.Model
	stash  stash.Model
	output output.Model

	root string
}

// New returns a Model with every modal closed.
func New() Model {
	return Model{
		bump:   bump.New(),
		push:   push.New(),
		pull:   pull.New(),
		stash:  stash.New(),
		output: output.New(),
	}
}

func (m *Model) OpenBump(root string) error {
	m.root = root
	return m.bump.Open(root)
}

func (m *Model) OpenPush(root string) error { return m.push.Open(root) }

// OpenPRPrompt opens the push modal straight at its "open PR" prompt.
func (m *Model) OpenPRPrompt(prURL string) { m.push.OpenAtPRPrompt(prURL) }

func (m *Model) OpenPull(root string) tea.Cmd { return m.pull.Open(root) }

func (m *Model) OpenStash(root string, stagedOnly bool) tea.Cmd {
	return m.stash.Open(root, stagedOnly)
}

// ActionOpen reports whether bump, push, pull or stash is open.
func (m Model) ActionOpen() bool {
	return m.bump.IsOpen || m.push.IsOpen || m.pull.IsOpen || m.stash.IsOpen
}

// IsOpen reports whether any of the five modals is open.
func (m Model) IsOpen() bool { return m.ActionOpen() || m.output.IsOpen }

// InputFocused reports whether an action modal has a focused text input.
func (m Model) InputFocused() bool {
	return m.bump.InputFocused() || m.push.InputFocused() || m.pull.InputFocused() || m.stash.InputFocused()
}

// StashStagedOnly reports whether the open stash modal stashes staged changes only.
func (m Model) StashStagedOnly() bool { return m.stash.StagedOnly() }

func (m Model) PushOpen() bool  { return m.push.IsOpen }
func (m Model) StashOpen() bool { return m.stash.IsOpen }

// SetOutput records output for later display without opening the modal.
func (m *Model) SetOutput(title, content string) { m.output.Set(title, content) }

func (m Model) HasOutput() bool  { return m.output.HasContent() }
func (m Model) OutputOpen() bool { return m.output.IsOpen }

func (m *Model) OpenOutput(width, height int) { m.output.Open(width, height) }

// Update routes msg to the open modal. Order: bump, push, pull, stash, output.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	switch {
	case m.bump.IsOpen:
		return m.updateBump(msg)
	case m.push.IsOpen:
		return m.updatePush(msg)
	case m.pull.IsOpen:
		return m.updatePull(msg)
	case m.stash.IsOpen:
		return m.updateStash(msg)
	case m.output.IsOpen:
		next, cmd := m.output.Update(msg)
		m.output = next
		return m, cmd, Result{}
	}
	return m, nil, Result{}
}

func (m Model) updateBump(msg tea.Msg) (Model, tea.Cmd, Result) {
	next, cmd, r := m.bump.Update(msg)
	m.bump = next
	res := Result{Kind: KindBump, Done: r.Done, Err: r.Err, NewTag: r.NewTag}
	if !r.Done {
		return m, cmd, res
	}
	if r.Err != nil || r.NewTag == "" {
		return m, nil, res
	}
	if err := m.push.OpenWithTag(m.root, r.NewTag); err != nil {
		res.Err = err
		return m, nil, res
	}
	return m, notify.Success("tag created"), res
}

func (m Model) updatePush(msg tea.Msg) (Model, tea.Cmd, Result) {
	next, cmd, r := m.push.Update(msg)
	m.push = next
	res := Result{Kind: KindPush, Done: r.Done, Err: r.Err, Aborted: r.Aborted}
	if !r.Done {
		return m, cmd, res
	}
	m.output.Set("Push output", r.Output)
	return m, tea.Batch(cmd, doneNotify("push", "pushed", r.Err, r.Aborted)), res
}

func (m Model) updatePull(msg tea.Msg) (Model, tea.Cmd, Result) {
	next, cmd, r := m.pull.Update(msg)
	m.pull = next
	res := Result{Kind: KindPull, Done: r.Done, Err: r.Err, Aborted: r.Aborted}
	if !r.Done {
		return m, cmd, res
	}
	m.output.Set("Pull output", r.Output)
	return m, tea.Batch(cmd, doneNotify("pull", "pulled", r.Err, r.Aborted)), res
}

func (m Model) updateStash(msg tea.Msg) (Model, tea.Cmd, Result) {
	next, cmd, r := m.stash.Update(msg)
	m.stash = next
	return m.mapStash(cmd, r)
}

func (m Model) mapStash(cmd tea.Cmd, r stash.Result) (Model, tea.Cmd, Result) {
	res := Result{Kind: KindStash, Done: r.Done, Err: r.Err, Outcome: r.Outcome, StagedOnly: r.StagedOnly}
	if !r.Done {
		return m, cmd, res
	}
	if r.Err != nil {
		m.output.Set("Stash output", r.Err.Error())
		return m, notify.Error("stash failed: " + r.Err.Error()), res
	}
	if r.Outcome != stash.OutcomeStashed {
		return m, nil, res
	}
	label := "stashed all changes"
	if r.StagedOnly {
		label = "stashed staged changes"
	}
	return m, notify.Success(label), res
}

func doneNotify(verb, past string, err error, aborted bool) tea.Cmd {
	if err != nil {
		return notify.Error(verb + " failed: " + err.Error())
	}
	if aborted {
		return nil
	}
	return notify.Success(past)
}

// ViewAction renders the open action modal (bump, push, pull, stash in that order).
func (m Model) ViewAction(width int) (string, bool) {
	switch {
	case m.bump.IsOpen:
		return m.bump.View(width), true
	case m.push.IsOpen:
		return m.push.View(width), true
	case m.pull.IsOpen:
		return m.pull.View(width), true
	case m.stash.IsOpen:
		return m.stash.View(width), true
	}
	return "", false
}

// ViewOutput renders the output modal.
func (m Model) ViewOutput() string { return m.output.View() }
