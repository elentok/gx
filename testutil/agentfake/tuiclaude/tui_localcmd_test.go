package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
)

const echoColor = "38;2;153;153;153;48;2;55;55;55"

func (f *tuiFake) localCommands() []string { return ReadTUILocalCommands(f.home, "sess-1") }

func (f *tuiFake) waitLocal(event string) {
	f.t.Helper()
	f.waitFor("local command "+event, func() bool { return slices.Contains(f.localCommands(), event) })
}

func (f *tuiFake) boxEmpty() bool {
	return claudeui.InputBox(f.rows()) && harness.ClaudeTyping().InputIsEmpty(f.replay().StyledRows()) == harness.InputEmpty
}

func (f *tuiFake) inputLog() string {
	data, _ := os.ReadFile(TUIInputPath(f.home, "sess-1"))
	return string(data)
}

// settle returns once the fake has read the keys just sent (the tail of its
// input log) and handled one later control, so a "nothing happened" check
// looks at a finished reaction.
func (f *tuiFake) settle(keys string) {
	f.t.Helper()
	f.waitFor("keys "+keys, func() bool { return strings.HasSuffix(f.inputLog(), keys) })
	f.control(Control{Action: ActionReport, State: "working"})
}

func countOf(events []string, event string) int {
	n := 0
	for _, e := range events {
		if e == event {
			n++
		}
	}
	return n
}

func TestLocalCommand_DialogHidesTheBoxAndKeepsStatusIdle(t *testing.T) {
	for _, tt := range []struct{ command, body, footer string }{
		{"/model", "Select model", "Esc to cancel"},
		{"/help", "Help  General", "Esc to close"},
	} {
		t.Run(tt.command, func(t *testing.T) {
			f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
			f.waitStatus(StatusIdle, "")
			f.typeLine(tt.command)
			rows := f.waitRows(tt.footer)
			if claudeui.InputBox(rows) {
				t.Errorf("a dialog shows an input box: %q", rows)
			}
			if !strings.Contains(strings.Join(rows, "\n"), tt.body) {
				t.Errorf("no %q row: %q", tt.body, rows)
			}
			if got := f.styledRow("❯"); !strings.Contains(got, echoColor) || !strings.Contains(got, "255;255;255") {
				t.Errorf("echo row is not styled as claude paints it: %q", got)
			}
			if rows[0] != "❯ "+tt.command {
				t.Errorf("echo row = %q, want %q at column 0", rows[0], "❯ "+tt.command)
			}
			f.idleStatus()
			if lines := f.transcriptLines(); len(lines) != 0 {
				t.Errorf("transcript while the dialog is open = %q", lines)
			}
		})
	}
}

func TestLocalCommand_ModelDialogShowsCursorOutOfColumnZero(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
	f.waitStatus(StatusIdle, "")
	f.typeLine("/model")
	rows := f.waitRows("Esc to cancel")
	if !slices.Contains(rows, "  ❯ 5.  Haiku 5.5 ✔") {
		t.Errorf("no cursor row at column 2: %q", rows)
	}
}

func TestLocalCommand_EscClosesToTheLandedScreenAndWritesRecordsNow(t *testing.T) {
	for _, tt := range []struct{ command, result, event string }{
		{"/model", "⎿  Kept model as Haiku 5.5", eventModelKept},
		{"/help", "⎿  Help dialog dismissed", eventHelpClosed},
	} {
		t.Run(tt.command, func(t *testing.T) {
			f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
			f.waitStatus(StatusIdle, "")
			f.typeLine(tt.command)
			f.waitRows("to ")
			f.typeRaw("\x1b")
			f.waitRows(tt.result)
			f.waitLocal(tt.event)
			if !f.boxEmpty() {
				t.Errorf("the landed screen has no empty box: %q", f.rows())
			}
			if !f.hasRow("❯ " + tt.command) {
				t.Errorf("echo row not kept: %q", f.rows())
			}
			if lines := f.transcriptLines(); len(lines) != 3 {
				t.Errorf("transcript = %d lines, want the 3 local-command lines", len(lines))
			}
			f.idleStatus()
		})
	}
}

func TestLocalCommand_LandedCommandIsOnScreenButNotInTheBox(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
	f.waitStatus(StatusIdle, "")
	f.typeLine("/help")
	f.waitRows("Esc to close")
	f.typeRaw("\x1b")
	f.waitRows("Help dialog dismissed")
	rows := f.rows()
	if !strings.Contains(claudeui.Normalize(rows...), "/help") {
		t.Errorf("the command is not on screen: %q", rows)
	}
	if !f.boxEmpty() {
		t.Errorf("box is not empty: %q", rows)
	}
	f.typeLine("hello")
	f.waitStatus(StatusBusy, "")
	if f.hasRow("Help dialog dismissed") {
		t.Errorf("landed rows survive the next prompt: %q", f.rows())
	}
}

