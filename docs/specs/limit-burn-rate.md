# Limit burn and daily tokens by agent group

Handoff spec from the `limit-burn-rate` map epic in the local ticket tracker (decisions `01` to `07`
in that epic's `issues/` directory). It builds on `docs/specs/limits-and-spending.md` (the top-bar
menu block) and `docs/specs/webui-stats.md` (the Stats page), and it partly reverses ADR 0041.

## Problem Statement

The person running gx wants to improve token efficiency, and cannot measure it. Claude's weekly
limit is the real constraint, but gx shows only its latest value. It keeps no history, so nobody can
tell how many points of the weekly limit a day used, whether today burns faster than yesterday, or
whether the week will reach 100% before the reset. Codex limits never reach the menu at all.

The token and spend charts on the Stats page are also incomplete. They sum ticket stamps, written
when work finishes (ADR 0041). Cancelled and started-over tickets never count, a paused ticket counts
only when it finishes, and every interactive session not linked to a ticket is left out. A Design
that crosses midnight lands on one day. The person cannot see how many tokens a day went into Queue
agents, Design agents, One-offs and everything else, and cannot check that all agents are counted.

## Solution

gx saves limit history and a daily token and spend ledger, and shows both.

- **Limit history.** gx saves a sample of each of four limits (Claude 5-hour, Claude weekly, Codex
  5-hour, Codex weekly) whenever it changes. **Limit burn** per local day comes from the rises between
  samples. Charts start on the day this ships, with no backfill of limit values.
- **Menu.** Each weekly row in the top-right menu gets a chip that compares today's limit burn so far
  with yesterday's (once yesterday is a full day of history), and a forecast line: where the weekly limit will stand at the reset, or when it
  runs out before it. A **Codex limits** block appears under **Claude limits** once gx has Codex limit
  data. The 5-hour rows stay as they are.
- **Stats limit burn.** Four limit burn charts, one per limit. The weekly cards show the 7-day pace
  and the forecast.
- **Token ledger.** gx polls every session it starts (Claude and Codex, with or without a ticket) and
  adds the change in tokens and dollars to a per-day bucket by agent group (Queue, Design, One-off,
  Other), harness and project. One Tokens chart and the Spend chart read the ledger, stacked by agent
  group. A one-time `gx tickets migrate` step rebuilds about 30 past days from transcripts. Days older
  than that show ticket stamps, marked partial.

There is no efficiency ratio. Limit burn and tokens are separate charts on the same days.

## User Stories

**Menu: weekly rows**

1. As the person running gx, I want the weekly limit row to show a chip with today's limit burn so far
   against yesterday's full day ("▲ 7 vs 5 pts"), so that I see at a glance if today burns faster
   than yesterday. I want no chip until yesterday is a full local day on or after the first saved
   sample of that limit, so that a thin history never gives a made-up comparison.
2. As the person running gx, I want the chip to be amber with "▲" when today so far is above
   yesterday and green with "▼" when it is below, so that I read the direction without the numbers.
3. As the person running gx, I want to hover, focus or tap the chip and see the full text ("Burn today
   7 pts so far, yesterday 5"), so that the short chip is never ambiguous, also on a phone.
4. As the person running gx, I want the comparison to say "today so far" and never scale today up to
   a full day, so that a morning row is not read as a prediction.
5. As the person running gx, I want burn shown in whole points, so that the row stays quiet.
6. As the person running gx, I want a forecast line under the reset line that says "~72% at reset"
   when the week will end under 100%, so that I know if I am on pace.
7. As the person running gx, I want the forecast line to say "Runs out ~Thu 14:00, before the reset"
   when the pace reaches 100% first, so that I know when I will be blocked.
8. As the person running gx, I want "Forecast needs 3 days of data" instead of a number until 3 full
   local days have passed since the first saved sample of that limit, so that a thin average never
   looks real.
9. As the person running gx, I want a stale weekly row to keep its muted stale marker and still show
   its forecast, computed from the last known value and the time left to the reset, so that an old
   reading still gives a useful pace. I want the forecast dropped only once the limit is past its
   reset, because no live window is left.
10. As the person running gx, I want a pace of zero to forecast the current percentage with no "runs
    out" line, and no forecast line at all when the weekly limit is at or above 100%, so that an
    idle week does not warn me and a full bar does not get a prediction.
11. As the person running gx, I want the Claude weekly and Codex weekly rows to follow the same rules
    with their own history, so that both limits read alike.
12. As the person running gx, I want the 5-hour rows left as they are, so that the menu does not get
    busier than it needs to be.

**Menu: Codex block**

13. As a Codex user, I want a "Codex limits" block under "Claude limits" with a 5-hour row and a
    weekly row, so that I see my Codex headroom next to Claude's.
14. As a Claude-only user, I want no Codex block until gx has Codex limit data, so that the menu holds
    nothing empty.
15. As the person running gx, I want the open menu to update when a new limit report arrives or a
    usage scan saves a new Codex sample, so that the chip and the forecast stay current without
    reopening it.
16. As a phone user, I want the menu with the chip and forecast to fit the screen, so that it stays
    readable at narrow widths.

**Stats: limit burn charts**

17. As the person running gx, I want one limit burn bar chart per limit (Claude 5-hour, Claude weekly,
    Codex 5-hour, Codex weekly), so that I see how much of each limit each day used.
18. As the person running gx, I want Claude drawn in blue and Codex in purple, so that the harness is
    clear at a glance.
19. As the person running gx, I want each card to follow the Stats range picker (7, 30, 90 days) with
    30 as the default, so that the new charts behave like the old ones.
20. As the person running gx, I want a 5-hour limit's burn to add up every window of the day (it can
    pass 100), so that a heavy day reads as heavy.
