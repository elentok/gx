# gx:test-suite-stability/11 - Vitest root causes

Question: for each of the 8 Vitest tickets in `test-flake-inventory.md`, what is the root cause and what is the fix?

Date of runs: 2026-11-17, worktree `test-suite-stability-design`, HEAD `f6fea9571`. No product or test file was changed in the tree: each injection below was a temporary edit, reverted with `git checkout` (`git status` is clean). Facts and guesses are marked.

## Answer in short

- **Today the suite is green when it runs alone.** `pnpm exec vitest run --maxWorkers=3`: 241 files, 3296 tests, 0 failed, 82 s. The 9 files that hold the 8 tickets passed 22 of 22 runs (10 plain, 12 under load), and 25 of 25 more under load (4 workers, 16 busy loops on 12 cores).
- **One real flake reproduced in a loaded full run: an unhandled error.** 5 full runs, 6 workers, load 22 to 29: 4 passed and 1 failed with all 3296 tests green. Vitest exits 1 because a 2 s timer in `src/ui/menu/Menu.tsx` fired after jsdom was torn down (`ReferenceError: document is not defined`). Neither a test title nor a ticket names it. The brief log has no test failure for it, so it looks like a silent red run.
- **Two groups of the tickets have a proven cause, by injection** (a delay added to a fake, then the test fails every time, and the delay is removed):
  - The test waits for one async source and reads another. EpicPage tone/comet (`webui-test-flakes/07`), "keeps its card's badge" (`follow-ups/530`), the three EpicPage menu tests: the card comes from the epic *list* fetch, the test waits on the *detail* fetch.
  - A stale page cache paints the old page. routes "a Merge blocked epic" (`webui-test-flakes/07`): the first half of the test writes the page cache, the second render paints it before its own fetch resolves.
- **`follow-ups/515` (ScreenModal Refresh) has a proven cause.** The Refresh button is `disabled={loading}`. The test waits for the first fetch call, not for the button to be enabled. A click on a disabled button does nothing, so the second call never comes and the wait runs out at 10 s.
- **`follow-ups/518` is already fixed** on this branch (`d343997b9`). `webui-flakes/01` is obsolete. `test-health/03` is not a flake of that test: it was a six-test failure of a staged tree (see the table).
- **InterruptControl (`webui-test-flakes/06`, `test-health/18`) is not reproduced.** The failing log shows the wait ran the full 10 s. That means the popover never closed, not that it closed slowly. A fix landed on Nov 15 (`88e8646cb`); it is not shown to work under load.
- **Timeouts are not the lever.** 47 of the 86 failing Vitest runs in the brief log that later passed on the same tree show a failure at 10 s (the whole `asyncUtilTimeout`). A wait that ran out 10 s did not need more time; its condition never came true.

## Method

- Read the 8 tickets, the inventory (Vitest table), `webui/vite.config.ts`, `webui/src/test-setup.ts` and each test.
- Ran each ticket's file alone (10 runs), with 16 busy loops (12 runs, 25 more runs), and the whole suite under load (5 runs). Load average at the full-run start: 22 to 29. Another gx session was running on the machine.
- Injection: changed a fake in the test to resolve 100 to 300 ms later, ran the one file, then `git checkout` of the file. This proves a race without waiting for a rare failure.
- Failing logs: read the brief logs named in the tickets, with `jq` (the logs are JSON). Counted failing runs in `~/.local/share/gx/brief/index.jsonl` that have a passing run with the same `tree_id` (a hash of the whole working tree, so the code did not change).
- Scratchpad: `/private/tmp/claude-501/-Users-david-dev-gx-test-suite-stability-design/c0cad4c3-ef97-4f18-9704-5e88bb1f94da/scratchpad/vt/` (`rep.sh` repeat runner, `load.sh`, `fullload.4.log`, `failfiles.tsv`).

## Facts: today's runs

| Run | Result |
| --- | --- |
| Full suite, `--maxWorkers=3`, load 3 to 6 | 241 files, 3296 passed, 82 s |
| The 9 ticket files, 3 workers, alone, 10 runs | 10 pass |
| Same files, 16 busy loops, 12 runs | 12 pass |
| 5 files (InterruptControl, newSession, ArchivePage, RestartDialog, ScreenModal), 4 workers, 16 busy loops, 25 runs | 25 pass |
| Full suite, 6 workers, load 22 to 29, 5 runs (about 7 min each) | 4 pass, 1 fail: all tests green, `Errors 1 error` (`document is not defined`, `Menu.tsx:269`, from `EpicPage.test.tsx`) |

