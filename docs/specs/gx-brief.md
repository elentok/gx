# gx brief: compact output for loud agent commands

## Problem Statement

Agents run loud commands: `go test -v`, `vitest`, `tsc`. A failing run can print thousands of lines. Agents pay for those tokens, and they lose the one thing they need: which test or file failed, and why.

The old fix, rtk, made this worse. It hid failure messages, so agents re-ran with `rtk proxy` to get the raw output and paid twice. Its deny-and-retry hook cost a turn per rejected command. rtk is removed.

Three more problems remain:

- A passing run still prints a wall of text, when one line would do.
- Agents in a fresh worktree call `npx` or a global binary, or guess a type-check command.
- Nobody can see which loud commands agents run again and again, so gx-evolve has no evidence for what to improve.

## Solution

`gx brief <tool> <args>` runs a loud command and prints only what an agent needs. Output is plain text and is for agents only.

- A pass prints one result line, then the log path.
- A failure prints the failing units (a test, a file, or a Go package that does not build) and the first lines of each failure.
- The full output of every run is saved to a log file. The last line of every run names its path, so an agent searches the log instead of re-running the command.
- The exit code tells the caller what happened: 0 pass, 1 fail, 2 brief could not run the tool.

The v1 tools are `go-test`, `vitest` and `tsc`. `go-build` and `go-vet` were added later: each prints one line when clean and groups `file.go:line:col` diagnostics by file on a failure. `brief` runs the local binary, never `npx` or a global install. It never installs dependencies: when they are missing, it prints one line naming the install command and exits 2.

`brief` compacts the output of a command it is given. It does not choose which commands verify a project. That belongs to gx:project-checks.

The skills gx-verify, gx-implement and gx-merge call `gx brief` in place of the raw commands.

## User Stories

