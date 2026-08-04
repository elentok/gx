# gx skill bundle

This directory is gx's canonical, installable skill bundle: the content `gx skills install` copies
into both Claude's and Codex's user skill/prompt roots (see the `gx skills` command once it ships).

## Origin

`gx-to-tickets`, `gx-tdd`, `gx-implement`, and `gx-resolving-merge-conflicts` are gx-specific
adaptations of four skills from [Matt Pocock's skills repo](https://github.com/mattpocock/skills):
`skills/engineering/to-tickets/SKILL.md`, `skills/engineering/tdd/SKILL.md`,
`skills/engineering/implement/SKILL.md`, and
`skills/engineering/resolving-merge-conflicts/SKILL.md`. They're inlined here, rather than fetched
at install time, for the same reason the personal dotfiles skill tree inlines its own copies:
security and ease-of-deployment — no network fetch during an unattended ralph-loop run.

## gx-specific adaptations

The upstream skills assume a human operator and a choice of issue tracker (local files, GitHub,
Linear, ...) selected by a separate setup step. gx's bundle drops both assumptions:

- **Tracker is fixed, not selected.** gx only ever writes gx's local markdown tracker (see
  [local-tracker.md](local-tracker.md)) — through `gx tickets` subcommands, not hand-edited YAML.
  There's no provider-selection step and no dependency on a personal dotfiles skill tree, setup
  workflow, or triage-label mapping file: `local-tracker.md` is the complete, self-contained
  contract.
- **Seams are decided in the ticket, not live.** Upstream `tdd` asks a human "what's the public
  interface, and which seams should we test?" at the start of every cycle. gx-to-tickets instead
  requires every generated ticket to declare its approved test seams up front (an explicit `none`
  with rationale when no automated seam applies), and gx-tdd consumes those declared seams — so the
  red/green loop runs unattended under ralph-loop without stalling on a question nobody's there to
  answer. A ticket authored before this convention existed may still proceed, but only when a
  minimal seam is unambiguous from its acceptance criteria; anything requiring real judgment stops
  the ticket at `needs-info` instead of guessing.
- **Context occupancy is inspected through gx, not by hand.** Upstream `implement` tails the raw
  session transcript JSONL and sums token fields to check context occupancy — a Claude-only,
  format-coupled trick. `gx-implement` calls `gx agent context-window` instead, which works
  identically under Claude and Codex and fails closed (any read/detection failure counts as
  over-budget) rather than guessing.

## Invocation policy

All four skills carry `name`/`description` frontmatter Claude Code reads to render the skill
picker. `gx-to-tickets` and `gx-implement` set `disable-model-invocation: true`, matching their
upstream: breaking work into tickets, and claiming/implementing one, are deliberate,
explicitly-invoked actions, never something the model should trigger on its own reading of a
conversation. `gx-tdd` and `gx-resolving-merge-conflicts` carry no such flag, also matching
upstream: it's fine for the model to reach for TDD guidance or merge-conflict resolution on its own
when a task calls for it.

Codex has no equivalent auto-invocation concept — a Codex custom prompt is only ever run by explicit
`/name` invocation, never launched by the model on its own. That's already at least as restrictive as
`disable-model-invocation: true`, so all four skills preserve their intended policy under Codex
without any extra metadata: `gx-to-tickets`'s and `gx-implement`'s explicit-only intent holds as-is,
and `gx-tdd`'s and `gx-resolving-merge-conflicts`' "the model may reach for it" intent is satisfied
whenever the calling skill (e.g. `gx-implement` invoking `gx-tdd`) explicitly invokes it.

## Installation

This directory is embedded in the `gx` binary at compile time (`bundle.go`), so a release, Homebrew,
`go install`, or locally built `gx` all ship the identical bundle. `gx skills install` copies it as
managed files into both Claude's and Codex's user skill roots — `~/.claude/skills` and
`~/.codex/prompts` — under the same relative layout this directory has, so every relative reference
(like `../local-tracker.md`) resolves identically under both agents:

```sh
gx skills install                            # install/upgrade for both agents
gx skills install --force <relative-path>    # replace a specific detected conflict
gx skills uninstall                          # remove gx's managed copies
gx skills uninstall --force <relative-path>  # remove a specific locally-modified file
```

Each target is reported as `installed`, `updated`, `skipped` (already up to date), `conflicted`
(left untouched — locally modified or unrelated content occupies that path; pass `--force` with the
path to override), or `removed`. A conflict aborts the whole install, so re-running `gx skills
install` after a `gx` upgrade is the way to refresh the bundle: unaffected files report `skipped`,
changed ones report `updated`.

### Development mode

Working from a gx source checkout, `--dev` symlinks this directory's files into both agents'
discovery roots instead of installing embedded copies, so an edit under `skills/**` shows up to both
agents immediately, without rerunning `gx skills install`:

```sh
go run . skills install --dev    # symlink this checkout's skill files
```

`--dev` resolves the git repository containing the current working directory (not the `gx` binary's
own location), so it works the same from a nested subdirectory or a linked worktree, and it refuses
to change anything if that repository doesn't contain gx's skill bundle. Re-running it from the same
checkout is idempotent (`skipped`); running it from a *different* gx checkout reports every path
`conflicted` rather than silently retargeting the existing symlinks — pass `--force` with each path to
relink to the new checkout.

To go back to a production install of gx's embedded, released skills, run `gx skills install` (no
`--dev`) and pass `--force` with each path it reports as `conflicted` — switching *out* of dev mode is
a mode change like any other and needs the same explicit confirmation:

```sh
gx skills install --force README.md --force gx-implement/SKILL.md ...   # one --force per reported conflict
```

## Layout

```
skills/
  README.md          — this file
  local-tracker.md    — the shared local ticket tracker contract
  gx-to-tickets/
    SKILL.md
  gx-tdd/
    SKILL.md
    tests.md
    mocking.md
  gx-implement/
    SKILL.md
  gx-resolving-merge-conflicts/
    SKILL.md
```

Every skill references `local-tracker.md` with a relative path (`../local-tracker.md`), so it must
be installed alongside the skill directories, at the bundle root, for those references to resolve.
