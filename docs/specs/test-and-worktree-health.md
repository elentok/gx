# Test and worktree health: dependencies installed at worktree creation, and hermetic Go tests

Source: wayfinder map `gx:test-and-worktree-health`. Tags like `[05]` point to the map ticket that
settled the point. The spec has two parts. Part A is worktree dependency setup (tickets 01, 05, 06,
07). Part B is hermetic Go tests (tickets 02, 03, 04).

## Problem Statement

An agent in a fresh iteration worktree should be able to run the project's tests on its first try.
Then a red run means its own change broke something. Today that is not true, for two reasons.

**A fresh worktree has no JavaScript dependencies.** gx already runs an install step when it
creates an iteration worktree, but only in the dead `runIteration` path, and it only looks for
lockfiles at the worktree root. gx's own lockfiles are in `webui/` and `web/`, so nothing gets
installed there. The server's live prepare path installs nothing at all. Agents hit
`Cannot find package '@vitejs/plugin-react'` or a 20K-character tsc dump on at least 10 tickets
across 5 retros. Some tried `npm ci` in a pnpm project and got `EUSAGE`. The doc rule at
`.ai/index.md:33` ("run web tests only through `make web-test`") did not stop it. gx runs agents in
other projects too, so a gx-only fix is not enough. [01]

**Some gx Go tests fail for reasons outside the change.**

- The 5 `TestScanWorktrees_RealRepo_*` tests in `cmd` scan the real repo the test runs in. They
  read sibling worktrees, live branches and `.scratch` epics. A branch that another worktree
  deletes mid-scan makes `scanWorktrees` fail, so the test fails in a fresh worktree or next to a
  running loop. `gx cleanup scan` has the same bug next to a running loop. [02]
- The full `go test ./server` run sometimes fails on the server lock. The server tests already use
  temp state dirs and never touch the live server's lock. The failure is a race. `Harness.Restart`
  stops the server and calls `acquireLock` on the same temp dir while a forked child process still
  holds the old lock fd. `acquireLock` fails at once with "already running (pid N)", and N is the
  test process's own pid. [03]

## Solution

**Part A.** When the server prepares a worktree for an agent, gx installs JS/TS dependencies in
every directory of that worktree that has a git-tracked lockfile. It picks the tool from the
`packageManager` field in that directory's `package.json`, with the lockfile as fallback, and runs
the tool's frozen install with `CI=true` and a 5-minute timeout. If anything goes wrong, the ticket
pauses with `iteration-error` before the agent starts, and gx discards the new worktree so a retry
starts clean. `.ai/index.md:33` changes to say that gx installs dependencies when it creates the
worktree. [01, 05, 06, 07]

**Part B.** The `TestScanWorktrees_RealRepo_*` tests become temp-repo tests. `scanWorktrees` skips
a branch that disappears mid-scan. `acquireLock` waits about 2 seconds when the lock file names
this process's own pid. `.ai/index.md` Testing gains the rule: tests never read live machine
state, and they use `t.TempDir()` and `testutil.TempRepo`. [02, 04]

## User Stories

### Part A: dependencies in a fresh worktree

1. As an agent in a fresh iteration worktree of a JS/TS project, I want `node_modules` already
   installed, so that my first test or type-check run reflects my change and not a missing
   package.
