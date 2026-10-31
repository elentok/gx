# Limits and spending in the web menu

Handoff spec from the Limits and spending wayfinder map (`limits-and-spending` in the local ticket
tracker, decisions `01` to `04` in that epic's `issues/` directory). The research behind it is
`docs/research/limits-and-spending-data-sources.md`.

## Problem Statement

The person running gx cannot see, from the web UI, how close their Claude account is to its rate
limits, or what their gx runs have cost. Claude Code knows the 5-hour and weekly limits, but only
the `gx claude statusline` hook ever sees them, and it only prints them into Claude's own terminal.
The server never receives them. The budget ledger keeps the money side per day, but the web UI
reads none of it. A person on a subscription has to look at a Claude terminal to learn how much
headroom is left. A person on an API key has no view of their weekly or monthly spend at all.

## Solution

The top bar menu gets one new block between the AFK switch and the build line. The block depends on
the account, with no setting.

- Subscription accounts, once Claude has reported rate limit data, see **Claude limits**: the 5h
  limit and the weekly limit, each with a whole-number percentage, a bar, and when it resets.
- Every other account sees **gx spend (est.)**: money spent today, since the most recent Sunday and
  since the 1st of the month.

The block is fed by a new report path from the statusline hook to the server, a state file that
remembers the last limits across restarts, and a new sum over the budget ledger's day totals. An
open menu refreshes on its own. The menu trigger (gx mascot plus chevron) does not change. The
approved look is the stored mock
`$(gx tickets root)/limits-and-spending/mocks/04-menu-layout.html`.

## User Stories

**Limits mode**

1. As a subscription user, I want to see my 5h limit as a percentage with a bar, so that I know how
   much of this window I have used.
2. As a subscription user, I want to see my weekly limit the same way, so that I can pace my week.
3. As a subscription user, I want each limit to say when it resets ("resets in 2h 10m"), so that I
   know how long I must wait if I am close.
4. As a subscription user, I want to hover, focus or tap the reset time and see the exact clock
   time, so that I can plan around it, also on a phone.
5. As a subscription user, I want the percentage and bar to turn amber from 80% and red from 95%, so
   that I notice a limit running out without reading numbers.
6. As a subscription user, I want the percentage shown as a whole number, so that the row stays
   quiet and easy to scan.
7. As a subscription user, I want the block titled "Claude limits", so that I know these numbers
   come from Claude and not from gx.
8. As a subscription user, I want old data marked with a muted "seen 3h ago" once it is over 10
   minutes old, so that I do not mistake an old reading for the current one.
9. As a subscription user, I want a limit whose reset time has passed to say "waiting" with "reset
   passed · waiting for a Claude session" and no bar, so that I am not shown a percentage that is
   no longer true.
10. As a subscription user, I want each limit row to stand on its own, so that a plan or session
    that reports only one window still shows that one.
11. As a subscription user, I want gx to remember the last limits across a server restart, so that
    the menu does not flip to spending just because the server restarted.
12. As a subscription user, I want limits mode to start the first time Claude reports limits and
    stay on, so that the block does not flicker between modes when no Claude session is open.
13. As a subscription user who has never had a report, I want to see the spending block, so that
    the menu always has something useful in it.
14. As a subscription user, I want the menu to update when a new report arrives, so that I see
    fresh numbers without reopening it.

**Reporting from the hook**

15. As a subscription user with the statusline hook installed, I want every hook run to send the
    reported limits to the gx server, so that the server has the data without any extra setup.
16. As a person using Claude Code, I want the hook to print its line first and look and behave
    exactly as before, so that my status line never prints new errors and waits at most the hook's
    report timeout when the server is slow.
17. As a person whose gx server is not running, I want the hook to skip the report quietly, so that
    my Claude session is never blocked or disturbed by gx.
18. As a person whose gx server is not running, I want the next hook run after it starts to
    deliver the limits, so that nothing needs a manual retry.
19. As a person who runs gx on a network, I want the report route not served on the web listener,
    so that nobody on the network can forge my limits.

**Spending mode**

20. As an API-key user, I want to see money spent today, so that I know my day's cost so far.
21. As an API-key user, I want to see money spent since the most recent Sunday (today if it is
    Sunday), so that I can track my week.
22. As an API-key user, I want to see money spent since the 1st of the month, so that I can track my
    month.
23. As an API-key user, I want the block titled "gx spend (est.)" with an info tooltip that says
    it is the estimated cost of Claude runs started by gx, not the bill, and that Codex runs and
    Claude sessions outside gx are not counted, so that I do not mistake it for my invoice.
24. As an API-key user, I want the three rows to be compact lines with the name on the left and the
    amount on the right, so that the block stays small.
25. As an API-key user on a new install, I want a row whose window starts before gx kept records to
    say "since Fri, Oct 30" under the amount, so that I know the total is partial.
26. As an API-key user with no recorded spend, I want to see $0.00 on each row, so that the block
    shows a clean zero instead of looking broken.
27. As an API-key user, I want the days to run from local midnight to local midnight, the same as
    the budget day, so that "today" matches the rest of gx.
28. As an API-key user, I want the amounts to follow the ledger as runs spend money, so that the
    menu does not stay stale until something else happens.

**Menu layout and refresh**

29. As a gx user, I want the menu order to be Settings, AFK, a separator, the limits or spending
    block, a separator, then the build line, so that the new block sits apart from the controls.
30. As a gx user, I want the menu to fetch fresh data each time I open it, so that I never see a
    number from the last time I looked.
31. As a gx user with the menu open, I want it to refresh every 60 seconds, so that the relative
    times ("resets in", "seen ago") and the amounts keep moving.
32. As a gx user, I want the menu to cost nothing while it is closed, so that the top bar stays
    light.
33. As a gx user on a phone, I want the menu to fit the screen, so that the block is readable at
    narrow widths.
34. As a gx user, I want budget limits (soft and hard) left out of this block, so that the menu
    shows spending facts only.

## Implementation Decisions

**Data path for limits**

- The statusline hook (`gx claude statusline`) sends the `rate_limits` part of its payload, parsed
  to the two windows, to the server on every run that carries it. It sends nothing else from the
  payload. A run with no `rate_limits` sends nothing.
- The report goes over the same channel every other CLI verb uses: the server's local unix socket,
  resolved from the same state directory. There is no token. The hook does not invent its own
  authentication. This settles the research's open question: no new auth is needed.
- The report is best-effort. A short timeout, no retry, and every failure (server not running,
  version mismatch, timeout) is swallowed. The hook never starts the server, never changes its
  printed status line, and never changes its exit code. The next run that finds the server
  delivers the data.
- The report route is a write route that exists on the local socket only. The web listener (the
  browser one, and the opt-in loopback TCP listener, which serves the same routes) does not expose
  it. The set of routes the CLI calls, and the web allow-list tests, keep passing.
- Every route needs a CLI verb with `--json` (`TestRouteTableHasCLIVerbs`). The report route uses
  a new verb, `gx claude report-limits --json`, which the statusline hook shares its code with. The
  read route uses `gx budget usage --json`. The implementer may rename them.
- The server stamps each window with its own received-at time. The report carries no time of its
  own. Each window has used percentage, reset time (epoch seconds) and received-at.
- The server merges per window and in an order-safe way, because an idle Claude session can resend
  the `rate_limits` of its last API response (for example on a vim mode change), and that old
  report must not replace newer numbers. For each window the report carries:
  - Same reset time as the stored window: keep the higher percentage.
  - Earlier reset time than the stored window, while the stored window's reset time is still in the
    future: drop the report for that window.
  - Later reset time, or no stored window, or a stored window already past reset: replace it.
  - The window's received-at changes only when the report changes the stored value or confirms it
    (the same percentage and reset time). A dropped report does not touch received-at.

  A window the report does not carry stays untouched. A report with no windows changes nothing.
  This matters because Claude Code drops a window from its payload once the window's reset time
  passes; the stored window then stays, and the menu shows it as past reset until a new report
  replaces it.
- The server persists the merged windows in a state file in its state directory, and loads it at
  start. The first stored window starts limits mode. Limits mode never turns off again; there is no
  expiry and no setting.
- Limits are shown only when the statusline hook is installed (it is opt-in, installed with
  `gx claude statusline --install`) and a Claude session has made an API call. Without the hook, a
  subscription user sees the spending block. This follows from "limits show when Claude reported
  rate limit data".

**Data path for spending**

- A new read over the budget ledger returns, in one call: today's total, the total since the most
  recent Sunday (today when today is Sunday), the total since the 1st of the current month, and the
  first day the ledger has. All sums are over the ledger's per-day totals, by day key, in the
  server's local zone, so they match the budget day.
- The sums include today. A window's total is the sum of every ledger day key from the window's
  start day through today.
- A window is "short" when the ledger's first day is later than the window's start day. The server
  returns the first day so the client can write "since <day>". A window that the ledger fully covers
  gets no note. An empty ledger has no first day and no notes, and every total is zero. The Today
  row never gets a note.

**Read route and events**

- One new read route, available to the web, returns the mode, the stored limit windows (with
  received-at) and the spend totals plus first day. The menu fetches it itself, so the snapshot
  does not grow. The implementer picks the route name and the exact response shape.
- Two new events join the existing event stream: `budget-changed` and `limits-changed`. Both are
  throttled, following the system-health precedent of publishing only on a real change.
  `limits-changed` fires when a stored window's whole-number percentage or reset time changes, when
  a window first appears, or when a window goes from stale to fresh (a report that confirms the same
  numbers), so the menu leaves "seen 3h ago". `budget-changed` fires when a ledger poll moves a
  total by a visible step, and is throttled to at most one per 60 seconds. The implementer picks
  the other throttle interval and the step size.
- The web client's event listeners today receive only the event's address, not its name. They
  change to receive the event name. The open menu block listens for the two new names only and
  refetches its own data on them, in addition to its 60-second refresh. (Every new event still
  makes the client refetch the snapshot and the open `useFetched` pages, as any event does.)

**Web menu**

- The menu keeps the trigger on main (the gx mascot plus chevron) and its items. The new block is
  added between the AFK switch and the build line, with a separator on each side, in the order
  from User Stories 29. Main's menu already has the Settings item; this spec builds on main's menu,
  not on this branch's older copy.
- `TopBar` stays presentational and takes a menu block slot. A connected child component renders
  inside `Menu.Content` (main's menu mounts its content lazily and unmounts on close). The child
  fetches on mount through its own small hook (not `useFetched`, which keeps old data after a
  failure and repaints from a stored copy), so it fetches on open and costs nothing while closed. It
  draws its own two separators, so with no data to show it renders nothing and adds none.
- The child refetches every 60 seconds while open, and on the two events. It does not fetch while
  closed.
- Until its first fetch succeeds, the block renders nothing. After a failed refresh it renders
  nothing too: it never keeps old totals or percentages on screen as if they were current. It never
  shows the other mode's block.
- The relative times ("resets in 2h 10m", "seen 3h ago") are computed on the client from epoch
  values and re-render with each refresh. The exact reset time in the tooltip uses the browser's
  local zone.
- The menu is 280px wide when a block is shown, and narrower on a viewport that cannot fit it.

**Limits block (mode `limits`, see mock states limits-fresh, limits-stale, limits-past-reset, limits-one-window)**

- Title row: "Claude limits".
- One row per stored window, in the order 5h then weekly. Each row is a stacked block: the name
  ("5h limit", "Weekly limit") over a large whole-number percentage, a 4px bar, and a muted line
  "resets in 2h 10m" with the exact time as a tooltip.
- Bar and percentage turn amber at 80 and above and red at 95 and above.
- A window is stale when its received-at is more than 10 minutes ago. A stale row adds a muted
  "seen 3h ago" on the same line as the reset time, right-aligned.
- A window is past reset when its reset time is not in the future. It shows "waiting" in place of
  the percentage, no bar, and the line "reset passed · waiting for a Claude session". It never
  shows "seen".
- Each row decides on its own. With one stored window, only that row shows. A past-reset 5h row
  can sit above a live weekly row.

**Spending block (mode `spending`, see mock states spend-normal, spend-short-ledger, spend-empty)**

- Title row: "gx spend (est.)" with an info marker. Hover or focus on the marker shows the tooltip text
  from the mock: "Estimated cost of Claude runs started by gx. Not the bill. Codex runs and Claude
  sessions outside gx are not counted."
- Three compact rows: "Today", "Since Sunday", "Since the 1st". Name left, amount right.
- Amounts are US dollars with two decimals.
- A short window adds a muted line under the row: "since Fri, Oct 30" (weekday, month, day of the
  first ledger day).
- Both tooltips (the exact reset time and the spend caveat) open on hover, on keyboard focus and on
  tap, so they work on touch screens.
- The block does not show the soft or hard budget limits or any budget state.

**Implementer's choices (not decided by the map)**

- Names: the new routes, the state file, the event constants, the report verb's wire shape, new
  components and props, and the fixture scenarios' final names.
- Throttle interval for the two events and the step for `budget-changed`.
- Time formats at the edges: under one hour ("resets in 45m"), under one minute, and the
  largest-two-units rule for days and hours. The mock fixes only the middle cases.
- Percentage rounding (nearest or floor). The color must be taken from the same whole number the
  row shows.
- The hook's timeout value.
- The thousands separator and any other number-format detail of dollar amounts, following the
  existing cost formatting in the codebase.
- The wire mechanism for the fixture to seed limits and ledger days.

## Testing Decisions

A good test checks what a person or a client sees: the menu's rendered rows, the read route's
response, the events on the stream, the state file's effect across a restart, and the hook's
visible behavior. It does not check internal fields or call counts.

Seams:

1. **Server, through servertest.** Reports go in over the socket, the read route and event stream
   come out. Covers merge-per-window, the first report starting limits mode, restart persistence
   (stop the server, start it on the same state directory, read again), the web listener refusing
   the report route, and event throttling. Covers the order-safe merge: a report with the same
   reset time and a lower percentage, and a report with an earlier reset time, leave the stored
   window and its received-at untouched. The ledger is seeded with day totals from the real clock,
   so these tests check the response shape, not calendar dates. Prior art: the budget snapshot
   tests and the system-health watcher tests (`server/healthwatch.go` and its neighbours).
2. **Window math.** The server has no clock seam (budget code calls `time.Now()` directly), so the
   window sums are a pure function of `now` and the day map. Table tests for the Sunday and 1st windows over ledger day keys: today a
   Sunday, today the 1st, month and year boundaries, a ledger that starts mid-window, a ledger
   with gaps, an empty ledger, and a daylight-saving week. Prior art: `server/budget.go` tests.
3. **CLI.** `cmd` tests for `gx claude statusline` against a fake server: a payload with both
   windows, one window, none; the printed line and exit code are the same as before; the server
   down, slow, or refusing leaves output and exit code unchanged. Plus the route-table tests
   (`TestRouteTableHasCLIVerbs`, `TestWeb_EverythingElseIs404`).
4. **Web.** Vitest for the menu block with props in each of the seven mock states, the amber and
   red edges (79, 80, 94, 95), stale at 10 minutes, past reset, one window, the tooltip text, the
   short-ledger note, the empty ledger, refetch on open, on the 60-second tick and on the two event
   names (and not on other events), no fetch while closed, nothing rendered before the first fetch
   succeeds, and nothing rendered after a failed refresh. Gallery entries for each state. Prior art:
   `webui/src/ui/top-bar/TopBar.test.tsx` and the `TopBar` gallery entry.
5. **Fixture.** `gxfixture` scenarios that start the server with seeded state: a `limits` scenario
   that seeds the limits state file in each of four shapes (fresh, stale, past reset, one window),
   and a `spend` scenario that seeds the ledger in three shapes (normal, short, empty). Seeds are
   relative to the real clock so the dates stay right. In the short shape, today is the first
   ledger day. The `spend` scenario also holds one fake agent running on a claimed ticket, so a flow
   can add spend while the server runs (see Flow D). Flows push fresh limits with the real
   verb: they pipe a statusline payload into `gx claude statusline` against the fixture server.
   Prior art: `disk-low` and the `health` scenario's control files. The implementer picks the
   mechanism.

## Verification

A verify ticket runs these against a temporary gx server from `gxfixture`. "Go test" and "Vitest"
name the seam from Testing Decisions that covers the story. A story that a flow, a CLI check or a mock
state already covers has no Go test or Vitest entry: the implement tickets ran those through `gx check`,
and the verify part does not rerun them. New flows take
the next free numbers in `webui/e2e/CATALOG.md`; the implementer assigns them.

New scenarios to write: `limits` and `spend` (Testing seam 5). New flows to write:

- **Flow A, "Limits block":** scenario `limits`, fresh shape. Open the menu. Check the title, both
  rows, the percentages, bars, the amber weekly row, the "resets in" text, the tooltip with the
  exact time, and the menu order. Pipe a new payload into `gx claude statusline`; check the open
  menu updates without a reload. Open the exact-time tooltip by hover and by tap. Check the menu
  in a 320px viewport and see that it does not overflow. (Restart persistence is covered by seam 1:
  `gxfixture` ends when its server exits.)
- **Flow B, "Limits states":** scenario `limits`, in the stale, past-reset and one-window shapes.
  Check "seen" text, "waiting" with no bar, and a single row.
- **Flow C, "Spending block":** scenario `spend`, normal shape. Open the menu. Check the title,
  tooltip text (opened by hover and by tap), three rows with amounts, and the order. In the short
  shape check the "since" note on each of the two longer rows except where today is Sunday or the
  1st (that row's window starts today, so it is not short), and none on Today. In the empty shape
  check $0.00 on all three. Check a 320px viewport and see that the menu does not overflow.
- **Flow D, "Menu refresh":** scenario `spend`. Open the menu, append a usage line to the transcript
  of the scenario's fake agent so the real ledger poll records the cost, and check the amount
  changes within a minute with no reopening. Close the menu and check no further fetches go out.
- **Flow E, "Hook report":** scenario `spend` with no limits stored. Pipe a statusline payload into
  the CLI. Check the menu flips to the limits block. Check the command's own output is the same as
  without a server.

Story entries:

1. Flow A. Mock state limits-fresh.
2. Flow A. Mock state limits-fresh.
3. Flow A. Mock state limits-fresh.
4. Flow A (tooltip, hover and tap).
5. Mock state limits-fresh (amber weekly row).
6. Vitest (seam 4): a fractional percentage renders as a whole number.
7. Flow A. Mock state limits-fresh.
8. Flow B. Mock state limits-stale.
9. Flow B. Mock state limits-past-reset.
10. Flow B. Mock state limits-one-window.
11. Go test (seam 1): the restart check.
12. Flow E.
13. Flow C. Mock state spend-normal.
14. Flow A (updates without reload).
15. CLI check: pipe a payload into `gx claude statusline` against the fixture; the read route then
    shows its percentages.
16. Flow E.
17. Go test (seam 3): server down, slow, or refusing leaves output and exit code unchanged and the
    command ends within its timeout.
18. Flow E (pipe a payload after the server is up).
19. Go test (seam 1): the web listener answers 404 for the report route; the socket accepts it.
    `TestWeb_EverythingElseIs404` passes.
20. Flow C. Mock state spend-normal.
21. Flow C. Mock state spend-normal.
22. Flow C. Mock state spend-normal.
23. Flow C (tooltip, hover and tap). Mock state spend-normal.
24. Mock state spend-normal.
25. Flow C (short shape). Mock state spend-short-ledger.
26. Flow C (empty shape). Mock state spend-empty.
27. Go test (seam 2): a ledger day at 23:59 and one at 00:01 fall on different days; the daylight
    saving week.
28. Flow D.
29. Flow A and Flow C.
30. Flow D.
31. Flow D.
32. Flow D (closed step).
33. Flow A and Flow C, in a 320px viewport: the menu does not overflow (jsdom does no layout, so
    Vitest cannot check this).
34. Vitest (seam 4): the spending and limits blocks hold no budget limit text. Go test (seam 1):
    the read route has no soft or hard limit field.

**Mock states** (menu width 280px in every state; fixture scenario `limits` for the first four lines
and `spend` for the last three, both listed above as new scenarios to write):

- `$(gx tickets root)/limits-and-spending/mocks/04-menu-layout.html`, limits-fresh, 280px, parts:
  settings, afk, title, row-5h, row-weekly, build. Scenario `limits`, fresh shape. The weekly row is
  amber.
- `$(gx tickets root)/limits-and-spending/mocks/04-menu-layout.html`, limits-stale, 280px, parts:
  settings, afk, title, row-5h, row-weekly, build. Scenario `limits`, stale shape. Both rows show
  "seen".
- `$(gx tickets root)/limits-and-spending/mocks/04-menu-layout.html`, limits-past-reset, 280px,
  parts: settings, afk, title, row-5h, row-weekly, build. Scenario `limits`, past-reset shape. The
  5h row reads "waiting".
- `$(gx tickets root)/limits-and-spending/mocks/04-menu-layout.html`, limits-one-window, 280px,
  parts: settings, afk, title, row-weekly, build. Scenario `limits`, one-window shape.
- `$(gx tickets root)/limits-and-spending/mocks/04-menu-layout.html`, spend-normal, 280px, parts:
  settings, afk, title, row-today, row-since-sunday, row-since-first, build. Scenario `spend`,
  normal shape.
- `$(gx tickets root)/limits-and-spending/mocks/04-menu-layout.html`, spend-short-ledger, 280px,
  parts: settings, afk, title, row-today, row-since-sunday, row-since-first, build. Scenario
  `spend`, short shape. The two longer rows show the "since" note.
- `$(gx tickets root)/limits-and-spending/mocks/04-menu-layout.html`, spend-empty, 280px, parts:
  settings, afk, title, row-today, row-since-sunday, row-since-first, build. Scenario `spend`,
  empty shape.

## Self-decided design questions

Decided while drafting and reviewing, without a map ticket. Each follows from the map's decisions.

- **Hook authentication.** The hook reports over the local unix socket with no token, as every CLI
  verb does. Reason: no new auth path is needed, and the web listener refuses the route.
- **Who stamps received-at.** The server does. The report carries no time. Reason: the hook's clock
  and the report's age are not trustworthy, and "stale" must mean "the server last heard".
- **Merge per window, order-safe.** See Implementation Decisions. Reason: Claude drops expired
  windows from its payload, and idle sessions resend old numbers.
- **State file saved on every changing report.** Reason: limits mode must survive a restart
  (story 11), and a report that changes nothing writes nothing.
- **Event rules.** `limits-changed` on a new, changed or stale-to-fresh window. `budget-changed`
  at most once per 60 seconds. Reason: keep the stream quiet while the open menu still updates.
- **Menu width.** 280px, narrower on small screens. Reason: it is the width of the approved mock.
- **Own read route, not the snapshot.** Reason: only the open menu needs the data, so the snapshot
  and every other client page stay unchanged.

## Out of Scope

- The TUI. It shows neither limits nor spending from this work.
- Budget limits (soft and hard) and any change to how they behave. The block does not show them.
- The menu trigger icon. The gx mascot plus chevron on main stays.
- A setting to choose the mode. The mode is always detected.
- Spend from Codex runs, Claude sessions outside gx, or the real bill. The label and tooltip say so.
- Installing the statusline hook for the person, or a prompt in the menu to install it. The hook
  stays opt-in.
- Reading limits from any other source, such as the headless runner's `rate_limit_event`.
- Turning limits mode off again, or expiring stored limits.
- Showing more than the two Claude windows (for example the gateway-only spend limit).

## Further Notes

- The two modes will not match. Claude's 5h and weekly windows reset at times Claude chooses. The
  spending windows reset at local midnight, Sunday and the 1st. The block titles say which is which.
- The research listed a few unknowns that stay unknown and do not change the design: whether the
  headless runner's rate limit event carries a percentage, and whether other users have the hook.
- The ledger today counts ticket iterations only (`pollBudget` walks tickets). Plain
  `gx agents create` sessions, and probably design chats, are not counted, so the approved tooltip
  "Claude runs started by gx" claims slightly too much. The approved wording stays, and the gap is
  filed as a follow-up in the `follow-ups` epic.
- This branch's `TopBar` is older than main's. Implement against main's menu, which has the
  Settings item and the mascot trigger.
