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
// a Go test (skills/gx-playwright-e2es, "Write an e2e only when...").
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
const maxFlows = 140

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
		t.Fatalf("%d e2e flows, budget is %d. Cover the new check with a vitest or Go test, or add it as a step in an existing flow. See skills/gx-playwright-e2es.", total, maxFlows)
	}
}
