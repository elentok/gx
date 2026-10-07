package tickets

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

// findIterationTab is a package-level seam so tests can fake herdr, same as
// reattachFindWorkspace/reattachTabList.
var findIterationTab = func(epicName, identifier string) (herdr.Tab, bool) {
	return ralphloop.FindIterationTab(herdr.FindWorkspace, herdr.TabList, epicName, identifier)
}

// ticketPaneLive reports whether a parked ticket's iteration pane still
// exists, which decides between "Answer in pane" and "Answer…". Only called
// when the menu opens, never per rendered row.
func ticketPaneLive(status tickets.RenderedStatus, epicName, identifier string) bool {
	if status != tickets.StatusNeedsAnswer {
		return false
	}
	_, live := findIterationTab(epicName, identifier)
	return live
}

// answerEditorFinishedMsg reports an "Answer…" editor launch. splitApp is set
// when the editor went into a split/tab: that message fires as soon as the
// pane exists, while the editor still runs, so no before/after comparison is
// possible and the person finishes by hand (see handleAnswerEditorFinished).
type answerEditorFinishedMsg struct {
	err          error
	splitApp     string
	path         string
	before       string
	worktreeRoot string
	settings     ui.Settings
	// resume replaces the file-write resume once the editor is done; server
	// mode sets it to ping and unpark through the server.
	resume tea.Cmd
}

// cmdAnswer opens $EDITOR at ticket path's "## Needs Answer" heading,
// snapshotting that section first for the in-place comparison.
func cmdAnswer(worktreeRoot string, settings ui.Settings, path string) tea.Cmd {
	return cmdAnswerThen(worktreeRoot, settings, path, nil)
}

func cmdAnswerThen(worktreeRoot string, settings ui.Settings, path string, resume tea.Cmd) tea.Cmd {
	raw, err := os.ReadFile(path)
	if err != nil {
		return notify.Error("answer: " + err.Error())
	}
	text := string(raw)
	editor := strings.Fields(os.Getenv("EDITOR"))
	if len(editor) == 0 {
		return notify.Warning("$EDITOR is not set")
	}
	args := ui.EditorLaunchArgs(editor[0], editor[1:], path, needsAnswerLine(text))
	before := needsAnswerSection(text)
	return terminalrun.Command(worktreeRoot, settings.Terminal, editor[0], args, func(err error, splitApp string) tea.Msg {
		return answerEditorFinishedMsg{
			err: err, splitApp: splitApp, path: path, before: before,
			worktreeRoot: worktreeRoot, settings: settings, resume: resume,
		}
	})
}

// handleAnswerEditorFinished asks what to do next once the editor exits
// in place: resume when the section changed, else offer to keep editing.
// onApplied builds the tab's own reload message after the resume write.
func handleAnswerEditorFinished(c confirm.Model, msg answerEditorFinishedMsg, onApplied func() tea.Msg) (confirm.Model, tea.Cmd) {
	if msg.err != nil {
		return c, notify.Error("answer failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return c, notify.Info("opened " + msg.splitApp + " split: edit the ticket by hand, then choose 'Resume (I answered)'")
	}
	raw, err := os.ReadFile(msg.path)
	if err != nil {
		return c, notify.Error("answer: " + err.Error())
	}
	if needsAnswerSection(string(raw)) != msg.before {
		resume := msg.resume
		if resume == nil {
			resume = cmdApplySuggestedAction(msg.path, actionResumeAnswered, onApplied)
		}
		return c.Open(confirm.Options{
			Prompt:     "Resume the ticket?",
			DefaultYes: true,
			AcceptCmd:  resume,
		}), nil
	}
	return c.Open(confirm.Options{
		Prompt:     "No answer found… Keep editing?",
		DefaultYes: true,
		AcceptCmd:  cmdAnswerThen(msg.worktreeRoot, msg.settings, msg.path, msg.resume),
	}), nil
}

// answerActionCmd dispatches the two answer menu items, which need more than
// a path (like actionInvestigate); ok is false for any other action.
func answerActionCmd(worktreeRoot string, settings ui.Settings, result actionsMenuResult) (tea.Cmd, bool) {
	switch result.Action {
	case actionAnswer:
		return cmdAnswer(worktreeRoot, settings, result.Path), true
	case actionAnswerInPane:
		return cmdFocusAnswerPane(result.EpicName, result.TicketID), true
	}
	return nil, false
}

func cmdFocusAnswerPane(epicName, identifier string) tea.Cmd {
	return func() tea.Msg {
		tab, live := findIterationTab(epicName, identifier)
		if !live {
			return notify.Warning("the ticket's pane is gone, reopen the menu to answer in the editor")()
		}
		if _, err := herdr.TabFocus(tab.TabID); err != nil {
			return notify.Error("focus pane: " + err.Error())()
		}
		return nil
	}
}

// needsAnswerSection returns text from the "## Needs Answer" heading up to
// the next "## " heading ("" when the heading is absent).
func needsAnswerSection(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != needsAnswerHeading {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "## ") {
				end = j
				break
			}
		}
		return strings.Join(lines[i:end], "\n")
	}
	return ""
}

// needsAnswerLine is the 1-based line of the heading, 0 (no line jump) when
// absent.
func needsAnswerLine(text string) int {
	for i, line := range strings.Split(text, "\n") {
		if line == needsAnswerHeading {
			return i + 1
		}
	}
	return 0
}
