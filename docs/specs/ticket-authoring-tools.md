## Problem Statement

A person who plans work with `gx-to-tickets` pays for the agent's whole authoring run, and the run is slow and costly in ways that gx code could remove.

In 36 finished runs (Oct 6 to Nov 1, 2026) the median run made 54 API responses and 49 Bash calls and cost $11.70. Of 1,936 Bash calls, 78.9% learned the code before the first ticket was written. That part is judgment and stays with the agent. The rest repeats mechanical work:

- Every run that published wrote its own helper to create tickets. 4 of 9 had a failed `gx tickets add`. A failed call shifts the ticket IDs, and the agent then renumbers the cross-references by hand.
- 6 of 11 runs that built verify tickets wrote a generator script for the budget packing.
- Agents run `--help`, `go test -list` and path lookups to check names in ticket text. These checks are small in count (about 1.6% of Bash calls) but they are the ones whose misses reach an implementer as a wrong claim.
- Agents stamp `verified at <commit>` by hand and check the context-window cap, the Test seams section and the Where to look section by eye.

Two more problems sit in the numbers the skill gives the agent.

- `expected_context_window` has no single definition. The skill says to budget "for the whole session", but the actual window includes a fixed start-up prefix of 43K to 50K tokens that the ticket's work does not cause. The skill also says cold estimates "run low". Measured on work alone they run 1.12 to 1.28 times too high. The median actual-to-expected ratio is 1.94 only because the actual keeps the prefix.
- The prefix is a different constant in different places (45K, 48K, 50K), and the overrun check in `gx evolve inputs` uses 50K.

gx's own `actual_cost` also overstates cost. It sums usage on every assistant line, and one API response writes several lines. Over the 36 runs it is 2.1 times the deduplicated cost. Any before-and-after comparison must not use it.

## Solution

gx gets five commands or command changes. The `gx-to-tickets` skill calls them and keeps the judgment work: slicing, estimating, and writing the Where to look prose.

- `gx tickets publish` creates a whole set of tickets from one manifest, maps the cross-references, and either writes all of them or none.
- `gx tickets lint-claims` checks that the names in ticket text exist (commands, paths, test patterns, flow ids) and prints only the misses. `publish` runs it.
- `publish` also checks the mechanical rules (Test seams, Where to look, the context-window cap), stamps the commit it checked against, and prints an advisory line for tickets that look bundled.
- `gx tickets verify-plan` builds the verify and collect tickets from the spec's user stories and adds them to the manifest.
- `gx tickets calibration` prints an epic's median ratio of real work to estimate.

`expected_context_window` now means the ticket's work only. gx adds the prefix from one place, so the estimate, the split limit, the overrun check and the ratios all use one definition.

All six pieces are built, in a fixed order, and the saving is measured afterwards with a committed script. The measure uses deduplicated cost and API responses per published ticket.

## User Stories

