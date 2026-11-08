package agentfake

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// typeRaw writes s to the fake's terminal as is.
func (f *tuiFake) typeRaw(s string) {
	f.t.Helper()
	if _, err := io.WriteString(f.stdin, s); err != nil {
		f.t.Fatal(err)
	}
}

// waitKeysRead waits until the fake read s off its terminal. The loop takes
// keys one at a time, so once the last key is logged the ones before it have
// been handled.
func (f *tuiFake) waitKeysRead(s string) {
	f.t.Helper()
	f.waitFor("keys "+s, func() bool {
		data, _ := os.ReadFile(TUIInputPath(f.home, "sess-1"))
		return strings.HasSuffix(string(data), s)
	})
}

func TestTUIClaude_VimWedgeDrawsTheFooterAndTakesNoKeys(t *testing.T) {
	for _, mode := range []string{"INSERT", "NORMAL"} {
		t.Run(mode, func(t *testing.T) {
			spec := mode + ":1:" + filepath.Join(t.TempDir(), "launches")
			f := startTUIFake(t, tuiLaunchArgs, TUIVimWedgeEnv+"="+spec)
			f.waitStatus(StatusIdle, "")
			f.waitScreen("-- " + mode + " --")
			before := f.screen.String()

			f.typeRaw("x\x15y")
			f.waitKeysRead("x\x15y")
			if got := f.screen.String(); got != before {
				t.Errorf("screen changed after keys: %q, was %q", got, before)
			}

			f.typeLine("hello")
			f.waitKeysRead("hello\r")
			f.waitStatus(StatusIdle, "")
			if got := f.screen.String(); got != before {
				t.Errorf("screen changed after Enter: %q, was %q", got, before)
			}
		})
	}
}

func TestTUIClaude_VimWedgeClearsAfterNLaunches(t *testing.T) {
	spec := "INSERT:1:" + filepath.Join(t.TempDir(), "launches")
	wedged := startTUIFake(t, tuiLaunchArgs, TUIVimWedgeEnv+"="+spec)
	wedged.waitScreen("-- INSERT --")

	healthy := startTUIFake(t, tuiLaunchArgs, TUIVimWedgeEnv+"="+spec)
	healthy.waitStatus(StatusIdle, "")
	healthy.typeRaw("z")
	healthy.waitScreen("> z")
	if strings.Contains(healthy.screen.String(), "-- INSERT --") {
		t.Errorf("second launch drew the footer: %q", healthy.screen.String())
	}
	healthy.typeLine("go")
	healthy.waitStatus(StatusBusy, "")
}

func TestTUIClaude_IgnoreEditorModeDrawsInsertAndTakesKeys(t *testing.T) {
	args := append([]string{}, tuiLaunchArgs...)
	args[5] = `{"editorMode":"normal"}`
	f := startTUIFake(t, args, TUIIgnoreEditorModeEnv+"=1")
	f.waitStatus(StatusIdle, "")
	f.waitScreen("-- INSERT --")

	f.typeRaw("k")
	f.waitScreen("> k")
	f.typeLine("go")
	f.waitStatus(StatusBusy, "")
}

func TestTUIClaude_WithoutVimKnobsNoFooterIsDrawn(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	f.typeRaw("k")
	f.waitScreen("> k")
	if strings.Contains(f.screen.String(), "-- ") {
		t.Errorf("unexpected footer: %q", f.screen.String())
	}
}
