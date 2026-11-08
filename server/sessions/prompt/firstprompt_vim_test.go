package prompt_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/transcript"
)

const vimEvent = "shown after an editorMode normal launch"

func (f launchFixture) vimEvents() int {
	data, _ := os.ReadFile(server.LogPath(f.h.StateDir))
	return strings.Count(string(data), vimEvent)
}

// typedIn counts the launch command records in conversation conv's transcript.
func (f launchFixture) typedIn(conv string) int {
	msgs, _, _ := transcript.ReadChat(transcript.PathIn(f.home, f.dir, conv), "")
	n := 0
	for _, m := range msgs {
		if m.Text == launchPrompt {
			n++
		}
	}
	return n
}

func (f launchFixture) currentConv(t *testing.T) string {
	t.Helper()
	conv, _ := readMeta(t, f.h, f.id)["conversation_id"].(string)
	return conv
}

func wedge(t *testing.T, mode string, launches int) string {
	return agentfake.TUIVimWedgeEnv + "=" + mode + ":" + strconv.Itoa(launches) + ":" + filepath.Join(t.TempDir(), "launches")
}

func TestFirstPromptVim_FooterOnceRestartsOnceAndTypesInTheNewConversation(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"INSERT", "NORMAL"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			f := startLaunchSession(t, wedge(t, mode, 1))
			first := f.conv
			servertest.WaitFor(t, 30*time.Second, "the prompt typed in the new conversation", func() bool {
				conv := f.currentConv(t)
				return conv != first && f.typedIn(conv) > 0
			})
			if n := f.typedIn(first); n != 0 {
				t.Errorf("prompt typed in the first conversation %d times, want 0", n)
			}
			meta := readMeta(t, f.h, f.id)
			if meta["id"] != f.id || meta["run"].(float64) < 2 {
				t.Errorf("meta = %v, want the same id and a new run", meta)
			}
			if n := f.vimEvents(); n != 1 {
				t.Errorf("vim events = %d, want 1", n)
			}
			f.assertNoActionable(t)
		})
	}
}

func TestFirstPromptVim_FooterOnBothLaunchesLogsTwiceAndEndsInTheExistingActionable(t *testing.T) {
	t.Parallel()
	f := startLaunchSession(t, wedge(t, "INSERT", 2))
	got := f.gxActionable(t)
	if !strings.Contains(got.Reason, "prompt not delivered") || len(got.Action) != 1 || got.Action[0] != launchPrompt {
		t.Errorf("actionable = %+v, want the prompt-not-delivered Actionable with the prompt as its Action", got)
	}
	if n := f.vimEvents(); n != 2 {
		t.Errorf("vim events = %d, want 2", n)
	}
	if run := readMeta(t, f.h, f.id)["run"].(float64); run != 2 {
		t.Errorf("run = %v, want exactly one restart", run)
	}
}

func TestFirstPromptVim_NoFooterMeansNoRestartAndNoEvent(t *testing.T) {
	t.Parallel()
	f := startLaunchSession(t)
	f.waitForUserRecord(t)
	if n := f.vimEvents(); n != 0 {
		t.Errorf("vim events = %d, want 0", n)
	}
	if conv := f.currentConv(t); conv != f.conv {
		t.Errorf("conversation = %s, want the first (%s)", conv, f.conv)
	}
}

func TestFirstPromptVim_CodexSessionShowsNoCheck(t *testing.T) {
	t.Parallel()
	h, _, _, id, _ := startFakeSessionWith(t, wire.CreateSessionRequest{Harness: "codex", Prompt: "fix the bug"},
		servertest.WithEnv("PATH="+servertest.FakeCodexDir(t)+string(os.PathListSeparator)+os.Getenv("PATH")))
	runBefore := readMeta(t, h, id)["run"]
	logPath := server.LogPath(h.StateDir)
	servertest.WaitFor(t, 30*time.Second, "the first prompt typed", func() bool {
		data, _ := os.ReadFile(logPath)
		return strings.Contains(string(data), "first prompt typed")
	})
	data, _ := os.ReadFile(logPath)
	if strings.Contains(string(data), vimEvent) {
		t.Error("vim event logged for a Codex session")
	}
	if got := readMeta(t, h, id)["run"]; got != runBefore {
		t.Errorf("run = %v, want %v: a Codex session is never restarted", got, runBefore)
	}
}
