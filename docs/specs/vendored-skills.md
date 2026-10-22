# Vendored skills: every skill a gx skill calls ships in the bundle

## Problem Statement

gx's skill bundle is not self-contained. `gx-design` drives `wayfinder`, `to-spec`, `grilling` and
`domain-modeling`, and also calls `unslop`. None of them ship with gx. A person must install them
from mattpocock/skills and cursor/plugins by hand, and `gx-design` step 0 stops when one is
missing. The same gap exists elsewhere. `gx-code-review` falls back to
`thermo-nuclear-code-quality-review`, and `gx-implement` runs `/verify`. Neither ships with gx.

The copies a person installs are not the ones gx was written against. The dotfiles tree carries a
hand-edited `grilling` (a block of notes about question style and `unslop`) and an `unslop` with a
changed frontmatter. Another machine gets plain upstream copies and different behaviour.

`wayfinder` and `to-spec` set `disable-model-invocation: true`. The Skill tool can't reach them, so
`gx-design` has to `Read` their `SKILL.md` and run it inline.

gx already vendors four skills (`gx-tdd`, `gx-to-tickets`, `gx-implement`,
`gx-resolving-merge-conflicts`), but it records no upstream commit for them and has no way to pull
later upstream changes. The dotfiles script that refreshes the other copies deletes the
destination before it copies, so every refresh wipes hand edits.

The bundle also ships no MIT notice for any upstream. MIT requires the copyright and permission
notice in every copy.

The webui-design map needs this now. It wants the `gx-question` block rule written into a grilling
skill that gx owns (gx:webui-design/02).

## Solution

Every skill a gx skill calls ships in gx's bundle as a `gx-*` skill. Eight upstream skills become
**vendored skills** (see `GLOSSARY.md`): `gx-wayfinder`, `gx-to-spec`, `gx-grilling`,
`gx-domain-modeling`, `gx-research`, `gx-prototype`, `gx-unslop` and
`gx-thermo-nuclear-code-quality-review`. The dotfiles `verify` becomes `gx-verify`, a bundled skill
gx owns, and it gains TypeScript and JavaScript checks.

Every call between bundled skills names the `gx-*` copy through a relative link to its `SKILL.md`.
The caller reads that file and follows it, the same way on Claude, Codex and OpenCode. `gx-design`
loses its dependency check. A new bundle test fails when a gx skill calls a skill outside the bundle.

A maintainer pulls upstream changes with a repo-local agent skill, `sync-vendored-skills`, backed by
a script. A manifest pins each vendored skill's upstream repo, path, last synced commit and **sync
strategy**. With `merge`, the script runs a 3-way merge of the upstream change into gx's copy. With
`port`, it prints the upstream diff and the agent carries changes over by hand. The agent reviews
every upstream hunk, asks the maintainer about behaviour changes, runs the bundle tests, and only
then moves the pins.

Each vendored skill directory ships the upstream `LICENSE` and a `README.md` that credits the
upstream.

## User Stories

### A person using gx

1. As a gx user, I want `gx skills install` to give me every skill `gx-design` needs, so that I
   don't have to find and install skills from other repos.
2. As a gx user, I want `gx-design` to run on a fresh machine with only gx installed, so that the
   dependency check never stops me.
3. As a gx user, I want the grilling rules gx relies on (unslop before each question, B2 wording,
   reminders on earlier question numbers) to come with gx, so that every machine asks questions the
   same way.
4. As a gx user with bare `grilling` or `wayfinder` copies already installed, I want gx to leave
   them alone, so that I don't have to remove anything.
5. As a gx user, I want gx skills to call only `gx-*` skills, so that my own bare copies never
   change how gx behaves.
6. As a gx user, I want `gx-code-review` to use the bundled thermo-nuclear review by default, so
   that code review works without a separate install.
7. As a gx user with `.skills["code-review"]` set, I want my configured review skills to keep
   working, so that the new default doesn't override my choice.
8. As a gx user, I want `gx-implement`'s pre-commit check to ship with gx, so that unattended
   ralph-loop runs don't depend on a skill I installed myself.
9. As a gx user working in a TypeScript or JavaScript package, I want `gx-verify` to type-check,
   test, format and lint the changed files, so that ralph-loop catches JS/TS breakage before it
   commits.
10. As a gx user, I want `gx-verify` to run each tool through my package manager's runner, so that
    it uses the tool versions my lockfile pins.