1. As an agent running `gx-to-tickets`, I want to publish all tickets of an epic with one `gx tickets publish` call, so that I do not write a helper script and make one call per ticket.
2. As an agent, I want to write `{{slug}}` in a ticket body wherever I mean another ticket, so that I never renumber cross-references by hand.
3. As an agent, I want `blocked_by` in the manifest to name slugs, so that I do not need to know ticket IDs before they exist. When I publish into an existing epic, I want `blocked_by` to also accept the ID of a ticket already in that epic, so that a new ticket can depend on an old one. A bare ID is the only accepted ID form. A form with a `/`, such as `other-epic/06`, is refused.
4. As an agent, I want a manifest that lists a blocker after its dependent to still publish, so that I do not have to sort by hand.
5. As an agent, I want a dependency cycle refused before anything is written, so that I find it while drafting.
6. As an agent, I want the whole manifest checked first (unique slugs, each slug starting with a letter, no slug named `commit`, valid fields, body files present, every `{{slug}}` known, every blocker ID real), so that a typo stops the run early.
7. As an agent, I want a failed publish to leave the epic unchanged, whether the failure is a write or a check, so that a retry does not start from shifted IDs and no draft is left behind.
8. As a person running ralph-loop, I want the published tickets to become schedulable together, so that the scheduler never claims one ticket of a half-written set.
9. As an agent, I want `--dry-run` to print the IDs that would be assigned and to change nothing, so that I can check the plan.
10. As an agent, I want a table (slug, address, window) after publishing, and `--json` for the same data, so that I can read the result or script it.
11. As an agent, I want `publish` to validate each new ticket and print only the failures, so that I do not run `gx tickets validate` once per ticket.
12. As an agent, I want `publish` to create the draft epic when it is missing, so that I do not need a separate `epic add` call.
13. As an agent, I want `gx tickets lint-claims` to check the claims in an epic's tickets, or in a manifest for a named epic, so that I can lint before and after publishing.
14. As an agent, I want the lint to read only backticked spans, with one exception, the plain-prose forms `flow N` and `new flow N`, so that ordinary prose never causes a false alarm.
15. As an agent, I want gx commands and flags checked against gx's own command tree, so that I do not run `--help` on each one.
16. As an agent, I want file paths checked against the working tree, and only spans that clearly are paths treated as paths, so that a wrong path is caught before an implementer follows it and a Go selector, a number or a glob causes no false miss.
17. As an agent, I want `-run` patterns checked against the `Test*` names in the repo's test files, so that I do not run `go test -list` for each pattern.
18. As an agent, I want a flow id checked against the e2e catalog, so that a flow that does not exist, or a "new" flow whose number is already taken, is caught.
19. As an agent, I want a name that the ticket or one of its blockers says it will create to print as "planned" and not as a miss, so that tickets about new code do not fail the lint. In a `verify` ticket, a name that any implement ticket of the same manifest or epic says it will create also prints as "planned", because a verify ticket has no blockers. Any other miss in a `verify` ticket prints as a warning and never refuses, because its text is the spec's and cannot be rewritten.
20. As an agent, I want real misses printed first, then planned names, then skipped checks, so that I read the important lines first.
21. As an agent, I want `publish` to refuse when the lint has a miss and `--allow-claim-misses` to override it, so that a false alarm I have checked does not block me.
22. As an agent, I want a "line N" or `file:N` reference to produce a warning that says to name a symbol, so that stale line numbers stop appearing without blocking a publish.
23. As an agent working in another project, I want each check to run only when its tool or file exists and the output to name each skipped check, so that a missing catalog never causes a false failure and I know what was not checked.
24. As an agent, I want the lint to print the short commit it checked against, so that I can state which code state the claims were checked on.
25. As an agent, I want `publish` to refuse an `implement` ticket that lacks `## Test seams`, so that I find the gap while drafting. The section passes when its heading is followed by at least one non-blank line. gx does not check the content, so `none — <reason>` and a bare `none` both pass. Verify and research tickets are exempt.
26. As an agent, I want `publish` to refuse an implement ticket that lacks `## Where to look`, so that the implementer never starts cold.
27. As an agent, I want `publish` to refuse an `implement` ticket with an `expected_context_window` above the split limit, so that an oversized ticket is split before it runs. Verify tickets follow the verify-plan packing rules and are not checked.
28. As an agent, I want one flag, `--allow-check-misses`, to override these three checks, so that I can publish a deliberate exception.
29. As an agent, I want to write `verified at {{commit}}` and have `publish` replace it with the short commit it linted against, so that I do not run `git rev-parse` myself.
30. As an agent, I want a one-line advisory when a ticket has 5 or more acceptance criteria and 5 or more files, so that I re-check the slice before going on.
31. As an agent, I want that advisory in `--dry-run` as well, and never as a refusal, so that it informs and never blocks.
32. As an agent, I want `gx tickets verify-plan` to turn my plan of per-story item counts into verify tickets, so that I do not hand-write the budget packing.
33. As an agent, I want verify-plan to copy each story's Verification entry word for word from the spec, so that the verify ticket tests exactly what the spec says. The only change is that every `{{` is written as `\{{`, so that `publish` does not read a quoted placeholder as a real one.
34. As an agent, I want verify-plan to produce one lone `verify` entry when everything fits one part, and `verify-part-N` entries plus a `verify-collect` entry otherwise, so that the result matches the verify format.
35. As an agent, I want verify-plan to write its entries into the publish manifest, so that all tickets are published in one call.
36. As a maintainer, I want the per-item estimates, the 15K base and the split limit in one Go table that `verify-plan --table` prints, so that the skill and `gx-evolve` stop carrying their own copies.
37. As an agent, I want the estimate defined as the ticket's work only, so that I do not add the session start-up cost to every number.
38. As a maintainer, I want the runner to stamp the prefix on each finished ticket from the same session whose peak is the ticket's actual window, and to leave it empty when that session was resumed, so that work is computed from a measured prefix and not a wrong one.
39. As a maintainer, I want one function that returns a finished ticket's work (actual minus prefix, else a single 48K fallback), so that every reader agrees.
40. As a person reading `gx evolve inputs`, I want the overrun flag to mean work above 2 times the estimate, so that the flag does not depend on a second prefix constant.
41. As an agent, I want `gx tickets calibration <epic>` to print the epic's median work-to-expected ratio and each finished sibling's numbers, so that I recalibrate the not-yet-started tickets from real data.
42. As an agent starting a new epic, I want calibration to say "no finished tickets, use 0.83" and exit 0, so that I know to use the default constant.
43. As an agent, I want calibration to say "approximate" when it used the fallback prefix, so that I weigh old tickets correctly.
44. As an agent, I want the skill to tell me that cold estimates run 1.12 to 1.28 times the real work, so that I estimate lower.
45. As an agent, I want the skill text for steps 3, 4b, 4c and 6 to call the commands instead of repeating the checks, so that my run has fewer calls.
46. As an agent running `gx-implement`, I want the two Edit rules to state their real reasons and not contradict each other, so that I follow each for the right cause.
47. As a person, I want a committed script that measures API responses and deduplicated cost per published ticket, so that I can compare against the baseline.
48. As a person, I want a follow-up that reminds me to run the script after 3 `-impl` epics have been authored with all six slices, so that I do not have to remember.
49. As a person, I want a follow-up that records the `actual_cost` double count with its evidence, so that the cost bug is tracked and not forgotten.
50. As a person, I want clear failure rules for the comparison, written in the reminder follow-up, so that I can decide per slice whether to keep it.
51. As a person with old tickets, I want finished tickets and `gx tickets validate` untouched, so that no new rule can fail an old ticket.

## Implementation Decisions

### Build or not, and order

All four candidates are built, plus two additions the measurements asked for: one definition of the estimate (the work function and the `prefix_tokens` stamp) and the calibration command. Nothing is dropped. The computed estimate is dropped as an idea (see "Estimates"). Make targets are dropped from the lint.

The upper bound of the saving is 17.6% of authoring cost for all four candidates (lint 10.1%, publish 3.1%, verify plans 3.0%, stamps and checks 1.8%). With the lint at its lower bound it is 8.3%. Each new command costs the agent 1 to 2 calls per run, which comes off these numbers. The lint's expected saving is only 1.5%. Its real value is fewer wrong claims reaching an implementer. Publish and verify-plan are built mainly to remove a failure class and two hand-written scripts.

The slices are built in this order, one slice each. The skill text change for a command ships in the same slice as the command.

1. The work function, the `prefix_tokens` stamp, the schema text, the `evolve inputs` overrun change, the skill's estimate text, and the table file in `tickets` that holds the split limit (with the skill test that keeps the skill's 60K equal to it). The Edit-batching skill fix (below) also ships here because it is small and independent.
2. `gx tickets publish`.
3. `gx tickets lint-claims`, wired into `publish`.
4. The `publish` checks, the `{{commit}}` stamp and the bundled-ticket warning.
5. `gx tickets verify-plan`, writing into the manifest of slice 2. It adds the verify estimate rows to the table file from slice 1.
6. `gx tickets calibration`, the committed measuring script, and the two follow-ups.

### Scope rules

- The new commands are used by `gx-to-tickets` only, but they work on any epic in any project (`project:epic` addresses).
- Each lint check runs only when its tool or file exists. The output names every check it skipped. A skipped check is never a failure.
- `gx tickets validate` does not change. It fails on errors in any ticket of the project, so a new rule there would fail old tickets.
- Checks for languages other than Go are out of scope. A check that does not apply is skipped and named.

