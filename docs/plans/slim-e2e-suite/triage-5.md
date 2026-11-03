# e2e triage 5: webui/e2e/agents-*.e2e.ts

Vitest dir: webui/src/agents/ (AgentsPage.{composer,sidebar,switcher,sessionView,phone,newSession}.test.tsx, AgentComposer, AgentChat, NewSessionForm, SessionActions), webui/src/design/{FailureBanner,Composer,ChatLog,DesignPage}.test.tsx, webui/src/ui/top-bar/TopBar.test.tsx, webui/src/drafts.test.ts.

| file | flow | title | verdict | reason | vitest file |
|---|---|---|---|---|---|
| agents-switcher | 84 | Session switcher | TO-VITEST | sections, Cmd/Ctrl-K, filter, current tag, Enter/click nav, Escape all already unit-tested; add filter-focus assert and the design-page Ctrl+K jump-menu check (mount route, no server) | webui/src/agents/AgentsPage.switcher.test.tsx (existing) |
| agents-commands-output | 155 | Output cut at 12 lines | KEEP-E2E | real fake-claude transcript -> server parse (ANSI strip, Caveat drop, not sent back to agent) -> chat; host of the merged flow. The 12-line cut + "Show all" alone should also get a unit test | proposed: webui/src/agents/AgentChat.test.tsx (message kind "output", 15 ANSI lines) |
| agents-commands-output | 155 | Hidden output | MERGE-INTO 155 Output cut at 12 lines | same scenario agents-commands, same reopen helper | - |
| agents-commands-output | 155 | Design page | KEEP-E2E | design agent run-end + note box over real server; host of design pair (autoAdvanceDelay 1h) | - |
| agents-commands-output | 159 | /exit, Agents page | MERGE-INTO 155 Output cut at 12 lines | real exited transition, same scenario; do it last (it ends the session) | - |
| agents-commands-output | 159 | /exit, design page | MERGE-INTO 155 Design page | same scenario and options; do it last | - |
| agents-tiers | 95 | New session tiers | KEEP-E2E | checks real launch args (--model/--effort) and resume keeps tier; tier picker UI itself covered by NewSessionForm.test.tsx | webui/src/agents/NewSessionForm.test.tsx (UI part, existing) |
| agents-tiers | 96 | Session model label | TO-VITEST | pure label + tooltip from session data (tier/model/effort) | webui/src/agents/AgentsPage.sessionView.test.tsx (existing "tier and model chips with a tier tooltip"); add 4 tier cases |
| agents-tiers | 97 | Label follows handoff | KEEP-E2E | label depends on server-side handoff after 1s auto-advance (charting-handoff) | - |
| agents-tiers | 98 | Label on interactive session | MERGE-INTO 85 Create session | creates a session in project "other" via UI; same "agents" scenario; label render covered by sessionView test | webui/src/agents/AgentsPage.sessionView.test.tsx (existing) |
| agents-create | 85 | Create session | KEEP-E2E | real create round trip (server, URL, CLI list); host for 86, 98, 90 | - |
| agents-create | 86 | Create in a new worktree | MERGE-INTO 85 Create session | real git worktree creation is the point; refusal bits (bad name, !ls keep input) already unit-tested | webui/src/agents/NewSessionForm.test.tsx (existing "a refusal shows in the form") |
| agents-create | 87 | Create entry points | TO-VITEST | prefill from "+", sidebar button, switcher row, Cmd-K keys, draft survives remount: all unit-tested | webui/src/agents/AgentsPage.newSession.test.tsx + NewSessionForm.test.tsx "draft survives a remount" (existing) |
| agents-create | 88 | Phone, Agents page | KEEP-E2E | boundingBox checks (switcher full-screen 0,0,w,h; sheet docked at bottom) need real layout; navigation half already in phone test, could drop it | webui/src/agents/AgentsPage.phone.test.tsx (nav part, existing) |
| agents-create | 90 | Agents offline | MERGE-INTO 85 Create session | real server stop is the point, but do it as the last step of 85 to share the server; Send/Create disabled offline are unit-tested | webui/src/agents/AgentComposer.test.tsx, NewSessionForm.test.tsx "disabled offline" (existing) |
| agents-commands-send | 153 | Typed once or nudged | KEEP-E2E | counts keystrokes the fake claude actually receives (/cost once, /compact starts turn, Codex $skill); host for /clear flow | - |
| agents-commands-send | 153 | Refused | DELETE-DUPLICATE | stale last_message refusal duplicates 78 Send refused; "!ls" in composer and first prompt are unit-tested | webui/src/agents/AgentsPage.composer.test.tsx ("a refusal keeps the draft", "stale refetches"), NewSessionForm.test.tsx "a refusal shows in the form" (existing) |
| agents-commands-send | 153 | /clear and Working | MERGE-INTO 153 Typed once or nudged | /clear resets open chat over real transcript; the Working-reason half duplicates 77 (vitest) | webui/src/agents/AgentsPage.composer.test.tsx "stale with a new conversation (/clear)", "Send is off while Working" (existing) |
| agents-commands-send | 154 | Design: /clear and /compact | KEEP-E2E | auto-advance deadline/Continue transitions are server state; host of 154 group | - |
| agents-commands-send | 154 | Design: keys and Type | MERGE-INTO 154 Design: /clear and /compact | same scenario+options; uses only CLI keys + countdown text | - |
| agents-commands-send | 154 | Design: Close dialog, Allow once, Deny | MERGE-INTO 154 Design: /clear and /compact | same scenario+options; overlaps 157/158 design clicks, adds "deadline unchanged" | - |
| agents-commands-send | 154 | Design: Continue with countdown off | KEEP-E2E | auto-advance toggle persists on server and gates Continue; default delay so not mergeable unless run first with option set | - |
| agents-permission | 157 | Close dialog, Agents page | KEEP-E2E | real Esc reaches fake claude, guard refuses second click; host (use autoAdvanceDelay 1h for all permission flows) | webui/src/design/FailureBanner.test.tsx covers button rendering (existing) |
| agents-permission | 157 | Close dialog, design page | MERGE-INTO 157 Close dialog, Agents page | same scenario agents-blocked; only options differ | - |
| agents-permission | 158 | Allow once/Deny, readable Claude prompt | KEEP-E2E | real Enter/Esc bytes to fake claude and state leaves blocked | - |
| agents-permission | 158 | Allow once refused (cannot approve) | MERGE-INTO 158 Allow once/Deny, readable Claude prompt | same scenario; server refusal text + bytes sent | - |
| agents-permission | 158 | Ignored Allow key not-delivered | MERGE-INTO 158 Allow once/Deny, readable Claude prompt | same scenario; waits 8s prompt grace, run it concurrently/last | - |
| agents-permission | 158 | Codex hook and screen | MERGE-INTO 158 Allow once/Deny, readable Claude prompt | real Codex hook decision channel; same scenario | - |
| agents-permission | 158 | Allow once/Deny, design page | MERGE-INTO 157 Close dialog, Agents page | design variant, same scenario with autoAdvanceDelay 1h | - |
| agents-sidebar | 72 | Agents tab | TO-VITEST | tab order, badge count text/aria-label, aria-current, deep link select, no badge when empty are render logic; also starts a second fixture ("empty") | webui/src/ui/top-bar/TopBar.test.tsx (badge, existing) + AgentsPage.sidebar.test.tsx "a click opens the session and marks its row" (existing) |
| agents-sidebar | 73 | Agents sidebar | KEEP-E2E | live SSE updates after CLI stop/remove/create are the real value; ordering/marks/tooltip already unit-tested, trim them | webui/src/agents/AgentsPage.sidebar.test.tsx "groups by project...", "shows every mark..." (existing) |
| agents-sidebar | 74 | Design sessions toggle | TO-VITEST | toggle, hidden count, remembered choice, selected keeps row all unit-tested (fresh page = localStorage) | webui/src/agents/AgentsPage.sidebar.test.tsx lines 56, 84 (existing) |
| agents-sidebar | 92 | Last viewed session | TO-VITEST | last-viewed in localStorage, redirect on #/agents, "Pick a session" all unit-tested | webui/src/agents/AgentsPage.sidebar.test.tsx lines 103, 117, 132 (existing) |
| agents-sidebar | 89 | Session not found | TO-VITEST | no-flash-before-snapshot, not-found panel, removed-while-viewed panel, Back link all unit-tested; "no other turns read" is server-side (Go test) | webui/src/agents/AgentsPage.sidebar.test.tsx lines 139, 152, 160 + AgentsPage.phone.test.tsx "gone panel" (existing) |
| agents-sidebar | 113 | Queue agents switch | TO-VITEST | same toggle logic as 74 (Queue + Design, accessible names, counts); "not counted as needing you" is a TopBar badge unit | webui/src/agents/AgentsPage.sidebar.test.tsx lines 46, 56 + TopBar.test.tsx (existing) |
| agents-session | 75 | Read a session | KEEP-E2E | scroll follow/stop-following and newest-in-viewport need real layout; unread cleared server-side on view; host for others. Header/failed-read parts already unit-tested | webui/src/agents/AgentsPage.sessionView.test.tsx (existing) |
| agents-session | 77 | Send while busy | TO-VITEST | Working reason + disabled Send + draft kept | webui/src/agents/AgentsPage.composer.test.tsx "Send is off while Working" (existing) |
| agents-session | 81 | Codex session has chat | TO-VITEST | file itself says NoChat is unit-tested; this checks Codex shows composer | webui/src/agents/AgentsPage.composer.test.tsx "a Codex session shows its chat" (existing) |
| agents-session | 82 | No messages yet | TO-VITEST | empty chat text | webui/src/agents/AgentsPage.sessionView.test.tsx "an empty chat says so" (existing) |
| agents-session | 80 | Failure notices | TO-VITEST | button sets per kind (Screen / Deny+Allow once / Retry / none), rate-limit reason, Retry posts "continue" are component logic; "No progress for 15 min" text comes from data. Keep one server-side check only if the stalled 3s threshold lacks a Go test | webui/src/design/FailureBanner.test.tsx + webui/src/agents/AgentsPage.composer.test.tsx "Retry on an auth failure" (existing) |
| agents-session | 80 | Failure notices, crashed | TO-VITEST | crashed notice + Exited composer + no Restore | webui/src/agents/AgentsPage.sessionView.test.tsx (propose new case; session {exit:"crashed", status exited}) |
| agents-session | 83 | Design session on Agents page | TO-VITEST | no sheet, no Continue, link to design page | webui/src/agents/AgentsPage.composer.test.tsx "a design session shows its plain chat..." (existing); click-navigate via routes in same test |
| agents-session | 103 | Rate-limit hold | KEEP-E2E | server-side hold clock, 60s tick, hold end after ~8s across epic/agents pages | - |
| agents-session | 104 | Usage gate and override | KEEP-E2E | Override button changes server verdict and the page reflects it | - |
| agents-keys | 156 | Key bar and Type field | KEEP-E2E | exact bytes (esc, enter, \r, arrows, ^C) reach fake claude, screen refetch; host (add Type refuses "!x" here) | - |
| agents-keys | 156 | Type refuses !x | MERGE-INTO 156 Key bar and Type field | same scenario agents-failures, same stalled session | - |
| agents-keys | 156 | Keys by session state | KEEP-E2E | no browser at all: CLI/server keys route by state; better moved to a Go server test. Host for the blocked-scenario flows | - |
| agents-keys | 156 | Queue agent shows no Screen | TO-VITEST | button absent for a Queue session | webui/src/agents/AgentsPage.sessionView.test.tsx (propose; queue:true session) |
| agents-keys | 156 | Error-blocked, Type and Enter | MERGE-INTO 156 Keys by session state | same scenario agents-blocked; real unblock transition | - |
| agents-keys | 160 | Phone, key input and Blocked notices | KEEP-E2E | boundingBox >=44px, no overflow, rows scroll sideways: real layout only | - |
| agents-composer | 76 | Send and draft | KEEP-E2E | prompt reaches the CLI chat; draft in a second tab; host for 78, 91 | webui/src/drafts.test.ts (draft part, existing) |
| agents-composer | 78 | Send refused | MERGE-INTO 76 Send and draft | real stale-conversation refusal from server; same scenario; rest unit-tested | webui/src/agents/AgentsPage.composer.test.tsx "stale refetches", "a refusal keeps the draft" (existing) |
| agents-composer | 79 | Keyboard send | TO-VITEST | Ctrl+Enter sends / narrow ignores / Enter newline / disabled does nothing | webui/src/agents/AgentComposer.test.tsx lines 138-204 (existing; matchMedia mock for 390 vs 1280) |
| agents-composer | 79 | Keyboard send, design page | TO-VITEST | unanswered-questions popover, Continue vs Send main button, key sends round: component logic | webui/src/design/Composer.test.tsx + DesignPage.test.tsx (existing); mock sendRound |
| agents-composer | 79 | Keyboard send while design works | TO-VITEST | key on disabled box does nothing | webui/src/design/Composer.test.tsx (propose: working=true, fireEvent.keyDown ctrl+Enter, expect no post) |
| agents-composer | 91 | Removed session draft | KEEP-E2E | CLI remove + cross-tab localStorage prune + fresh session starts empty; unsure drafts.test.ts covers prune-on-snapshot | webui/src/drafts.test.ts (check for prune case) |
| agents-composer | 93 | Shared draft | KEEP-E2E | two real pages sharing draft via storage events, half-answered round in new tab, Send answers to real chat | - |
| agents-composer | 114 | Read-only Queue agent | TO-VITEST | read-only line, confirm dialog, Cancel focus, turn-on enables box; "reopen is read-only" is local state | webui/src/agents/AgentsPage.composer.test.tsx "Cancel keeps a Queue agent's composer off" (existing); add confirm case |
| agents-commands | 149 | Command list freshness, refresh | KEEP-E2E | server cache age vs file written to disk vs page 60s cache (fake clock + held route) | webui/src/agents/AgentsPage.composer.test.tsx lines 306-410 cover menu states (existing) |
| agents-commands | 150 | Codex command menu | TO-VITEST | `/` built-ins only, `$` skills, fill on Enter/Tab, Claude `$` no menu, `/gx-` hint | webui/src/agents/AgentsPage.composer.test.tsx lines 425-480 (existing); mock command list fetch |
| agents-commands | 151 | Menu on design page and First prompt | TO-VITEST | menu on design Note box and First prompt; per-worktree list is a request param (mock fetch keyed on dir) | webui/src/agents/NewSessionForm.test.tsx lines 168-194 + webui/src/design/Composer.test.tsx (existing/propose) |
| agents-commands | 152 | Phone, command menu | KEEP-E2E | touch tap, two-line row height >36, menu fits in 420px short viewport, menu placement below field: real layout | - |

