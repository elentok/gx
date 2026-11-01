package agentfake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/harness"
)

func holdInputProgram(t *testing.T, env ...string) *heldProgram {
	t.Helper()
	conv := newConversationID(t)
	// The canary config says vim; gx's launch overrides it, so the test does too.
	cmd := Harness{}.Launch(harness.LaunchOptions{SessionID: conv, Args: []string{"--settings", `{"editorMode":"normal"}`}})
	return holdProgram(t, cmd, conv, append([]string{ProgramInputEnv + "=1"}, env...)...)
}

func (p *heldProgram) styled() []string {
	p.t.Helper()
	rows, err := p.client.StyledScreen()
	if err != nil {
		p.t.Fatal(err)
	}
	return rows
}

func (p *heldProgram) keys(s string) {
	p.t.Helper()
	if err := p.client.SendKeys(s); err != nil {
		p.t.Fatal(err)
	}
}

func (p *heldProgram) waitInput(want harness.InputState) {
	p.t.Helper()
	p.waitFor("the input state to be settled", func() bool {
		return Harness{}.InputIsEmpty(p.styled()) == want
	})
}

func (p *heldProgram) submitted() string {
	data, _ := os.ReadFile(ProgramSubmittedPath(p.conv))
	return string(data)
}

func TestProgramInput_EmptyBoxShowsDimPlaceholder(t *testing.T) {
	p := holdInputProgram(t)
	p.waitScreen(ProgramPlaceholder)

	if !strings.Contains(strings.Join(p.styled(), "\n"), "\x1b[2m") {
		t.Errorf("placeholder is not dim: %q", p.styled())
	}
	p.waitInput(harness.InputEmpty)
}

func TestProgramInput_TypedTextIsPlainAndLFBreaksLines(t *testing.T) {
	p := holdInputProgram(t)
	p.waitScreen(ProgramPlaceholder)

	p.keys("one\ntwo")
	p.waitScreen("  two")
	p.waitInput(harness.InputNotEmpty)
	if screen := strings.Join(p.rows(), "\n"); strings.Contains(screen, ProgramPlaceholder) || !strings.Contains(screen, "❯ one") {
		t.Errorf("screen = %q; want a two-line draft without the placeholder", screen)
	}
}

func TestProgramInput_EnterSubmitsAndLogsTheDraft(t *testing.T) {
	p := holdInputProgram(t)
	p.waitScreen(ProgramPlaceholder)

	p.keys("one\ntwo\r")
	p.waitFor("the submit", func() bool { return p.submitted() == "\"one\\ntwo\"\n" })
	p.waitInput(harness.InputEmpty)
}

func TestProgramInput_CtrlUClearsOneLinePerPress(t *testing.T) {
	p := holdInputProgram(t)
	p.waitScreen(ProgramPlaceholder)

	p.keys("a\nb\nc")
	p.waitScreen("  c")
	// 2N-1 presses clear N lines; one fewer leaves a line.
	p.keys(strings.Repeat(Harness{}.ClearKey(), 4))
	p.waitFor("one press short of empty", func() bool {
		return Harness{}.InputIsEmpty(p.styled()) == harness.InputNotEmpty && !strings.Contains(strings.Join(p.rows(), "\n"), "  c")
	})
	p.keys(Harness{}.ClearKey())
	p.waitInput(harness.InputEmpty)
	p.waitScreen(ProgramPlaceholder)
}

func TestProgramInput_SeededDraft(t *testing.T) {
	p := holdInputProgram(t, ProgramDraftEnv+"=old\ndraft")
	p.waitScreen("  draft")
	p.waitInput(harness.InputNotEmpty)
}

func TestProgramInput_SwallowedEnterKeepsTheDraft(t *testing.T) {
	p := holdInputProgram(t)
	p.waitScreen(ProgramPlaceholder)

	p.control(Control{Action: ActionSwallowEnter})
	p.control(Control{Action: ActionWrite, Output: "\r\nswallow on"})
	p.waitScreen("swallow on")
	p.keys("hi")
	p.waitScreen("❯ hi")
	p.keys("\r")
	p.keys("x")
	p.waitScreen("❯ hix")
	if got := p.submitted(); got != "" {
		t.Errorf("submitted = %q; want the first Enter swallowed", got)
	}
	p.keys("\r")
	p.waitFor("the second Enter to submit", func() bool { return p.submitted() == "\"hix\"\n" })
}