1. As an agent, I want `gx brief go-test ./pkg` to print one result line and the log path when the tests pass, so that a pass costs almost no tokens.
2. As an agent, I want the pass line to include the number of tests run (and, for Go, the number of packages) and the run duration, so that I can see how much ran and how long it took.
3. As an agent, I want a failing Go test shown with its name and the first lines of its output, so that I see why it failed without the whole run.
4. As an agent, I want each failing test cut to its first 10 lines, so that one noisy test cannot flood my context.
5. As an agent, I want lines longer than 300 characters cut, so that one huge line cannot flood my context.
6. As an agent, I want the first 5 failing tests shown in full and the rest listed by name only, so that a broken package still gives a short report.
7. As an agent, I want the Go failure shown under its package, so that I know where to look.
8. As an agent, I want a Go package that fails to build reported as one failure named by the package, so that a build error is not lost between tests.
9. As an agent, I want `vitest` failures to follow the same limits and the same field order as Go failures (location, name, text), so that I read one format for every tool.
10. As an agent, I want a failing vitest test shown by file, test name and the first lines of its message, so that I can find it fast.
11. As an agent, I want `tsc` errors grouped by file, with the diagnostics of each file and an error count, so that I can fix one file at a time.
12. As an agent, I want the same limits applied to `tsc` files (first 10 lines per file, first 5 files in full, then names only), so that a type error cascade stays short.
13. As an agent, I want the log path as the last line of every run, so that I always know where the full output is.
14. As an agent, I want the log to hold the complete raw output, so that I can search it with `rg` for a message that the summary cut.
15. As an agent, I want a "re-run only the failing tests" line after a `go-test` or `vitest` failure, so that I can copy the command and skip the passing tests.
16. As an agent, I want no such line after a `tsc` failure, so that I do not get a command that does not exist.
17. As an agent, I want a non-zero exit with nothing parsed to print the first 10 raw lines and the log path with exit 1, so that an unknown failure shape is still visible.
18. As an agent, I want exit 0 on pass and exit 1 on fail, so that I can chain `brief` in scripts and read the result without parsing.
19. As an agent, I want exit 2 only when `brief` could not run the tool, so that I can tell a broken setup from a failing test.
20. As an agent, I want arguments after the tool name passed to the tool, so that `-run`, `-count`, `-timeout` and file lists work as before.
21. As an agent, I want `go-test` to always add `-json` and keep `-v`, so that `brief` can parse per-test results and I do not need to know about it.
22. As an agent, I want `vitest` to run once with the JSON reporter, so that the output is parseable and nothing hangs in watch mode.
23. As an agent, I want a `--watch` argument for `vitest` refused with exit 2 and one line saying why, so that I never start a process that does not end.
24. As an agent, I want `tsc` to run with `--pretty false`, so that diagnostics are plain text.
25. As an agent, I want `gx brief -C webui tsc --noEmit` to run the tool in a subpackage, so that monorepo packages work.
26. As an agent, I want `-C` to be a `gx brief` flag before the tool name, so that it is never confused with a flag of the tool itself.
27. As an agent, I want `brief` to find the tool binary by walking up from the target directory to the git root, so that hoisted workspace installs work.
28. As an agent, I want `brief` to use the local `node_modules/.bin` binary only, so that I never run a global version by accident.
29. As an agent, I want one line naming the install command (from the lockfile) and exit 2 when the binary or dependencies are missing, so that I know what to run.
30. As an agent, I want `tsc` in a directory without a `tsconfig.json` to exit 2 with one line saying so, so that I do not get a misleading pass.
31. As an agent, I want `vitest` and `tsc` to exit 2 with one line outside a git repository, so that the walk-up rule has a clear failure. `go-test` needs no git root and works anywhere.
32. As an agent, I want `brief` to never install dependencies, so that a verify run has no side effects. gx installs them when it prepares worktrees.
33. As an agent, I want `brief` to work with no config file, so that it works in any project without setup.
34. As an agent in a non-gx project (Go, pnpm, vitest, tsc), I want `brief` to give the same output for the same tool output without any gx file in the repo, so that gx skills carry over to other repos.
35. As a gx-evolve run, I want an index with one JSON line per `brief` run, so that I can count loud commands that agents run again and again.
36. As a gx-evolve run, I want each index line to hold the tool, the arguments, the directory, the result, the duration and the log path, so that I can group runs and open the log.
37. As a person with limited disk, I want logs and index lines removed after 14 days, so that the data dir does not grow forever.
38. As an agent running gx-verify, I want its `go test`, `tsc` and `vitest related` rows to run through `gx brief`, so that each row prints one result line on a pass and at most the capped failure report on a fail.
39. As an agent running gx-verify, I want a `brief` exit 2 reported as `fail` with brief's one-line reason, so that a broken setup is never read as a pass.
40. As an agent running gx-verify, I want prettier and eslint left unchanged, so that this effort stays small.
41. As an agent running gx-verify, I want the frozen-install step kept as it is, so that dependencies exist before `brief` runs.
42. As an agent in gx-implement, I want the Go test flags (`-timeout 60s`, `-run`) to go through `gx brief go-test`, so that iteration output is short.
43. As an agent in gx-implement, I want the `server/` background-run rule kept, so that long runs do not hit the Bash timeout.
44. As an agent in gx-implement, I want a rule to search the log path on a fail, so that I do not re-run with `rtk proxy` or raw output.
45. As a triage subagent in gx-merge, I want the flaky-test rerun to use `gx brief go-test -count=1 -run '^TestName$' ./pkg`, so that three reruns cost three short lines.
46. As a maintainer, I want the new gx-implement rule to offer no `rtk proxy` option, so that no skill points at a removed tool. (rtk was already removed from the skills on 2026-10-23.)

## Implementation Decisions

### Command shape

- `gx brief` is a cobra command (ADR 0003) with one subcommand per tool: `go-test`, `vitest`, `tsc`. Each subcommand takes the rest of the arguments unparsed and passes them to the tool.
- `-C <dir>` is a flag of `gx brief`, placed before the tool name: `gx brief -C webui tsc --noEmit`. It sets the directory the tool runs in. The default is the current directory.
- A tool is a small module with one job: build the command line, run it, and parse the output into failure units. A shared core owns the limits, the log, the index, the exit codes and the final line. Adding a tool later means adding one parser, not touching the core. `go build` and `go vet` are not in v1.
- Output is plain text, with no colour and no box drawing. It is for agents only; there is no human mode and no `--json`.

### Running the tool