### Estimates and the work function

- `expected_context_window` means the ticket's work only: the reads, edits and test runs the ticket needs. The fixed start-up prefix is not in it. Stub windows from `ensure-code-review` and `ensure-verify` stay as they are.
- The runner stamps a new frontmatter field, `prefix_tokens`, on a finished ticket. It holds the context occupancy of the first model call of the same session whose peak becomes `actual_context_window` (in `writeMergedMetrics` in `ralphloop/report_metrics.go`, the merging session). If that session was resumed, its first call can carry old history, so the field stays empty. It is stamped next to `actual_context_window`, on every path that stamps that field, and for every agent kind whose transcript reader can supply the first-call occupancy. A transcript that cannot supply it leaves the field empty. The measured prefix ranged from 43.5K to 49.6K (median 49.2K) and moved over time, so one constant is not enough. The implementer picks the exact field name only if `prefix_tokens` clashes with an existing name.
- One function returns a ticket's work. Work is the actual window minus the ticket's prefix. If the ticket has no prefix, the function uses one fallback constant of 48K. Work never goes below 0. The function reports whether it used the fallback, so callers can say "approximate".
- Finished tickets are not backfilled. Their ratios use the fallback and are labelled approximate.
- The ticket schema text and frontmatter docs describe the new field. The web UI keeps showing the peak window; no new UI field is added.
- `gx evolve inputs` drops its own 50K baseline constant. Its overrun flag becomes: work above 2 times `expected_context_window`. It calls the work function.
- The split limit is 60K of work. It stays at 60K as an experiment, to be checked after 2 or 3 epics. It lives in one Go constant, in a table file in `tickets` that slice 1 creates. Slice 5 adds the verify estimates to the same table. The skill text states the same number, and a skill test keeps the two equal.
- The computed estimate is dropped. On 469 finished tickets the formula's median error against work was 0.40, and the agent's cold estimate's was 0.32. At the 60K split the formula flagged 2 tickets and got none right; the cold estimate flagged 92 and got 41 right. No command computes or prints an estimate. The agent keeps estimating by hand, and `publish` stores the number as written and never rescales it.
- The calibration constant is 0.83 (the best single constant). It is applied by the agent by hand, as text in the skill, and not by a command.

### `gx tickets publish`

Form: `gx tickets publish <project:epic> --manifest <file> [--dry-run] [--json]`. It replaces the per-ticket `gx tickets add` loop for the initial set. `gx tickets add` stays for forks and one-off tickets.

- Manifest. YAML. One entry per ticket with these fields: `slug`, `type`, `blocked_by` (a list of slugs, or bare IDs of tickets that already exist in the same epic), `expected_context_window` (full tokens, not thousands) and `body` (a file path relative to the manifest). A slug matches `[a-z][a-z0-9-]*`: it starts with a letter, so a digits-only slug such as `07` is refused. In `blocked_by`, an entry that is only digits is always an ID. Unknown fields are refused. `parent` and `commitless` are not manifest fields.
- Cross-references. A body writes `{{slug}}` wherever it means another ticket. gx replaces it with the real ticket ID at write time. A placeholder is exactly `{{[a-z][a-z0-9-]*}}`. The escape `\{{` is turned into a literal `{{` and is never replaced, so a body can quote a placeholder (for example `\{{b}}` or `verified at \{{commit}}`). `gx tickets verify-plan` writes every `{{` of copied text with this escape. Any other brace form (for example a Go template such as `{{.Name}}`) passes through unchanged. An unknown slug refuses the whole manifest. `commit` is a reserved slug, because `{{commit}}` is the commit stamp, so a manifest entry with that slug is refused. A blocker that is an existing ticket ID must exist in the same epic, or the manifest is refused. Any `blocked_by` entry with a `/` (for example `other-epic/06`) is refused, because the manifest names tickets of its own epic only. To name an existing ticket in a body, the agent writes its ID directly.
- Order. IDs are assigned in manifest order with blockers first. A blocker listed after its dependent is moved up. A cycle is refused.
- Failure. All or nothing. gx checks the whole manifest first (unique slugs that start with a letter, no reserved slug, no cycles, valid fields, body files exist, placeholders resolve, blocker IDs exist, the lint and the checks below). It also renders every ticket in memory and runs the validate check on the rendered tickets before the first write. It then writes under one epic lock and removes everything it wrote if a write, or a check that still fails after writing, goes wrong.
- Draft first. Tickets are written as `draft` and opened together at the end, after every new ticket validates. The scheduler never sees a half set. This holds for a new epic and for an existing one.
- Epic. If the epic is missing, `publish` creates it as a draft loop epic. Opening the epic stays the skill's last step, after the Summary, `ensure-code-review` and `ensure-verify`.
- Output. A table of slug, address and window, and `--json` for the same data. Before the first write, gx validates each rendered ticket (the same check as `gx tickets validate`, limited to the new tickets) and prints only failures. A failure exits non-zero and writes nothing. If a check still fails after writing, gx removes everything it wrote, as for a write failure, so no draft is left behind.
- `--dry-run` prints the IDs that would be assigned, the lint result and the advisories, and changes nothing.
- Flow numbers (follow-up 113). Nothing is reserved. The skill names a new flow by its describe title, and the implementer picks the number. The lint flags a "new" flow number that is already taken.
- Out of reach of `publish`. The Summary, `ensure-code-review`, `ensure-verify`, and a parent ticket. It does not close or change any source ticket.

### `gx tickets lint-claims`

Form: `gx tickets lint-claims <project:epic>` or `gx tickets lint-claims <project:epic> --manifest <file>`. The epic address is always required: with a manifest, it names the epic whose existing tickets the manifest can use as blockers and whose implement tickets count for the verify rule. `publish` runs the same lint on its manifest and epic.

