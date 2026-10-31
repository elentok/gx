# Limits and spending: where the server can get the data

Research for ticket gx:limits-and-spending/01. Read against the worktree at branch
`limits-and-spending-design` (HEAD f1d7d459) on 2026-10-31. All paths are repo-relative.

## Summary

- Rate limits: gx installs the statusline hook, but the hook only prints. Nothing sends the payload
  to the server today. The server needs a new write route (or a file the hook drops) to get it.
- Rate limits exist only for claude.ai Pro/Max/Ultra subscribers and only after a session's first API
  response. API-key accounts get none.
- Spend: the ledger keeps every day since it started (22 days on the author's machine), so "since
  Sunday" and "since the 1st" are a sum over `Days` keys. No code does that sum today.
- The ledger counts only gx-run Claude iterations, as estimated API-equivalent dollars. It misses
  interactive Claude sessions outside gx and all Codex runs.
- The web UI already gets `budget` (today only) in the snapshot. Any new field in the snapshot rides
  the existing SSE refetch, but budget changes publish no event, so a live total is stale until some
  other event fires.

## 1. Rate limits

### What Claude Code sends (official docs)

Source: https://code.claude.com/docs/en/statusline

- The command gets JSON on stdin. Fields: `rate_limits.five_hour.used_percentage` and
  `rate_limits.seven_day.used_percentage` (0 to 100), `rate_limits.five_hour.resets_at` and
  `rate_limits.seven_day.resets_at` (Unix epoch seconds). The doc's example shows
  `{"five_hour":{"used_percentage":23.5,"resets_at":1738425600},"seven_day":{...}}`.
- "`rate_limits`: appears only for claude.ai Pro, Max, and Ultra subscribers, or behind a Claude apps
  gateway that sets a spend limit for you. Even then, it appears only after the first API response in
  the session. Each window may be independently absent, and Claude Code drops a window once its
  `resets_at` time passes." So API-key accounts: absent. Confirmed.
- Extra, gateway-only: `rate_limits.spend_limit` (`used_percentage`, `resets_at`, and optional
  `used_usd`, `limit_usd`, `period`; needs Claude Code v2.1.251 and v2.1.284). Not relevant to a
  subscription account.
- `cost.total_cost_usd` is the session's estimated cost at list price ("may differ from your actual
  bill"), and resets on `/clear`. It is per session, not per week or month.
- The command runs at session start, on each new assistant message, after `/compact`, on permission
  mode or vim mode change, on a `refreshInterval` timer if set (minimum 1 second), and when a
  rate-limit window reaches its `resets_at`. Updates are debounced at 300 ms, and an in-flight run is
  cancelled by a new trigger. Each Claude Code session runs its own hook, so the payload exists only
  while some session is open and active.
- Consequence: the data is a per-session push. When no Claude session is running (or none has made
  an API call), nobody knows the number. A page that reads it must show "unknown" or a last-seen age.

### What gx does today

- gx installs the hook on request: `gx claude statusline --install` writes
  `statusLine: {type: command, command: "gx claude statusline"}` into `~/.claude/settings.json`
  (`cmd/claude_statusline_install.go:15-35`, flag at `cmd/claude.go:50-52`). It is opt-in, not
  automatic. The author's own settings have it (`~/.claude/settings.json:76-79`); other users may not.
- The hook parses the same fields (`cmd/claude_statusline.go:48-57`) and renders a second line like
  "11% of 5h (resets 14:00) · 72% of weekly" (`cmd/claude_statusline.go:328-351`). It treats a missing
  `rate_limits` as normal (silent, `cmd/claude_statusline.go:111-116`).
- It does not contact the server. `runClaudeStatusline` reads stdin, prints, and returns
  (`cmd/claude_statusline.go:73-122`). No server code mentions the statusline or `rate_limits`
  (`rg statusline|rate_limits server/` finds nothing), and the route table has no endpoint for it
  (`server/server.go:502-560`, list starts at 502).
- So the server cannot receive the payload today. Options, for the design ticket to weigh:
  1. The hook POSTs the parsed `rate_limits` to a new server route (needs the server address and auth
     from the hook process; not checked how other CLI verbs find the server, see Unknown).
  2. The hook writes a small file (for example under the state dir) and the server reads it.
  Either way the server stores the last payload with a received-at time.
- Related rate-limit data that already reaches the server, but is not usable here: the headless
  runner parses `rate_limit_event` from Claude's stream-json, but keeps only `status` and `resetsAt`,
  and only acts on `rejected` (`nativerunner/events.go:45-48`, `nativerunner/events.go:223-227`).
  Whether that event carries a used percentage is unknown; the fake used in tests has none
  (`nativerunner/headless_test.go:156`).
- Subscription detection exists but is a different thing: `subscription.Detect` reads
  `oauthAccount.hasExtraUsageEnabled` from `~/.claude.json` (`subscription/check.go:33-61`) and the
  server exposes it as `snapshot.extra_usage` (`server/extra_usage.go:31-41`, `server/index.go:88-90`).
  It says whether extra usage auto-purchase is on, not whether the account is a subscription. The field
  is undocumented and internal (`subscription/check.go:29-32`).

## 2. Spend

### Ledger shape and retention

- `budgetLedger` persists `Days map[string]float64` keyed `2006-01-02` (`server/budget.go:26`,
  `server/budget.go:38-41`), plus per-day `Projects` and `Roots` splits (`server/budget.go:42-45`).
  File: `<StateDir>/budget-ledger.json` (`server/budget.go:23`, `server/budget.go:73`).
- Nothing prunes `Days`. `save` only deletes `Seen` entries older than 48 h
  (`server/budget.go:425-433`, `ledgerSeenRetention` at `server/budget.go:25`). The only reads of
  `Days` are today, a named day, and the latch (`server/budget.go:99-106`, `188`, `239`, `262`;
  `rg '\.Days\b' server/`). Prior-day total is read for the notification
  (`server/budget_notify.go:83`).
- Measured on the real file `~/.local/state/gx/budget-ledger.json`: 22 day keys, 2026-10-07 to
  2026-10-30. So history reaches back to when the ledger started on this machine. A "since Sunday"
  or "since the 1st" sum works whenever that start is earlier than the cut-off. Before that, the sum is
  an undercount, and a fresh install has less history. Unbounded growth is about one key per day.
- There is no existing helper for week or month sums (`rg 'time.Sunday|Weekday' server/` finds
  nothing outside stats). Adding one is a new method over `Days` under `l.mu`.
- Per-day project/root splits for older days may be partial: "Spend recorded before projects were
  tracked is in the total only" (`server/budget.go:167-168`). Use `Days` for totals.

### What it counts

- Only gx-run agents. `pollBudget` walks the index's tickets, keeps those `claimed` or registered as a
  run, resolves each live iteration's session, and reads its cost (`server/budget.go:477-495`). Cost
  comes from the iteration's Claude transcript via `ralphloop.SessionCost`
  (`server/budget.go:448-456`, `ralphloop/report_metrics.go:249-260`). "All ticket kinds count"
  (`server/budget.go:478`).
- Not counted: Claude sessions you run yourself outside gx; Codex runs (`docs/specs/run-budget-limits.md:24`,
  and `SessionCost` is Claude-only); anything between polls that never got polled (poll is every 30 s,
  `server/budget.go:24`, `591-605`). Whether design-chat sessions (not ticket iterations) are metered
  is unknown; no reference to them was found in `pollBudget`.
- The unit is an estimated API-equivalent dollar from a hardcoded pricing table, not billed spend, and
  not a measure of subscription use (`docs/specs/run-budget-limits.md:19-24`). The UI must label it so.
- Cumulative cost per session is tracked in `Seen`; each poll adds only the delta
  (`server/budget.go:118-140`), and a lower reading adds nothing.

### Day boundary

- Budget day = local midnight to local midnight, using the server process's local zone
  (`server/budget.go:36-37` comment; `now.Format(budgetDayLayout)` at `99`, `249`, `292`;
  `nextMidnight` uses `t.Location()` at `server/budget.go:419-422`). An iteration that crosses
  midnight is split in time proportion across both days (`server/budget.go:108-139`).
- "Most recent Sunday" and "the 1st" should use the same local zone and the same `Days` keys. Pick
  whether "since Sunday" includes today: it should, since today's total is in `Days[today]`.
- Not the same as Claude's reset windows: the 5 h and 7 day windows reset at server-defined times
  (`resets_at`), not at local midnight on Sunday. The two numbers will not line up. Say so in the UI.

### Other spend source, for contrast

- `GET /v1/stats` has per-day spend series (`StatsCost.SpendDesign/SpendImplementation`,
  `server/stats.go:46-53`), built from ticket frontmatter stamps by finish date
  (`server/statsbuild.go`, not read in full). Ranges are week, month, quarter
  (`server/stats.go:20`). That is attributed to the day a ticket finished, not when money was spent,
  and has `NoData` gaps. The ledger is the better source for a spend total.

## 3. How the web UI gets budget data today

- Server: `GET /v1/budget` returns `BudgetStatus` (`server/server.go:516`, `server/budget.go:415-417`).
  The same struct is embedded as `budget` in `GET /v1/snapshot` (`server/index.go:86-87`,
  `server/server.go:637`). Fields: `day`, `total`, `soft_limit`, `hard_limit`, `budget_paused`,
  `hard_latched`, `override`, `projects`, `roots` (`server/budget.go:270-284`). Today only.
- Web: the TS type is generated in `webui/src/generated/server.ts:12` (`BudgetStatus`) and
  `:1397` (`Snapshot.budget`). No non-generated web code reads `snapshot.budget` or `/v1/budget`
  (`rg budget webui/src` outside `generated/` and tests finds only a gallery caption at
  `webui/src/gallery/registry.tsx:442`). The TUI Queue header shows it (`ui/tickets/queue_header.go`).
  So the web UI has the data in hand but shows nothing yet.
- Write routes exist: `POST /v1/budget/override` and `/increase` (`server/server.go:517`, `524`).

## 4. How the top bar menu gets its data, and the live channel

- `TopBar` is a pure presentational component. Props come in from `ConnectedTopBar`, which reads the
  shared snapshot from the connection context: `snapshot?.build`, `mode`, `afk`, `health`
  (`webui/src/nav/ConnectedTopBar.tsx:7-23`). The menu (a Chakra `Menu`) holds the AFK checkbox and the
  build string; it appears only if `onToggleAfk` or `build` is set (`webui/src/ui/top-bar/TopBar.tsx:89-123`).
  The gallery renders it with plain props (`webui/src/gallery/registry.tsx:349-361`).
  New rows are new optional props plus new fields on the snapshot (or a fetch of their own).
- Live channel: one SSE stream, `GET /v1/events?since=N&client=ID` (`server/server.go:667-724`). The
  broker numbers events, keeps a 256-event replay window, and sends a heartbeat comment
  (`server/events.go:1-60`). The web client reads it as raw text; any event just means "snapshot is
  stale" and triggers a snapshot refetch (one in flight, one queued)
  (`webui/src/connection.ts:140-190`, comment at `connection.ts:172-173`). There is also a 60 s `tick`
  that re-renders but does not refetch (`webui/src/connection.ts:8`, `205-207`).
- Pages that fetch their own data reuse this: `useFetched` / `useRefetchOnEvents` refetch 500 ms after
  an event and on reconnect (`webui/src/useFetched.ts`, `webui/src/refetch.ts:5-26`).
- Closest precedent to copy: health. `healthwatch` polls every 5 s and publishes
  `EventHealthChanged` only when a level changes (`server/healthwatch.go:12`, `78-140`,
  `server/healthtypes.go:42`), and the snapshot carries `health` (`server/server.go:640`, snapshot
  struct at `server/index.go:91-92`).
- Gap: budget never publishes an event. `pollBudget` saves the ledger and notifies but calls no
  `events.publish` (`server/budget.go:480-505`; the only budget-related publish is
  `server/budget_grace.go:118`, a queue-changed event). So `snapshot.budget.total` is refreshed only
  when some unrelated event triggers a refetch. A live-ish spend row needs either a new
  `budget-changed` event from `pollBudget` (throttled, for example on a cent or dollar step) or a
  periodic client refetch.
- Rate-limit rows would need the same: an event when a new payload arrives, plus a stored last-seen
  time so the UI can show "as of 3 min ago".

## Unknown

- Whether other gx users have the statusline hook installed (it is opt-in; only the author's
  settings were checked).
- How `gx` CLI verbs find and authenticate to the server, so whether the hook can POST from inside
  Claude's process environment. Not read; the design ticket should check `apiclient/`.
- Whether the real `rate_limit_event` in stream-json carries a used percentage. Only
  `status` and `resetsAt` are parsed and no real sample was inspected.
- Whether design-chat (non-ticket) Claude sessions feed the ledger.
- Whether the Claude Code statusline fires on a schedule when idle: only with `refreshInterval` set,
  otherwise event-driven (doc above). gx's installer does not set it
  (`cmd/claude_statusline_install.go:32-35`).
- How a server restart interacts with a stored rate-limit payload (nothing exists yet).
