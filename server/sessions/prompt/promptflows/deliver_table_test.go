package promptflows_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// expectEnters waits for want Enters at the fake, then holds still for a few
// grace windows: a stacked Enter or a retype would come within them.
func (c tuiChat) expectEnters(t *testing.T, want int) {
	t.Helper()
	c.waitKeys(t, "the Enters", func(k string) bool { return strings.Count(k, "\r") >= want })
	time.Sleep(3 * firstPromptGrace) // negative wait: an extra Enter would come within it
	if got := strings.Count(c.keys(), "\r"); got != want {
		t.Errorf("fake read %d Enters, want %d: %q", got, want, c.keys())
	}
}

func (c tuiChat) expectTypedOnce(t *testing.T, text string) {
	t.Helper()
	if n := strings.Count(c.keys(), text); n != 1 {
		t.Errorf("typed %q %d times, want once: %q", text, n, c.keys())
	}
}

func (c tuiChat) expectCompactions(t *testing.T, want int) {
	t.Helper()
	got := 0
	for _, e := range agentfake.ReadTUILocalCommands(c.home, c.conv) {
		if e == "compact-start" {
			got++
		}
	}
	if got != want {
		t.Errorf("compactions = %d, want %d", got, want)
	}
}

func (c tuiChat) card(t *testing.T) wire.SessionActionable {
	t.Helper()
	return servertest.WaitForSession(c.ctx, t, c.h, c.evs, c.id, func(s wire.SessionInfo) bool { return s.Actionable.GxRaised }).Actionable
}

func (c tuiChat) swallowFirstEnter(t *testing.T) {
	t.Helper()
	if err := agentfake.SendTUIControl(c.home, c.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
		t.Fatal(err)
	}
}