- Binary lookup:
  - `go-test` runs `go` from `PATH`.
  - `vitest` and `tsc` run `node_modules/.bin/<tool>`. `brief` looks in the target directory, then each parent up to and including the git root. The first hit wins. It never uses `npx`, `pnpm exec` or a global binary.
  - For `vitest` and `tsc` only: if no git root is found above the target directory, `brief` exits 2 with a one-line reason. `go-test` does not need a git root.
  - If `go` is not on `PATH`, `brief` exits 2 with one line saying so.
- Missing binary or missing dependencies: a dependency is missing when the tool binary is not found by the walk-up above; `brief` does not inspect `node_modules` any further. `brief` prints one line that names the install command and exits 2. The command comes from the nearest lockfile at or above the package, using the same table as gx-verify's frozen install (pnpm, npm, bun, yarn). With no lockfile, the line says to install dependencies and that no lockfile was found. `brief` itself never installs.
- Forced arguments:
  - `go-test` adds `-json` and keeps `-v` in the arguments it passes. If the caller passes `-json` or `-v`, they are not added twice.
  - `vitest` gets the JSON reporter and a single run (`run` mode). A caller `--watch` (or `-w`) exits 2 with one line.
  - `tsc` gets `--pretty false`.
  - Every other argument passes through unchanged. The caller's `--reporter` for vitest is replaced, and `brief` prints a one-line note saying so before the result.
- `tsc` needs a tsconfig. When the target directory (or the `-p` project the caller passes) has none, `brief` exits 2 with one line. `brief` does not guess a project.
- `vitest related <files>` is passed through as written. `brief` adds `run` mode and the reporter around it.

### Failure units and limits

- A failure unit is:
  - for `go-test` and `vitest`: one failing test;
  - for `tsc`: one file, with its diagnostics and an error count;
  - for a Go package that fails to build: one unit named by the package, with the compiler output as its text.
- Limits, the same for all tools:
  - at most 10 lines of text per unit;
  - lines longer than 300 characters are cut;
  - the first 5 units are shown in full, the rest are listed by name only.
- The report lists units under their package (Go) or file (vitest, tsc), so the reader sees where each one lives.
- The cut text always says how much was cut, so the agent knows to search the log.
- Non-zero exit with nothing parsed (for example a panic before any test starts, a crashed reporter, or output in an unknown shape): exit 1, the first 10 raw lines, and the log path.
- When the tool exits 0 but zero tests ran (no Go test files, or vitest found no tests), the exit code stays 0 and the result line says "no tests ran" instead of a count.
- Logs have no size cap. The 14-day sweep is the only limit.
- A child process killed by a signal counts as a failure: exit 1, and the log keeps the partial output. When `brief` itself gets an interrupt, it forwards the signal to the child, waits for it, and writes the log and the index line before it exits.
- Exit codes:
  - 0: the tool passed;
  - 1: the tool failed;
  - 2: `brief` could not run the tool (missing binary or dependencies, no tsconfig, caller `--watch`, no git root, the log could not be written).

### Pass and fail lines

- A pass is one line: result, counts, and the run duration. The log path is still the last line, so a pass is two lines in total.
- After a `go-test` or `vitest` failure, `brief` prints a "re-run only the failing tests" line before the log path. It is a ready-to-copy `gx brief` command with the tool's own filter flags (`-run` with an anchored pattern for Go, a name filter for vitest).
  - It covers all failing tests, including those listed by name only, and keeps the caller's other arguments and `-C`.
  - A build-error unit has no test name, so a run with only build errors prints no such line.
  - `tsc` has no such line.
- The log path is always the last line of every run, pass or fail. The exact wording and layout of every line is the implementer's choice, but the log path stays last, and a pass and a fail must be easy to tell apart on the first line.

### Log and index