11. As a gx user, I want `gx-verify` to skip a tool that neither my package nor its workspace root
    lists, so that it never runs a linter or formatter the project doesn't use.
12. As a gx user on Codex, I want the same skills installed under `~/.codex/prompts` with the same
    layout, so that relative links resolve the same way as on Claude.
13. As a gx user on a machine without some check tool (for example `shellcheck` or `golangci-lint`),
    I want `gx-verify` to skip that tool, name it in the report and run the other checks, so that a
    missing tool never blocks a ralph-loop commit.
14. As a gx user, I want `gx-verify` to say `no applicable checks` and list the skipped tools when
    every matching tool was missing, so that I can tell "nothing ran" apart from "everything
    passed".
15. As a gx user, I want an installed tool whose command fails to make `gx-verify` report `fail`, so
    that a real error is never hidden as a skip.
16. As a gx user in a fresh worktree, I want `gx-verify` to run the lockfile's frozen install once
    when the lockfile's directory has no `node_modules`, so that the JS checks can run.
17. As a gx user in a workspace (pnpm, npm, yarn or bun), I want that install to run once at the
    workspace root, and not again because a member package has no `node_modules` of its own, so
    that a change touching several packages doesn't install several times.
18. As a gx user, I want a failed frozen install to make `gx-verify` report `fail` with the install
    output, so that a lockfile out of sync with `package.json` shows up before the commit.
19. As a gx user with a `package.json` but no lockfile, I want `gx-verify` to skip that package's JS
    checks and say "no lockfile", so that it never runs an unpinned `npm install`.

### gx-design and other calling skills

20. As a `gx-design` session on any harness (Claude, Codex or OpenCode), I want to reach
    `gx-wayfinder` and `gx-to-spec` through a relative link to their `SKILL.md`, so that the call
    works the same way whether or not the harness has a skill tool.
21. As a `gx-wayfinder` session, I want my tracker rules to come from `gx-local-tracker.md`, so
    that I never tell a person to run a tracker-setup skill.
22. As a `gx-wayfinder` session, I want the rule "use the tracker's Map epics section for every
    ticket operation" in my own text, so that every caller gets it, not only `gx-design`.
23. As a `gx-wayfinder` session, I want to call `gx-grilling`, `gx-domain-modeling`, `gx-research`
    and `gx-prototype` for the ticket types, so that each ticket type runs a bundled skill.
24. As a `gx-design` session, I want to call `gx-grilling`, `gx-domain-modeling` and `gx-unslop`
    before I ask questions, so that the question rules are gx's own.
25. As a `gx-grilling` session, I want to call `gx-unslop` before each question, so that the notes
    rule works without the dotfiles copy.
26. As a `gx-to-tickets` session, I want to point people at `/gx-design` for decisions, so that I
    never send them to `/wayfinder` or `/myway`.
27. As a `gx-to-spec` session, I want to write the spec where the gx tracker says specs live, so
    that `gx-design` and `gx-to-tickets` find it.

### A gx maintainer syncing upstream

28. As a gx maintainer, I want to see which vendored skills have upstream changes since their pin,
    with commit counts, without changing any file, so that I can decide when to sync.
29. As a gx maintainer, I want a sync to 3-way merge each upstream change into gx's copy, so that
    gx's own edits survive.
30. As a gx maintainer, I want conflicts written as conflict markers and listed per skill, so that
    the agent knows exactly what to resolve.
31. As a gx maintainer, I want a file that upstream added to appear in gx's copy, so that new
    upstream content is not lost.
32. As a gx maintainer, I want a file that upstream deleted to go away when gx never changed it,
    and to show up as a conflict when gx did, so that gx's edits are never dropped silently.
33. As a gx maintainer, I want the near-rewrite skills (`gx-to-tickets`, `gx-implement`) synced by
    port, so that an automatic merge doesn't produce a wall of conflicts.
34. As a gx maintainer, I want the agent to suggest `port` when most of a `merge` skill's hunks
    conflict, and leave the switch to me, so that the strategy changes only on purpose.
35. As a gx maintainer, I want the agent to review every upstream hunk, clean ones too, so that a
    clean merge can't slip in an unwanted behaviour change.
36. As a gx maintainer, I want the agent to handle `gx-*` renames, tracker links and
    intent-preserving conflicts on its own, so that I'm only asked about real decisions.