2. As an agent in a repo whose lockfiles live in subdirectories (gx's `webui/` and `web/`), I want
   every one of those directories installed, so that tests in any of them run. [01]
3. As a developer running gx on another project, I want gx to find lockfiles with `git ls-files`
   at any depth, so that the install works for any layout and not only gx's. [01]
4. As a developer, I want gx to install only where a lockfile is tracked by git, so that an
   untracked or ignored lockfile (for example one inside `node_modules`) never triggers an
   install. [01]
5. As a developer, I want no opt-out key, so that every project behaves the same way. [01]
6. As a developer whose `package.json` sets `packageManager`, I want gx to use that tool, so that a
   pnpm project never gets `npm ci`. [01, 05]
7. As a developer whose `package.json` has no `packageManager`, I want gx to pick the tool from the
   lockfile in that directory, so that plain projects still work. [01]
8. As a developer, I want gx to leave the tool's version to pnpm and corepack, so that gx does not
   pin or download package-manager versions. [01]
9. As a developer of an npm project, I want gx to run `npm ci`, and never `pnpm import`, so that gx
   never writes or commits a pnpm lockfile in my repo. [01]
10. As a developer of a pnpm project, I want `pnpm install --frozen-lockfile`. [05]
11. As a developer of a Yarn classic project, I want `yarn install --frozen-lockfile`. [05]
12. As a developer of a Yarn Berry project, I want `yarn install --immutable`. [05]
13. As a developer of a Yarn project, I want gx to tell Berry from classic by the `packageManager`
    major version, or, without `packageManager`, by a `__metadata:` block in `yarn.lock`, so that
    the right flag is used. [05]
14. As a developer of a bun project, I want `bun install --frozen-lockfile` for either `bun.lock`
    or `bun.lockb`. [05]
15. As a developer whose directory has both `bun.lock` and `bun.lockb`, I want gx to treat them as
    one manager, so that a bun migration in progress does not stop my iterations. [07]
16. As a developer whose `packageManager` names a different tool than the lockfile, I want
    `packageManager` to win, so that the declared tool is the one that runs. If its frozen install
    fails, the iteration stops like any install failure. [05]
17. As a developer whose directory has two lockfiles from different managers and no
    `packageManager`, I want the iteration to stop with an error naming both files, so that gx
    never guesses. The reason ends in `lockfiles of more than one package manager: <a>, <b>`
    (exact wording is the implementer's choice, but it names every lockfile). [05]
18. As a developer whose `packageManager` names a tool gx has no command for, I want the iteration
    to stop on the install-failure path, so that gx never silently falls back to the lockfile. The
    reason ends in `unsupported packageManager "<value>"`. [07]
19. As a developer whose `package.json` gx cannot read or parse, I want the iteration to stop on the
    install-failure path with no lockfile fallback. The reason ends in
    `reading package.json: <err>`. [07]
20. As a developer, I want every install to run with `CI=true` added to its environment, so that a
    package manager never waits on a prompt that no one can answer. [06]
21. As a developer, I want the installs of one prepare to share a 5-minute timeout, so that a hung
    install pauses the ticket instead of holding the launch forever. [06]
22. As a developer reading a paused ticket, I want the pause reason to say
    "launch failed: installing dependencies in <dir>: <command>: <output tail> (full output:
    <log path>)", so that I know which directory, which command and why, without a 20K-character
    reason. The full output is in the log file. [01, 05]
23. As a developer reading a timed-out install, I want the reason to end in "timed out after 5m".
    [06]
24. As a developer, I want an install failure to pause the ticket with `iteration-error` before the
    agent starts, so that no agent turn is spent on a broken tree. [01, 05]
25. As a developer, I want gx to discard an iteration or commitless worktree whose install failed,
    so that a retry starts from a clean checkout. [05]
26. As a developer, I want gx to keep the feature worktree when a merge ticket's install fails, as
    it always keeps that worktree. [05]
27. As a developer, I want commitless tickets in a detached worktree to get the same install, so
    that a commitless agent can run tests too. [05]
28. As a developer of a merge ticket, I want the feature worktree installed on every prepare, so
    that the merge agent can run tests after a rebase. A frozen pnpm install on a ready tree takes
    about 1s, so the repeat cost is small. [05]
29. As a developer of a `vcs: none` project, I want a commitless ticket's scratch dir to get no
    install, so that gx never runs package managers outside a git checkout. [05]
30. As a developer whose server restarted mid-iteration, I want the resumed run to skip the
    install, so that a live agent's tree is never changed under it. [05]
31. As a developer, I want no `node_modules` symlinks from the main worktree, so that one
    worktree's install never changes another's. `git worktree remove --force` deletes the
    untracked `node_modules` with the worktree. [01]
32. As a developer of a Python project, I want gx to run no `poetry install` or `uv sync`, so that
    the install covers JS/TS only, as decided. [05]
33. As an agent reading `.ai/index.md`, I want the web-tests line to say that gx installs
    dependencies when it creates the worktree, so that the doc matches what happens. [05]

### Part B: hermetic Go tests

34. As an agent in a fresh worktree, I want `go test ./cmd` to pass without reading this repo's
    real branches, worktrees or `.scratch`, so that a red run means my change broke something. [02]
35. As a maintainer, I want the 5 `TestScanWorktrees_RealRepo_*` tests deleted and their cases
    rebuilt on `testutil.TempRepo`, so that the cases keep their coverage without live fixtures.
    The cases are a current-pattern iteration branch that is merged and done, a legacy-pattern
    iteration branch that is merged and done, a feature branch, an "other" branch, and an active
    branch that never gets a recommendation. [02]
36. As a developer running `gx cleanup scan` next to a running loop, I want a branch that
    disappears mid-scan to be skipped, so that the scan finishes instead of failing. [02]
37. As a maintainer, I want the full `go test ./server` run to never fail on its own lock during
    `Harness.Restart`, so that the slow server suite is trustworthy. [03, 04]
38. As a developer starting `gx server` while a forked child of the same process still holds the
    old lock fd, I want `acquireLock` to retry for about 2 seconds, so that the restart wins once
    the child's fd closes. [04]
39. As a developer whose lock is held by another process, I want `acquireLock` to fail at once
    with "already running (pid N)", as today, so that a second server never waits. [04]
40. As a developer whose same-pid retry ran out, I want the error to name that case, so that I can
    tell it from a second server. [04]
41. As an agent writing a new Go test, I want `.ai/index.md` Testing to say that tests never read
    live machine state and use `t.TempDir()` and `testutil.TempRepo`, so that new tests stay
    hermetic. [02]

## Implementation Decisions

### Part A: dependency install

- **Where it runs.** The install is part of the server's live prepare path: `PrepareIteration`
  (in the iteration worktree, not the feature worktree it also ensures), `PrepareCommitless` (only
  for a detached worktree, never for a scratch dir) and `PrepareFeature` (in the feature
  worktree). `ResumeIteration`, `ResumeCommitless` and `ResumeFeature` never install. Session
  worktrees from the web (`addSessionWorktree`) and TUI Worktrees-tab creates do not install. [05]
- **Discovery.** gx lists tracked files in the new worktree with `git ls-files` and keeps each
  directory that holds one of these lockfiles: `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`,
  `bun.lock`, `bun.lockb`. A directory with no lockfile gets no install, even if its `package.json`
  sets `packageManager`. A worktree with no tracked lockfile installs nothing and does not fail. A
  `git ls-files` error takes the install-failure path. [01, 05]
- **Choosing the manager, per directory.**
  1. gx reads `package.json` in the lockfile's directory. If the file is missing, unreadable or not
     valid JSON, the iteration stops on the install-failure path. There is no lockfile fallback.
     Every supported frozen install fails without a `package.json`, so a missing file is treated
     as unreadable. [07]
  2. If `packageManager` is set, it decides the manager. A value gx has no command for stops the
     iteration on the install-failure path. That includes a value that does not parse as
     `<name>@<version>`, since gx needs the Yarn major version to choose a command and corepack
     rejects such a value too. [05, 07]
  3. Without `packageManager`, the lockfile decides. `bun.lock` and `bun.lockb` count as one
     manager. Lockfiles of two or more different managers stop the iteration with an error naming
     the files. [05, 07]
- **Commands.** [05]

  | Manager      | Lockfile                 | Command                            | Picked when                                                      |
  | ------------ | ------------------------ | ---------------------------------- | ---------------------------------------------------------------- |
  | npm          | `package-lock.json`      | `npm ci`                           | `packageManager` `npm@…`, or the lockfile                        |
  | pnpm         | `pnpm-lock.yaml`         | `pnpm install --frozen-lockfile`   | `packageManager` `pnpm@…`, or the lockfile                       |
  | Yarn classic | `yarn.lock`              | `yarn install --frozen-lockfile`   | `packageManager` `yarn@1.…`, or `yarn.lock` without `__metadata:` |
  | Yarn Berry   | `yarn.lock`              | `yarn install --immutable`         | `packageManager` `yarn@2.…` or later, or `yarn.lock` with `__metadata:` |
  | bun          | `bun.lock` / `bun.lockb` | `bun install --frozen-lockfile`    | `packageManager` `bun@…`, or either lockfile                     |

  gx never runs `pnpm import`, never generates a lockfile and never pins the tool's version. [01]
- **Environment.** Each install runs in its directory with the inherited environment plus
  `CI=true`. The tool lookup keeps the per-run PATH override that `Deps.InstallDeps` uses today. [06]
- **Timeout.** All installs of one prepare share a fixed 5-minute budget, counted from the first
  install, so N lockfile directories never block for N x 5 minutes. When the budget runs out, gx
  stops the running install process and takes the install-failure path with "timed out after 5m"
  in place of the output. The timeout is not configurable. [06]
- **Accepted cost.** The install runs on the scheduler's tick, so a hung install blocks the tick
  for up to 5 minutes. This epic accepts that cost. [06]
- **Failure path.** Any install error (a failed or timed-out command, a missing executable, an
  unreadable `package.json`, an unknown `packageManager`, two lockfiles) makes the prepare fail.
  The server's existing launch-failure handling then pauses the ticket with Pause reason
  `iteration-error` and reason text "launch failed: installing dependencies in <dir>: …". The agent
  never starts. [01, 05, 07]
  - `<dir>` is the absolute path of the lockfile's directory. For a `git ls-files` error, which has
    no lockfile directory, it is the worktree root.
  - The install step (seam 1) writes the whole "installing dependencies in <dir>: …" text, because
    only it knows the directory. The prepare functions return its error unwrapped, so today's
    wrap at `ralphloop/iteration.go:74` goes. The server adds "launch failed: " as today.
  - The rest of the reason is one of:
    - a command failure: `<command>: <output tail> (full output: <log path>)`;
    - a timeout: `<command>: timed out after 5m`;
    - an unknown or unparseable `packageManager`: `unsupported packageManager "<value>"`;
    - a missing, unreadable or invalid `package.json`: `reading package.json: <err>`;
    - two managers' lockfiles: a message naming every lockfile in the directory.
- **Output trimming.** A failed install can print 20K characters, so the reason keeps only the
  tail of the command's combined output. The tail size and how it is cut are the implementer's
  choice. gx writes the full output to a log file and names its path in the reason. The file
  lives outside the worktree, so it survives the discard, for example next to the epic's event
  log in the scratch dir. The exact location and file name are the implementer's choice. Only a
  failed install writes the file.
- **`InstallDeps` shape.** `Deps.InstallDeps` becomes
  `func(path string) (commands []string, err error)`, with one entry per directory installed, in
  the form `<dir>: <command>`. An empty slice with no error means no tracked lockfile. The implementer may add input parameters, for example the
  directory for the log file.
- **Discard on failure.** An iteration or commitless worktree whose install failed is removed with
  its iteration branch, the same cleanup `DiscardIteration` and `DiscardCommitless` do for a failed
  launch. The feature worktree is never removed. Today a prepare error returns before any discard,
  so this needs new handling. Whether the prepare function cleans up after itself or the server's
  prepare-and-launch step discards is the implementer's choice. [05]
- **Removed behaviour.** The `poetry.lock` and `uv.lock` markers go. The root-only marker scan
  with its "npm wins over pnpm" order goes; `packageManager` and the two-lockfile rule replace it.
  [05]
- **Docs.** The ticket that adds the install rewrites `.ai/index.md:33`. The line then says that
  gx installs dependencies when it creates an iteration worktree, so web tests work in a fresh
  worktree. [05]
- **Implementer's choice.** Sequential or parallel installs across directories (the reason names
  the directory that failed); one `deps-installed` event per directory or one combined event;
  whether to delete the dead `runIteration` install code; how the timeout stops the process; the
  names and location of the detection helpers. Whether the install runs while prepare holds
  `worktreeLock` is the implementer's choice. Today all three prepare functions hold it for the
  whole call. The recommendation is to run the install after the lock is released, so a slow
  install in one ticket does not delay another ticket's `git worktree add`. This is not a
  requirement. [05, 07]

