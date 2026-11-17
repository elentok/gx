# Root cause of the two deterministic test regressions

Ticket `gx:test-suite-stability/08`. Read-only investigation on branch `test-suite-stability-design` (trunk `74bfde98f`). No product code or test was changed. Both tests were re-run alone on 2026-11-17 with `gx brief go-test` and still fail.

## Summary

| Test | First bad commit | Wrong side | Confidence |
|---|---|---|---|
| `TestSessionAnswer_CodexQuestion*` (`server/sessions/prompt`) | `c60114f61` | Product code: the screen matcher `checkOverlay` | High. The cause is read directly from the code. |
| `TestConfigReload_StoreCommitDebounceApplies` (`server/lifecycle`) | `1f7173cc4` | Most likely product code (the new commit loop); the test is not stale | Low to medium. The exact trigger is not proven. |

- Case 1: the title change was intended, but the commit forgot a second reader of the title. The matcher compares the title to the text Codex draws on screen, and the screen never had the header. Fix the matcher. Leave the test fixtures alone.
- Case 2: the test does not assume the old polling loop. It follows the contract in the new `SetSettings` comment ("the new debounce applies from the next change"). The test edits a file after the reload, so a correct loop commits about 1s later. It does not. I could not run an instrumented build (the permission system refused a scratch worktree), so the trigger stays a hypothesis. The next step is one debug run.

## Case 1: `TestSessionAnswer_CodexQuestion*`

### Facts

- Failure (re-run today): `codexquestion_test.go:255: ... Refused:true Reason:codex-mismatch Message:the question text on the screen differs from the card; nothing was sent`.
- `c60114f61` (2026-11-16, "Ask cards: number questions by position and fold the header into the title"). Its message: the web showed "QApproach" and a cramped header in the number dot, so the label is now the 1-based position and the header prefixes the title. This is a deliberate UI change.
- It changed `server/sessions/chat/questioncards.go` so that `CodexQuestionCard` and `ClaudeDialogCard` now set `Label: strconv.Itoa(i+1)` and `Title: headed(it.Header, it.Question)` (for example `"Color: Which color?"`).
- It updated the tests that assert the card shape: `server/sessions/chat/questioncards_test.go`, `server/sessions/chat/codexquestion_internal_test.go`, `server/sessions/askflows/askhook_test.go`. It did not touch `server/sessions/prompt`.
- The matcher is `checkOverlay` in `/Users/david/dev/gx/test-suite-stability-design/server/sessions/prompt/codexquestion.go`, line 126: `claudeui.Normalize(o.Question) != claudeui.Normalize(q.Title)`.
- `o.Question` comes from `codexui.ParseOverlay` (`agentrunner/codexui/overlay.go`). It joins the lines between the `Question N/M (K unanswered)` header and the first numbered row. It never contains the header.
- The Codex fake draws the overlay as `"  Question %d/%d (...)"` followed by `"  " + q.Question` (`testutil/agentfake/codex/codexquestion.go:109-110`). The header is not drawn.
- The prompt tests use `Header: "Header "+id, Question: "Question "+id+"?"` (`codexquestion_test.go:28`). The screen shows `Question a?`; the card title is now `Header a: Question a?`. So they differ.
- The fixtures are not at fault. The tests build the screen from the same `Question` text the real overlay shows. They build the card through the product's `CodexQuestionCard`, so they follow whatever title the product makes.
- `gx:follow-ups/532` (draft) reports the same 8 failures on main, with the suggested fix "fix the matcher or the tests' fixture screen". It has no root cause. This research gives it one.
- `c60114f61^` passes and `c60114f61` fails (ticket 06).

### Verdict

- The change was intended. The matcher is the bug: it equates "card title" with "text on the screen", and that stopped being true.
- Which side must change: product code, `checkOverlay`. Do not edit the prompt-package fixtures to draw the header. That would make the fake disagree with real Codex (hypothesis: real Codex draws only the question; the parser and fake were written that way and the card used to match it).

### Exact fix

File: `/Users/david/dev/gx/test-suite-stability-design/server/sessions/prompt/codexquestion.go` (the comparison at line 126), plus the card builder in `/Users/david/dev/gx/test-suite-stability-design/server/sessions/chat/questioncards.go`.

Preferred (clean, no string guessing):