## Table

| Ticket | Cause | Evidence | Fix | Validation |
| --- | --- | --- | --- | --- |
| webui-test-flakes/07 (EpicPage "-impl page uses its card's tone and comet") and follow-ups/530 (EpicPage "keeps its card's badge") | **Waits on one async source, reads another.** The test waits for `findByText("1/2")` (the epic *detail* fetch) and then reads the ring's `data-tone` and the comet. The card comes from the epic *list* fetch (`epicList`), a separate promise, in `EpicPageBody` (`joinedCard`). Under load the list can resolve after the detail. | Proven by injection: with `epicList` resolving 300 ms late, 5 tests fail: `expected 'muted' to be 'running'`, `expected 'implementation' to be 'followups'`, and the three menu tests (`expected ['Copy epic ID...'] to deeply equal [...4 items]`). Passes today without the delay. | In the `render` helper of `EpicPage.test.tsx`, return after both fetches settled (await the list and the detail), or wait on the card-dependent output (`waitFor` on the ring's `data-tone`, `findAllByRole("menuitem")` count). Do not read card output after waiting for detail text. | Same injection (300 ms list delay): 0 failures; 20 loaded runs of the file: 0 failures |
| webui-test-flakes/07 (routes "a Merge blocked epic ... a Merging epic its label") | **A stale page cache paints before the fetch.** The test renders `/e/gx:e` twice in one test (`cleanup()` between). The first render writes the page cache in `sessionStorage`; `afterEach` clears it only after the test. The second render paints the cached "Paused" header at once, so `findByTestId("stats-line")` resolves on the old content and the next line reads `Paused` for the `Merging` epic. | Proven by injection: second fetch resolving 100 ms late gives `expected 'Paused · $0.00 · 0s agent time' to match /^Merging · /`. Same cache the "seeded page cache paints the header" test uses on purpose. | Wait on the text that proves the new epic painted: `await waitFor(() => expect(statsLine()).toMatch(/^Merging · /))`. Also `sessionStorage.clear()` between the two renders, so the cache is not an input. | Injection: 0 failures after the change; 20 loaded runs: 0 failures |
| follow-ups/515 (ScreenModal "Refresh does one plain read and sends no key") | **Clicks a disabled button.** `Refresh` is `disabled={loading}` (`ScreenModal.tsx:122`). The test waits for the fetcher's first call, which happens as the load starts, then clicks. If the first fetch has not settled, the button is still disabled, `fireEvent.click` does nothing, and the second `waitFor` runs out. | Proven by injection: fetcher resolving after 150 ms gives `expected "spy" to be called 2 times, but got 1 times` after 10 030 ms. Passes today with an instant fetcher. | Wait until the first load shows (`findByText("row one")`) or the button is enabled before the click. | Injection (150 ms fetcher): 0 failures after the change; 25 loaded runs: 0 failures |
| webui-test-flakes/06 and test-health/18 (InterruptControl "Esc again closes the confirm...") | **Not reproduced. Likely a lost Esc.** The failing run on Nov 15 (`20261115T141548...-vitest.log`) shows the failure at 10 019 ms: the popover was open for the whole 10 s wait, so Esc never reached the confirm. `88e8646cb` (Nov 15) now waits for focus inside the dialog before sending Esc. | Fact: failure duration = `asyncUtilTimeout`. The old test sent Esc to `document.activeElement ?? document.body` right after `findByRole("alertdialog")`. Guess: focus had not moved into the dialog yet, so the key went to the body. | Keep the focus wait. Replace the `{ timeout: 5000 }` overrides (they are below the 10 s global) with the global. | 47 loaded runs of the file with the fix: 0 failures. A bad-then-good proof needs a delayed focus (not done). |
| webui-test-flakes/07 (why both failed together) | Two different races (rows 1 and 2), one per test. They shared a run because both read a second async source under the same load. | Run `20261109T082318` in the ticket; its log is gone from the brief dir. | Rows 1 and 2. | Rows 1 and 2 |
| (not in any ticket) **Unhandled error fails the run with every test green** | `keepFocusInDialog()` in `src/ui/menu/Menu.tsx:265` sets a 2 s `setTimeout` (`SETTLE_MS`) at each menu close and never cancels it. If the test file ends first, jsdom is torn down and the timer calls `document.removeEventListener`, which throws. Vitest reports an uncaught exception and exits 1. | Fact: loaded full run 4 of 5: `ReferenceError: document is not defined` at `Menu.stop` (`Menu.tsx:269`), `This error originated in "src/epic/EpicPage.test.tsx"`, `Test Files 241 passed`, `Errors 1 error`. Not reproduced by a one-test scratch file (the worker exits before 2 s). Guess: the same leak can come from any test file that closes a menu near its end. | Product side: `keepFocusInDialog` returns a disposer, and the menu calls it on unmount. Test side: `test-setup.ts` `afterEach` runs a `cancelFocusWindow()` that the module exports. A leaked timer then cannot outlive the test. | 10 loaded full runs, 0 `Errors`. A unit test: pick a menu item, unmount, and `vi.getTimerCount()` is 0. |
| test-health/03 (AgentsPage newSession "closing the New session form stays on the page") | **Not a flake of this test. Stale.** The failing run is a `related` run of a staged tree that failed 6 tests in 2 files in 21 s, including `Unable to find an element with the text: Pick a session`: the page body never rendered. The test is now titled "closing the New chat form stays on the page". | Fact: log `20261113T172146.803563000/test.log` (6 failures, two files, run from the `main` checkout). 0 failures in 47 runs today. Guess: the staged tree itself was broken at that moment. | Close. No test change. | 47 runs: 0 failures |
| follow-ups/518 (routes "every page route pattern has one of the 16 route names") | **Real failure, fixed.** The guard counted the new `/rest/break` demo route. `d343997b9` filters `/rest/*`. | The test passes in the full run today. | Close with the commit link. | Full suite green |
| webui-flakes/01 (NewDesignForm refusal) | **Obsolete.** The component was renamed to `DraftPage` and the dialog tests were dropped (`inventory`). | No such test exists. | Close. | None needed |
| follow-ups/530 (ArchivePage "spinner before data", RestartDialog "once") | **Not reproduced.** Their sibling in the same ticket (EpicPage badge) is row 1. The ArchivePage test sets `resolve` from inside the fetcher and calls it after `findByRole("status")`, which is safe only while the router calls the loader before it renders the spinner (it does). | 37 runs of both files under load: 0 failures. | Close the two with "no repro". Do not add waits without a failure. | None possible without a failure |