### Part B: hermetic tests

- **Scan tests.** Delete the 5 `TestScanWorktrees_RealRepo_*` tests and their helpers
  (`realDevRepoScan`, `realDevRepoScanWithEpics`, `requireBranch`, `requireEpic`). Rebuild the
  same 5 cases on `testutil.TempRepo`, creating the branches, commits and merges each case needs
  and passing `tickets.Epic` values built in the test. No case reads the real repo, its worktrees
  or its `.scratch`. [02]
- **Vanished branch.** `scanWorktrees` skips a branch that no longer exists when it classifies it,
  and leaves it out of the result. Other classification errors still fail the scan. How it detects
  a vanished branch is the implementer's choice. [02]
- **Scan split.** `scanWorktrees` splits in two. It lists the branches, then calls a helper that
  takes a given branch list and classifies each one. The helper is the test seam for the vanished
  branch. Its name and signature are the implementer's choice.
- **Lock retry.** `acquireLock` keeps its non-blocking flock. When the flock is taken and the pid
  in the lock file is this process's own pid, it retries for about 2 seconds. If the flock frees up
  in that window, it takes the lock as usual. If not, the error names the same-process case. When
  the pid is any other value, it fails at once with today's "already running (pid N)". The poll
  interval and the exact error wording are the implementer's choice. `Harness.Restart` does not
  change. [04]
