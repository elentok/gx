# How other tools declare checks (gx:better-tests/02)

How existing tools let a repo declare its tests and other checks, so gx's check config can borrow a
shape people already know. Sources are the tools' official docs and, where the docs are thin, their
source repos. Read on 2026-10-27.

## Summary table

| Tool | File | Command and working dir | Changed files to task | When it runs | Timeout | Parallelism |
|---|---|---|---|---|---|---|
| mise | `mise.toml` `[tasks.<name>]`, or script files in `mise-tasks/` | `run` (string or list), `dir` (default: config root) | None. `sources`/`outputs` skip a task whose outputs are newer than its sources | On demand (`mise run`). `depends`, `depends_post`, `wait_for` order tasks | `timeout` per command (`30s`, `10m`) | `jobs` setting, default 8. `mise run a ::: b` runs tasks in parallel |
| just | `justfile` (any case) or `.justfile` | Recipe body. Runs in the justfile's dir. `[working-directory(...)]`, `[no-cd]` change it | None | On demand. Dependencies run first | None found | `[parallel]` runs a recipe's dependencies at once. `--jobs` caps it (1.56.0) |
| Taskfile | `Taskfile.yml` (8 accepted names, see below) | `cmds`, `dir` (default: the Taskfile's dir) | None. `sources`/`generates` + `method` skip up-to-date tasks | On demand. `deps`, `run: always/once/when_changed` | `timeout` per command, exit code 124 on timeout | `deps` run at once by default. `--parallel`, `--concurrency` (default 0, no limit) |
| turbo | `turbo.json` at the root, optional per-package `turbo.json` with `"extends": ["//"]` | The task name maps to the `package.json` script of the same name in each package. Runs in the package dir | `--affected`: any changed file in a package selects all its tasks. `inputs` globs feed the cache hash | On demand. `dependsOn` (`^build` = dependencies' build first) | No task timeout | `--concurrency`, default 10 |
| nx | `project.json` or `package.json` `"nx"`, defaults in `nx.json` `targetDefaults` | `executor` or `command`. `nx:run-commands` takes `cwd`, `parallel` | `nx affected`: git diff against `--base` (default `main`), selects tasks whose `inputs` include a changed file | On demand. `dependsOn` | Only `readyWhen.timeout` for continuous targets (default 10 min) | `--parallel`, default 3. Per target `"parallelism": false` |
| lefthook | `lefthook.yml` (also `.lefthook.yml`, `.config/`, TOML, JSON, JSONC) | `run` (via `sh`), `root` changes the cwd | `glob`, `exclude` filter files. `{staged_files}`, `{push_files}`, `{all_files}`, `{files}` substitute the list. No match skips the job | Git hook name is the top-level key (`pre-commit:`, `pre-push:`) | None found | `parallel: true` per hook, default sequential. Long file lists are split into several runs |
| pre-commit | `.pre-commit-config.yaml` | `entry` + `args`, file names appended | `files`, `exclude` (regex), `types` (identify tags, ANDed). `pass_filenames: false`, `always_run` | `stages` (values are git hook names, plus `manual`). `--all-files`, `--from-ref/--to-ref` | None found | Parallel by default. `require_serial: true` uses one process |
| package.json scripts | `package.json` `"scripts"` | Runs from the package root, `node_modules/.bin` on `PATH` | None | `npm test` runs `pretest`, `test`, `posttest` | None | None |
| Makefile | `Makefile` | Recipe, runs in the Makefile's dir | None (make compares file timestamps, not git changes) | GNU standard target `check` runs self-tests after build, before install | None | `make -j` |
| AGENTS.md | `AGENTS.md`, nearest one to the edited file wins | Free Markdown. Suggested sections: "Build and test commands", "Testing instructions" | None | The agent "will attempt to execute relevant programmatic checks" | None | None |

## Notes per tool

### mise

- Task fields: `run`, `file`, `dir`, `depends`, `depends_post`, `wait_for`, `env`, `sources`,
  `outputs`, `skip_if`, `timeout`, `description`, `alias`, `hide`, `confirm`, `tools`, and others
  ([task configuration](https://mise.jdx.dev/tasks/task-configuration.html)).
- `dir` defaults to the config root. `timeout` is the "maximum run time for each command".
- Freshness: a task with `sources` and `outputs` is skipped when its outputs are newer than its
  sources. `--force` runs it anyway. There is no git-changed-file filter
  ([running tasks](https://mise.jdx.dev/tasks/running-tasks.html)).
- `jobs` defaults to 8 (`MISE_JOBS`) ([settings](https://mise.jdx.dev/configuration/settings.html)).
- Wildcards: `mise run 'test:*'` runs every task in a `:` group.

### just

- Names: `justfile` or `.justfile`, any case ([README](https://github.com/casey/just)).
- `[working-directory(...)]` (1.38.0) and `set working-directory` change the cwd. `[no-cd]` runs in
  the invocation dir.
- `[parallel]` runs a recipe's dependencies at once. `--jobs` (1.56.0) caps it.
- `[group]` and `[doc]` organise `just --list`. No timeout and no file filter.

### Taskfile

- Task fields include `cmds`, `deps`, `dir`, `sources`, `generates`, `method`, `status`,
  `preconditions`, `platforms`, `requires`. `timeout` exists on commands and on task dependencies; a
  timed-out command exits 124 ([schema](https://taskfile.dev/docs/reference/schema)).
- Tasks run in the Taskfile's dir unless `dir` says otherwise
  ([running tasks](https://taskfile.dev/docs/guide/running-tasks)).
- "Dependencies run concurrently by default." `--concurrency` caps them
  ([dependencies](https://taskfile.dev/docs/guide/dependencies)).

### turbo

- `turbo.json` `tasks.<name>` fields: `dependsOn`, `inputs`, `outputs`, `cache`, `persistent`,
  `interactive`, `env`, `passThroughEnv`, `outputLogs`, `with`
  ([configuration](https://turborepo.dev/docs/reference/configuration)).
- turbo runs no commands of its own. It runs each package's `package.json` script with the task's
  name.
- `--affected` compares `main...HEAD` (override with `TURBO_SCM_BASE`, `TURBO_SCM_HEAD`). A changed
  file in a package selects all of that package's tasks. `futureFlags.affectedUsingTaskInputs`
  narrows this to each task's `inputs` ([run](https://turborepo.dev/docs/reference/run)).

### nx

- Targets name an `executor` or a `command`, with `dependsOn`, `inputs`, `outputs`, `cache`,
  `continuous`, `parallelism` ([project configuration](https://nx.dev/docs/reference/project-configuration)).
- Config layers: inferred tasks, then `nx.json` `targetDefaults`, then the project's own file. Each
  layer overwrites the one before.
- `nx affected` selects a task "when your change touches one of its inputs". `--files` names the
  changed files directly ([affected](https://nx.dev/docs/features/ci-features/affected)).
- `--parallel` defaults to 3 (`packages/nx/src/command-line/yargs-utils/shared-options.ts` in
  nrwl/nx).

### lefthook

- The top-level keys are git hook names. Each hook holds `commands`, `scripts` or `jobs`
  ([configuration](https://github.com/evilmartians/lefthook/tree/master/docs/configuration)).
- Job fields: `run`, `root`, `glob` (one or a list), `exclude`, `files` (a command that lists
  files), `tags`, `skip`, `only`, `priority`, `env`, `stage_fixed`, `fail_text`, `interactive`.
- `glob` without a file template in `run` still filters `{staged_files}` (pre-commit) or
  `{push_files}` (pre-push). If no file is left, lefthook skips the job.
- `glob` is matched from the repo root even when `root` is set. Its `**` matches one or more
  directories, not zero.
- `stage_fixed: true` runs `git add` on the files after a fixer runs. This is the same as
  gx-verify's re-staging of formatted files.
- Hooks run their jobs in order unless `parallel: true`. There is no timeout option in the option
  list.

### pre-commit

- `.pre-commit-config.yaml` lists repos. `repo: local` holds the repo's own hooks, which must set
  `id`, `name`, `language`, `entry`, and `files` or `types` ([pre-commit.com](https://pre-commit.com/)).
- File selection: `files`/`exclude` regexes plus `types`/`types_or`/`exclude_types` tags from the
  identify library. Matched file names go after `args`.
- `stages` takes git hook names plus `manual`, which no git hook triggers.
- `pre-commit run --from-ref origin/HEAD --to-ref HEAD` checks only the files changed in a range.
- Hooks run in parallel unless `require_serial: true`.
- The `language` field means pre-commit installs each hook's tools in its own environment. gx does
  not need that; it runs the repo's own tools.

### package.json scripts

- "Scripts are run from the root of the package folder." `pre<name>` and `post<name>` run around
  `<name>`. Dependency binaries are on `PATH`
  ([npm scripts](https://docs.npmjs.com/cli/v10/using-npm/scripts)).
- No timeout, parallelism or file filter.

### Makefile

- GNU's standard targets: `check` "Perform self-tests (if any)", run after build and before install
  ([GNU standard targets](https://www.gnu.org/prep/standards/html_node/Standard-Targets.html)).
- `make check` and `make test` are names people expect. Make has no notion of changed files beyond
  timestamps.

### AGENTS.md

- Plain Markdown with no schema. Common sections are "Build and test commands" and "Testing
  instructions". The nearest `AGENTS.md` to the edited file wins, and the user's chat prompt beats
  all of them ([agents.md](https://agents.md/)).
- An agent can read it, but gx cannot parse it into checks. It is a place to point at the gx
  entry point, not a config source.

## Local, uncommitted overrides

| Tool | Override | How it merges |
|---|---|---|
| mise | `mise.local.toml` ("Personal overrides. Keep it out of version control."), also `mise.<env>.local.toml` | Overrides `mise.toml` in the same dir. It is a full config file, so it can hold `[tasks]` too ([configuration](https://mise.jdx.dev/configuration.html)) |
| lefthook | `lefthook-local.yml` (dot prefix must match the main file) | Merged last, after `extends` and `remotes`, so it can override everything. Works with no main file. Its docs show it wrapping a job's `run` with `{cmd}` |
| Taskfile | Commit `Taskfile.dist.yml`, keep a local `Taskfile.yml` in `.gitignore` | Not a merge. The local file is found first and replaces the shared one |
| just | `import? 'local.just'` with `set allow-duplicate-recipes` | The optional import is skipped when missing. With the setting, the last recipe of a name wins |
| turbo | Per-package `turbo.json` with `extends` | Committed, not local. Arrays replace root values unless `$TURBO_EXTENDS$` |
| nx | `targetDefaults` then project config | Committed layers, no local file |
| pre-commit | `-c/--config <file>` | A whole different file, no merge |
| Makefile | `-include local.mk` | Optional include, a GNU make feature |
| gx itself | `project.json` in the ticket store | Already overrides `config.json` per project, outside the repo (`docs/project-schema.json`) |

The common pattern is a `<name>.local.<ext>` file next to the committed one, gitignored, merged on
top by key. mise and lefthook both use it.

## What none of them do

- No tool maps changed files to checks the way gx-verify needs: group files by package (nearest
  `package.json`) or by Go package dir, then run once per group in that dir. lefthook and pre-commit
  filter by glob and pass a flat file list. turbo and nx select whole packages or tasks.
- No tool shards a test run. Sharding lives in the test runner (`go test` package lists, Vitest
  `--shard`) or in a script, as gx does in `scripts/test-shards.sh`.
- No tool says "this check is a formatter, run it first and re-stage its output" except
  lefthook's `stage_fixed` and `priority`.

## Recommendation

Define gx's own small file. Do not read mise, Taskfile or lefthook directly.

- The runners (mise, just, Taskfile, Make, npm scripts) know how to run a command but not which
  changed files it applies to. Reading them would still need a gx-side mapping.
- lefthook is the closest shape (glob, `{files}`, `root`, `parallel`, `stage_fixed`), but its file is
  keyed by git hook and owned by lefthook. Reading it would tie gx checks to a repo's hook setup and
  to lefthook's own `**` rules.
- A gx entry's `run` can call any of these tools (`mise run lint`, `make test-server`,
  `pnpm turbo run test --filter ...`). The repo keeps its runner and gx keeps one schema.
- Borrow names people know: `run`, `glob`, `exclude`, `dir`, `timeout`, `parallel`, `{files}`.
- Local override: `<file>.local.<ext>` next to the committed file, gitignored, merged per check by
  name (mise, lefthook). For a repo that cannot take a gx file, the same entries could live in the
  ticket store's `project.json`, which already overrides per project.

### Sketch of an entry

JSON with a schema, to match gx's `config.json` and `project.json`. The file name and placement are
for "Check config shape" (gx:better-tests/03) to settle.

```jsonc
{
  "checks": {
    "go-fmt": {
      "kind": "format",            // format | lint | typecheck | test. Formatters run first.
      "glob": ["**/*.go"],         // repo-relative, matched against changed files
      "exclude": [],
      "run": "gofmt -w {files}",   // {files}: matched files, relative to dir
      "fixes": true                // rewrites files, so gx re-stages them (lefthook stage_fixed)
    },
    "go-test": {
      "kind": "test",
      "glob": ["**/*.go"],
      "group": "dir",              // run once per group: "dir", "package.json", "go.mod", or none
      "dir": "{group}",            // working dir; default repo root
      "run": "gx brief go-test .",
      "timeout": "5m",
      "parallel": true             // may run alongside other checks (pre-commit require_serial inverse)
    },
    "server-tests": {
      "kind": "test",
      "glob": ["server/**/*.go"],
      "replaces": "go-test",       // for these files, run this instead (today's make test-server case)
      "run": "make test-server",
      "timeout": "15m"
    }
  }
}
```

Fields:

- `kind`: orders checks (format, then lint and typecheck, then test) and lets skills ask for one
  kind.
- `glob`, `exclude`: which changed files select the check. No `glob` means the check always runs.
- `run`: a shell command. `{files}` adds the matched files. Without it, gx passes no files.
- `group`, `dir`: how gx-verify's "per package dir" and "per nearest `package.json`" rules become
  data.
- `timeout`: per run, as in mise and Taskfile. gx reports a timeout as a failure.
- `parallel`: whether the check may run alongside others.
- `fixes`: the check rewrites files, so gx re-stages them in staged and commit-all modes.
- `when` (optional, open question for ticket 03): `changed` for gx-verify, `full` for epic verify.
  Same idea as lefthook's hook keys and pre-commit's `stages`.
