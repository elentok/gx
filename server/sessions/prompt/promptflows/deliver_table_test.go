package promptflows_test

import (
	"slices"
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

func (c tuiChat) control(t *testing.T, ctl agentfake.Control) {
	t.Helper()
	if err := agentfake.SendTUIControl(c.home, c.conv, ctl); err != nil {
		t.Fatal(err)
	}
}

func (c tuiChat) swallowFirstEnter(t *testing.T) {
	t.Helper()
	c.control(t, agentfake.Control{Action: agentfake.ActionSwallowEnter})
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
	t.Run("ReadScreens", func(t *testing.T) {
		t.Parallel()
		long := "/compact " + strings.Repeat("keep the plan and the notes ", 25)
		for _, r := range []struct {
			name, text string
			env        []string
			setup      func(t *testing.T, c tuiChat)
			enters     int
		}{
			{"IdleEmptyWithPlaceholder", "/cost", nil, nil, 2},
			{"TypedWithSuggestionMenu", "/copy", nil, nil, 2},
			{"ModelDialogOpen", "/model", nil, nil, 1},
			{"HelpDialogOpen", "/help", nil, nil, 1},
			{"CompactRunning", "/compact", nil, nil, 1},
			{"CompactDone", "/compact", []string{agentfake.TUICompactDurationEnv + "=300ms"}, nil, 1},
			{"WrappedMultiRowDraft", "/cost " + strings.Repeat("word ", 40), nil, nil, 2},
			{"PasteOver512Bytes", long, nil, nil, 1},
			// A person's draft is cleared before the send types, so the settle read
			// meets only our text.
			{"PersonsOwnDraftIsClearedFirst", "/cost", []string{agentfake.TUIDraftEnv + "=my own words"}, nil, 2},
			// The dialog's "❯ 1. Yes" sits inside rules but is no input box: no settle Enter.
			{"PermissionDialog", "/cost", nil, func(t *testing.T, c tuiChat) {
				c.control(t, agentfake.Control{Action: agentfake.ActionBlock, Dialog: agentfake.BashPermissionDialog("rg x")})
			}, 1},
		} {
			t.Run(r.name, func(t *testing.T) {
				t.Parallel()
				c := startTUIChat(t, r.env...)
				if r.setup != nil {
					r.setup(t, c)
				}
				c.send(t, r.text)
				c.expectEnters(t, r.enters)
			})
		}
	})
	t.Run("Undecided", func(t *testing.T) {
		t.Parallel()
		// The plain screen has no input box: our text stays visible on a row that is
		// neither the echo nor a menu row and the Enter changes nothing, so the settle
		// read cannot tell whether the command ran. Each entry point maps it its own way.
		const maybe = "May not have been sent: "
		for _, r := range []struct {
			name   string
			run    func(t *testing.T, c tuiChat)
			suffix string
			typed  string
		}{
			{"ChatRaisesTheCard", func(t *testing.T, c tuiChat) { c.send(t, "/cost") }, "", "/cost"},
			{"RenameRaisesTheCard", func(t *testing.T, c tuiChat) {
				if _, err := c.h.Client.Sessions.Rename(c.ctx, wire.SessionRenameRequest{ID: c.id, Name: "new name"}); err != nil {
					t.Fatal(err)
				}
			}, "", "/rename new name"},
			{"ActionStepRaisesTheCardAndDropsLaterSteps", func(t *testing.T, c tuiChat) { c.runAction(t, "/first", "second") }, "1 later step not sent", "/first"},
		} {
			t.Run(r.name, func(t *testing.T) {
				t.Parallel()
				c := startTUIChat(t, agentfake.TUIPlainScreenEnv+"=1")
				r.run(t, c)
				got := c.card(t)
				if !strings.HasPrefix(got.Reason, maybe) || !strings.HasSuffix(got.Reason, r.suffix) || len(got.Action) != 0 {
					t.Errorf("card = %+v, want the May not have been sent card with no Action to resend", got)
				}
				c.expectEnters(t, 1)
				c.expectTypedOnce(t, r.typed)
				if strings.Contains(c.keys(), "second") {
					t.Errorf("keys = %q, want the later step dropped", c.keys())
				}
			})
		}
		t.Run("FirstPromptRaisesNoCard", func(t *testing.T) {
			t.Parallel()
			c := startTUIChatPrompt(t, "/first", agentfake.TUIPlainScreenEnv+"=1")
			c.waitKeys(t, "the prompt", func(k string) bool { return strings.Contains(k, "/first") })
			if servertest.Poll(6*time.Second, func() bool { return servertest.ReadMeta(t, c.h, c.id)["actionable"] != nil }) {
				t.Errorf("actionable = %v, want none for a first prompt", servertest.ReadMeta(t, c.h, c.id)["actionable"])
			}
			c.expectEnters(t, 1)
		})
	})
	t.Run("Exits", func(t *testing.T) {
		t.Parallel()
		// Observed reasons: the first prompt gives up with "agent never started", not
		// "Send failed" (see Spec deviations on the ticket).
		for _, r := range []struct {
			name   string
			env    []string
			run    func(t *testing.T, c tuiChat)
			reason string
			action []string
		}{
			{"PlainGiveUp", []string{agentfake.TUISwallowEnterEnv + "=100"}, func(t *testing.T, c tuiChat) { c.send(t, "hello") }, "Send failed: timed out waiting for agent to start", []string{"hello"}},
			{"DraftWillNotClear", []string{agentfake.TUIClearIgnoredEnv + "=1", agentfake.TUIDraftEnv + "=mine"}, func(t *testing.T, c tuiChat) { c.send(t, "hello") }, "Send failed: input would not clear", []string{"hello"}},
			{"ActionStepGiveUp", []string{agentfake.TUISwallowEnterEnv + "=100"}, func(t *testing.T, c tuiChat) { c.runAction(t, "hello", "second") }, "Action failed: step 0: timed out waiting for agent to start", []string{"hello", "second"}},
		} {
			t.Run(r.name, func(t *testing.T) {
				t.Parallel()
				c := startTUIChat(t, r.env...)
				r.run(t, c)
				got := c.card(t)
				if got.Reason != r.reason || !slices.Equal(got.Action, r.action) {
					t.Errorf("card = %+v, want reason %q and Action %q", got, r.reason, r.action)
				}
			})
		}
		t.Run("FirstPromptGivesUp", func(t *testing.T) {
			t.Parallel()
			c := startTUIChatPromptWith(t, "fix it", nil, agentfake.TUISwallowEnterEnv+"=100")
			got := c.card(t)
			if !strings.HasPrefix(got.Reason, "agent never started") || !slices.Equal(got.Action, []string{"fix it"}) {
				t.Errorf("card = %+v, want the agent never started card with the prompt to resend", got)
			}
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
			// The retype comes every RetypeEvery inside the Window.
			c := startTUIChatPromptWith(t, "/my-skill", func(cfg *server.Config) {
				cfg.LaunchTimings.Window = 2 * time.Second
				cfg.LaunchTimings.RetypeEvery = 300 * time.Millisecond
			}, agentfake.TUIUnknownSkillEnv+"=1")
			got := c.card(t)
			if !strings.Contains(got.Reason, "Unknown command") || len(got.Action) != 1 || got.Action[0] != "/my-skill" {
				t.Errorf("card = %+v, want a card naming Unknown command with the text to resend", got)
			}
			if n := strings.Count(c.keys(), "/my-skill"); n < 2 {
				t.Errorf("typed %d times, want a retype inside the window: %q", n, c.keys())
			}
		})
		t.Run("CodexLaunchPromptRetypesForTheWindowThenEndsInTheCard", func(t *testing.T) {
			t.Parallel()
			f := fromPrompt(rigs.StartCodexPromptSessionAs(t, func(cfg *server.Config) {
				cfg.LaunchTimings.Window = 2 * time.Second
				cfg.LaunchTimings.RetypeEvery = 300 * time.Millisecond
			}, wire.CreateSessionRequest{Prompt: "/handoff"}))
			got := waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.GxRaised }).Actionable
			if !strings.Contains(got.Reason, "Unrecognized command") || len(got.Action) != 1 || got.Action[0] != "/handoff" {
				t.Errorf("card = %+v, want a card naming Unrecognized command with the text to resend", got)
			}
			if n := strings.Count(submitted(t, f.conv), "/handoff"); n < 2 {
				t.Errorf("submitted %d times, want a retype inside the window: %q", n, submitted(t, f.conv))
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
