---
name: gx-verify
description:
  Verify changed files after completing a task or before commit using cheap targeted checks. Use for
  staged, commit-all, or working-changes verification when handing work back or when another skill
  needs changed-file checks.
---

Run cheap, targeted verification for changed files.

## Workflow

1. Select the mode. The caller names it:
   - `staged`: verify only what is staged for the commit
   - `commit-all`: verify every changed and new file, because the caller will commit all of them
   - `working-changes`: verify every changed and new file after completing a task, without staging
2. Discover files for the selected mode:
   - `staged`: use tracked files from `git diff --cached --name-only --diff-filter=ACMR`
   - `commit-all`: use tracked files from `git diff --name-only HEAD --diff-filter=ACMR` plus
     untracked files from `git ls-files --others --exclude-standard`
   - `working-changes`: use tracked files from `git diff --name-only HEAD --diff-filter=ACMR` plus
     untracked files from `git ls-files --others --exclude-standard`
   - deduplicate the file list
3. Apply the check table. Run every matching command and never invent extra checks.
4. Run formatters before analyzers or tests.
5. If a formatter changes files:
   - in `staged` mode, run `git add` on the formatted files that were already staged
   - in `commit-all` mode, run `git add` on the formatted files that are part of the intended commit
   - in `working-changes` mode, do not stage files; report that formatting changed the working tree
6. Report exactly one result:
   - `pass`: at least one check ran, every check that ran passed, and none failed
   - `fail`: any installed tool's command failed, or a frozen install failed (include the install
     output). A `gx brief` exit 2 is a `fail`, with brief's one-line reason as the detail. Never skip
     it and never count it as a pass
   - `no applicable checks`: no file matched the check table, or every matching check was skipped.
     List each skipped tool or package with its reason. The commit goes ahead.

## Check Table

Every row skips a tool that is not installed. Name each skipped tool in the report and run the other
checks.

- Shell:
  - files matching `*.sh`
  - files without an extension whose first line starts with `#!` and names `sh`, `bash`, or `zsh`
  - run `shfmt -w <shell-files>`
  - run `shellcheck <shell-files>`
- Python:
  - files matching `*.py`
  - run `basedpyright <python-files>`
  - run `ty check <python-files>`
- Go:
  - files matching `*.go`
  - run `gofmt -w <go-files>`
  - for each unique directory containing a matched Go file, run `gx brief go-test ./<dir>`; use
    `gx brief go-test .` for files in the repo root. For `server` in a repo whose `Makefile` has a
    `test-server` target, run `make test-server` instead: it shards the slow package
- TypeScript and JavaScript:
  - files matching `*.ts`, `*.tsx`, `*.js`, `*.jsx`, `*.mjs`
  - group the files by their nearest `package.json` and run each check once per package, from the
    package directory
  - run `gx brief -C <pkg> tsc --noEmit` when the package has a `tsconfig.json`
  - run `gx brief -C <pkg> vitest related <files>` (brief adds single-run mode), only when the tool
    is listed
  - run `prettier --write <files>` and `eslint <files>`, each only when the tool is listed. A tool is listed when the package's own `package.json` lists it, or when
    the `package.json` in the lockfile's directory (the workspace root) lists it
  - the lockfile is the nearest one at or above the package. It picks the runner and the frozen
    install:

    | Lockfile                  | Runner      | Frozen install                   |
    | ------------------------- | ----------- | -------------------------------- |
    | `pnpm-lock.yaml`          | `pnpm exec` | `pnpm install --frozen-lockfile` |
    | `package-lock.json`       | `npx`       | `npm ci`                         |
    | `bun.lock` or `bun.lockb` | `bun x`     | `bun install --frozen-lockfile`  |
    | `yarn.lock`               | `yarn`      | `yarn install --frozen-lockfile` |

  - when the lockfile's directory has no `node_modules`, run the frozen install once in that
    directory before the checks. A member package without its own `node_modules` does not trigger an
    install. A failed install is `fail`
  - when no lockfile exists at or above the package, skip that package's JS checks and report the
    package with the reason "no lockfile". Never fall back to `npm install`
- No matches:
  - report `no applicable checks`

## Boundaries

- Do not run whole-repo build, lint, format, or test commands unless the user explicitly asks.
- Do not perform secret scanning or semantic review; those belong to the caller.
