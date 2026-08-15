package ralphloop

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// isValidTelegramMarkdownV2 lives in markdown_validity_test.go, with its own
// independently-written special-character list — importable from both this
// e2e test and notification_markdownv2_validity_test.go's fast unit tests.

// fakeValidatingTelegramServer starts an httptest.Server standing in for the
// Telegram Bot API that, like the real API, rejects (400) any sendMessage
// body whose text isn't valid MarkdownV2 — so a notification-text call site
// that forgets to escape a literal (the exact bug this test guards against;
// see follow-ups/issues/07-iteration-finished-cost-decimal-unescaped-markdownv2.md)
// gets caught the same way a live Telegram send would catch it, not by
// re-deriving the expected escaped string by hand.
func fakeValidatingTelegramServer(t *testing.T) (*httptest.Server, func() []telegramRequest) {
	t.Helper()
	var mu sync.Mutex
	var requests []telegramRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req telegramRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		if !isValidTelegramMarkdownV2(req.Text) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	return server, func() []telegramRequest {
		mu.Lock()
		defer mu.Unlock()
		out := make([]telegramRequest, len(requests))
		copy(out, requests)
		return out
	}
}

// TestRun_FullEpicRealGit_TelegramNotifications_AllValidMarkdownV2 drives a
// full two-ticket epic (01 -> 02, sequential so both an iteration-started
// and iteration-finished land for each before the run completes) through
// Run with production Git/herdr wiring and a real chatEventSink/
// telegramTransport pointed at a fake Telegram server, so every chat
// notification a real epic run emits (epic started, iteration started ×2,
// iteration finished ×2, epic complete) is rendered and sent through the
// exact same code path a live run uses. The fake server independently
// validates MarkdownV2 the way the real Telegram API would, so this test
// fails if any notification-text call site emits an unescaped reserved
// character — the class of bug (unescaped "." in a cost) that silently
// dropped every ticket-completion message on a real bugs-07 run before
// notification_text.go's counts/detail strings were escaped.
func TestRun_FullEpicRealGit_TelegramNotifications_AllValidMarkdownV2(t *testing.T) {
	// not parallel-safe: herdrfake.Start calls t.Setenv for the helper socket
	// path and PATH.
	realGitTimeoutWatchdog(t, realGitTestTimeout)
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)
	wtDir := testWorktreeDir(t, repoDir)

	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# B\n",
	})

	home := t.TempDir()
	transcriptStart := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	for _, id := range []string{"01", "02"} {
		writeFakeTranscript(t, home, iterationWorktreePath(wtDir, epicName, id), "sess-"+iterLabel(epicName, id), transcriptStart,
			[3]any{"claude-sonnet-5", 900, 0},
			[3]any{"claude-sonnet-5", 1000, 100},
		)
	}

	var mu sync.Mutex
	status := map[string]string{}
	session := map[string]string{}

	handler := func(argv []string) ([]byte, int) {
		if len(argv) < 2 {
			return herdrfake.CommandError(fmt.Sprintf("command too short: %v", argv))
		}
		switch argv[0] + " " + argv[1] {
		case "workspace list":
			return herdrfake.Result(map[string]any{"workspaces": []any{}})
		case "workspace create":
			return herdrfake.Result(map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}})
		case "tab create":
			label := realGitFlagValue(argv, "--label")
			tabID, pane := "tab-"+label, "pane-"+label
			return herdrfake.Result(map[string]any{
				"tab":       map[string]any{"tab_id": tabID, "label": label, "workspace_id": realGitFlagValue(argv, "--workspace")},
				"root_pane": map[string]any{"pane_id": pane},
			})
		case "tab close":
			return []byte(`{"result":null}`), 0
		case "tab list":
			return herdrfake.Result(map[string]any{"tabs": []any{}})
		case "agent start":
			pane := realGitFlagValue(argv, "--pane")
			sess := "sess-" + argv[2]
			mu.Lock()
			status[pane] = "idle"
			session[pane] = sess
			mu.Unlock()
			return agentJSON(pane, "idle", sess)
		case "agent prompt":
			pane, text := argv[2], argv[3]
			mu.Lock()
			sess := session[pane]
			mu.Unlock()
			if id, ok := ticketIDFromImplementPrompt(text); ok {
				dir := iterationWorktreePath(wtDir, epicName, id)
				if err := commitIterationWork(dir, id); err != nil {
					t.Errorf("commitIterationWork(%s): %v", id, err)
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				status[pane] = "idle"
				mu.Unlock()
			}
			return agentJSON(pane, "working", sess)
		case "agent wait":
			pane := argv[2]
			until := parseUntil(argv[3:])
			mu.Lock()
			cur, sess := status[pane], session[pane]
			mu.Unlock()
			if len(until) == 0 || slices.Contains(until, cur) {
				return agentJSON(pane, cur, sess)
			}
			return herdrfake.CommandError("timed out waiting for agent status")
		case "agent send-keys":
			pane := argv[2]
			mu.Lock()
			cur, sess := status[pane], session[pane]
			mu.Unlock()
			return agentJSON(pane, cur, sess)
		case "agent read":
			return []byte(""), 0
		default:
			return herdrfake.CommandError("unimplemented command: " + argv[0] + " " + argv[1])
		}
	}

	herdrfake.Start(t, handler)

	deps := testDepsWithOverrides(DepsOverrides{Home: home})
	deps.Sleep = func(time.Duration) {}
	deps.VerifySkill = func(AgentKind, string) error { return nil }

	server, getRequests := fakeValidatingTelegramServer(t)
	sink := newTelegramEventSink(noopEventSink{}, "test-token", "chat-1", server.URL, scratchDir, epicName)
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	if err := Run(RunOptions{EpicName: epicName, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir}, deps, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	sink.Close()

	requests := getRequests()
	if len(requests) == 0 {
		t.Fatalf("telegram requests = 0, want at least 1")
	}
	joined := ""
	for _, req := range requests {
		joined += req.Text + "\n"
	}
	for _, want := range []string{"epic started", "epic complete"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no request text contained %q; requests = %#v", want, requests)
		}
	}
	for _, emoji := range []string{"\U0001f680", "▶️", "✅", "\U0001f389"} { // epic-started, iteration-started, iteration-finished, epic-complete
		if !strings.Contains(joined, emoji) {
			t.Errorf("no request text contained emoji %q; requests = %#v", emoji, requests)
		}
	}
	for _, id := range []string{"01", "02"} {
		if !strings.Contains(joined, epicName+"/"+id) {
			t.Errorf("no request text contained identity line for ticket %s; requests = %#v", id, requests)
		}
	}

	events, ok, err := ReadEvents(scratchDir, epicName)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if !ok {
		t.Fatalf("ReadEvents ok = false, want run-log.jsonl to exist")
	}
	for _, ev := range events {
		if ev.Type == eventNotificationFailed {
			t.Errorf("unexpected notification-failed event: %+v", ev)
		}
	}
}
