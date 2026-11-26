package playwrightguard

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// maxFlows caps the number of Playwright flows. Every flow starts its own
// fixture server and Chromium, so the suite's run time grows with this number.
// Lower it whenever flows are removed (docs/plans/slim-e2e-suite.md aims for
// about 100). Raise it only with a reason in the commit, after trying vitest or
// a Go test (gx-only-e2es skill, "Write an e2e only when...").
// Composer image flows (+6): paste, drop and the file picker need real browser
// clipboard/drag events and a server round trip to the session's image dir,
// which jsdom tests cannot see.
// Cleanup flow (+1): the gear menu pages, the size scan, the unmerged-worktree
// removal on disk and the one-off's prompt need the real server and a git
// worktree on disk; component tests and server/cleanup tests cover each piece alone.
// Triage journey flow (+1): start, the Verify chain with its countdown, the Follow-ups
// card moving between sections and the sidebar badge need the server round trip, SSE,
// the fake Triage agent and a real browser; vitest and Go tests cannot see those together.
// Settings page flows (+2): config.json and project.json round trips through the
// real server (refused save, secrets never echoed, restart-needed, hand-written
// notifications block kept) and the 390px layout; SettingsPage.test.tsx covers each
// control alone but cannot see the files or the layout.
// Context badge flow (+1): the badge's live update, tooltip, Note placement and dark
// scheme on the Agents, Design and Triage pages need the server's transcript reads, SSE and
// a real browser; FooterLine.test.tsx and the server snapshot tests cover each part alone.
// Pause recovery tests (+7): the single "every pause reason recovers" flow was split into one
// parallel test per recovery (hung relaunch, live task, dirty worktree, Codex task, classifier
// outage, unreachable holder, process storm) so they run side by side with a short launch grace.
// Each needs a real pane, holder or process tree, which vitest and Go tests cannot stand in for.
// Dev mode flow (+1): the pill journey needs the scripted fake Vite over a real server round trip (SSE reload,
// the dev worker's unregistration in a real browser); the DevPill, DevPopover and settings row vitest tests
// cover each state alone.
// Bulk actions flow (+1): select, Move, Hold, Stop and the Archive confirm need a server round trip and real
// layout (the menu above the bar, the footer's width) that vitest cannot see; the select model, bar and page
// tests cover each rule alone.
// Chords flow (+1): split out of the Workspaces flow, which timed out at 120 s in the full run; both now fit
// their time limits with margin.
// Sidebar rows flow (+1): the Workspaces flow was still 64 s alone after the Chords split, over the
// 45 s slow limit; its self-contained sidebar-row block runs as its own flow.
// Workspaces touch flow (+1): Workspaces was still 58 s alone after the Sidebar rows split, so its narrow-width
// and footer half (swipes, 744px and 390px, New chat and New design, footer sizes) runs as its own flow.
// Schedules flow (+1): the menu entry, the list and page across fixture shapes, Run now against a real queue and
// a real delete need a server round trip and a real browser; vitest and Go tests cover each rule alone.
// Diff page flow (+1): the View diff link round trip over real git ranges, URL reload, real p/n keys, the phone
// layout, the virtualized scroll box and computed styles need a real server and browser; vitest covers each render alone.
// Hosted files wide flow (+1): the sandbox, the token redirect, the cookie, the hover card and the 30-odd link
// checks need a real server, a real iframe and real layout; vitest and Go tests (the route matrix in
// server/web/web_test.go, the detector tests) cover each rule alone but cannot see them together.
// Hosted files phone flow (+1): the bottom sheet needs the real (hover: none) media query, a touch tap and the
// clipboard in a real browser; FileLinkSheet.test.tsx (vitest) covers the sheet for a forced rule but cannot see
// the media query. It is its own flow because it needs a touch, 390px context the wide flow does not have.
// Model switch flow (+1): the picker needs the server's model query and its refresh on open, the relaunch the fake
// agent logs, the switch route's refusal and real keyboard focus; ModelPickerHost.test.tsx (vitest) covers each render alone.
// Verify finish block flow (+1): the pause after two refused finishes and Retry's fresh count need the
// server's finish path, a fake agent that keeps reporting finished and the recovery card in a real browser;
// ralphloop/finish_block_test.go and server/sched/launchtest/finish_block_test.go cover the rule alone.
// Rest screen flows (+2): the Break countdown and unlock are a real server round trip seen by two
// pages at once, and the part order and mascot size need real layout at 900px and 375px;
// RestScreen.test.tsx and the Go rest tests cover each rule alone but cannot see either.
// Task list flow (+1): the chat read from fixture transcripts and rollouts to the page, the same rows on the Design and
// Agents pages, a real clock ticking with no request, and layout at 390px need a real server and browser;
// WorkingCard.test.tsx (vitest) covers order, marks, formats, tick, details and command alone but cannot see them.
//
// Chat draft page flow (+1): the Start round trip to a new chat on the draft's exact model and effort, the redirect and
// the failed Start with its leftover worktree need a real server, the fixture's fake agents and a real page;
// DraftPage.test.tsx (vitest) and the Go Start tests cover each rule alone.
//
// Draft history popup flow (+1): the popup over the seeded lists needs a real server round trip, real layout at 1280 and 390px,
// real key handling (Leader then h) and a reload; the vitest and Go tests cover each rule alone.
//
// Initiative flows (+2): the Epics menu item, the list and its row links, the Initiative page in each state and the
// Milestone link between Epics cards, the Initiative page and the epic header need the server payload and navigation
// across pages; vitest pins every render alone.
// Epic PWA flow (+1): flow 2 ran near the 45 s slow limit with its PWA steps, so Force refresh and the cold open run
// as their own flow.
// Interrupt flow (+1): flow 75 ran over the 45 s slow limit under load, so the tools-in-flight and Interrupt steps run
// as their own flow.
// Triage split (+2): flow 199 ran over the 45 s slow limit under load, so the backlog and old-epics shapes run as their
// own flows.
// Delivery cards flow (+1): Copy needs the real clipboard, Dismiss across a second tab needs a real browser, and
// Show screen and the later good send need the fixture's fake agents; DeliveryCard.test.tsx covers each card alone.
// Question card flow (+1): the long title at 390px, the static button row and a One-off's Cancel need real layout and a
// real server, and flow 3 is near the 45 s slow limit; ActionCard.test.tsx covers each render alone.
// Answer page flows (+2): the footer outside the scroll, the history stack, a reload and the keyboard viewport need a
// real browser and a server round trip; AnswerForm.test.tsx and the route tests cover each rule alone.
// Waiting epic flow (+1): the real blocked-by-cycle refusal, the event stream moving the page, a reload-free hash
// navigation between Epic pages and the sidebar and cards reading one server state need a real server and browser;
// EpicPage.test.tsx covers the chips and picker alone.
// Worktree diff flows (+3): the real worktree on disk (status and index left untouched, the committed and uncommitted
// files, the stacked and paused bases), the server's snapshot and the Show diff action over the browser need a real
// fixture server; DiffPage.test.tsx and the server tests cover each piece alone.
//
// Subagent page flow (+1): the chat read from fixture transcripts, meta.json files and rollouts to the page, the journey
// across the Agents, Design and subagent pages with the browser Back button and a reload, a live file growth seen with
// no reload and layout at 390px need a real server and browser; SubagentPage.test.tsx, AgentsPage.sessionView.test.tsx,
// WorkingCard.test.tsx, ChatLog.test.tsx and the Go server tests cover each rule alone but cannot see them together.
//
// Nav buttons flow (+1): history moves, reload, three layouts, hover in WebKit and a touch tap need a real browser;
// component tests cover each rule alone.
//
// Nav keys flow (+1): real chord keys through history, focus, sidebar scroll, new tabs and a reload need a real
// browser; ChordHost and nextPrev tests cover each rule alone; split from Nav buttons for the time limit.
//
// Nav current row flow (+1): the last row across a reload, new tabs and a workspace switch need a
// real browser; nextPrev tests cover each rule alone.
//
// Working card states flow (+1): fixture transcripts to server to page, including a result appended to a subagent file;
// webui/src/design/WorkingCard.test.tsx renders given props and cannot see that.
const maxFlows = 174

var flowStart = regexp.MustCompile(`(?m)^\s*test\(`)

func TestFlowBudget(t *testing.T) {
	files, err := filepath.Glob("../webui/e2e/*.e2e.ts")
	if err != nil || len(files) == 0 {
		t.Fatalf("no flow files found: %v", err)
	}
	total := 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		total += len(flowStart.FindAll(src, -1))
	}
	if total > maxFlows {
		t.Fatalf("%d e2e flows, budget is %d. Cover the new check with a vitest or Go test, or add it as a step in an existing flow. Load the gx-only-e2es skill (.claude/skills/gx-only-e2es/SKILL.md).", total, maxFlows)
	}
}