37. As a gx maintainer, I want the agent to ask me about behaviour changes and conflicts with a gx
    decision, so that upstream never overrides a choice gx made.
38. As a gx maintainer, I want the agent to ask me when a pinned upstream path is gone, after it
    searched upstream for the same `name:`, so that I choose between following the move, keeping the
    skill as gx-owned, or removing it.
39. As a gx maintainer, I want the agent to ask me when a vendored skill starts calling a skill
    outside the bundle, so that I choose between vendoring it, dropping the call, or pointing it at
    a `gx-*` skill.
40. As a gx maintainer, I want the pins to move only after the bundle tests pass, so that a broken
    sync never records itself as done.
41. As a gx maintainer, I want every skipped upstream change listed in the sync commit message with
    a one-line reason, so that a later reader knows it was skipped on purpose.
42. As a gx maintainer, I want a sync to stop a skill whose upstream `LICENSE` changed and warn me,
    so that I check the new license still lets gx vendor it before anything updates.
43. As a gx maintainer, I want a skill whose new license doesn't allow vendoring to stay at its pin,
    so that gx never ships a version it may not redistribute.

### Credit and licensing

44. As an upstream author, I want my MIT notice shipped in every installed copy of my skill, so
    that gx meets the license.
45. As a gx user reading a vendored skill directory, I want a `README.md` that names the upstream
    repo, path and author and says gx changed it, so that I know where the skill came from.
46. As a gx user reading `skills/gx.md`, I want a list of the vendored skills and their upstreams,
    so that I see the bundle's origins in one place.

### Bundle safety

47. As a gx maintainer, I want a bundle test that fails when a gx skill calls a skill outside the
    bundle, so that the bundle stays self-contained.
48. As a gx maintainer, I want a bundle test that fails on conflict markers under `skills/`, so that
    an unresolved sync can't ship.

## Implementation Decisions

### The vendored skills

| gx skill                                | Upstream repo     | Upstream path                                               | Strategy | Pin seed           |
| --------------------------------------- | ----------------- | ----------------------------------------------------------- | -------- | ------------------ |
| `gx-wayfinder`                          | mattpocock/skills | `skills/engineering/wayfinder`                              | merge    | commit copied from |
| `gx-to-spec`                            | mattpocock/skills | `skills/engineering/to-spec`                                | merge    | commit copied from |
| `gx-grilling`                           | mattpocock/skills | `skills/productivity/grilling`                              | merge    | commit copied from |
| `gx-domain-modeling`                    | mattpocock/skills | `skills/engineering/domain-modeling`                        | merge    | commit copied from |
| `gx-research`                           | mattpocock/skills | `skills/engineering/research`                               | merge    | commit copied from |
| `gx-prototype`                          | mattpocock/skills | `skills/engineering/prototype`                              | merge    | commit copied from |
| `gx-unslop`                             | cursor/plugins    | `pstack/skills/unslop`                                      | merge    | commit copied from |
| `gx-thermo-nuclear-code-quality-review` | cursor/plugins    | `cursor-team-kit/skills/thermo-nuclear-code-quality-review` | merge    | commit copied from |
| `gx-tdd` (already in the bundle)        | mattpocock/skills | `skills/engineering/tdd`                                    | merge    | `bd453a6`          |
| `gx-to-tickets` (already in the bundle) | mattpocock/skills | `skills/engineering/to-tickets`                             | port     | `19c50d5`          |
| `gx-implement` (already in the bundle)  | mattpocock/skills | `skills/engineering/implement`                              | port     | `386d4ff`          |

- Each new skill starts as a copy of the upstream skill directory at the latest upstream commit.
  Its pin is that commit.
- `gx-resolving-merge-conflicts` has no manifest entry. Upstream deleted the skill in `daa01d8a`, so
  it is gx-owned now. `gx-verify` has no entry either.
- No vendored skill keeps its bare name. gx skills never call a bare copy, so a person's existing
  bare copies need no cleanup.

### Edits made on first vendoring

These are ordinary gx edits to gx's copy. Later merges keep them like any other gx change.

- **All new skills**: `name:` becomes the `gx-*` name, and every reference to another vendored
  skill names its `gx-*` copy. Each keeps upstream's other frontmatter unless a bullet below says
  otherwise.