- Every run writes its full raw output to a log file under the gx data dir (the one beside the ticket store). The directory layout and the file names are the implementer's choice.
- Every run appends one JSON line to an index file in the same place. The index is JSON lines (`index.jsonl` beside the logs). Appending one line is a single write, so parallel runs (several worktrees or subagents) do not interleave lines. The field names are fixed, because gx-evolve reads them: `time` (RFC 3339), `tool`, `args` (array), `dir`, `exit`, `units`, `duration_ms`, `log`. Teaching gx-evolve to read the index is a follow-up, outside this effort's skill wiring. gx-evolve reads this file to find commands that agents run again and again.
- Logs and index lines older than 14 days are removed. The mechanism is the implementer's choice (for example a sweep at the start of a run). The sweep must never fail a run: an error there is ignored. When it rewrites the index, it must keep lines that a concurrent run appends.
- If the log cannot be written, `brief` exits 2 with one line. It does not print a log path that does not exist.
- A run in an isolated test environment (`XDG_DATA_HOME` set to a temp dir) writes there, never to the real data dir.

### Skill wiring

Verified against the current skill files.

- gx-verify:
  - the Go row `go test ./<dir>` becomes `gx brief go-test ./<dir>` (the `go test .` rule for the repo root stays, as `gx brief go-test .`);
  - the TypeScript row `tsc --noEmit -p <pkg>` becomes `gx brief -C <pkg> tsc --noEmit`, run when the package has a `tsconfig.json`, as today;
  - the row `vitest related --run <files>` becomes `gx brief -C <pkg> vitest related <files>`. `brief` adds the single-run mode, so `--run` is dropped;
  - prettier, eslint and gofmt rows are unchanged;
  - the frozen-install step stays as it is, and runs before the checks;
  - a `brief` exit 2 is reported as `fail`, with brief's one-line reason as the detail. It is never skipped and never reported as a pass.
- gx-implement ("Other notes"):
  - the `go test` flags rule (`-timeout 60s`, `-run <pattern>`, one package per call) now reads `gx brief go-test` with the same flags;
  - the `server/` background-run rule stays: those tests run in the background, and the agent waits for the completion notice;
  - new rule: on a fail, search the log path with `rg` instead of re-running with `rtk proxy` or with raw output.
- gx-merge (flaky-test triage, step 1): the rerun becomes `gx brief go-test -count=1 -run '^TestName$' ./pkg`, three times.
- The `.ai/index.md` testing notes keep the web-test and type-check lines. The test-and-worktree-health epic owns the line about fresh worktrees and `node_modules`.
- Choosing which commands verify a project (the verify web row, the implement command list, `make web-types`, per-project command config) is not part of this wiring. gx:project-checks owns it.

## Testing Decisions

- A good test drives `gx brief` as a user would and checks what comes out: stdout text, the exit code, the log file and the index line. It does not test the parser's internal types.
- Seam (one, the highest): the `gx brief` cobra command through the existing command factory, driven with `SetArgs` and `SetOut` like the other CLI tests (ADR 0003). A temp directory stands in for the project, with `XDG_DATA_HOME` pointed at a temp dir.
- The tools under test are real where cheap and fake where not:
  - `go-test` runs the real `go` against a tiny temp module with a passing test, failing tests, a build error and a panic;
  - `vitest` and `tsc` use stub executables placed in a temp `node_modules/.bin`, which print recorded real output (JSON reporter output, tsc diagnostics). No Node install is needed in CI. Recorded samples are real tool output, not hand-written.
- Cases at that seam: pass line; fail layout and limits (10 lines, 300 characters, first 5 in full); build-error package unit; unparsed non-zero exit; exit code 2 for each cause; forced and de-duplicated arguments; `-C`; binary walk-up to the git root; log content and path last; index line; 14-day cleanup using files with old times; re-run line present for `go-test` and `vitest`, absent for `tsc`.
- Skill text is not unit-tested. The Verification section below covers the skills.
- Prior art: the cobra CLI tests that drive the root with fake deps, `testutil.TempRepo` for a temp git repo, and `claudeenv.Isolate()` in `TestMain` so no test reads the real config.
- Tests never read live machine state or the real data dir.

## Verification

This effort changes the CLI and three skills. There is no web or server change, so no catalog flow applies. Checks run in a temp Go module and a temp git repo with stub tools, with `XDG_DATA_HOME` set to a temp dir.

