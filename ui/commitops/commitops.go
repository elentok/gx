// Package commitops owns the commit actions that ui/commit and ui/log both
// write: the amend confirm, the reword flow and the commit yanks. Each parent
// handles the finish step itself (switching tabs, reloading), so the package
// imports neither page.
package commitops

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/amend"
	"github.com/elentok/gx/ui/clip"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/reword"
)

// ClipboardWrite is the clipboard hook; tests replace it.
var ClipboardWrite = clip.Write

// Kind names the flow an Update result came from.
type Kind int

const (
	KindNone Kind = iota
	KindAmend
	KindReword
)

// Result tells the parent what happened. Only meaningful when Done is true.
// Failures are already reported through the returned command; the parent
// should only run its finish step when Err is nil.
type Result struct {
	Kind       Kind
	Done       bool
	Err        error
	NewSubject string // subject of the amended or rewritten commit
}

// Model holds the amend and reword modals.
type Model struct {
	amend  amend.Model
	reword reword.Model
}

// New returns a Model with both modals closed.
func New() Model {
	return Model{amend: amend.New(), reword: reword.New()}
}

// IsOpen reports whether the amend or reword flow is running. Containers must
// keep forwarding every message while it is true, or the async step and
// spinner messages are dropped and the flow stalls.
func (m Model) IsOpen() bool { return m.amend.IsOpen || m.reword.IsOpen }

// AmendOpen reports whether the amend confirm is showing.
func (m Model) AmendOpen() bool { return m.amend.IsOpen }

// RewordOpen reports whether the reword running modal is showing.
func (m Model) RewordOpen() bool { return m.reword.IsOpen }

// OpenAmend opens the amend confirm for the commit.
func (m *Model) OpenAmend(root, hash, subject string) error {
	if hash == "" {
		return fmt.Errorf("no commit loaded")
	}
	return m.amend.Open(root, hash, subject)
}

// OpenReword opens the editor for the commit message.
func (m *Model) OpenReword(root, hash, subject, body string) tea.Cmd {
	if hash == "" {
		return notify.Warning("no commit loaded")
	}
	pushed, _ := history.IsCommitPushed(root, hash)
	cmd, err := m.reword.CmdOpenEditor(root, hash, subject, body, pushed)
	if err != nil {
		return notify.Error("reword: " + err.Error())
	}
	return cmd
}

// Update routes a message to the open modal, or finishes the editor step.
func (m Model) Update(root string, msg tea.Msg) (Model, tea.Cmd, Result) {
	if editor, ok := msg.(reword.EditorFinishedMsg); ok && !m.IsOpen() {
		cmd := m.editorDone(root, editor.Err)
		return m, cmd, Result{}
	}
	if m.amend.IsOpen {
		next, cmd, r := m.amend.Update(msg)
		m.amend = next
		if !r.Done {
			return m, cmd, Result{}
		}
		if r.Err != nil {
			return m, notify.Error("Amend failed: " + r.Err.Error()), Result{Kind: KindAmend, Done: true, Err: r.Err}
		}
		return m, cmd, Result{Kind: KindAmend, Done: true, NewSubject: m.amend.Subject}
	}
	if m.reword.IsOpen {
		next, cmd, r := m.reword.Update(msg)
		m.reword = next
		if !r.Done {
			return m, cmd, Result{}
		}
		if r.Err != nil {
			return m, notify.Error("reword failed: " + r.Err.Error()), Result{Kind: KindReword, Done: true, Err: r.Err}
		}
		return m, cmd, Result{Kind: KindReword, Done: true, NewSubject: m.reword.NewSubject}
	}
	return m, nil, Result{}
}

func (m *Model) editorDone(root string, err error) tea.Cmd {
	if err != nil {
		return notify.Error("reword: editor failed: " + err.Error())
	}
	changed, newMsg, err := m.reword.ReadEditorResult()
	if err != nil {
		return notify.Error("reword: " + err.Error())
	}
	if !changed {
		return notify.Info("reword: no changes")
	}
	cmd, err := m.reword.StartRunning(root, newMsg)
	if err != nil {
		return notify.Error("reword: " + err.Error())
	}
	return cmd
}

// Overlay draws the open modal centered over out.
func (m Model) Overlay(out string, width, height int) string {
	if m.amend.IsOpen {
		out = ui.OverlayCenter(out, m.amend.View(width), width, height)
	}
	if m.reword.IsOpen {
		out = ui.OverlayCenter(out, m.reword.View(width), width, height)
	}
	return out
}

func yank(text, done string) tea.Cmd {
	if err := ClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info(done)
}

// YankBody copies the commit body.
func YankBody(body string) tea.Cmd {
	if body == "" {
		return notify.Warning("no commit body to yank")
	}
	return yank(body, "yanked commit body")
}

// YankHash copies the full commit hash.
func YankHash(hash string) tea.Cmd {
	if hash == "" {
		return notify.Warning("no commit selected")
	}
	return yank(hash, "yanked commit hash")
}

// YankSubject copies the commit subject.
func YankSubject(subject string) tea.Cmd {
	if subject == "" {
		return notify.Warning("no commit selected")
	}
	return yank(subject, "yanked commit subject")
}

// YankMessage copies the subject, then the body after a blank line.
func YankMessage(subject, body string) tea.Cmd {
	if subject == "" {
		return notify.Warning("no commit selected")
	}
	msg := subject
	if body != "" {
		msg += "\n\n" + body
	}
	return yank(msg, "yanked commit message")
}