- **`gx-wayfinder`**: keeps upstream's `disable-model-invocation: true`. Callers read the file, so
  the flag blocks no call. Drops upstream's `/setup-matt-pocock-skills` lines and links
  `../gx-local-tracker.md` instead. Gains the rule "use the tracker's Map epics section for
  every ticket operation (chart, claim, close, frontier)", which leaves `gx-design`. Its ticket-type
  calls name `gx-grilling`, `gx-domain-modeling`, `gx-research` and `gx-prototype`.
- **`gx-to-spec`**: keeps upstream's `disable-model-invocation: true`, for the same reason. Drops
  upstream's `/setup-matt-pocock-skills` lines and links `../gx-local-tracker.md` instead. The tracker doc already says a spec lives at
  `docs/specs/<epic-slug>.md`, so `gx-to-spec` writes it there. gx has no triage labels, so the
  "apply `ready-for-agent`" step goes.
- **`gx-grilling`**: gains the dotfiles notes as plain gx rules, with no "elentok@'s notes" label
  and no first person. Call `gx-unslop` before each question. Write questions and recommendations
  at CEFR B2 level: short sentences, no needless jargon, no filler, one line when one line does it,
  bullets over long paragraphs, and every fact that changes the decision kept. When a question
  refers to an earlier question or section by number, add a short reminder (for example "Q3
  (storage backend)").
- **`gx-unslop`**: drops `disable-model-invocation`. Bundled callers only read it, but a user's own
  `AGENTS.md` may tell the model to call it with the Skill tool (the dotfiles do, after the dotfiles
  follow-up renames it). It keeps upstream's description, "Cut AI tells from any writing." gx
  does not carry the dotfiles description extension ("Use when writing or editing prose ..."), so
  the description is not a gx edit. A user who wants unslop on all prose says so in their own
  `AGENTS.md` or `CLAUDE.md`.
- **`gx-thermo-nuclear-code-quality-review`**: only the name changes.

### `gx-verify`

- A bundled skill gx owns, copied from the dotfiles `verify`. It has no upstream, no manifest entry,
  and no LICENSE.
- It keeps the dotfiles workflow, check table and boundaries. The mode names that refer to the
  dotfiles `commit` skill are reworded so the caller (`gx-implement`) names the mode.
- It gains a TypeScript and JavaScript row:
  - Files: `*.ts`, `*.tsx`, `*.js`, `*.jsx`, `*.mjs`.
  - Group the files by their nearest `package.json`. Run each check once per package.
  - `tsc --noEmit -p <pkg>` when the package has a `tsconfig.json`.
  - `vitest related --run <files>` when the package lists `vitest`.
  - `prettier --write <files>` and `eslint <files>`, each only when the package lists it.
  - A tool counts as listed when the package's own `package.json` lists it, or when the
    `package.json` in the lockfile's directory (the workspace root) lists it. The checks still run
    from the package directory.
  - The lockfile is the nearest one at or above the package. It picks the manager and the runner:

    | Lockfile                  | Runner      | Frozen install                   |
    | ------------------------- | ----------- | -------------------------------- |
    | `pnpm-lock.yaml`          | `pnpm exec` | `pnpm install --frozen-lockfile` |
    | `package-lock.json`       | `npx`       | `npm ci`                         |
    | `bun.lock` or `bun.lockb` | `bun x`     | `bun install --frozen-lockfile`  |
    | `yarn.lock`               | `yarn`      | `yarn install --frozen-lockfile` |

  - When the directory that holds the lockfile has no `node_modules`, `gx-verify` runs the frozen
    install once in that directory, then runs the checks. A member package without its own
    `node_modules` does not trigger an install: npm, yarn and bun hoist into the root. A workspace
    therefore installs once at its root, and only when the root's `node_modules` is missing.
  - A failed frozen install makes the result `fail`, with the install output in the report.
  - A `package.json` with no lockfile at or above it gets no JS checks. The report names the package
    and the reason ("no lockfile"). `gx-verify` never falls back to `npm install`.
  - The formatter-before-analyzer and re-staging rules apply to `prettier` like any other
    formatter.
- **Missing tools.** Every row (Shell, Python, Go, TS/JS) skips a tool that is not installed, the
  same way the TS/JS row skips a tool the package doesn't list. The report names each skipped tool,
  and the other checks still run.
- **Results.** There is no new result value.
  - `pass` needs at least one check that ran and passed, and none that failed.
  - An installed tool whose command fails makes the result `fail`.
  - When every matching check was skipped (missing tools, or no lockfile), the result is
    `no applicable checks`, followed by the skipped tools and reasons. The commit goes ahead.
- Model-invocable (no `disable-model-invocation`), like the dotfiles `verify`.

### Callers

- **How a bundled skill calls another.** Every call is a relative link to the callee's `SKILL.md`
  (for example `../gx-wayfinder/SKILL.md`). The caller reads that file and follows it. This is the
  one rule on every harness. Codex has no skill tool, and Claude's and OpenCode's skill tools
  depend on the install folder and on invocation flags. `gx skills install` keeps the same layout
  in every root, so the links resolve everywhere. `gx-implement` already reaches `gx-tdd` this way.
- `gx-design`: step 0 (dependency check) goes. Its paragraph about `Read`ing `wayfinder` and
  `to-spec` becomes the rule above, applied to `gx-wayfinder` and `gx-to-spec` (the step 4
  subagent gets the `gx-to-spec` link). "Asking questions" links `gx-grilling`,
  `gx-domain-modeling` and `gx-unslop`. `gx-design` keeps
  `disable-model-invocation: true`. Its description drops "depends on external skills".
- `gx-implement`: `/verify` becomes a link to `gx-verify` in `working-changes` mode. That mode
  checks every changed and new file and never stages, so it is right before `git add`. The agent
  stages what it commits after the check.
- `gx-to-tickets`: points at `/gx-design` instead of `/wayfinder` and `/myway`.
- `gx-code-review`: the fallback list becomes `["gx-thermo-nuclear-code-quality-review"]`. Its note
  that the default skill sets `disable-model-invocation` is reworded to match the vendored copy.
  The `SKILL.md` lookup becomes: `skills/<name>/SKILL.md` from the reviewed repo's root, then
  `../<name>/SKILL.md` relative to `gx-code-review`'s own file. The second path is the folder gx
  installed into, on any harness, so the bundled default resolves on Codex too. The hard-coded
  `~/.claude/skills/` step goes. A configured review skill installed in the same folder still
  resolves.
- `config` package: the code-review default becomes `gx-thermo-nuclear-code-quality-review`. The
  config schema doc's default and example change to match. An explicit `.skills["code-review"]`
  value is used as before.
- `gx-local-tracker.md`: references to "the `wayfinder` skill" name `gx-wayfinder`.
- Server code does not change. `server/sessionactionable.go` matches the `❓ **Q` text, not a skill
  name, and `server/index.go` names wayfinder only in a comment.

### `skills/gx.md`

- "Origin" lists every vendored skill with its upstream repo and path, credits Matt Pocock
  (mattpocock/skills), Lauren Tan (cursor/plugins `pstack`) and Cursor (cursor/plugins
  `cursor-team-kit`), and points at each directory's `LICENSE`. It notes that
  `gx-resolving-merge-conflicts` came from a skill upstream has since removed. The pins live only in
  the manifest, so `gx.md` doesn't repeat them.
- "gx-specific adaptations" loses the "not vendored" exception for `gx-design`.
- "Invocation policy" covers the new skills: `gx-wayfinder` and `gx-to-spec` keep upstream's flag
  (bundled callers read the file, so it blocks nothing), `gx-unslop` drops it (a user's own
  instructions may call it with the Skill tool), and `gx-verify` and the other new skills are
  model-invocable.
- "Layout" lists the new directories and their files, `LICENSE` and `README.md` included.

### Credit files

- Every directory derived from an upstream skill holds that upstream's `LICENSE`, copied word for
  word. That is the eleven directories in the table above plus `gx-resolving-merge-conflicts`.
  - mattpocock/skills: the repo-root `LICENSE` (MIT, Matt Pocock).
  - `gx-unslop`: `pstack/LICENSE` (MIT, Lauren Tan). cursor/plugins has no root LICENSE.
  - `gx-thermo-nuclear-code-quality-review`: `cursor-team-kit/LICENSE` (MIT, Cursor).
- Each of those directories also holds a `README.md` with the upstream repo link, the upstream
  path, the author's name and link, and a line saying gx changed the skill. The thermo-nuclear
  author is Eric Zakariasson (https://github.com/ericzakariasson). The agent writes the README once,
  on first vendoring.
- For a manifest skill, the sync never merges `LICENSE`. The script compares the upstream LICENSE
  at the pin with the one at latest, for both strategies:
  - Unchanged: nothing to do.
  - Changed: the script reports "license changed" with the diff and touches nothing for that skill.
    The agent asks the maintainer whether the new license still allows gx to vendor the skill. If
    yes, the sync copies the new `LICENSE` word for word and updates the skill. If no, the skill
    stays at its pin and keeps its old `LICENSE`, and the commit message says why.
  - The script never touches `README.md`. `gx-resolving-merge-conflicts` keeps both files as written.
- The bundle embeds whole skill directories, so both files install with the skill. Only the
  `//go:embed` line gains the new directory names.

### Harness metadata

- Upstream's `agents/openai.yaml` files and frontmatter keys ship as copied, apart from the
  `name:` and flag edits above. Codex reads `agents/openai.yaml` only from `~/.codex/skills`, and
  gx installs Codex files under `~/.codex/prompts`, so today they have no effect. Keeping them
  means gx:follow-ups/51 (Codex and OpenCode install) needs no sync.

### The manifest

- One file in the repo holds an entry per manifest skill: the gx skill name, the upstream repo, the
  upstream skill path, the upstream LICENSE path, the last synced commit, and the strategy (`merge`
  or `port`; `merge` when absent).
- It is maintainer data, read by the sync script. It is not part of the installed bundle.

### The sync skill and script

- A repo-local agent skill, `.claude/skills/sync-vendored-skills/`, backed by a script. Maintainers
  run it by hand. It is not in the bundle. The script fetches, merges and reports. The agent judges.
- The script fetches each upstream repo with enough history to reach every pin, and resolves the
  latest commit on the default branch.
- **`--check`** lists each manifest skill whose upstream path has new commits since its pin, with
  the commit count. It writes nothing. A person runs it by hand. There is no CI job or schedule.
- **`merge` strategy.** For each file in the skill (except `LICENSE` and `README.md`), with base =
  upstream at the pin, ours = gx's copy, theirs = upstream at latest:
  - A file in all three goes through `git merge-file`. A conflict leaves standard markers in gx's
    file.
  - A file only in theirs (upstream added it) is copied in.
  - A file in base but not in theirs (upstream deleted it) is deleted when gx's copy equals base.
    Otherwise it is a conflict.
  - A file in base but not in ours (gx deleted it) stays deleted when theirs equals base. Otherwise
    it is a conflict. This mirrors the upstream-delete rule.
- **`port` strategy.** The script prints the upstream diff for the skill path and touches no file.
  A changed `LICENSE` is handled as in "Credit files".
  The agent carries each upstream change that matters to gx's copy over by hand.
- The report prints, per skill: the upstream commits since the pin, the upstream diff for the
  skill path, and for `merge` the clean files and the conflicted files.
- When a pinned upstream path no longer exists at latest, the script reports it and touches nothing
  for that skill.
- The script does not list new upstream skills that no manifest entry names.

### The sync agent's rules

- It reviews every upstream hunk, clean ones too, and resolves every conflict marker.
- It decides alone on: `gx-*` renames of new upstream skill references (the call guard flags
  them), tracker links, and conflicts where both sides keep their intent.
- It asks the maintainer, using `gx-grilling`, about:
  - an upstream behaviour change;
  - a conflict with a gx decision;
  - a pinned upstream path that is gone. It first searches upstream for a skill with the same
    `name:`, then asks: follow the move, keep the skill as gx-owned (drop the manifest entry), or
    remove it;
  - an upstream `LICENSE` that changed: does the new license still allow vendoring? If not, the skill
    stays at its pin;
  - a vendored skill that now calls a skill outside the bundle. It asks: vendor that skill, drop the
    call, or point it at a `gx-*` skill.
- When most of a `merge` skill's hunks conflict, it suggests switching that skill to `port`. The
  maintainer decides.
- It runs the bundle tests. It moves the pins to the synced commits only after they pass.
- It commits the sync. The commit message lists every skipped upstream change (a change not ported,
  or a merge hunk it dropped) with a one-line reason. A skipped change still moves the pin.
- Renames are ordinary gx edits that the merge keeps. There is no rename table.

### Bundle tests

- **Call guard.** A test fails when any bundle file names a skill outside the bundle as a
  Skill-tool call or as a `/name` invocation. Harness built-in commands (`/clear`, `/new`,
  `/compact` and the like), URL paths in examples, and ticket types (`grilling`, `research`,
  `prototype`) are not skill calls. The `.skills["code-review"]` config may still name other skills;
  the guard checks bundle text, not config.
- **Conflict-marker guard.** A test fails when any file under `skills/` holds a git conflict
  marker line.
- `requiredFiles` gains every new file (each `SKILL.md`, supporting files, `LICENSE`, `README.md`).
  The existing exact-match test against `BundleFiles` and the every-directory-is-embedded test
  then cover the embed line.
- `wantInvocationPolicy` gains the new skills. `gx-wayfinder` and `gx-to-spec` are `true`.
  `gx-grilling`, `gx-domain-modeling`, `gx-research`, `gx-prototype`, `gx-unslop` and `gx-verify`
  are `false`.
  `gx-thermo-nuclear-code-quality-review` keeps upstream's value.

### Implementer's choices

- The manifest's file name, location and field names.
- The sync script's language and flags beyond `--check`, how it fetches upstream, and its exit
  codes. `scripts/` holds bash today.
- How the pin moves (a script subcommand or an edit by the agent).
- Whether the script formats base and theirs the same way gx's copies are wrapped before merging,
  so line wrapping alone never conflicts.
- How the call guard detects a call, and its list of harness built-ins.
- Exact wording of `README.md`, the `gx.md` sections, and the reworded `gx-grilling` rules.
- How `gx-verify` detects that a tool is not installed.

## Testing Decisions

A good test checks external behaviour: what the bundle contains and says, what the config defaults
to, and what the sync script writes and prints for a given upstream history. It does not check how
any of them are built.

- **Seam 1: the `skills` package bundle tests.** Extend them with the call guard, the
  conflict-marker guard, the new `requiredFiles` entries and the new `wantInvocationPolicy` entries.
  The existing relative-link test covers the new `../gx-local-tracker.md` links. The call guard
  fails on today's bundle (bare `grilling`, `/wayfinder`, `/verify`), which proves it works before
  the renames land.
- **Seam 2: the sync script's command line.** Tests build a fixture upstream git repo in a temp dir
  with a few commits, plus a fixture gx skill copy and manifest that point at it. They assert on
  the files written and the report printed:
  - a clean merge, a conflicting merge (markers present, file listed as conflicted);
  - an upstream-added file, an upstream-deleted file with and without gx changes, a gx-deleted
    file;
  - an unchanged `LICENSE` and `README.md` left untouched;
  - a changed upstream `LICENSE` reported, with nothing written for that skill;
  - `port` printing the diff and writing nothing;
  - a missing upstream path reported;
  - `--check` printing commit counts and writing nothing.
- **Seam 3: the `config` package tests.** The existing default tests change to expect
  `gx-thermo-nuclear-code-quality-review`.
- The agent's judgment (hunk review, when to ask) and `gx-verify`'s new rules (the TS/JS row, the
  frozen install, missing-tool skips, the results) are prose. A maintainer
  checks them by hand. This is accepted.
- Prior art: `skills/bundle_test.go` and `skills/webui_docs_test.go` for bundle content pins,
  `config/config_test.go` for defaults, `scripts/create-test-repos.sh` for fixture git repos.

## Out of Scope

- Removing the dotfiles copies of these skills, and the dotfiles script's list. That is a separate
  dotfiles follow-up.
- Vendoring dotfiles third-party skills that no gx skill calls (`codebase-design`, `triage`,
  `herdr`, and others).
- Moving the Go packages into a subdirectory.
- The `gx-question` block rule in `gx-grilling`. The webui-design map adds it once `gx-grilling`
  exists.
- Server code changes.
- Installing into `~/.codex/skills` and adding Codex and OpenCode invocation metadata
  (gx:follow-ups/51).

## Further Notes

- This spec records the old bases of `gx-tdd`, `gx-to-tickets` and `gx-implement` as their pins.
  Pulling their upstream changes since then is the first ordinary sync run after this lands.
- Sources: `docs/research/upstream-licenses.md` (branch `research/upstream-licenses`),
  `docs/research/vendored-skills-local-changes.md` (branch `research/local-changes-inventory`) and
  `docs/research/thermo-nuclear-license.md` (branch `research/thermo-nuclear-license`).