- **Docs.** `.ai/index.md` Testing gets one bullet: tests never read live machine state, such as
  sibling worktrees, real branches or the live server's state; use `t.TempDir()` and
  `testutil.TempRepo`. [02]

## Testing Decisions

- A good test checks behaviour a caller can see: the commands that ran and in which directory,
  the error text, the ticket's status and Pause reason, whether the worktree still exists. It does
  not check internal helpers or call order.
- Every new test uses `t.TempDir()` and `testutil.TempRepo`, the rule this spec adds.

### Part A seams

1. **The install step** (detection plus running the command, with an explicit PATH). This is the
   seam `ralphloop/deps_test.go` uses today: stub executables in a temp bin dir log their argv and
   exit with a chosen code. Today `stubBin` logs only `$0 $*`. Logging the cwd and `CI` is new
   work in the stub. The tests move from loose marker files to a
   `testutil.TempRepo` with committed lockfiles and `package.json` files, since discovery reads
   `git ls-files`. Cases: lockfiles at the root and in subdirectories; an untracked lockfile is
   ignored; each manager's command; `packageManager` beating the lockfile; Yarn classic and Berry
   from `packageManager` and from `__metadata:`; `bun.lock` with `bun.lockb` as one manager; two
   lockfiles without `packageManager`; an unknown or unparseable `packageManager`; a missing or
   invalid `package.json`; a failing stub; the exact reason text of each failure; a long output
   is cut to its tail and the full output is in the named log file; `CI=true` in the stub's
   environment; no lockfile is a no-op. The 5-minute timeout is a constant; the timeout test may shorten it through a test-only
   value of the implementer's choosing so it runs in well under a second. One case checks that two
   directories share one budget.
