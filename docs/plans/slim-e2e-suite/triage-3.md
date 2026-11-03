# E2E triage 3: menu, united, epics, pause, codex

Vitest files named below are under webui/src. "Existing" = already has a matching test; "proposed" = would host it.

| file | flow | title | verdict | reason | vitest file |
|---|---|---|---|---|---|
| menu | 117a | Limits block: rows, live update, tooltip, narrow | KEEP-E2E (trim) | Keep only: statusline CLI report reaches the open menu over SSE. Rows, tones, order, hover tooltip are render-only | usage/LimitsBlock.test.tsx, usage/UsageMenuBlock.test.tsx (existing: fresh rows, order, tooltip) |
| menu | 117b | Limits tooltip opens on tap | TO-VITEST | Pure tooltip open logic; no layout needed | usage/LimitsBlock.test.tsx (existing "tooltip opens on focus"; add click/tap case) |
| menu | 117c | Limits 320px menu stays in viewport | MERGE-INTO 180a | Same menu container; only the widest menu (Claude+Codex) needs the real-layout check, run it with setViewportSize inside 180a | none (real layout) |
| menu | 119a | Spending block: normal | MERGE-INTO 120 | Same `spend`/`normal` fixture; 120 already reads the Today amount. Row text/order/tooltip are covered by vitest | usage/SpendBlock.test.tsx (existing "normal", "tooltip"), TopBar.test.tsx (existing order) |
| menu | 119b | Spending tooltip opens on tap | TO-VITEST | Tooltip open logic | usage/SpendBlock.test.tsx (existing "opens on focus and click") |
| menu | 119c | Spending: short | TO-VITEST | Renders "since <date>" notes from server flags; pure render. Confirm a Go test covers the flag computation | usage/SpendBlock.test.tsx (existing "short ledger", "flags drive the notes") |
| menu | 119d | Spending: empty | TO-VITEST | Renders $0.00 x3 from data | usage/SpendBlock.test.tsx (existing "empty") |
| menu | 119e | Spending 320px menu in viewport | DELETE-DUPLICATE | Same assertion path as 117c / 180c; menu width is a fixed 280px (TopBar.test) | ui/top-bar/TopBar.test.tsx (existing "280px wide") |
| menu | 120 | Menu refresh: amount rises, closed menu stops fetching | KEEP-E2E | Real ledger append -> server -> fetch; page clock; request counting. Absorbs 119a | usage/UsageMenuBlock.test.tsx covers the fetch/close logic (existing) |
| menu | 121 | Hook report: open menu flips to limits; CLI output equals no-server run | KEEP-E2E | Real CLI round trip and multi-process (with/without server) | none |
| menu | 180a | Menu limit burn: chips, forecasts, Codex block, live updates | KEEP-E2E (trim) | Keep only the live updates (statusline payload, Codex rollout scan). Chips, forecast text, order, colors, hover tooltip move to vitest. Absorbs 117c narrow check | usage/LimitsBlock.test.tsx (existing chip, forecast, Codex block cases) |
| menu | 180b | Chip tooltip opens on tap | TO-VITEST | Tooltip open logic | usage/LimitsBlock.test.tsx (existing "chip tooltip opens on focus") |
| menu | 180c | 320px menu in viewport (limit-burn) | MERGE-INTO 180a | Keep as one viewport step in 180a; real layout engine needed | none |
| menu | 181a | Limit burn: few-days | TO-VITEST | Renders "Forecast needs 3 days" from data | usage/LimitsBlock.test.tsx (existing "none status and past-reset rows show no forecast"; add few-days row) |
| menu | 181b | Limit burn: stale | TO-VITEST | Seen marker + forecast from data | usage/LimitsBlock.test.tsx (existing "stale row keeps the seen marker and the forecast") |
| menu | 181c | Limit burn: past-reset | TO-VITEST | Render from data | usage/LimitsBlock.test.tsx (existing "past reset") |
| menu | 118a | Limits states: stale | TO-VITEST | Render from data | usage/LimitsBlock.test.tsx (existing "shows seen only past 10 minutes") |
| menu | 118b | Limits states: past-reset | TO-VITEST | Render from data | usage/LimitsBlock.test.tsx (existing "past reset: waiting, no bar...") |
| menu | 118c | Limits states: one-window | TO-VITEST | Render from data | usage/LimitsBlock.test.tsx (existing "one window shows one row") |
| united | 15 | Redirects | TO-VITEST | Router redirect table; hash router works in jsdom | routes.test.tsx (existing: "/", "/q", "/designs", "/nonsense/here" land on Epics) |
| united | 16 | Needs you | KEEP-E2E (anchor for united) | Server classification of 9 Needs-you reasons incl. a real worker's permission ask (60s). Section/collapse/filter/pill rendering is already in vitest. Absorbs 17-ish steps 29, 30, 28, 63 | epics/EpicsPage.test.tsx, epics/EpicCard.test.tsx (existing) |
| united | 17 | Merge from the card | TO-VITEST | Refusal toast and disabled-while-pending are client logic | epics/EpicCard.test.tsx (existing "a refused Merge shows its reason in a toast", "paused Merge shows a warning toast"); disabled-while-pending needs a deferred fetch mock |
| united | 18 | Joined card | KEEP-E2E | CLI design-step and queue transitions arrive over SSE and re-shape one card; real navigation between design/impl pages | epic/EpicPage.test.tsx covers the partner buttons (existing) |
| united | 19 | Follow-ups triage | TO-VITEST | Card text and dialog from data; link is a hash | epics/EpicCard.test.tsx (existing "Triage opens a dialog with the command and a copy button") |
| united | 63 | Card times | MERGE-INTO 16 | Same `united` fixture. Format logic is unit-tested; keep only the queue remove/add -> "<1m" server round trip as a step in 16 | epics/cardTime.test.ts, EpicCard.test.tsx time cases (existing) |
| united | 28 | Menu: AFK, build text, Paused chip | MERGE-INTO 16 | Same `united` fixture. Keep as steps in 16: AFK/vimode flip via `gx afk`/`gx vimode` and Paused chip after `queue pause` (server state over SSE). Menu item rendering is in vitest | ui/top-bar/TopBar.test.tsx (existing AFK, Vi mode, build text, Paused chip) |
| united | 31 | Menu keyboard | TO-VITEST | Enter/Arrow/Escape key handling; AFK server flip is already in 28. Risk: Chakra menu focus timing in jsdom (use userEvent); fall back to a step in 16 if flaky | ui/top-bar/TopBar.test.tsx (proposed keyboard case) |
| united | 32 | Epics project filter | TO-VITEST | Picker options, filtering incl. follow-ups, stored choice | epics/EpicsPage.test.tsx (existing: picker lists projects, filters every card, stored, several at once). Note the e2e still uses the old select; it may be stale |
| united | 33 | No project filter with one project | TO-VITEST | Render condition | epics/EpicsPage.test.tsx (existing "picker shows only with two or more projects") |
| united | 29 | Grid columns at desktop width | MERGE-INTO 16 | Real layout; cheap as a setViewportSize step in 16 | none |
| united | 30 | Open epic on a deadlock card | MERGE-INTO 16 | Navigation click; same fixture, one extra step (or vitest in EpicCard.test.tsx: Answer/Open link already tested) | epics/EpicCard.test.tsx (existing link-target cases) |
| united | 51 | Merge from the epic page | TO-VITEST | Button order, tone, merge-block notice are render-from-data | epic/EpicPage.test.tsx (existing "the buttons run partner first and the action last, Merge in the done tone", "needs-rebase pause reads Can't merge...") |
| united | 52 | Draft impl epic | TO-VITEST | Disabled Queue + tooltip from data | epic/EpicPage.test.tsx (existing "a draft -impl epic has a disabled Queue with its tooltip") |
| epics | 1 | Queue an epic | MERGE-INTO 6 | Same `ready` fixture and same card-moves-to-Implementing assertion. Do the UI click first, then `queue remove` and the CLI add for the outside-change step. Click/disabled/no-toast logic is in vitest | epics/EpicCard.test.tsx (existing "Queue posts in place", "Queue shows no success toast") |
| epics | 2 | Epic looks right | KEEP-E2E | Real worker run (merges, ask, failure) over time; the static top half (ring, ticket rows, no More stats) duplicates vitest and can be dropped | epic/EpicPage.test.tsx (existing rows, 0/0, More stats, Answer card) |
| epics | 7 | Epic completion | KEEP-E2E | Held worker release -> last merge moves epic Implementing -> Finished over SSE; real vertical layout order | epic/EpicPage.test.tsx (existing divider, More stats) |
| epics | 8 | Edge states | TO-VITEST | Empty-epic card and page render, filter. The "empty counts as implemented" classification is server logic: confirm a Go test, else keep as a step in 2 | epic/EpicPage.test.tsx (existing "an empty epic reads 0/0"), epics/EpicsPage.test.tsx (existing filter) |
| epics | 54 | Done list | TO-VITEST | 3 newest + Show N more / fewer is pure component logic | epic/EpicPage.test.tsx (existing line 415, same assertions) |
| epics | 6 | Outside changes show up | KEEP-E2E | CLI queue add -> SSE -> card moves; absorbs 1 | none |
| pause | 3-Answer | Pause reasons: Answer | KEEP-E2E (anchor for `paused` fixture) | Real recover round trip over the wire. All 8 rows use fixture `paused` with different tickets; run them as steps in one test on one server | ticket/AnswerForm.test.tsx covers the form (existing) |
| pause | 3-Choose base | Pause reasons: Choose base | MERGE-INTO 3-Answer | Ticket 04, same server | ticket/ActionCard.test.tsx |
| pause | 3-Retry | Pause reasons: Retry | MERGE-INTO 3-Answer | Ticket 07, same server | ticket/ActionCard.test.tsx (existing "Retry posts the state token") |
| pause | 3-Merge again | Pause reasons: Merge again | MERGE-INTO 3-Answer | Ticket 05, same server | ticket/ActionCard.test.tsx |
| pause | 3-Start over | Pause reasons: Start over | MERGE-INTO 3-Answer | Ticket 08, same server | ticket/ActionCard.test.tsx |
| pause | 3-Resume | Pause reasons: Resume | MERGE-INTO 3-Answer | Same ticket 08 as Start over, so it cannot share the run unless the fixture adds a ticket; else cover Resume in vitest | ticket/ActionCard.test.tsx (proposed Resume case) |
| pause | 3-Recommended | Pause reasons: Recommended | MERGE-INTO 3-Answer | Ticket 09, same server | ticket/ActionCard.test.tsx (existing action_info/tooltips) |
| pause | 3-Approve | Pause reasons: Approve | MERGE-INTO 3-Answer | approve-epic in same `paused` fixture; real worker permission ask. Absorbs 115 | ticket/PermissionCard.test.tsx (existing) |
| pause | 3-Unknown skill | Unknown skill | KEEP-E2E | Own fixture; real agent rejects skill, pause appears, Retry clears | none |
| pause | 3-Repair | Repair | MERGE-INTO 3-Answer | Same `paused` fixture; "+6 more", Fix button below link (layout) as steps. Data/text parts are in vitest | epic/EpicPage.test.tsx (existing needs-repair card cases) |
| pause | 3-Tooltip Merge again/05 | Tooltip: Merge again | MERGE-INTO 3-Answer | A title attribute check; 4 server starts for 4 attributes. Text is server-supplied (also pin it in a Go test) | ticket/ActionCard.test.tsx (existing line 111 title wiring) |
| pause | 3-Tooltip Resume/08 | Tooltip: Resume | MERGE-INTO 3-Answer | Same | ticket/ActionCard.test.tsx |
| pause | 3-Tooltip Retry/09 | Tooltip: Retry | MERGE-INTO 3-Answer | Same | ticket/ActionCard.test.tsx |
| pause | 3-Tooltip Start over/08 | Tooltip: Start over | MERGE-INTO 3-Answer | Same | ticket/ActionCard.test.tsx |
| pause | 3-Retry disabled | Retry is disabled while the call runs | TO-VITEST | Client pending state; mock fetch with a deferred promise | ticket/ActionCard.test.tsx (existing "Retry posts the state token..."; add pending case) |
| pause | 147 | Resume on worker | KEEP-E2E | Own fixture `model-mismatch`; server pause + resume transition | ticket/ActionCard.test.tsx could host the button-label part |
| pause | 55 | Fix it with an agent | KEEP-E2E | Real clipboard, real agent session created in trunk dir, second start refused (server state, 2 pages) | epic/EpicPage.test.tsx (existing fix dialog cases) |
| pause | 115 | Blocked pause on PTY | MERGE-INTO 3-Approve | Same `paused` fixture and same Allow once path; add the "Agent stuck" pill and "press Approve or Deny" text assertions to the Approve step | ticket/PermissionCard.test.tsx |
| pause | 116 | Retry on a question menu | KEEP-E2E | Own fixture `blocked-menu`; real PTY stuck state, 15s dwell, Retry | none |
| pause | 56 | Fix it with an agent, remote control | KEEP-E2E | Needs the server config flag in the start-agent response; unsure what the client gets. If the response carries a flag, it is a vitest case | epic/EpicPage.test.tsx (proposed, if flag is in the response) |
| codex | 117a | Default harness setting | KEEP-E2E (trim) | Real config writes (global/project, other keys stay), session created with codex, waiting-ticket count. Drop pure render checks | settings/AgentsSettingsPage.test.tsx (existing) |
| codex | 117b | Tier without a model | TO-VITEST | Label "Worker · CLI default" from session data | ui/harness-label/HarnessLabel.test.tsx (existing "CLI default"), agents/AgentsPage.sessionView.test.tsx |
| codex | 117c | Headless note | TO-VITEST | Note shown when config is headless | settings/AgentsSettingsPage.test.tsx (existing "Codex runs on PTY") |
| codex | 118a | Codex queue rows | KEEP-E2E | Scheduler picks codex for the first slot with real config change; chip text itself is in vitest | epic/EpicPage.test.tsx (existing "loop rows show harness chips and queue notes") |
| codex | 118b | codex missing from PATH | KEEP-E2E | Server verdict equals `tickets explain` CLI output | none |
| codex | 120 | Codex rate-limit wait | KEEP-E2E | Server rate-limit hold, Override click starts a real run | none |
| codex | 121 | Codex error pause | KEEP-E2E | Real codex error -> stuck pause -> Retry (fixture codex-error) | none |
| codex | 122 | Codex design run | KEEP-E2E | Real design conversation, answers, Continue, stop -> stats | none |
| codex | 119a | Codex approval: Allow once | KEEP-E2E | Hook decision file written (`codexDecisions`); real round trip | ticket/PermissionCard.test.tsx (existing UI) |
| codex | 119b | Codex approval: Deny with note | MERGE-INTO 119a | Same `codex-blocked` fixture; needs a second blocked ticket in the fixture so one server serves both decisions | ticket/PermissionCard.test.tsx |

## Summary

Counts (69 flows/rows):
- KEEP-E2E: 22 (several trimmed to the real-server part)
- TO-VITEST: 24
- MERGE-INTO: 22
- DELETE-DUPLICATE: 1

Five largest wins:
1. pause: collapse 8 recovery rows + Repair + 4 tooltip tests (13 server starts) into one `paused` run, and fold 115 into the Approve step (14 starts saved).
2. menu: 181a-c and 118a-c (6 fixtures) move to LimitsBlock.test.tsx, which already covers them; plus 117b/119b/180b tap tests.
3. united: 16 becomes the single `united` run; 63, 28, 29, 30 fold in; 15, 17, 19, 31, 32, 33, 51, 52 go to vitest (12 server starts saved).
4. menu: 119 (normal/short/empty/narrow) -> SpendBlock.test.tsx; narrow checks 117c/119e/180c become one step.
5. epics: 54, 8 to vitest; 1 folds into 6.