func TestProgramInput_VimNormalIgnoresKeysUntilI(t *testing.T) {
	p := holdInputProgram(t)
	p.waitScreen(ProgramPlaceholder)

	p.control(Control{Action: ActionVimNormal})
	// Controls and keys come over different channels; a later control showing
	// proves the first one applied.
	p.control(Control{Action: ActionWrite, Output: "\r\nvim on"})
	p.waitScreen("vim on")
	p.keys("hello")
	p.keys("i")
	p.waitScreen("-- INSERT --")
	p.keys("ok")
	p.waitScreen("❯ ok")
	if screen := strings.Join(p.rows(), "\n"); strings.Contains(screen, "hello") {
		t.Errorf("screen = %q; want NORMAL-mode keys not inserted", screen)
	}
}

func TestProgramInput_InsertWithoutMarkerDrawsNoFooter(t *testing.T) {
	p := holdInputProgram(t, ProgramInsertNoMarkerEnv+"=1")
	p.waitScreen(ProgramPlaceholder)

	p.keys("ok")
	p.waitScreen("❯ ok")
	if screen := strings.Join(p.rows(), "\n"); strings.Contains(screen, "INSERT") {
		t.Errorf("screen = %q; want no INSERT marker", screen)
	}
}

func TestProgramInput_PretypedLoneIAndDraft(t *testing.T) {
	lone := holdInputProgram(t, ProgramDraftEnv+"=i")
	lone.waitScreen("❯ i")
	lone.waitInput(harness.InputNotEmpty)

	draft := holdInputProgram(t, ProgramDraftEnv+"=half a thought")
	draft.waitScreen("❯ half a thought")
	draft.waitInput(harness.InputNotEmpty)
}

func TestProgramInput_ClearLeavesOneIThenSucceeds(t *testing.T) {
	p := holdInputProgram(t, ProgramClearLeavesIEnv+"=1", ProgramDraftEnv+"=draft")
	p.waitScreen("❯ draft")

	p.keys(Harness{}.ClearKey())
	p.waitScreen("❯ i")
	p.keys(Harness{}.ClearKey())
	p.waitInput(harness.InputEmpty)
	p.waitFor("both clear keys logged", func() bool {
		return p.submitted() == strings.Repeat("\""+ProgramClearEntry+"\"\n", 2)
	})
}

func TestProgramInput_ToggleIntoVimChangesScreenAndSettingsFile(t *testing.T) {
	cfg := t.TempDir()
	p := holdInputProgram(t, "CLAUDE_CONFIG_DIR="+cfg)
	p.waitScreen(ProgramPlaceholder)
	if screen := strings.Join(p.rows(), "\n"); strings.Contains(screen, "INSERT") {
		t.Fatalf("screen = %q; want normal editing before the toggle", screen)
	}

	p.control(Control{Action: ActionVimToggle})
	p.waitScreen("-- INSERT --")
	data, err := os.ReadFile(filepath.Join(cfg, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"editorMode":"vim"`) {
		t.Errorf("settings.json = %s; want editorMode vim", data)
	}
}

func TestProgramInput_DelayedReadyDropsEarlyKeys(t *testing.T) {
	p := holdInputProgram(t, ProgramReadyDelayEnv+"=500ms")
	p.keys("early")
	p.waitScreen(ProgramPlaceholder)
	if screen := strings.Join(p.rows(), "\n"); strings.Contains(screen, "early") {
		t.Errorf("screen = %q; want a key typed before ready dropped", screen)
	}
}

func TestProgramInput_ClearCommandChangesConversationWithoutATurn(t *testing.T) {
	p := holdInputProgram(t)
	p.waitScreen(ProgramPlaceholder)

	p.keys(ProgramClearCommand + "\r")
	p.waitFor("the new conversation id", func() bool { return p.watcher.ConversationID() == p.conv+ProgramClearedSuffix })
	if got := p.watcher.Status(); got.State == harness.Working {
		t.Errorf("status = %+v; want no turn", got)
	}
}