1. Story 1, 2: in a module with a passing test, `gx brief go-test ./...` prints a result line with the test count, the package count and the duration, then the log path last; exit 0.
2. Story 3 to 8 (a failure appears under its package, story 7): with failing tests (one with 30 output lines, one with a 1000-character line, 7 failing in all) and a second package with a build error, `gx brief go-test ./...` shows 10 lines max per test, lines cut at 300, 5 tests in full and 2 by name, the build error as one unit named by the package; exit 1.
3. Story 9, 10: with a stub `vitest` that prints recorded JSON for a failing test, the layout matches the Go one; exit 1.
4. Story 11, 12: with a stub `tsc` that prints 7 files of diagnostics, files are grouped with an error count, 5 in full, 2 by name; exit 1.
5. Story 13, 14: the last line of every run above is the log path; `rg` for a cut line finds it in the log.
6. Story 15, 16: the re-run line is present after a Go and a vitest failure, and absent after a tsc failure.
7. Story 17, 19: a module whose test binary panics before any test prints the first 10 raw lines and the log path; exit 1.
8. Story 18, 19: exit codes are 0, 1 and 2. One check per exit-2 cause: unwritable log (read-only data dir), caller `--watch`, no git root (`vitest`), `go` not on `PATH`, missing binary, no tsconfig. A child killed by a signal gives exit 1 and a log with the partial output.
9. Story 20 to 24, 26: a fake `go` script on `PATH` records its arguments, so the tests see that `-run` and `-count` reach `go test`, that `-json` and `-v` appear once (also when the caller passes `-v`), and that `-C` is not passed to the tool. The vitest and tsc stubs record theirs: single run with the JSON reporter, and `--pretty false`; `gx brief vitest --watch` exits 2 with one line.
10. Story 25 to 28: `gx brief -C webui tsc --noEmit` runs in `webui`; a binary only at the repo root is found from a subpackage; a global `tsc` on `PATH` is never used.
11. Story 29 to 32: with no `node_modules`, `gx brief vitest` prints one line with the lockfile's install command and exits 2, and nothing is installed; `tsc` without a tsconfig exits 2; outside a git repo exits 2.
12. Story 33, 34: the same runs work in a temp repo with no gx config.
13. Story 35 to 37: after a few runs the index file has one JSON line per run with the listed fields; files with old times (older than 14 days) are removed on the next run, newer ones stay.
14. Story 38 to 41: read the gx-verify skill and check its rows against the Implementation Decisions. Run the skill on a change touching one Go file and one TypeScript file and see the three row types go through `brief`, with prettier and eslint unchanged. Force a missing `node_modules` after the frozen install and see `fail` with the one-line reason.
15. Story 42 to 44: read gx-implement and check the three changes (flags, `server/` rule kept, search-the-log rule).
16. Story 45: read gx-merge and check the rerun command.
17. Story 46: `rg -i rtk` over the three skills returns nothing.

No new catalog flows or scenarios are needed. The harness for stories 1 to 13 is the Go test suite from the Testing Decisions.

## Out of Scope

- Fixing or listing flaky tests.
- Comparing failures with trunk to label them new or existing. It doubles run time and needs a clean trunk worktree.
- Choosing which commands verify a project (the verify web row, the implement command list, `make web-types`, per-project command config). gx:project-checks owns it.
- Scoping a run to the packages that changed, the slow-package refusal and a pass cache. These moved to gx:project-checks or were dropped.
- `go build`, `go vet` and other tools beyond `go-test`, `vitest` and `tsc`.
- A human-facing mode, colour or a `--json` output.
- Installing dependencies. gx installs them when it prepares iteration, commitless and feature worktrees.
- A config file for `brief`.

## Further Notes

- Why rtk went away: two cost benchmarks found little or no saving ( https://quesma.com/blog/does-rtk-make-ai-coding-cheaper/ , https://blog.jetbrains.com/ai/2026/07/rtk-claude-code-token-savings/ ). Its filters hid failure messages and its hook cost a turn per rejected command.
- gx-evolve already reports loud or hand-built commands seen in more than one session and files them as `gx brief` ideas. Those follow-ups are the evidence for which tool to add next.
- Helpers stay project-aware, not gx-repo-specific, because gx runs agents in other projects too.
- Exact wording of lines, log file names, the 14-day cleanup mechanism and any internal markers are the implementer's choice. The log path stays the last line, and the exit codes are fixed.