## Summary

Counts (65 test() blocks):
- KEEP-E2E: 20
- TO-VITEST: 24
- MERGE-INTO: 20
- DELETE-DUPLICATE: 1

Approx. savings: 45 fewer fixture+Chromium starts (24 to vitest, 20 merged, 1 deleted).

5 largest wins:
1. agents-sidebar 72/74/92/89/113 -> vitest (5 flows; 72 also starts a second fixture). Already covered by AgentsPage.sidebar.test.tsx + TopBar.test.tsx.
2. agents-permission: 7 flows on scenario agents-blocked -> 2 hosts (157 Agents page, 158 readable). Saves 5 servers; the 158 "Ignored Allow" 8s wait runs once.
3. agents-composer 79 x3 + 114 + agents-session 77/81/82/80/80-crashed/83 -> vitest (10 flows). All covered by AgentComposer.test.tsx, AgentsPage.composer.test.tsx, FailureBanner.test.tsx, design/Composer.test.tsx.
4. agents-commands 150/151 -> vitest (menu tests already in AgentsPage.composer.test.tsx / NewSessionForm.test.tsx), plus 153 Refused deleted as duplicate of 78.
5. agents-commands-output + agents-commands-send 154: 5 + 4 flows collapse to 2 hosts each (155 Output cut / 155 Design; 154 /clear / 154 Continue-off).

Notes: "156 Keys by session state" has no page use at all; port to a Go server test.