1. Add `Question string` (`json:"question,omitempty"`) to `wire.QuestionCardQuestion` in `server/wire/sessions.go`. It holds the raw question text, with no header.
2. Set it in `CodexQuestionCard`: `Question: it.Question`.
3. In `checkOverlay`, compare `o.Question` to `cmp.Or(q.Question, q.Title)`.
4. Check whether the web type for this wire struct is generated or hand-written, and update it if hand-written.

Fallback (no wire change): in `checkOverlay`, accept the screen when `Normalize(o.Question) == Normalize(q.Title)` or `strings.HasSuffix(Normalize(q.Title), ": "+Normalize(o.Question))`. It is smaller but guesses where the header ends. gx must never send keys on a guess, so prefer the preferred fix.

Also add a regression test next to `TestSessionAnswer_CodexQuestion*` only if a gap remains. The existing 8 tests already cover it once the matcher is fixed.

Not affected (fact, by reading): the Claude dialog path has its own screen check in `server/sessions/prompt/claudedialog.go`; `rg` finds no `Title` comparison in it. Verify this when fixing, because the same commit changed `ClaudeDialogCard`.

## Case 2: `TestConfigReload_StoreCommitDebounceApplies`

### Facts

- Failure (re-run today): `configreload_store_test.go:40: timed out waiting for a commit after the reloaded debounce`, after 10.13s. The log shows the index `Refresh` firing every 200ms until the server stops.
- `1f7173cc4` (2026-11-12) replaced the loop's ticker (`pollFor(debounce)`, at most 1s, and `debounce/4` after a reload) with a `kick` channel, a `quiet` timer and a 60s `fallback` ticker. `server/index/watch.go` calls `OnStoreChange` for every non-`.git` watch event, and the server wires it to `Holder.TouchStoreCommits`, which calls `Loop.Touch()`.
- New loop behaviour (`/Users/david/dev/gx/test-suite-stability-design/storecommit/storecommit.go`, `run`): `Touch` calls `restart()`, which arms `quiet = time.After(debounce)` and replaces any earlier timer. Only when `quiet` fires does it call `commit`. `SetSettings` only stores the new values; its comment says "the new debounce applies from the next change".
- A later commit, `9b2485904`, added `Check()` and a poll-driven `OnPoll`. `OnPoll` runs only when `events == nil`, so it is off while the watch is live (and in this test, where watch is on).
- The test: server starts with `StoreCommitDebounce = time.Hour`, waits for the start-up flush to commit the seed, reloads with `ticket-store.commit-debounce: 1` (1 second, see `config/ticket_store.go` and `settings.go:174`), writes `edit.md`, then waits 10s for a new commit.
- Reload is accepted (the test's `res.Changed` check passes; the failure is at line 40, after it).
- Old loop: it polled `git status`, so a new debounce was picked up at the next tick, regardless of events. That is why the test passed before. `83148dda0` passes and `1f7173cc4` fails (ticket 06).
- `gx:follow-ups/499`: first reported against base `a91460c44`. Its suggested fix is "check the loop's debounce re-arm in `settings.ApplyStoreCommits`". It has no root cause.

### Verdict

- The test does not assume the old polling loop. It writes the edit after the reload and waits 10s for a 1s debounce. Under the new design a correct loop commits about 1s after the edit's watch event, so the test's expectation is valid. It still depends on a wake-up path (a watch event), which the new design allows.
- So the product side is more likely wrong: the loop does not commit after the new debounce. I cannot say which path fails without instrumentation.

### Hypotheses (unproven), in order of likelihood

1. The quiet period never ends. `OnStoreChange` fires for every non-`.git` event "relevant to the index or not", and each `Touch` restarts the quiet timer. A steady stream of events under the store (the server's own writes, such as appends to non-ticket files) would starve the 1s debounce. Under the old loop only the dirty-file signature mattered, and the `git status` signature ignored unchanged writes. Supports it: the loop commits only when 1s passes with no event at all.
2. The edit's event never reaches `Touch` (watch event lost or filtered), and with the watch live neither `OnPoll`/`Check` nor the 60s fallback helps within 10s. Supports it: `Check` is wired only when the watch is off; the fallback is 60s, longer than the test wait.
3. A pending timer is not re-armed on reload. `SetSettings` does not touch an already armed `quiet` (possibly set to `time.Hour` by start-up events). A later `Touch` replaces it, so this alone would not fail the test, but it is a real gap for "reloaded debounce applies" when no further event comes.

### Decisive experiment for the impl epic (one run)

In a scratch worktree, add `println` on: each `Touch`/`kick` (with `settings()` debounce), `quiet` firing, and each event in `KeepFresh` (name). Run the test with `-v`. Read: no events after the write means hypothesis 2; a steady stream of events means hypothesis 1; one event then no `quiet` fire means something else.

### Fix direction (by hypothesis)

- H1: ignore events that do not change the dirty set. Make the watch hook call `Check()` (restart only when the dirty signature changes) instead of `Touch()`, or filter events on the files `git status` would report. Keep the 1s test unchanged.
- H2: wire `OnPoll` / `Check` also while the watch is live, at a bounded rate, or shorten the fallback; do not change the test timeout.
- H3: have `SetSettings` re-arm the timer when one is pending (send on `kick` or `check` after storing). Cheap and fits the documented contract. Do it either way.
- Do not change the test's expectation. A test-side tweak (a longer wait, or a second write) would hide a real product gap. The product contract after `1f7173cc4` is: a reloaded debounce applies to a change made after the reload.

Files: `/Users/david/dev/gx/test-suite-stability-design/storecommit/storecommit.go`, `/Users/david/dev/gx/test-suite-stability-design/server/settings/holder.go`, `/Users/david/dev/gx/test-suite-stability-design/server/index/watch.go`, `/Users/david/dev/gx/test-suite-stability-design/server/server.go`. Test: `/Users/david/dev/gx/test-suite-stability-design/server/lifecycle/configreload_store_test.go`.

## Validation plan

Always capped, and always `-count=1`.

Case 1:
1. `gx brief go-test ./server/sessions/prompt -run 'TestSessionAnswer_CodexQuestion' -count=1` (8 entries must pass).
2. `gx brief go-test ./server/sessions/prompt -count=1` (whole package).
3. `gx brief go-test ./server/sessions/chat ./server/sessions/askflows ./server/wire -count=1` (the card shape tests of `c60114f61` must stay green).
4. Web: if the wire struct gained a field, run the web type check and the unit tests for the question card component (`pnpm run <script>`, check `package.json` first).

Case 2:
1. `gx brief go-test ./server/lifecycle -run 'TestConfigReload_StoreCommitDebounceApplies' -count=1`, then 20 times (`-count=20`) to catch a flake.
2. `gx brief go-test ./storecommit -count=1` (the loop's own tests from `1f7173cc4`).
3. `gx brief go-test ./server/lifecycle ./server/settings ./server/index -count=1`.
4. `gx brief go-test ./server/ticketops -run StoreCommit -count=1` (uses `DisableWatch`, the poll path).

Both cases, then the full suite capped: `gx check --full` (or the repo's capped full run via `scripts/proc-cap`). Never plain `go test ./...`. Pass condition: the 8 Codex entries and the debounce test pass, and nothing else changed state compared with the inventory in ticket 05. Run the four Nov 16 packages together once (`server/prflow`, `server/sched`, `server/sessions/prompt`, `server/lifecycle`) to confirm they pass under load.

## Lessons for the Go skill

From case 1 (a UI-text change broke a screen matcher in another package):
- Find every reader of a field you change. A card title feeds both the web UI and a screen-comparison safety check. Before changing a wire or card field, `rg` the field (or use the `gx-gopls` references) across all packages, not only the one with the tests that assert it.
- Run the dependent packages' tests before commit, not only the package you edited. The commit updated three packages' tests and still broke a fourth.
- Do not use presentation text as a comparison key. Keep a raw, machine-meaningful field (the raw question) next to the display text (the headed title). Display text then can change without breaking checks.
- A test that builds its expected value from the product's own builder (the card) and its input from an independent fake (the screen) catches this kind of drift. Keep that shape. Do not "fix" such a failure by editing the fake to match the new display text.
- A safety check ("never send keys to a screen we did not verify") must fail loudly and early in tests. It did; the failure was clear but sat unnoticed on main for days because nobody ran the package.

From case 2 (a wake-up path replaced a poll):
- When you replace a poll with an event wake-up, list every setting and state change that used to take effect at the next tick, and give each its own wake-up. A reload that changes a timer's length must re-arm a pending timer.
- Event-driven loops that restart a quiet timer on every event can starve under a steady event stream. Debounce on the change that matters (the dirty set), not on every raw event.
- A test of such a loop depends on a wake-up path. State that dependency in the test comment, and make the harness deterministic (for example, a test hook that triggers the wake-up) when the path is lossy. Do not depend on a platform watcher (FSEvents) for a 10s wait in a unit test.
- Keep a fallback poll short enough to cover lost events within a test's wait, or make it configurable for tests (the test harness already shortens `PollInterval` for the index; the commit loop's `fallbackPoll` is a constant).
- Verify a recurring failure against the commit that changed the mechanism, not only the failing test. Ticket 06 found the commit by hand; both bugs sat in main for days because the first reports (`gx:follow-ups/499`, `/532`) never got a root cause.

## Facts vs hypotheses

Facts (seen in code, commits or runs):
- Both tests fail alone on this branch today (2026-11-17): Codex 8 failures with `codex-mismatch`; debounce test 10.13s timeout.
- `c60114f61` changed card Label and Title for Codex and Claude cards, intentionally, and updated chat and askflows tests only.
- `checkOverlay` compares the card Title to the overlay's question text; the overlay and the Codex fake never contain the header.
- `1f7173cc4` replaced the polling ticker with kick, quiet and a 60s fallback; `SetSettings` no longer touches any timer.
- With the watch live, `OnPoll`/`Check` does not run (`events == nil` guard in `KeepFresh`); the fallback is 60s.
- Debounce 1 in config means 1 second.
- The test's reload step succeeds; only the commit wait fails.

Hypotheses (not proven):
- Real Codex draws only the question, not the header (the parser, fake and old matcher all agree, but I did not check the real Codex).
- The Claude dialog check is unaffected.
- Case 2 cause: event storm starving the quiet timer (H1), lost event (H2), or no re-arm on reload (H3). An instrumented run is needed. I tried one in a scratch worktree, but the permission system refused it.
- The web type for `QuestionCardQuestion` may need a manual update if hand-written.

## Addendum: Case 2 cause confirmed by a debug run (2026-11-17)

Temporary debug prints (reverted, nothing committed) in `storecommit.run`, `index.KeepFresh` and `Startup.AfterMigrations`, running the test alone with `gx brief go-test`:

### Facts

- The commit loop starts (`run start` printed). `KeepFresh` arms the watch (`events` non-nil) and its first scan finishes in about 1 ms.
- During the whole 10 s wait, **zero events** arrive on the store watch channel (`case ev := <-events` never ran), so `Touch` is never called. The same with the sandbox off.
- `KeepFresh` calls `OnPoll` (`CheckStoreCommits`) only when `events == nil`. With a live watch it is never called. The loop's own fallback is 60 s. So the edit is never seen inside the test's 10 s.
- One-line experiment: call `OnPoll` on every poll tick (drop the `events == nil` condition). The test then passed 3 of 3 (14 s total, `-count=3`). Experiment reverted.

### Conclusion

H2 is confirmed as the trigger: a lost or never-delivered store event with no poll fallback to the commit loop. H1 and H3 are not needed to explain the failure. `1f7173cc4` made the poll (every second) the old guarantee into a 60 s fallback, and `KeepFresh`'s comment ("The poll is the guarantee; the watch only makes updates arrive sooner") no longer holds for the commit loop.

Why FSEvents delivers nothing for this test store is not established (the darwin code already says events can be lost). It does not matter for the fix: the poll must be the guarantee.

### Fix for the impl epic

- In `server/index/watch.go`, call `cfg.OnPoll()` on every poll tick, watch live or not. `Check` restarts the quiet period only when the dirty files change, so it is safe to call each tick. With a live watch the poll is `watchedPollInterval`, so the cost is one `git status` per interval; the implementer measures that against the existing per-tick rescan and picks the interval, within the rule that the poll stays the guarantee.
- Also do H3 (re-arm a pending quiet timer in `SetSettings`): it is a real gap against the documented contract, cheap, and a separate failure mode.
- Do not change the test, its 10 s wait or its debounce.
- Validation: the test with `-count=20` alone, then `storecommit`, `server/settings`, `server/index`, `server/lifecycle`, then the full suite, all capped.

### Lessons for the Go skill (addition)

- When a change replaces a poll with an event, keep a poll that still covers the lost event, and test the lost-event path.
- A debug run with prints answers "which wake-up fired" in minutes; a design that says "cause unproven" is not done.