2. **The prepare functions** (`PrepareIteration`, `PrepareCommitless`, `PrepareFeature` and their
   `Resume*` twins) with a `Deps` whose `InstallDeps` is a fake that records its path and can
   fail. Cases: each prepare installs in the agent's worktree; the iteration prepare does not
   install in the feature worktree; a scratch-dir commitless prepare and every resume never call
   the install. Prior art: the fake `InstallDeps` in `ralphloop/loop_test.go`.
3. **The server, through `server/servertest`.** One end-to-end case: a failing install pauses the
   ticket with `iteration-error`, the reason starts with
   "launch failed: installing dependencies in", the agent is never started, and the iteration
   worktree and branch are gone. One case for a merge ticket keeps the feature worktree. The
   harness reaches the install through a new server config hook for `InstallDeps`, like the
   existing `Sleep` hook; its name is the implementer's choice. Prior art:
   `TestLaunchPort_FailureAfterClaimPausesStuck` in `server/realgit_launch_test.go`.

### Part B seams

- **`scanWorktrees`** called directly on a `testutil.TempRepo`, for the 5 rebuilt cases. Prior art:
  other `cmd` tests built on `testutil.TempRepo`.
- **The classify helper from the scan split**, for the vanished-branch case. The test passes a
  branch list that includes a name with no branch in the TempRepo, and expects the scan to succeed
  without that name in the result.
- **`acquireLock`**, with two unit tests in `server/lock_test.go`. flock locks belong to an open
  file description, so a second open fd in the same process can hold the lock. Test one: hold the
  lock on a second fd with this pid in the file, release it after a short delay, and expect
  `acquireLock` to succeed. Test two: hold it with another pid in the file and expect an immediate
  "already running" error. Prior art: `TestLockHeldBy`.

## Out of Scope

- Dependency setup for languages other than JS/TS (Python venvs, `uv sync`). Go already shares a
  module cache across worktrees.
- Session worktrees from the web (`addSessionWorktree`) and TUI Worktrees-tab creates. The
  Destination names iteration worktrees only. [05]
- An audit of other gx tests for live-state reads. File a follow-up if more cases show up.
- Making `go test ./server` faster (about 300s today).
- Moving the install off the scheduler's tick. [06]
- Prompt and doc changes other than the two `.ai/index.md` edits above: a type-check command, a
  `verify` row for web files, and the "use the targeted test command" hint. gx-brief owns these
  (gx:gx-brief/03 waits on this map).
- Moving gx's `web/` from npm to pnpm. That is gx:follow-ups/108.
- Changes to `make web-test`. It keeps its own `pnpm install --frozen-lockfile`, which becomes a
  fast no-op in a worktree gx already installed.

## Further Notes

- Benchmarks from ticket 01 (fresh worktree, warm cache): `pnpm install --frozen-lockfile` in
  `webui/` takes 0.35 to 0.8s; `npm ci` in `web/` takes 2.8s on every run. [01]
- pnpm 12 has no abort for a missing TTY. `CI=true` is for pnpm 9 and 10 in other projects. [06]
- If `gx brief` later adds a shared helper for trimming command output, the install's trimming
  can switch to it.
- This epic answers gx:gx-brief/03's question about how web checks run in a fresh worktree:
  dependencies are already installed. gx-brief then needs only a one-line failure message.