- Extraction. Backticked spans, in every section of a ticket body, plus one named exception: the plain-prose forms `flow N` and `new flow N`, which carry flow ids. No other unticked prose is read. Each span is sorted by its shape. It is a command if it starts with `gx `. It is a path if it contains a `/` and either ends in a known file extension from one fixed list (`go`, `md`, `yaml`, `yml`, `json`, `ts`, `tsx`, `js`, `py`, `sh`, `sql`, `toml`) or starts with `<dir>/`, where `<dir>` is a top-level directory of the repo. A top-level directory counts only when a `/` follows it. A span with a known extension but no `/`, such as `SKILL.md` or `report_metrics.go`, is skipped. A span that contains `*`, `~`, `$`, a space or `://` is skipped and never a path. A Go selector such as `schema.StatusDraft` or `tickets.Work`, a shell command such as `git rev-parse --short HEAD`, a number such as `0.83` and a `project:epic` form are therefore not paths. It is a test pattern if it is a `-run '...'` form. A `file:func` form checks the file and, for Go files, the function.
- Kinds checked: gx commands and flags, file paths, `-run` patterns, flow ids. Make targets are not checked (5 of 140 checked segments in the baseline).
- Checks, with no subprocess. Commands and flags are checked against gx's own command tree. Paths are checked by stat from the repo root. `-run` patterns are checked by parsing the Go test files for `Test*` names and matching the regexp. This needs no compile, so it avoids the cost of `go test -list`. Subtests after `/` are not checked. Flow ids are checked against the flow catalog.
- Miss and planned. A found name is silent. A missing name that appears in `## What to build` or `## Acceptance criteria` of the same ticket, or of any transitive blocker, is "planned": one printed line and no stop. A `type: verify` entry has no blockers, so for it a name is also "planned" when any `implement` ticket in the same manifest or epic names it in What to build or Acceptance criteria. An existing ticket that a `blocked_by` ID names counts as a blocker: its What to build and Acceptance criteria count for "planned" as well. Any other missing name is a "miss", except in a `type: verify` entry. There the text is the spec's and cannot be rewritten, and it also names things the implementer will pick (test names) or that are missing on purpose, so a miss prints as a warning and never refuses. This fits the fact that tests and files a ticket or its blocker creates do not exist at lint time.
- Output order: misses, planned names, skipped checks. The header prints the short commit and says it checked the current checkout's working tree.
- Stop or warn. `publish` refuses on a miss in an `implement` or `research` ticket. `--allow-claim-misses` overrides it, for a false alarm the agent has checked. Planned names never refuse, and neither does a warning from a `verify` entry. Run alone, the lint exits 1 on a miss in an `implement` or `research` ticket and 0 otherwise.
- Follow-ups 102, 10 and 113. Wrong paths are misses. A "line N" or `file:N` reference is a warning that says "name a symbol, not a line", never a stop. A flow id next to the word "new" must be free in the catalog (a taken number is a miss). A flow id without "new" must exist.
- What stays with the agent. A lint proves that a name exists. It does not prove that a seam's setup works, or that a described behaviour is true. Those checks stay in the skill.
- How to tell a real miss from a false alarm. A miss in a path or command is real until the agent finds a reason (for example a file created by an earlier unpublished ticket). The agent then rewrites the claim (not in a `verify` entry, whose text stays as the spec has it) into the planned form (the name appears in What to build or Acceptance criteria) or passes `--allow-claim-misses`. Slice 3's skill text says this.

### Mechanical checks, the commit stamp and the bundled warning (in `publish`)