func TestLocalCommand_EnterSavesInModelAndDoesNothingInHelp(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
	f.waitStatus(StatusIdle, "")
	f.typeLine("/help")
	f.waitRows("Esc to close")
	f.typeRaw("\r")
	f.settle("/help\r\r")
	if !f.hasRow("Esc to close") || len(f.localCommands()) != 0 {
		t.Errorf("Enter acted in /help: %q %q", f.rows(), f.localCommands())
	}
	f.typeRaw("\x1b")
	f.waitLocal(eventHelpClosed)

	f.typeLine("/model")
	f.waitRows("Esc to cancel")
	f.typeRaw("\r")
	f.waitRows("⎿  Set model to Haiku 5.5")
	f.waitLocal(eventModelSaved)
	if !f.boxEmpty() {
		t.Errorf("no empty box after the save: %q", f.rows())
	}
}

func TestLocalCommand_CompactShowsARunningFrameWithAnEmptyBox(t *testing.T) {
	for _, command := range []string{"/compact", "/compact foo"} {
		t.Run(command, func(t *testing.T) {
			f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
			f.waitStatus(StatusIdle, "")
			f.typeLine(command)
			f.waitStatus(StatusBusy, "")
			rows := f.waitRows("esc to interrupt")
			if !f.hasRow("Compacting conversation…") || !f.hasRow("❯ "+command) {
				t.Errorf("no echo or spinner row: %q", rows)
			}
			if !f.boxEmpty() {
				t.Errorf("the running frame has no empty box: %q", rows)
			}
			f.typeRaw("\r")
			f.typeRaw("\r")
			f.settle(command + "\r\r\r")
			f.waitLocal(eventCompactStart)
			if n := countOf(f.localCommands(), eventCompactStart); n != 1 {
				t.Errorf("compact-start lines = %d, want 1", n)
			}
			f.control(Control{Action: ActionFinish})
			f.waitStatus(StatusIdle, "")
			rows = f.waitRows(compactDoneStdout)
			if !f.hasRow("✻ Conversation compacted") || !f.hasRow("❯ "+command) || !f.boxEmpty() {
				t.Errorf("compacted frame = %q", rows)
			}
			f.waitLocal(eventCompactDone)
			if n := countOf(f.localCommands(), eventCompactStart); n != 1 {
				t.Errorf("compact-start lines = %d, want 1", n)
			}
		})
	}
}

func TestLocalCommand_CompactDurationEndsItselfAndWithoutItOnlyAControlDoes(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, truecolorEnv, TUICompactDurationEnv+"=300ms")
	f.waitStatus(StatusIdle, "")
	f.typeLine("/compact")
	f.waitStatus(StatusBusy, "")
	f.waitRows(compactDoneStdout)
	f.waitStatus(StatusIdle, "")

	g := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
	g.waitStatus(StatusIdle, "")
	g.typeLine("/compact")
	g.waitStatus(StatusBusy, "")
	time.Sleep(600 * time.Millisecond)
	if m := g.pidFile(); m["status"] != StatusBusy {
		t.Errorf("a compaction without a duration ended: %v", m["status"])
	}
}

func TestLocalCommand_UndecidedModeKeepsTheTextAndSwallowsEveryEnter(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
	f.waitStatus(StatusIdle, "")
	f.control(Control{Action: ActionUndecided})
	f.typeRaw("/help")
	f.typeRaw("\r")
	rows := f.waitRows("  /help")
	if claudeui.InputBox(rows) {
		t.Errorf("an undecided frame has a box: %q", rows)
	}
	for _, r := range rows {
		if strings.Contains(r, "/help") && (strings.HasPrefix(r, "❯") || strings.HasPrefix(strings.TrimSpace(r), "❯")) {
			t.Errorf("our text is on an echo or menu row: %q", r)
		}
	}
	f.typeRaw("\r")
	f.typeRaw("\r")
	f.settle("/help\r\r\r")
	f.idleStatus()
	if got := f.screen.String(); strings.Contains(got, "Esc to close") {
		t.Errorf("a command ran: %q", got)
	}
	if len(f.localCommands()) != 0 || len(f.transcriptLines()) != 0 {
		t.Errorf("a command ran: %q %q", f.localCommands(), f.transcriptLines())
	}
	if got := strings.Count(f.inputLog(), "\r"); got != 3 {
		t.Errorf("the fake read %d Enters, want 3 (all swallowed)", got)
	}
}

func TestLocalCommand_UndecidedEnvArmsAtLaunch(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, truecolorEnv, TUIUndecidedEnv+"=1")
	f.waitStatus(StatusIdle, "")
	f.typeLine("/model")
	rows := f.waitRows("  /model")
	if claudeui.InputBox(rows) || len(f.localCommands()) != 0 {
		t.Errorf("undecided env did not hold: %q", rows)
	}
}