## What the brief log says about Vitest flakes

Facts, from `~/.local/share/gx/brief/index.jsonl` (12 days):

- 86 failing Vitest runs have a passing run on the same `tree_id` (no code change). 64 failed in one file, 14 in two files, 8 in three or more.
- 47 of the 86 show a failure that took about 10 s (the global wait). All 8 runs with 3 or more failing files do. Those runs took 120 to 170 s against a median of 82 s for a clean full run: the machine was overloaded, and each wait ran out.
- The brief log lists failures by test but gives no unhandled errors, so the `document is not defined` run (this report) would have no failing test name.

Guess, not tested: most of the 86 runs share the two races above (waiting on one async source, reading another), since the tests that failed most often (`Composer`, `ActionCard`, `AgentsPage.composer`) read results from several fetches. The report did not run those files.

## Lessons for the Vitest skill

- **Wait on what you read.** If the assertion reads output from fetch B, wait on output from fetch B, not on text from fetch A.
- **A test that runs after another half of itself shares its cache.** Clear `sessionStorage` and any page cache between two renders in one test, or wait on the new content.
- **Never click a disabled control.** `fireEvent.click` on a disabled button is silent. Wait until it is enabled first.
- **Never fix a 10 s failure with a longer wait.** A wait that ran out the whole timeout means the condition was never true. Find the lost event or stale state.
- **A timer in a product module needs a disposer,** or a leaked timer fires after jsdom is gone and fails the run with every test green. Test setup should cancel module-level timers in `afterEach`.
- **To prove a race, inject delay into the fake,** run the one file, then remove it. A race you cannot make fail is a guess.
- **Check the whole run, not only failed tests.** Vitest prints `Errors` and exits 1 with no failed test; the brief log does not show them.
- **Raise nothing blindly:** the repo already sets `asyncUtilTimeout` to 10 s and `testTimeout` to 20 s (`0a0b52c10`, `e0ef7f874`). The rest is wiring.

## Not done

- The InterruptControl lost-Esc hypothesis was not reproduced; a delayed-focus injection would settle it.
- The 5 other `Composer`, `ActionCard` and `AgentsPage.composer` clusters in the brief log were counted, not reproduced.
- The `Menu.tsx` timer was not made to fail in a small scratch test; the cause is the stack trace plus the code.