// TestDeliverTable drives every commit-1 entry point through the server against
// the Claude TUI fake and checks what a person sees: the bytes and Enters the
// fake read, how often a command ran, and which card the session carries. The
// table is split by what it pins; one Test func holds all of them, the package
// being close to its Test-func limit.
func TestDeliverTable(t *testing.T) {
	t.Parallel()
	t.Run("EntryPoints", func(t *testing.T) {
		t.Parallel()
		for _, r := range []struct {
			name   string
			run    func(t *testing.T, c tuiChat)
			enters int
		}{
			{"ChatPlainSend", func(t *testing.T, c tuiChat) {
				c.send(t, "hello there")
				c.waitKeys(t, "the text", func(k string) bool { return strings.Contains(k, "hello there") })
			}, 1},
			{"ChatSlashSend", func(t *testing.T, c tuiChat) {
				c.send(t, "/cost")
				c.waitKeys(t, "the text", func(k string) bool { return strings.Contains(k, "/cost") })
			}, 2},
			{"CompactSend", func(t *testing.T, c tuiChat) {
				c.send(t, "/compact")
				c.waitKeys(t, "the compaction", func(string) bool { return len(agentfake.ReadTUILocalCommands(c.home, c.conv)) > 0 })
			}, 1},
			{"CompactWithArgumentsSend", func(t *testing.T, c tuiChat) {
				c.send(t, "/compact keep the plan")
				c.waitKeys(t, "the compaction", func(string) bool { return len(agentfake.ReadTUILocalCommands(c.home, c.conv)) > 0 })
			}, 1},
			{"Rename", func(t *testing.T, c tuiChat) {
				if _, err := c.h.Client.Sessions.Rename(c.ctx, wire.SessionRenameRequest{ID: c.id, Name: "new name"}); err != nil {
					t.Fatal(err)
				}
				c.waitKeys(t, "the rename", func(k string) bool { return strings.Contains(k, "/rename") })
			}, 2},
			{"ActionPlainStep", func(t *testing.T, c tuiChat) {
				c.runAction(t, "fix the bug")
				c.waitKeys(t, "the step", func(k string) bool { return strings.Contains(k, "fix the bug") })
			}, 1},
			{"ActionSlashStep", func(t *testing.T, c tuiChat) {
				c.runAction(t, "/cost")
				c.waitKeys(t, "the step", func(k string) bool { return strings.Contains(k, "/cost") })
			}, 2},
		} {
			t.Run(r.name, func(t *testing.T) {
				t.Parallel()
				c := startTUIChat(t)
				r.run(t, c)
				c.expectEnters(t, r.enters)
			})
		}
	})
	t.Run("FirstPrompt", func(t *testing.T) {
		t.Parallel()
		for _, r := range []struct {
			name, prompt string
			enters       int
		}{
			{"Plain", "fix the bug", 1},
			{"Slash", firstSlash, 2},
		} {
			t.Run(r.name, func(t *testing.T) {
				t.Parallel()
				c := startTUIChatPrompt(t, r.prompt)
				c.waitKeys(t, "the prompt", func(k string) bool { return strings.Contains(k, r.prompt) })
				c.expectEnters(t, r.enters)
				c.expectTypedOnce(t, r.prompt)
			})
		}
	})
	t.Run("SwallowedEnter", func(t *testing.T) {
		t.Parallel()
		// A hidden character in the first 20 characters: gx strips U+200B; U+1BCA0
		// is outside gx's set and the fake's box cleans it out. Either way the
		// command runs once, with the settle Enter at most one extra.
		for _, r := range []struct {
			name, text string
			enters     int
		}{
			{"ZeroWidthSpace", "/co​st", 2},
			{"HiddenCharacterOutsideGxStripSet", "/co\U0001BCA0st", 2},
		} {
			t.Run(r.name, func(t *testing.T) {
				t.Parallel()
				c := startTUIChat(t)
				c.send(t, r.text)
				c.expectEnters(t, r.enters)
				if strings.Contains(c.keys(), "​") {
					t.Errorf("keys = %q, want no zero-width space", c.keys())
				}
			})
		}
		t.Run("CompactWithTheFirstEnterSwallowed", func(t *testing.T) {
			t.Parallel()
			c := startTUIChat(t)
			c.swallowFirstEnter(t)
			c.send(t, "/compact")
			c.expectEnters(t, 2)
			c.expectCompactions(t, 1)
		})
	})
	t.Run("Strip", func(t *testing.T) {
		t.Parallel()
		const family = "👨‍👩‍👧"
		t.Run("EmojiZWJInsideACommandStays", func(t *testing.T) {
			t.Parallel()
			c := startTUIChat(t)
			text := "/compact " + family
			c.send(t, text)
			c.waitKeys(t, "the compaction", func(string) bool { return len(agentfake.ReadTUILocalCommands(c.home, c.conv)) > 0 })
			c.expectEnters(t, 1)
			c.expectTypedOnce(t, text)
			c.expectCompactions(t, 1)
		})
		t.Run("ZWJAtTheEdgeIsDropped", func(t *testing.T) {
			t.Parallel()
			c := startTUIChat(t)
			c.send(t, "/compact‍")
			c.waitKeys(t, "the compaction", func(string) bool { return len(agentfake.ReadTUILocalCommands(c.home, c.conv)) > 0 })
			c.expectEnters(t, 1)
			if strings.Contains(c.keys(), "‍") {
				t.Errorf("keys = %q, want the edge ZWJ dropped", c.keys())
			}
			c.expectCompactions(t, 1)
		})
	})
	t.Run("UnknownCommand", func(t *testing.T) {
		t.Parallel()
		t.Run("ChatSlashFailsAtOnceWithoutARetype", func(t *testing.T) {
			t.Parallel()
			c := startTUIChat(t, agentfake.TUIUnknownSkillEnv+"=1")
			c.send(t, "/my-skill")
			got := c.card(t)
			if !strings.HasPrefix(got.Reason, "Send failed: ") || !strings.Contains(got.Reason, "Unknown command") || len(got.Action) != 1 || got.Action[0] != "/my-skill" {
				t.Errorf("card = %+v, want the red Send failed card with the text to resend", got)
			}
			c.expectEnters(t, 1)
			c.expectTypedOnce(t, "/my-skill")
		})
		t.Run("LaunchPromptRetypesForTheWindowThenEndsInTheCard", func(t *testing.T) {
			t.Parallel()
			c := startTUIChatPromptWith(t, "/my-skill", func(cfg *server.Config) { cfg.LaunchProofCaps.UnknownGrace = 2 * time.Second }, agentfake.TUIUnknownSkillEnv+"=1")
			got := c.card(t)
			// Observed: the launch proof ends the window in the "prompt not delivered"
			// card with no retype of the text (see Spec deviations on the ticket).
			if !strings.Contains(got.Reason, "Unknown command") || len(got.Action) != 1 || got.Action[0] != "/my-skill" {
				t.Errorf("card = %+v, want a card naming Unknown command with the text to resend", got)
			}
		})
	})
	t.Run("Codex", func(t *testing.T) {
		t.Parallel()
		// A slash send types once with one Enter and no settle Enter; a plain one too.
		for _, r := range []struct{ name, text string }{{"SlashIsOneSubmission", "/status"}, {"PlainIsOneSubmission", "fix the bug"}} {
			t.Run(r.name, func(t *testing.T) {
				t.Parallel()
				f := fromPrompt(rigs.StartCodexPromptSession(t))
				if res := f.prompt(t, r.text); res.Refused {
					t.Fatalf("prompt = %+v", res)
				}
				want := strconv.Quote(r.text) + "\n"
				waitForSubmitted(t, f.conv, want)
				time.Sleep(3 * firstPromptGrace) // negative wait: a settle Enter or a second submission would come within it
				if got := submitted(t, f.conv); got != want {
					t.Errorf("submitted = %q, want one submission", got)
				}
			})
		}
		t.Run("UnrecognizedCommandFailsTheSend", func(t *testing.T) {
			t.Parallel()
			f := fromPrompt(rigs.StartCodexPromptSession(t))
			if res := f.prompt(t, "/handoff"); res.Refused {
				t.Fatalf("prompt = %+v", res)
			}
			got := waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.GxRaised }).Actionable
			if !strings.HasPrefix(got.Reason, "Send failed: ") || !strings.Contains(got.Reason, "Unrecognized command") || len(got.Action) != 1 || got.Action[0] != "/handoff" {
				t.Errorf("card = %+v, want the red Send failed card with the text to resend", got)
			}
		})
	})
}