21. As the person running gx, I want the headline to be the range sum in points with the daily average
    under it, so that the cards read like the other Stats cards.
22. As the person running gx, I want the headline to say "partial" with the start day (for example
    "starts Oct 9") when the range reaches back before gx saved history, so that I do not read a short
    history as a quiet one.
23. As the person running gx, I want a day before the first saved sample drawn as a gray hatched
    "no data" bar, so that missing history never looks like a zero.
24. As the person running gx, I want the weekly cards to show the 7-day average below the chart ("7-day
    average: 9 points a day"), so that I see the pace behind the forecast.
25. As the person running gx, I want the weekly cards to show the same forecast wording as the menu,
    so that one fact has one phrasing.
26. As the person running gx, I want the 5-hour cards to carry no forecast, so that only the weekly
    limit is predicted.
27. As the person running gx, I want a reset day marked "↺" in the tooltip, so that I understand why
    its number is split across two windows.
28. As the person running gx, I want the tooltip to give the date and the day's burn, so that I can
    read exact numbers.
29. As the person running gx, I want the limit burn charts to stay account-wide under the project
    filter, so that a limit that counts the whole account is never shown as one project's.
30. As the person running gx, I want an "All projects" tag and a short line ("The limit counts all
    use on your account") on each limit card while a project is selected, so that I know why the card
    did not change.
31. As the person running gx, I want the forecast to stay account-wide under any filter, so that it
    matches its chart.
32. As the person running gx, I want a Codex burn chart with no Codex history to show only hatched "no
    data" days, so that the page does not hide a harness I have not used yet.

**Stats: Tokens and Spend**

33. As the person running gx, I want one Tokens chart stacked by Queue, Design, One-off and Other, so
    that I see where the tokens go.
34. As the person running gx, I want the Claude and Codex split in the tooltip, so that the bars stay
    simple and the harness is one hover away.
35. As the person running gx, I want tokens counted on the day they were used, not the day work
    finished, so that a Design that crosses midnight is spread over the days it ran.
36. As the person running gx, I want every session gx starts counted: Queue agents, Design agents,
    One-offs and other interactive sessions, with or without a ticket, so that no agent is missing.
37. As the person running gx, I want tokens to sum input, output, cache writes and cache reads, with
    subagent tokens included, so that the number matches what the model processed.
38. As the person running gx, I want Codex tokens counted with Codex's own total, so that Codex work
    is not invisible.
39. As the person running gx, I want work on cancelled, started-over and still-paused tickets counted
    on the day it ran, so that wasted work shows too.
40. As the person running gx, I want the Spend chart to use the same four groups and the same ledger,
    so that tokens and dollars always agree on the day and the group.
41. As the person running gx, I want Codex to add $0 to spend and never make a day "no data", so that
    the chart never shows a made-up price.
42. As the person running gx, I want both charts to follow the project filter, so that I can look at
    one project's tokens and spend.
43. As the person running gx, I want a session outside any project to show only under "All projects",
    so that no project is blamed for it.
44. As the person running gx, I want the headline to be the range sum with the daily average, and to
    say "partial" when any day in range is stamp-only or rebuilt, so that I do not trust an
    incomplete total.
45. As the person running gx, I want days rebuilt from transcripts shown lighter (or hatched) with a
    tooltip note "rebuilt from transcripts", so that I know the group split there is approximate.
46. As the person running gx, I want days older than the rebuild shown from ticket stamps (Design as
    Design, Implementation as Queue), lighter, with the tooltip note "ticket stamps only", so that
    old history is still there and still marked.
47. As the person running gx, I want each day to come from one source only, never a mix of ledger and
    stamps, so that a bar never counts the same work twice.
48. As the person running gx, I want a day with no data from any source to stay a gray hatched bar,
    and a quiet day inside the ledger's live range to be a zero, so that "no data" never hides a
    quiet day.
49. As the person running gx, I want the stamps to stay in ticket frontmatter for the epic page, so
    that epic Stats rows do not change.

**The token ledger**

50. As the person running gx, I want gx to poll the cumulative tokens of every session it starts and
    add only the change since the last poll to today's bucket, so that a long session spreads over the
    days it ran.
51. As the person running gx, I want a bucket keyed by day, project, agent group and harness, so that
    the charts can split and filter by each.
52. As the person running gx, I want a session's agent group fixed when it starts, so that a session's
    tokens never move between groups.
53. As the person running gx, I want a session seen for the first time counted only from that poll on,
    so that old tokens are not dumped on today.
54. As the person running gx, I want the ledger to remember each session's last cumulative total, so
    that a server restart never counts tokens twice.
55. As the person running gx, I want the ledger to survive a server restart, so that history is not
    lost.
56. As the person running gx, I want the ledger to write its file only when a delta is non-zero, so
    that an idle server writes nothing.
57. As the person running gx, I want the ledger kept in the server state directory, not in the ticket
    store, so that frequent writes never churn the git-backed store.
58. As the person running gx, I want the ledger to store dollars next to tokens, priced when the
    delta is polled with the existing pricing code, so that spend never needs a second pass.
59. As the person running gx, I want every day kept with no pruning, so that history keeps growing
    with me.
60. As the person running gx, I want the Stats page to read the in-memory ledger and no transcripts,
    so that it loads as fast as before.

**Limit history**

61. As the person running gx, I want gx to save a sample (limit, used percentage as the raw number,
    reset time, receive time) when the percentage or reset time changes, so that history has every
    change.
62. As the person running gx, I want at most one confirming sample per hour when nothing changed, so
    that the file grows slowly and "fresh" stays provable. The receive time of a Codex limit's
    newest sample is its freshness for the stale check.
63. As the person running gx, I want a day's burn to be the sum of rises between consecutive samples
    with the same reset time, credited to the local day of the later sample, so that burn follows the
    day it was seen.
64. As the person running gx, I want a fall inside one window ignored, as the current merge already
    does, so that a stale report never creates negative burn.
65. As the person running gx, I want a reset time that moves forward, or a window that has ended, to
    start a new window, and a reset time that moves backward dropped as stale, so that history
    follows Claude's real windows.
66. As the person running gx, I want the first sample after a reset to count in full on its own day,
    and the unknown part before the reset to count as 0, so that gx never invents burn it did not see.
67. As the person running gx, I want points first shown after a gap with no samples to go to the day
    of the later sample, never spread over the gap, so that gx never guesses.
68. As the person running gx, I want Codex samples saved from the usage scan into the same history,
    with the highest percentage across agents for each window and the 5-hour and weekly windows told
    apart by length, so that Codex gets the same charts. I want a scan that saves a new Codex sample
    to emit `limits-changed`, so that the open menu updates.
69. As the person running gx, I want the history kept in the server state directory next to the
    existing limits file, which stays for the Claude stale check, and a Codex row to use the receive
    time of its newest sample with the same stale threshold, so that both harnesses go stale by the
    same rule.
70. As the person running gx, I want every sample kept with no pruning and no backfill, so that charts
    start on the day this ships.

**Migration of past tokens**

71. As the person running gx, I want a `gx tickets migrate` step that rebuilds daily tokens and
    dollars from transcripts (about 30 days, the transcript age limit), so that the charts have
    history from the first day.
72. As the person running gx, I want the step to skip every day that already has live rows, so that a
    live day is never rewritten and a second run changes nothing.
73. As the person running gx, I want the agent group from session metadata and ticket logs where they
    survive, and every other transcript counted as Other, so that gx never guesses from a folder or a
    prompt.
74. As the person running gx, I want the project from the same transcript data, with no project when
    it cannot be found, so that the project filter works on rebuilt days.
75. As the person running gx, I want Codex transcripts included where session metadata or a ticket log
    names the harness, and the rest in Other, so that Codex history is not dropped.
76. As the person running gx, I want a dry run and a printed summary, so that I see what the step
    would write before it writes.
77. As the person running gx, I want the rebuild to be a named migration step that the server applies
    at its first start after the upgrade (ADR 0045, auto-migration), so that I run nothing by hand
    and transcripts do not age out before the rebuild reads them.
78. As the person running gx, I want the rebuild to finish before the token poll starts, and a manual
    `gx tickets migrate <path>` while the server runs to skip every day the live poll already wrote,
    so that a live day is never rewritten.

**Records and parity**

79. As a script author, I want `gx server stats --json` to carry the new limit burn, forecast, Tokens
    and Spend data, so that I can read them without a browser.
80. As a script author, I want the menu's read route and its CLI verb to carry the burn comparison and
    the forecast, so that `--json` shows what the menu shows.
81. As a maintainer, I want an ADR that supersedes the "no precomputed daily table" part of ADR 0041
    for tokens and spend, so that the reason for the ledger is on record.
82. As a maintainer, I want the glossary to name the token ledger and the four agent groups, so that
    the vocabulary stays shared.

## Implementation Decisions

**Vocabulary**

- The glossary already defines **Limit burn** (the points of one usage limit used in one local day; a
  5-hour limit's burn adds up every window of that day, so it can pass 100; avoid "burn rate").
  The glossary reserves "kind" and "agent kind" against **Harness**, so this spec says **agent group**
  for Queue, Design, One-off and Other. The implementer picks the final glossary word and adds the
  **token ledger** entry. It must not reuse "kind".
- Agent group of a session, fixed when the session starts: a session working a ticket in the project's
  `one-offs` epic is One-off. Any other session the server runs for a ticket (an implementation ticket
  and the investigate, code-review, verify and conflict-resolution tickets that follow the same
  queue) is Queue. An interactive session whose **Session ticket** is a map epic or map ticket is
  Design. Any other interactive session is Other. The backfill uses Other for everything it cannot
  place.

**Limit history**

- A new state file in the server state directory, next to the limits file and the token ledger. It is
  not in the ticket store, which is git-backed and would churn. The limits file stays and keeps
  driving the Claude stale check. A Codex row uses the receive time of its newest sample, with the
  same stale threshold as Claude. The implementer picks the file name, its shape and the in-memory structure.
- A sample holds: limit (Claude 5-hour, Claude weekly, Codex 5-hour, Codex weekly), used percentage
  as the raw number, reset time and receive time.
- gx saves a sample when the percentage or the reset time changes, and at most one confirming sample
  per hour otherwise. Claude samples come from the same report path as the limits store (the
  statusline hook). Codex samples come from the usage scan, which takes the highest percentage across
  agents for each window and tells the 5-hour and weekly windows apart by length. A scan that saves a
  new Codex sample emits `limits-changed`, so the open menu updates.
- Burn per local day is the sum of rises between consecutive samples with the same reset time,
  credited to the local day of the later sample. The rules for a fall, a reset, a moving reset time
  and a gap are User Stories 63 to 67. Days use the server's local zone.
- The history keeps all samples. There is no backfill. Days before the first sample are "no data" on
  the limit burn charts.

**Forecast**

- Applies to the current window of Claude weekly and Codex weekly only. Pace is the sum of limit burn
  over the last 7 full local days (today excluded) divided by the days available. Days available is
  the number of full local days since the first saved sample of that limit, capped at 7. Quiet days
  inside that span count as 0.
- The minimum history is 3 full local days since the first saved sample of that limit. Below it the
  menu and Stats show "Forecast needs 3 days of data" and no number.
- Forecast at reset = last known used percentage + pace x time left to the reset (fractional, so
  today's unfinished part counts). Values round to whole points. Over 100%, the time to the limit is
  (100 - used percentage) divided by pace, shown in local time.
- A pace of 0 gives a forecast equal to the current percentage and no "runs out" line. A weekly limit
  at or above 100% gets no forecast line, because the bar is already full.
- A stale limit (the existing stale check) keeps its muted stale marker and still shows its forecast,
  computed from the last known value and the time left to the reset. A limit past its reset has no
  live window, so it gets no forecast. Codex follows the same rules with its own history, and its
  stale check uses the receive time of its newest sample with the same threshold as Claude.
- Precedence of the forecast line: a past-reset row shows no forecast line, ahead of "Forecast needs
  3 days of data". A stale row with fewer than 3 days of history shows "Forecast needs 3 days of
  data".
- Wording in the menu and on Stats: "~72% at reset" under 100%, "Runs out ~Thu 14:00, before the
  reset" over it. The menu row already carries the name "Weekly limit", so the line drops the "Weekly
  limit:" prefix; Stats cards drop it too, because the card title names the limit. Stats adds the pace
  line "7-day average: 9 points a day" under the weekly charts.
- The server computes the pace, the forecast and the comparison. The client only draws them. The
  payload shape and field names are the implementer's choice.

**Menu**

- Weekly rows of Claude limits and, once data exists, Codex limits get the chip on the name line and
  the forecast line under the reset line. The chip compares today's limit burn so far with yesterday's
  full day, with no interpolation. It reads "▲ N vs M pts" (amber) when today so far is above
  yesterday and "▼ N vs M pts" (green) when below. The chip comes from history, so it also shows on a
  stale or past-reset row; only a past-reset row loses its forecast. The row shows no chip until
  yesterday is a full local day on or after the first saved sample of that limit. After that, a
  quiet day inside the history counts as 0 burn. Stats still draws days before the first sample as
  no data. The look when the two numbers are equal is the implementer's choice, following the mock's
  chip style.
- The chip has a tooltip "Burn today N pts so far, yesterday M". It opens on hover, focus and tap,
  like the existing menu tooltips.
- The Codex block sits under the Claude block with its own title, "Codex limits". It appears only
  once gx has Codex limit data. A Codex limit with no stored window shows no row, as for Claude. The
  5-hour rows of both blocks do not change.
- The menu keeps its existing fetch rules (fetch on open, refresh every 60 seconds, refetch on
  `limits-changed`, which a Claude report and a Codex scan that saves a sample both emit). The
  existing read route and its CLI verb carry the new values. The implementer
  picks the response shape.

**Stats: limit burn**

- Four bar charts, one per limit, 30 days by default, ranges 7, 30 and 90 unchanged. Claude blue,
  Codex purple, in theme tokens (the design-system guard test applies).
- Headline: the range sum with the daily average under it, "partial" and the start day when days in
  range are before the first sample. Days before the first sample are gray hatched "no data" bars.
- Weekly cards show the 7-day pace line and the forecast below the chart. 5-hour cards show a note
  that reset days are marked "↺" in the tooltip. The tooltip marks a reset day in the weekly cards
  too.
- Under a project filter the four cards and their forecast stay account-wide. While a project is
  selected each card carries an "All projects" tag and the line "The limit counts all use on your
  account". With no project selected, neither shows.
- The implementer picks where the four cards sit among the page's sections. The mock shows only the
  cards and their look.

**Token ledger**

- A new state file in the server state directory, next to the budget ledger, loaded into memory at
  server start. The Stats page sums the in-memory rows and reads no transcripts. The poll writes the
  file only when a delta is non-zero. It is local to the machine and is not synced with the ticket
  store. The implementer picks the file name and shape.
- The server polls the cumulative tokens of every session gx starts, for Claude and Codex, with or
  without a ticket. It adds the change since the last poll to the bucket for today, project, agent
  group and harness. A session first seen counts only from that poll on. The ledger remembers each
  session's last cumulative total, so a restart never double counts. Tokens sum input, output, cache
  writes and cache reads, subagents included; Codex adds its own reported total.
- Bucket key: day (local), project, agent group, harness. A session outside any project counts under
  "no project", which shows only under "All projects". Retention: all days, about 8 rows per day per
  project.
- Each bucket also stores dollars, priced at poll time with the existing pricing code. Codex adds
  $0. A bucket carries a flag "rebuilt" when the migrate step wrote it. The ledger stores its first
  rebuilt day and its first live day, so the chart knows where each source ends.
- Tokens and Spend by agent group are the only chart data that move to the ledger. Ticket stamps
  stay in frontmatter and are never rewritten; they keep feeding the epic page and old days.
- The budget ledger, its soft and hard limits and `pollBudget` do not change. Spend on the Stats page
  now comes from the token ledger, so it covers sessions the budget ledger skips, and chart totals
  still do not match the budget ledger.
- How a session's group and project are known at poll time, how the poll finds each session's
  cumulative total, and where in the server the poll runs are the implementer's choices.

**Stats: Tokens and Spend**

- One Tokens chart replaces the stamp chart. It stacks Queue, Design, One-off and Other from the
  ledger. The Claude and Codex split shows only in the tooltip. Spend reads the ledger with the same
  four series. Both follow the project filter and the 7, 30 and 90 day ranges.
- Source per day, one source only: a day inside the ledger's live range comes from live rows. A day
  inside the rebuilt range comes from rebuilt rows. A day older than the first rebuilt day comes from
  ticket stamps (Design to Design, Implementation to Queue), under the existing stamp rules for
  Design and Implementation, "no data" and map remainders in `docs/specs/webui-stats.md`.
- "No data": a day with no data in its own source stays a gray hatched "no data" bar only on stamp
  days, by the existing stamp rule. Inside the live and rebuilt ranges a day with no rows is a zero, an
  empty slot on the baseline. Codex adds $0 and never makes a day "no data".
- Rebuilt days and stamp-only days draw lighter (or hatched), and their tooltips say "rebuilt from
  transcripts" or "ticket stamps only".
- Headline: the range sum with the daily average, "partial" when any day in range is rebuilt or
  stamp-only. The average counts only days with data, as before.

**Changes to `docs/specs/webui-stats.md`**

- Story 31 ("Tokens" stacked Design and Implementation) and story 35 ("Spend" stacked Design and
  Implementation) change: both stack Queue, Design, One-off and Other from the ledger on live and
  rebuilt days, and stack Design and Implementation (as Design and Queue) from stamps on older days.
- The "Cost and effort counting rules" no longer apply to live and rebuilt days. They keep applying
  to old days: Design is map epics and their tickets, Implementation is everything else, stamps count
  on the day they were stamped, map epic remainders, the "no data" rule and the Codex $0 rule.
- Stories 32 to 34, 37 and 38 hold on the ledger: tokens sum all four counts, subagents count, Codex
  counts, Codex adds $0 and never makes "no data".
- Stories 36 and 41 to 47 (spend summed per ticket, a ticket stamped only when every session can be
  read, stamps counted once, unlinked interactive sessions left out, conversation records) describe
  stamps. They stay true for the stamp source and the epic page, and no longer limit live days:
  unlinked interactive sessions count on live days, under Other.
- Stories 11 to 16 (headline, average, "partial", tooltip, hatched "no data", zero days) apply to the
  new cards. Story 13's "partial" meaning widens to rebuilt and stamp-only days.
- ADR 0041 stays for everything but tokens and spend: Stats still read stamps for throughput, lines of
  code and friction. A new ADR supersedes only its "no precomputed daily table" part for tokens and
  spend, notes that the ledger holds dollars, and records that chart totals still do not match the
  budget ledger.
- The Stats page layout in the repo has a Grilling section that the stats spec does not list. It does
  not change here.

**Migration step**

- One named, per-project step in the migration registry (ADR 0045, auto-migration). The server applies
  it at start-up and records it on the project's applied list, so it runs once. `gx tickets migrate
  <path>` reruns it by hand, with a dry run and a printed summary like the other steps. It rebuilds daily tokens and
  dollars from transcripts, up to the transcript age limit (about 30 days), into the token ledger.
- It skips every day that already has live rows, so a live day is never rewritten and a second run
  changes nothing. It covers only days before the ledger's first live row.
- Agent group comes from session metadata and ticket logs where they survive (about 28 sessions).
  Every other transcript goes to Other. Project comes from the same transcript data. Codex transcripts
  are included where session metadata or a ticket log names the harness; the rest go to Other.
- Each rebuilt bucket is flagged "rebuilt". The step sets the ledger's first rebuilt day. Dollars use
  the same pricing code. It never writes limit history.
- The server runs the step before the token poll starts, because no work is scheduled until all
  migrations end. The ledger file has its own file lock, which the poll and the step both take. A
  manual run while the server is up takes that lock and skips every day the live poll already wrote.
  A failure saves a Migration block like any other step. The step keys rows by project, so it runs per
  project; transcripts with no project are rebuilt once, by the first project run of the pass.

**Parity and contracts**

- Per ADR 0028 every route has a CLI verb with `--json`. The Stats route and `gx server stats` gain
  the limit burn, forecast, Tokens and Spend data. The menu's read route and its verb gain the
  comparison and the forecast. No new route is required. If the implementer adds one, it comes with a
  verb, the route table entry and the parity test.
- The generated web types come from the server's wire types, so the new payload types need the
  generation step.

**Implementer's choices (not decided by the map)**

- Names: both state files, the field names of every payload, the glossary word for agent group, new
  components and props, and the fixture scenarios' final names.
- The mechanism that finds a session's cumulative tokens, group and project at poll time, the poll
  interval and where the poll runs.
- The look of the chip when today so far equals yesterday, and the exact lighter or hatched style of
  rebuilt and stamp-only bars, within the mock.
- Number, time and date formats at the edges: "~Thu 14:00" for dates beyond a week, token units,
  and the tooltip text beyond the quoted lines.
- Whether a ledger change publishes a throttled event so the open Stats page refreshes, and the
  interval.

## Testing Decisions

A good test checks what a person or a client sees: the menu's rendered rows, the Stats payload and
page for a seeded ledger and history, the state files across a restart, and what a migrate run
writes. It does not check internal structures, call counts or helper functions.

Seams:

1. **Server, through servertest and the web mux.** Seed the limit history and the token ledger, then
   read the Stats route, the menu's read route and the CLI verbs. Ledger cases: fake sessions of each
   agent group and each harness report cumulative tokens, the poll adds only deltas, a restart does
   not double count, a first-seen session counts from its first poll, the project key and the "no
   project" rule, Codex adds $0, the file is written only on a non-zero delta, and a cancelled or
   started-over ticket's tokens still count. Limit history cases: a Claude report and a Codex scan
   each save samples, a change saves and a confirmation saves at most once an hour, the history
   survives a restart, a scan that saves a Codex sample emits `limits-changed`, and a Codex row goes
   stale by the receive time of its newest sample. Prior art: the budget snapshot tests, the limits store tests, the usage sample
   tests, `server/stats_test.go` and `server/stats_bench_test.go`.
2. **Day math, pure functions of `now` and the saved samples or rows.** The server has no clock seam,
   so burn per day, the pace, the forecast and the source per day are table tests. Burn: a rise inside
   a window, a fall ignored, a reset (first sample counts in full, unknown tail is 0), a reset time
   moving forward, a reset time moving backward dropped, a gap credited to the later day, a day
   boundary at 23:59 and 00:01, a daylight-saving week, Codex windows told apart by length. Chip: no
   chip until yesterday is a full local day on or after the first sample, then a quiet day counts as
   0. Forecast: fewer than 3 full local days since the first sample gives no number, days available
   is capped at 7 and divides the sum, quiet days count as 0, a zero pace, a forecast under and over
   100%, the time of "runs out", a weekly limit at or above 100% gives no forecast line, a stale
   limit keeps its forecast from the last known value, a stale limit with fewer than 3 days gives "needs 3
   days", a limit past its reset gives no forecast even with fewer than 3 days. Source
   per day: live, rebuilt, stamp-only and no-data days, and the zero-versus-no-data rule at each edge.
   Prior art: the window sums in `server/budget.go` tests.
3. **Migrate.** `gx tickets migrate` against a fixture store with fake transcripts, ticket logs and
   session metadata: the rebuild, a rerun that changes nothing, a live day skipped, the project and
   group sources, Other for the rest, Codex where named, dollars priced, the dry run writes nothing,
   and the summary. Prior art: the existing migrate tests.
4. **Web, Vitest.** The menu block with props for the chip (above, below, equal, absent on the first
   day), its tooltip, each forecast line (under 100, over 100, fewer than 3 days, stale with the
   stale marker and a forecast, past reset with none, weekly at or above 100% with none, pace zero),
   the Codex block present
   and absent, the 5-hour rows unchanged. The Stats page with a canned payload: four limit burn
   cards, the weekly pace line and forecast, "partial" with the start day, hatched days before
   history, the "All projects" tag only under a filter, Tokens and Spend stacked by four groups, the
   harness split in the tooltip, lighter rebuilt and stamp-only days, the headline's "partial", and a
   zero day inside the live range. Prior art: the Stats page tests, the TopBar tests and the
   limits block tests.
5. **Fixture and flows.** `gxfixture` scenarios that seed limit history and the token ledger with
   dates relative to the real clock, so the days stay right. Prior art: the `limits` and `spend`
   scenarios, which seed state files and hold one fake agent.

CLI parity: the route-table test (`TestRouteTableHasCLIVerbs`) and the web allowlist test keep
passing.

## Verification

A verify ticket runs these against a temporary gx server from `gxfixture`. "Go test" and "Vitest" name
the seam from Testing Decisions that covers the story. A story that a flow, a CLI check or a mock
state already covers has no Go test or Vitest entry: the implement tickets ran those through `gx
check`. New flows take the next free numbers in `webui/e2e/CATALOG.md`; the implementer assigns them.
Existing flows cited by name run against their updated scenario.

New scenarios to write (working name `limit-burn`, with shapes the implementer names). The catalog
has no scenario that seeds limit history or the token ledger:

- `limit-burn`, shape `menu`: Claude limits with weekly at 81% and 7 full days of history that give
  today 7 so far against 5 yesterday and a forecast over 100% (runs out); Codex limits with weekly at
  40% and a forecast of ~72%; both 5-hour rows. The menu is 280px.
- `limit-burn`, shape `few-days`: Claude weekly at 22%, with the first saved sample earlier today, so
  yesterday is not a full local day of history. No chip, and the forecast line needs 3 days. No Codex
  data.
- `limit-burn`, shape `stale`: Claude weekly and Codex weekly, each with 7 days of history, a reset
  still in the future, and a newest reading older than the stale threshold. The row keeps the muted
  stale marker, the last known value and a forecast from that value.
- `limit-burn`, shape `past-reset`: Claude weekly with 7 days of history and a reset time already in
  the past. The row shows no forecast line.
- `limit-burn`, shape `stats`: 30 days of limit history for all four limits (history starting Oct 9
  of the seed's calendar, so the first days are before history), a reset day in each 5-hour history,
  two projects, and a token ledger with live days, rebuilt days and, further back, ticket stamps,
  with all four agent groups, both harnesses, and a day with no data from any source. Width 680px.
- `limit-burn`, shape `live`: a fake agent for each of Queue, Design, One-off and an interactive
  session with no ticket, whose transcripts a flow can extend, so the real poll fills the ledger.
  Prior art: the `spend` scenario's fake agent and `fixture.agentTranscript`.

New flows to write:

- **Flow A, "Menu limit burn":** scenario `limit-burn`, shape `menu`. Open the menu. Check the Claude
  weekly chip "▲ 7 vs 5 pts" (amber), the Codex weekly chip with "▼" (green), the tooltips by hover
  and by tap, "today so far", the Claude forecast line "Runs out ~..., before the reset", the Codex
  line "~72% at reset", the "Codex limits" block under "Claude limits", and 5-hour rows with no chip
  or forecast. Pipe a new statusline payload into `gx claude statusline`; check the open menu updates
  without a reload. Trigger a usage scan that saves a new Codex sample (the implementer picks how the
  fixture does it) and check the open menu updates without a reload. Check the menu in a 320px viewport and see that it does not overflow.
- **Flow B, "Menu limit burn states":** scenario `limit-burn`. Shape `few-days`: no chip on the first
  day, "Forecast needs 3 days of data", no Codex block. Shape `stale`: the muted stale marker, the
  last value and a forecast line still shown, for Claude and Codex. Shape `past-reset`: no forecast
  line, and not "Forecast needs 3 days of data".
- **Flow C, "Stats limit burn":** scenario `limit-burn`, shape `stats`. Open `#/stats`. Check the
  four cards, the weekly pace line and forecast, the 5-hour reset-day note, the "↺" in the tooltip
  of a reset day, the hatched days before history and the "partial ... starts" headline. Pick a
  project; check the four cards carry the "All projects" tag and the same numbers. Pick 7d and 90d.
  At 375px check one column with no horizontal scroll.
- **Flow D, "Stats tokens and spend by group":** scenario `limit-burn`, shape `stats`. Check Tokens
  and Spend stack Queue, Design, One-off and Other, the harness split in the tooltip, lighter rebuilt
  and stamp-only days with their tooltip notes, a gray hatched no-data day, a zero day inside the live
  range, and the "partial" headline. Pick a project; check the "no project" rows drop out and the
  bars change. Check Codex adds $0 on Spend.
- **Flow E, "Live token ledger":** scenario `limit-burn`, shape `live`. Append usage lines to each
  fake agent's transcript, wait for the poll, open `#/stats`, and check today's Tokens bar shows each
  of the four groups. Append again and check only the delta is added. Run `gx server stats --json`
  and check it matches the page.

CLI and server verb checks:

- `gx server stats --json` on `limit-burn` (shape `stats`) has the four limit burn series, the weekly
  forecasts and the Tokens and Spend series by agent group, and matches what Flow C and Flow D read.
- `gx usage summary --json` on shape `menu` carries the weekly burn comparison and forecast.
- `gx tickets migrate --dry-run` on a fixture store prints the summary and writes nothing; the real
  run writes rebuilt days; a second run prints no changes and exits 0.
- Restart the server on the same state directory (Go test, seam 1): the ledger and limit history
  are intact and the next poll adds no double count.

Story entries:

1. Flow A. Mock state menu-weekly-delta. Flow B (`few-days`: no chip on the first day). Go test
   (seam 2): the chip rule.
2. Flow A (amber and green). Mock state menu-weekly-delta.
3. Flow A (tooltip, hover and tap).
4. Flow A ("today so far"). Vitest (seam 4): today's number is never scaled.
5. Vitest (seam 4): fractional burn renders as whole points.
6. Flow A (Codex line "~72% at reset"). Mock state menu-weekly-delta.
7. Flow A (Claude line "Runs out"). Mock state menu-weekly-delta.
8. Flow B. Mock state menu-few-days. Go test (seam 2): the minimum is 3 full local days since the
   first sample, and a stale row with fewer than 3 days says "Forecast needs 3 days of data".
9. Flow B (`stale` shape: stale marker and forecast shown; `past-reset` shape: no forecast). Go test
   (seam 2): a stale limit gets its forecast from the last known value, and a limit past its reset
   gets none.
10. Go test (seam 2): a zero pace gives the current percentage and no "runs out"; a weekly limit at
    or above 100% gives no forecast line.
11. Flow A (Claude and Codex weekly rows). Mock state menu-weekly-delta.
12. Flow A (5-hour rows). Mock state menu-weekly-delta.
13. Flow A. Mock state menu-weekly-delta.
14. Flow B (no Codex block in the `few-days` shape). Mock state menu-few-days.
15. Flow A (updates without a reload, after a Claude statusline payload and after a Codex scan that
    saves a sample). Go test (seam 1): the scan emits `limits-changed`.
16. Flow A, in a 320px viewport: the menu does not overflow (jsdom does no layout, so Vitest cannot
    check this).
17. Flow C. Mock state stats-limit-burn-and-tokens.
18. Flow C. Mock state stats-limit-burn-and-tokens.
19. Flow C (7d, 30d and 90d).
20. Go test (seam 2): a 5-hour burn with several windows in a day passes 100.
21. Flow C. Mock state stats-limit-burn-and-tokens.
22. Flow C ("partial ... starts").
23. Flow C (hatched days before history).
24. Flow C. Mock state stats-limit-burn-and-tokens.
25. Flow C. Mock state stats-limit-burn-and-tokens.
26. Flow C (5-hour cards have no forecast).
27. Flow C ("↺" in the tooltip).
28. Flow C (tooltip).
29. Flow C (project filter keeps the same numbers).
30. Flow C (the "All projects" tag and line, shown only under a filter).
31. Flow C (the forecast is unchanged under a filter).
32. Vitest (seam 4): a Codex card with no history shows only hatched days.
33. Flow D. Mock state stats-limit-burn-and-tokens.
34. Flow D (tooltip split).
35. Go test (seam 1): a ticket's tokens used on two days land on two days. Flow E.
36. Flow E (Queue, Design, One-off and Other).
37. Go test (seam 1): all four token counts and subagent tokens sum.
38. Go test (seam 1): a Codex session adds its own total.
39. Go test (seam 1): a cancelled or started-over ticket's tokens count on the day they ran.
40. Flow D. Mock state stats-limit-burn-and-tokens.
41. Flow D (Codex adds $0). Go test (seam 1): a Codex-only day is not "no data".
42. Flow D (project filter).
43. Flow D (project filter, "no project" rows drop out).
44. Flow D ("partial" headline). Mock state stats-limit-burn-and-tokens.
45. Flow D (rebuilt days lighter, tooltip note). Mock state stats-limit-burn-and-tokens.
46. Flow D (stamp-only days lighter, tooltip note). Mock state stats-limit-burn-and-tokens.
47. Go test (seam 2): the source per day is one source, never a mix.
48. Flow D (hatched no-data day and zero live day). Go test (seam 2): the zero-versus-no-data rule at
    each source edge.
49. Existing flows for the epic page stay unchanged; Vitest (seam 4): the epic page Stats section
    tests still pass.
50. Flow E (only the delta is added). Go test (seam 1).
51. Go test (seam 1): the bucket key splits by day, project, group and harness.
52. Go test (seam 1): a session's group does not change after it starts.
53. Go test (seam 1): a first-seen session adds nothing at its first poll.
54. Go test (seam 1): the restart check.
55. Go test (seam 1): the restart check.
56. Go test (seam 1): an idle server writes no file change.
57. Go test (seam 1): the ledger file is in the state directory and the ticket store has no new
    file.
58. Go test (seam 1): a delta is priced when polled; a Codex delta is $0.
59. Go test (seam 1): a ledger with old days keeps them all.
60. Go test (seam 1): the Stats route reads no transcripts (fake transcripts removed after the poll).
    `gx server stats --json` check.
61. Go test (seam 1): a change saves a sample with all four fields.
62. Go test (seam 1): a confirmation saves at most once an hour; a Codex row goes stale by the
    receive time of its newest sample.
63. Go test (seam 2): burn per day from consecutive samples with the same reset time, credited to the
    later sample's day.
64. Go test (seam 2): a fall inside one window.
65. Go test (seam 2): a reset time moving forward, an ended window, a reset time moving backward.
66. Go test (seam 2): a reset.
67. Go test (seam 2): a gap.
68. Go test (seam 1): a usage scan saves Codex samples; Go test (seam 2): the two windows told apart
    by length and the highest percentage across agents. The scan emits `limits-changed` (see 15).
69. Go test (seam 1): the history file sits next to the limits file, the Claude stale check reads the
    limits file only, and the Codex stale check reads its newest sample's receive time with the same
    threshold.
70. Go test (seam 1): history keeps every sample and the first charted day is the first sample's
    day.
71. CLI check: the real migrate run writes rebuilt days from fixture transcripts. Go test (seam 3).
72. CLI check: the second run changes nothing; a live day is skipped. Go test (seam 3).
73. Go test (seam 3): the group sources and Other for the rest.
74. Go test (seam 3): the project source and "no project".
75. Go test (seam 3): Codex where named, Other for the rest.
76. CLI check: `--dry-run` writes nothing and prints the summary.
77. Go test (seam 1): a server start on a store without the step on its applied list runs it and records it; the token poll starts after it.
78. Go test (seam 3): a manual run with live rows present skips those days.
79. CLI check: `gx server stats --json`.
80. CLI check: `gx usage summary --json`.
81. Check the new ADR exists and states that it supersedes the "no precomputed daily table" part of
    ADR 0041 for tokens and spend.
82. Check the glossary has the token ledger entry and the agent group word, and does not use "kind".

**Mock states** (mock `$(gx tickets root)/limit-burn-rate/mocks/07-menu-and-charts-look.html`):

- `$(gx tickets root)/limit-burn-rate/mocks/07-menu-and-charts-look.html`, menu-weekly-delta, 280px,
  scenario `limit-burn` (shape `menu`, new, listed above), parts: title-claude, row-5h,
  row-weekly-claude, title-codex, row-5h, row-weekly-codex. The Claude weekly chip is amber with a
  "Runs out" line; the Codex weekly chip is green with "~72% at reset".
- `$(gx tickets root)/limit-burn-rate/mocks/07-menu-and-charts-look.html`, menu-few-days, 280px,
  scenario `limit-burn` (shape `few-days`, new, listed above), parts: row-weekly-claude. The row has
  no chip, reads "Forecast needs 3 days of data" and there is no Codex block.
- `$(gx tickets root)/limit-burn-rate/mocks/07-menu-and-charts-look.html`,
  stats-limit-burn-and-tokens, 680px, scenario `limit-burn` (shape `stats`, new, listed above),
  parts: card-claude-weekly-burn, card-codex-weekly-burn, card-claude-5-hour-burn,
  card-codex-5-hour-burn, card-tokens, card-spend.

## Out of Scope

- The TUI. It shows neither limits nor spending today.
- Claude use outside gx in the token numbers, because gx did not start those sessions. The limit
  itself still counts all use on the account.
- Changes to the budget limits and how they behave, and the budget ledger itself.
- Using limit burn or the forecast to pause the queue.
- An efficiency ratio, such as tokens per burn point. Revisit it only as a fresh effort, if the two
  charts show one is useful.
- Backfill of limit values. Charts start on the day this ships.
- A forecast for 5-hour limits.
- Syncing the token ledger or limit history between machines.
- Rewriting ticket stamps, or removing them.

## Further Notes

- Limit burn counts all use on the account, including use outside gx. It is good for pacing and weak
  as a measure of gx efficiency. The Stats page says so under a project filter.
- The stored weekly value can be a whole number, so a small day can be off by about 1 point.
- Claude limit samples arrive only when a Claude session makes a call (the statusline hook), so days
  with no Claude use have no samples and burn 0. Codex limits reach the history through the usage
  scan.
- Tokens used while the server is down are not counted, and a session first seen after a restart
  counts only from then on. The ledger is a record of what the server saw.
- On backfilled days the group split is mostly Other, because only about 28 sessions kept metadata.
- 4 of the last 11 Designs crossed midnight from start to completion, and 2 of those covered three
  calendar days. The ledger spreads them over the days they ran, where stamps put them on one day.
- Decisions behind this spec are on the map epic `gx:limit-burn-rate`, tickets 01 to 07.