- Checks, run before anything is written, on `type: implement` tickets only: `## Test seams` is present, meaning its heading is followed by at least one non-blank line (the content is not checked, so a bare `none` passes and an empty section is refused); `## Where to look` is present; `expected_context_window` is at or under the split limit. Verify and research tickets are exempt from all three. Verify parts follow the verify-plan packing rules, so an over-budget part from verify-plan can still publish. `publish` refuses on a miss. One flag, `--allow-check-misses`, covers all three. Claim-lint misses keep their own flag.
- Stamp. `publish` replaces `{{commit}}` in bodies with the short commit it linted against, in the same pass as the `{{slug}}` replacement. The agent writes `verified at {{commit}}` after building a caller list with gopls. A list built with `rg` keeps the `not verified` label.
- Bundled warning. `publish` prints one advisory line per ticket that has 5 or more acceptance criteria and 5 or more files. The line has the slug, the two counts and "consider splitting". It prints in `--dry-run` too, and never refuses. The backtest on 595 finished tickets gave precision 0.36 against an 18% base rate, and flagged 15% of tickets. The two thresholds live in one Go constant, to retune after 2 or 3 epics. Looser rules (2 or more criteria, more than 4 files, any file over 10 KB) were dropped because they flagged 44% to 88% of good tickets.
- Counting rules. Criteria are the bullets and numbered items under `## Acceptance criteria`, minus boilerplate lines such as "tests pass" or "`make verify` is green". Files are the distinct backticked paths (by the lint's path rule) in `## Where to look` and `## Test seams`. New files count. No other prose is parsed. A hand count matched the scenario count in 8 of 12 tickets and was never off by 2, which a threshold of 5 tolerates.

### `gx tickets verify-plan`

Form: `gx tickets verify-plan <project:epic> --plan <plan.yaml>`, plus `--table`. The spec is `docs/specs/<name>.md` for epic `<name>-impl`, read from the current checkout. It builds the `verify`, `verify-part-N` and `verify-collect` entries and writes them, with their body files, into the publish manifest. The implementer picks how the command is given the manifest file (a flag or a field in the plan).

- Plan. YAML. Per story: its number, its scenario group, and its items by kind (web flow, CLI check, Go test run, skill quote), plus its mock states by name. Mock states are given by name, not by count, so that a state shared by two stories counts once. The implementer picks the field names.
- Spec reading. gx finds each story's `## Verification` entry by story number and copies it word for word, except that it writes every `{{` as `\{{` (the escape of the publish cross-references), so that `publish` leaves quoted placeholders alone. It finds a Mock states line by its state name and copies it the same way. There is no prose parser and no new spec format. A story in the plan that has no entry in the spec is an error.
- Table. The Go table file from slice 1, which already holds the split limit, gets these rows in this slice: the per-item estimates (web flow 6K, CLI check 3K, Go test run 4K, skill quote 2K, mock state 8K), the 15K part base, the 30K collect estimate and the 60K limit. `verify-plan --table` prints it. The skill and `gx-evolve` point to that output and drop their copies. The current stub value used by `ensure-verify` is a different thing and stays.
- The base. The 15K base is fixture start and the ticket read. That is work, so it stays a plain constant.
- Packing. Greedy in story order. Stories that share a scenario group stay in one part. A group over the limit is split across parts. One story over the limit gets a part of its own, with an over-budget note in the body. Each mock state counts once, even when two stories share it.
- Output. When every story fits one part: one lone `verify` entry, no collect entry. Otherwise: `verify-part-N` entries (top level, no parent, no blockers) and one `verify-collect` entry blocked by every part, with a copy of the spec's `## Out of Scope`. A part is never a child of the collect ticket. The body formats follow the verify-epic skill. Every entry sets `expected_context_window`.
- When there is no user-visible behaviour (a docs-only, skills-only or refactor epic), the agent writes no plan and skips the command.
- `ensure-verify` still runs after `publish`, and stays a no-op.

### `gx tickets calibration`

Form: `gx tickets calibration <project:epic> [--json]`. It is a new command, not a line in `gx evolve inputs`. It uses the work function, so the two agree.

- Output. The median work-to-expected ratio, the number of finished tickets used, and each sibling's expected and work numbers. No grouping: one median per epic, no new field and no path parsing. Per-kind calibration was no better than one ratio (median error 0.346 against 0.345).
- Counted tickets. Done `implement` tickets that have both numbers set. Stub windows and commitless tickets are skipped, and so are tickets whose work is 0.
- Edge cases. With no finished tickets it prints "no finished tickets, use 0.83" and exits 0. It prints "approximate" whenever any counted ticket used the fallback prefix.
- The command never changes a number. The agent applies the ratio by hand.

### Skill text changes

All changes below are made to the `gx-to-tickets` skill unless the heading says otherwise. They are ordered by step. The implementer rewords them in B2 English and the gx-unslop rules. The quoted passages are the current text. After each change, the skill tests that assert old phrases (for example "verified at <commit>" and the estimate table phrase) are updated in the same slice.

**Before step 1 and step 3: estimation text (slice 1).**

1. Replace the paragraph that starts "Cold estimates run low. Across about 1,700 finished implement tickets, the actual context window had a median of 2 times `expected_context_window`." Say instead that the estimate is the ticket's work only and gx adds the prefix. Say that cold estimates run 1.12 to 1.28 times the real work, and that they look low only because the actual keeps the prefix. Tell the agent to estimate lower (the constant is 0.83). Keep the advice to split when a ticket sits near the limit or its prose does not list the files it touches, and keep the sentence that a ticket that runs long costs more per token. Drop the "median of 2 times" claim.
2. Replace the opening of "When estimating, budget for the whole session, not just the diff size." The list that follows (full reads, the verify pass, TDD iteration, variant fan-out, test-file fan-out) stays as the list of what makes up work. Remove the words "for the whole session".
3. Keep "if a ticket will need more than 60K tokens, split it", and state that the 60K is work only.

**Step 3: estimation text, bundled warning and calibration.**

4. (Slice 4) In the "Test-file fan-out" bullet, keep "3+ files ... split by file group" as it is. Add one sentence: `publish` warns on 5 or more acceptance criteria with 5 or more files, and a warning means the agent re-checks the slice before going on. Add a note that the 3 or more test files rule is not shown to predict overruns (precision 0.18, recall 0.31).
5. (Slice 6) Replace the paragraph "If tickets earlier in the same epic are already implemented, check their actual cost (`gx tickets schema`'s `actual_context_window`/`elapsed_time` fields, handoff notes, session logs) before finalizing later estimates." with one call to `gx tickets calibration <project:epic>`. With 3 or more finished tickets the agent applies the epic's ratio instead of 0.83 to the tickets not yet started, and may split a ticket that now looks oversized. With fewer, it keeps 0.83.

**Step 4: Test seams (slice 4).**

19. Change "Every ticket must declare ... a `## Test seams` heading" to "Every `implement` ticket must declare ...", so that the skill and the `publish` check agree. Verify and research tickets have no such section. Say that any non-blank line under the heading passes, including `none — <reason>`.

**Step 4b: Where to look.**

6. (Slice 4) In the bullet "Caller lists are verified, not recalled", replace "stamp it `verified at <commit>` (`git rev-parse --short HEAD`)" with the text `verified at {{commit}}`, and say `publish` fills in the commit. The `rg` fallback and its `not verified` label stay.
7. (Slice 4) Keep the authoring reminders for the Test seams section, the Where to look section and the context-window cap in steps 3, 4 and 4b. Add to step 6 that `publish` enforces them.

**Step 4c: Check every claim against the code.**

8. (Slice 3) Add a short lead-in saying that the agent runs `gx tickets lint-claims --manifest <file>` once, before publishing, and fixes each miss or checks it as a false alarm. The lint also runs inside `publish`.
9. (Slice 3) Remove or shorten the bullets that the lint now covers:
   - "CLI commands the ticket quotes: run `--help` on every `gx` command ... Never write a command from memory."
   - "`-run` patterns: run the pattern with `go test -list` ..." (keep the advice to keep the pattern narrow, because a broad pattern can hit the test-run time cap).
   - The path and flow-id part of "Reserved numbers" and "Names a seam or hint relies on". Reading the highest migration number and the highest ticket ID stays with the agent for migration numbers. Ticket IDs are no longer written by hand because of `{{slug}}`.
10. (Slice 3) Keep as judgment rules: current behaviour, where a value is set, signature fallout, wire shapes, helper prerequisites, wiring, and behaviour that must not change. Say that a lint proves a name exists and not that a setup works.
11. (Slice 3) Add one line on "line N" references: name a symbol, not a line.

**Step 6: Publish the tickets.**

12. (Slice 2) Replace the block that starts "Create a new implementation epic as a `draft` first" and the `gx tickets epic add` command: `publish` creates the draft epic. Keep the rule that opening the epic is the last step.
13. (Slice 2) Replace "Create one ticket per call ... in dependency order (blockers first)", the `gx tickets add` example, and the `--blocked-by` paragraph with the manifest and one `publish` call. Keep "Write each body to a scratchpad file with Write ... never a heredoc". Keep the Summary and the rule about hand-driven versus loop-driven epics.
14. (Slice 2) Replace "Before considering any ticket published, run `gx tickets validate <addr>` on it" with a sentence that `publish` validates and prints failures. Keep the instruction for tickets added with `gx tickets add`.
15. (Slice 2) Keep "Tickets can also be forked off mid-flight" and `ensure-code-review` as they are.
16. (Slice 5) In "Verify tickets", replace the estimate table, "Pack the parts", the "One part" and "Several parts" `gx tickets add` examples, and "Keep these numbers in one place so gx-evolve can compare them" with: write the plan, run `gx tickets verify-plan <project:epic> --plan <file>`, and publish once. Say that `gx tickets verify-plan --table` prints the estimates. Keep the "skip when there is no user-visible behaviour" rule and the `ensure-verify` call.
17. (Slice 5) The paragraph "Implement tickets use the same table: add 10000 to `expected_context_window` per mock state ... allow at most 4 mock states" stays as an authoring rule with its own numbers. Reword it so it no longer says "the same table".

**Skill `gx-implement` (slice 1, outside the `gx-to-tickets` order).**

18. In "Make one Edit per contiguous region, not one per line", replace the reason "each call is a turn that re-reads the whole context". The data does not support it: 85% of Edit calls share a message with another Edit, and merging every adjacent Edit would remove at most 1.0% of turns and 0.7% of cost. The new text states two reasons, one per rule.
    - Batching. Adjacent edits to one file cost extra calls, so group them into one Edit.
    - Parallel. "Send file edits to one file one after another, never in parallel" stays, but its old reason ("one failed edit makes the rest of the batch fail") is not supported: 45% of Edits already break the rule and their failure rate is not higher. The only reason kept is that a later edit may depend on text an earlier one changed.
    The implementer words the final text but must keep these reasons, so that the two rules do not contradict each other. Both rules stay. No mechanism is built for batching.

### Measuring the saving

- The measure is the deduplicated cost and the number of distinct API responses per published ticket, for the authoring runs of an epic. gx's own `actual_cost` is not used. It sums every assistant line, and over 36 runs it is 2.1 times the deduplicated cost (one ticket: $2.515 against $0.762). This spec does not fix `actual_cost`.
- The baseline is in the research on the unmerged branch `research/authoring-bash-split` (36 runs to Nov 1, 2026; per epic, authoring runs divided by tickets without a `parent`). Median 2.0 API responses and $0.38 deduplicated cost per ticket; p90 3.6 responses and $1.22. The baseline is not re-run unless the script changes.
- Slice 6 commits the baseline script into the repo with a short usage note. The implementer picks the path. The script exists today only on that branch.
- Slice 6 files two follow-ups in `follow-ups` with the `gx-follow-up` skill:
  1. A bug: gx's `actual_cost` sums every assistant line and is 2.1 times the deduplicated cost. It carries the ratio and the example as evidence.
  2. A reminder: "After 3 `-impl` epics have been authored with all six slices, run the script and compare with the baseline." The body of this follow-up carries the three failure rules below, the baseline numbers and the script path, not only the one sentence.
- The person runs the script, not the epic. They run it after 3 `-impl` epics are authored with all six slices (at least 8 publishing runs; wait for 2 more epics if there are fewer), on the same model tier as the baseline.
- Wrong claims are measured by the share of tickets with `paused-repair` or `has-fix-tickets` in `gx evolve inputs`, before and after.
- Failure rules. The comparison fails if any of these hold: that share rises by more than 5 percentage points; the median API responses per ticket does not fall; the median actual-to-expected work ratio moves farther from 1. The person then decides per slice whether to keep it. The person's write-up names the slice most likely to have caused the change.

## Testing Decisions

- A good test checks behaviour at a public seam: a command's exit code and output, the files in a throwaway ticket store, or the text of a skill. It does not test internal helpers.
- Seam 1, the CLI. Each command (`publish`, `lint-claims`, `verify-plan`, `calibration`) is tested through the Cobra command against a temporary store and a temporary repo. This covers the manifest, the lint kinds, the checks, the stamp, the warning and the packing. Prior art: the `cmd` tests for `tickets add`, `ensure-code-review` and `evolve inputs`.
- Seam 2, the `tickets` package. The work function and the packing table are tested as table tests: prefix stamped, prefix missing (fallback), actual below prefix (work 0). Prior art: the schema table tests.
- Seam 3, the runner stamp. A `ralphloop` test with a fake transcript checks that `prefix_tokens` is stamped next to `actual_context_window`, comes from the session whose peak is `actual_context_window`, and is left empty when the transcript has no first-call occupancy or when that session was resumed. Prior art: the merged-metrics tests.
- Seam 4, skill text. The existing skill test files are extended (one shared file per skill area, each phrase asserted once). They assert structure: headings, command names, link paths and numbers, never long prose. One test keeps the 60K in the skill equal to the Go constant. The tests that assert old phrases ("verified at <commit>", the estimate table phrase) are updated.
- Failure cases that need a test. A failed write removes everything it wrote. The rollback test uses a fault-injecting writer at the `cmd` seam, because all tickets of an epic share one `issues/` directory. A check that fails after writing also removes everything. The status write that opens the tickets happens once, at the end. A cycle, an unknown `{{slug}}`, the reserved slug `commit`, an unknown blocker ID and a missing body file each refuse with nothing written. A `{{.Name}}` in a body passes through unchanged. An escaped `\{{b}}` and `\{{commit}}` in a body become `{{b}}` and `{{commit}}` after publish and are not replaced. A digits-only slug is refused, and a bare number in `blocked_by` is read as an ID. A `blocked_by` with a `/` is refused. A `verify` entry with a missing name prints a warning and does not refuse, while the same name in an `implement` ticket refuses. An empty `## Test seams` is refused and a bare `none` passes. `tickets.Work`, `git rev-parse --short HEAD` and `SKILL.md` give no lint miss. A Go selector, a number and a glob in backticks give no lint miss. A planned name is not a miss. A skipped check is named and does not fail. `--allow-claim-misses` and `--allow-check-misses` override only their own checks. Calibration prints the empty and approximate cases.
- The lint's `-run` check needs no compile, so its tests use plain Go test files in a temp repo.
- No test reads the developer's real config. The new tests follow the project rule on `HOME` and the Claude config directory.

## Verification

All stories are verified with CLI and skill-text checks against a temporary store. There are no web changes, so no catalog flow applies and there is no Mock states list. The verify ticket builds one throwaway fixture with CLI commands in a temp dir: a temp repo with a few Go test files, a flow catalog, a spec with three user stories, and a manifest of three tickets in a blocker chain (no cycle). The implementation tickets' tests do not supply it. The fixture is the verify ticket's own spec, so the text of its Verification entries has `{{b}}`, `{{commit}}`, misses and test names on purpose. Entries 48 to 50 are the exception. They read the real `gx:follow-ups` epic, not the temporary store.

In the commands, `$E` is the fixture epic address, `$M` the manifest and `$P` the plan.

1. Run `gx tickets publish $E --manifest $M`. Expect exit 0 and a table with one row per manifest entry; `gx tickets show $E` lists all of them.
2. In `$M`, one body writes `{{b}}`. After publish, `rg '{{' ` over the epic's tickets finds nothing, and the body holds the real ID of `b`. A body that quotes `{{.Name}}` keeps it unchanged. A body that writes `\{{b}}` holds the literal `{{b}}` after publish.
3. Check the `blocked_by` of the published tickets. They hold IDs and not slugs. Publish into an existing epic with a `blocked_by` that holds the ID of a ticket already in it. The ID is kept. An ID that does not exist exits non-zero and writes nothing. A blocker `other-epic/06` also exits non-zero and writes nothing.
4. Put a blocker after its dependent in `$M`. Publish exits 0 and the blocker has the lower ID.
5. Add a cycle to `$M`. Publish exits non-zero, names the cycle and writes nothing (`gx tickets show $E` is unchanged).
6. Put an unknown `{{x}}`, a missing body file, a duplicate slug, a slug named `commit` and a digits-only slug such as `07` in five runs. Each exits non-zero and writes nothing. A bare `07` in `blocked_by` is read as an existing ticket ID, not as a slug.
7. Run the Go test at the `cmd` seam that publishes through a fault-injecting writer, which fails on the last write (`gx brief go-test -run 'TestPublishRollback' ./cmd`; the implementer picks the test name). A read-only directory does not work, because all tickets share one `issues/` directory. Expect that publish exits non-zero, the epic holds no new tickets and no draft, and a second run without the fault assigns the same IDs.
8. Run the unit test that checks the status write that opens the tickets happens once, at the end, after every new ticket validated (`gx brief go-test -run 'TestPublishOpensOnce' ./cmd`; the implementer picks the test name). Do not watch `gx tickets show` during a running publish.
9. Run `gx tickets publish $E --manifest $M --dry-run`. Expect exit 0, the planned IDs and no change in the store.
10. Run `gx tickets publish $E --manifest $M --json`. Expect valid JSON with slug, address and window per ticket.
11. Add a ticket with an invalid field value. Publish exits non-zero, prints the validation failure and only that, and writes nothing (the epic holds no draft).
12. Run publish for an epic that does not exist. Expect exit 0 and a draft loop epic (`gx tickets show $E` prints `status: draft`).
13. Run `gx tickets lint-claims $E` and `gx tickets lint-claims $E --manifest $M`. Expect the same findings and a header with the short commit.
14. A prose word that looks like a path but is not backticked produces no finding. A body with `` `schema.StatusDraft` `` (a Go selector) and `` `webui/e2e/*.spec.ts` `` (a glob) gives no miss either. Nor do `` `tickets.Work` ``, `` `git rev-parse --short HEAD` `` and `` `SKILL.md` ``, even though `tickets` and `git` are top-level directories and `SKILL.md` has a known extension. `` `tickets/nosuch.go` `` yields a miss.
15. A body with `` `gx tickets nosuch` `` and `` `gx tickets add --nosuch` `` yields two misses and exit 1.
16. A body with `` `docs/nowhere.md` `` yields a miss. The same path in the body of a blocker's What to build yields a "planned" line and exit 0.
17. A body with `` `-run 'TestNoSuch'` `` yields a miss. `` `-run 'TestPublish'` `` with a matching test file yields none.
18. These are plain prose without backticks, the one exception to backtick-only. A body with "flow 999" (absent) yields a miss. "new flow 5" with 5 taken yields a miss. "new flow 998" free yields none.
19. A body with a name that its own What to build says will be created prints it as planned and does not refuse. In a `verify` entry, a command such as `gx tickets publish` that an `implement` ticket of the manifest names in its Acceptance criteria also prints as planned and does not refuse. With `--manifest`, an `implement` ticket already in `$E` that names the command in its Acceptance criteria gives the same "planned" line, and so does an existing blocker (named by ID) for a new ticket. In a `verify` entry, a miss such as `gx tickets nosuch` or `TestNoSuch` that no ticket plans prints as a warning, and publish exits 0 without `--allow-claim-misses`. The same miss in an `implement` ticket still refuses.
20. The output lists misses, then planned names, then skipped checks, in that order.
21. A manifest with a miss makes `publish` exit non-zero and write nothing. The same run with `--allow-claim-misses` exits 0.
22. A body with `` `server/x.go:42` `` prints the "name a symbol, not a line" warning and exits 0.
23. Run the lint in a temp repo without Go files or a flow catalog. Expect exit 0 and lines that name the skipped Go and flow checks.
24. The lint output header shows the same short commit as `git rev-parse --short HEAD` in the checked checkout.
25. Remove `## Test seams` from an `implement` ticket in `$M`. Publish exits non-zero and names the ticket. A heading with no non-blank line under it also exits non-zero. A `none — <reason>` entry passes, and so does a bare `none`. The same removal on a `verify` or `research` ticket passes.
26. Remove `## Where to look` from an `implement` ticket. Publish exits non-zero. The same on a `research` or `verify` ticket passes.
27. Set `expected_context_window` of an `implement` ticket above the split limit. Publish exits non-zero and prints the limit. The same value on a `verify` entry passes.
28. With `--allow-check-misses`, the runs in entries 25, 26 and 27 exit 0. The same flag does not override a claim miss.
29. A body that contains `verified at {{commit}}` holds the short commit after publish, equal to the commit in the lint header.
30. A ticket with 5 criteria and 5 files prints one advisory line with slug, `5`, `5` and "consider splitting". A ticket with 4 and 5 prints none. Both runs exit 0.
31. The same advisory prints under `--dry-run`, and no run is refused because of it.
32. Run `gx tickets verify-plan $E --plan $P` on a plan of three stories that fits one part. Expect one `verify` entry in `$M` and no collect entry. With a plan over the limit, expect `verify-part-1`, `verify-part-2` and `verify-collect`.
33. The body of each verify entry contains each story's Verification entry from the spec word for word. In `$M`, every `{{` of the copied text is written `\{{`. After publish, the quoted block in the ticket (with `{{commit}}` left as written, not stamped) has an empty `diff` against the spec's entry.
34. In the several-parts case, the collect entry is blocked by every part, the parts have no parent and no blockers, and the collect body holds the spec's Out of Scope section.
35. After `verify-plan`, `gx tickets publish $E --manifest $M` publishes implement and verify tickets in one call and exits 0. `gx tickets ensure-verify $E` then exits 0 and adds nothing.
36. Run `gx tickets verify-plan --table`. Expect the per-item estimates, the 15K base, the 30K collect estimate and the split limit. The skill text no longer contains a copy of the table (a `rg` of the skill for "Web flow run" finds nothing).
37. Read the skill text. It says the estimate is work only, contains no "budget for the whole session", and contains "1.12" and "1.28" or an equivalent statement. The skill test for the 60K equals the Go constant passes (`gx brief go-test -run 'TestToTicketsSplitLimit' ./skills`; the implementer picks the test name).
38. After a finished fixture ticket run through the runner test seam, its frontmatter holds `prefix_tokens` next to `actual_context_window`. A transcript without a first call leaves it empty. The prefix comes from the session whose peak is `actual_context_window`, and a resumed session leaves it empty (run the `ralphloop` runner test for this).
39. A Go test of the work function: prefix 45K and actual 70K gives work 25K; no prefix gives actual minus 48K; actual 40K gives 0.
40. Run `gx evolve inputs` on a fixture with a ticket of work 2.5 times its estimate and one of 1.5 times. Only the first has the overrun flag. `rg sessionBaselineTokens` in the code finds nothing.
41. Run `gx tickets calibration $E` on an epic with three finished implement tickets. Expect exit 0, the median ratio, `3` as the count and one line per sibling with expected and work. `--json` gives the same data as valid JSON.
42. On an epic with no finished tickets, expect the line "no finished tickets, use 0.83" and exit 0.
43. On an epic whose finished tickets have no `prefix_tokens`, expect the word "approximate" in the output. With stamped prefixes it is absent.
44. Read the skill text. It says cold estimates run 1.12 to 1.28 times real work and that the agent should estimate lower, and it names the 0.83 constant.
45. Read the skill text for steps 3, 4, 4b, 4c and 6. Step 3 contains `calibration`. Step 4 says "Every `implement` ticket" for the Test seams rule and no longer says "Every ticket must declare". Together they contain `lint-claims`, `publish`, `verify-plan`, `{{commit}}` and `calibration`. It does not contain `git rev-parse --short HEAD`, the per-ticket `gx tickets add` example, or the "go test -list" instruction.
46. Read the `gx-implement` skill. The batching rule says that adjacent edits to one file cost extra calls, so they are grouped. The parallel rule keeps only the reason that a later edit may depend on text an earlier one changed, and no longer says "one failed edit makes the rest of the batch fail". Neither rule says "each call is a turn". Both Edit rules are still present.
47. The measuring script exists in the repo at the path the slice chose, and `python3 -I <path> --help` exits 0 with the usage described in its note.
48. `gx tickets show` for the `follow-ups` epic lists a follow-up whose text contains "After 3" and "all six slices".
49. `gx tickets show` for the `follow-ups` epic lists a follow-up on `actual_cost` that contains the ratio 2.1.
50. The follow-up in entry 48 states the three failure rules (more than 5 points, API responses not falling, work ratio farther from 1), the baseline numbers and the script path.
51. `gx tickets validate` on an old ticket without `prefix_tokens` exits 0. A grep of the validate code and its tests shows no new body-section rule.

New flows or scenarios to write: none for the web. The verify ticket builds the temp-repo fixture above with CLI commands.

## Out of Scope

- Changes to the `gx-implement` fork behaviour and to the code-review fix ticket skills. The one exception is the Edit-batching reason in slice 1.
- Lint checks for languages other than Go.
- The judgment rules of follow-ups 47, 45, 102 and 10 (a setup that does not do what the ticket needs, false "fails on HEAD" premises, incomplete call-site lists, big moves retyped by hand). They stay as follow-ups.
- A command that computes or prints an estimate.
- Grouping calibration by kind of ticket.
- Fixing gx's `actual_cost`. It is filed as a follow-up.
- Backfilling `prefix_tokens` on finished tickets.
- Reserving flow numbers.
- Make-target checks in the lint.
- A mechanism that merges Edit calls.
- Running the comparison. The person runs the script later.

## Further Notes

- Baseline numbers come from 36 finished `gx-to-tickets` runs. The first sample of 25 runs in the initial prompt was not available, so the numbers differ. Only 9 of the 36 runs published.
- The 60K split limit and the 5-and-5 bundled threshold are experiments. Each lives in one constant and is retuned after 2 or 3 epics.
- Cold estimates are right on average but noisy: the median error against work is 0.32. The agent's estimate is therefore a hint for the split, and the calibration ratio is a correction for the epic, not a guarantee.
- The prefix has moved over time (43.5K in early October, 48K to 49.6K late October, 45.5K on Oct 31 and Nov 1). If it moves again, new tickets are right because the runner stamps it. Only old tickets use the 48K fallback.
- A lint false alarm costs the agent a call, so planned names (names a ticket or its blocker creates) are printed and never block.
