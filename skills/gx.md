# gx skill bundle

This directory is gx's canonical, installable skill bundle: the content `gx skills install` copies
into both Claude's and Codex's user skill/prompt roots (see the `gx skills` command once it ships).

## Origin

These skills are gx-specific adaptations of upstream skills. They're inlined here, rather than
fetched at install time: security and ease-of-deployment — no network fetch during an unattended
ralph-loop run. Each vendored directory has a `LICENSE` (MIT) and a `README.md` naming its upstream.

From [Matt Pocock's skills repo](https://github.com/mattpocock/skills) (Matt Pocock):

| Skill | Upstream path |
| --- | --- |
| `gx-to-tickets` | `skills/engineering/to-tickets` |
| `gx-tdd` | `skills/engineering/tdd` |
| `gx-implement` | `skills/engineering/implement` |
| `gx-research` | `skills/engineering/research` |
| `gx-prototype` | `skills/engineering/prototype` |
| `gx-wayfinder` | `skills/engineering/wayfinder` |
| `gx-to-spec` | `skills/engineering/to-spec` |
| `gx-domain-modeling` | `skills/engineering/domain-modeling` |
| `gx-grilling` | `skills/productivity/grilling` |

From [cursor/plugins](https://github.com/cursor/plugins):

| Skill | Upstream path | Author |
| --- | --- | --- |
| `gx-unslop` | `pstack/skills/unslop` | Lauren Tan (`pstack`) |
| `gx-thermo-nuclear-code-quality-review` | `cursor-team-kit/skills/thermo-nuclear-code-quality-review` | Cursor (`cursor-team-kit`) |

`gx-resolving-merge-conflicts` came from a skill Matt Pocock's repo has since removed, so it has no
upstream to pin; gx owns it now. `gx-verify` is gx's own.

## gx-specific adaptations

The upstream skills assume a human operator and a choice of issue tracker (local files, GitHub,
Linear, ...) selected by a separate setup step. gx's bundle drops both assumptions:

- **Tracker is fixed, not selected.** gx only ever writes gx's local markdown tracker (see
  [gx-local-tracker.md](gx-local-tracker.md)) — through `gx tickets` subcommands, not hand-edited
  YAML. There's no provider-selection step and no dependency on a personal dotfiles skill tree,
  setup workflow, or triage-label mapping file: `gx-local-tracker.md` is the complete, self-contained
  contract. Every call between bundled skills is a relative link to the callee's `SKILL.md`.
- **Seams are decided in the ticket, not live.** Upstream `tdd` asks a human "what's the public
  interface, and which seams should we test?" at the start of every cycle. gx-to-tickets instead
  requires every generated ticket to declare its approved test seams up front (an explicit `none`
  with rationale when no automated seam applies), and gx-tdd consumes those declared seams — so the
  red/green loop runs unattended under ralph-loop without stalling on a question nobody's there to
  answer. A ticket authored before this convention existed may still proceed, but only when a
  minimal seam is unambiguous from its acceptance criteria; anything requiring real judgment stops
  the ticket with `gx tickets ask` instead of guessing.
- **Context occupancy is inspected through gx, not by hand.** Upstream `implement` tails the raw
  session transcript JSONL and sums token fields to check context occupancy — a Claude-only,
  format-coupled trick. `gx-implement` calls `gx agent context-window` instead, which works
  identically under Claude and Codex and fails closed (any read/detection failure counts as
  over-budget) rather than guessing.

## Invocation policy

All skills carry `name`/`description` frontmatter Claude Code reads to render the skill picker.
`gx-implement` sets `disable-model-invocation: true`, matching its upstream: claiming/implementing a
ticket is a deliberate, explicitly-invoked action, never something the model should trigger on its
own reading of a conversation. `gx-cleanup` sets the same flag for the same reason: archiving epics
and deleting/merging branches is deliberate, explicit-invoke-only housekeeping, never something to
trigger on the model's own reading of a conversation. `gx-merge` sets the same flag for the same
reason: merging a branch onto main is deliberate, explicit-invoke-only, never something to trigger on
the model's own reading of a conversation. `gx-bump` sets the same flag for the same
reason: cutting a release - running tests, committing a changelog entry, tagging and pushing - is
deliberate, explicit-invoke-only, never something to trigger on the model's own reading of a
conversation. `gx-one-off` sets the same flag: the server launches it for `type: prompt` tickets,
never the model on its own. One-offs never commit: `gx server one-off` has no `--commits` or
`--base`. `gx-evolve` sets it too: the server launches it for `type: evolve`
retros, never the model on its own. `gx-triage-follow-ups` sets it as well: emptying a follow-ups
epic closes, moves and creates tickets in bulk, so a person starts it, never the model. `gx-design`
sets it too: a person walks one epic through map, review and implement. `gx-to-tickets`, `gx-tdd`,
`gx-resolving-merge-conflicts`, and `gx-changelog` carry no such flag: it's fine for the model to reach for
ticket breakdown, TDD guidance, or merge-conflict resolution on its own when a task calls
for it — e.g. a code-review ticket that needs to spin up follow-up tickets can invoke
`gx-to-tickets` itself, and `gx-bump` invokes `gx-changelog` as a sub-agent, which a flagged skill
would block. `gx-investigate` likewise carries no flag: a bug report should
reflexively pull in ralph-loop's log/state inventory without a human having to name the skill.
`gx-follow-up` carries no flag either: filing a follow-up is a cheap, safe write that a person often
asks for in passing, and other skills must be able to call it.

The vendored skills keep their upstream policy. `gx-wayfinder`, `gx-to-spec` and
`gx-thermo-nuclear-code-quality-review` set `disable-model-invocation: true`, as upstream does: a
person starts a map, a spec or a harsh review, and other skills reach them by reading their
`SKILL.md` by relative link, not through the Skill tool. `gx-research`, `gx-prototype`,
`gx-grilling`, `gx-domain-modeling`, `gx-unslop` and `gx-verify` carry no flag, so the model and
other skills can call them.

Codex has no equivalent auto-invocation concept — a Codex custom prompt is only ever run by explicit
`/name` invocation, never launched by the model on its own. That's already at least as restrictive as
`disable-model-invocation: true`, so `gx-implement`, `gx-cleanup`, `gx-merge`, and `gx-bump`
preserve their intended policy under Codex without any extra metadata: their explicit-only intent
holds as-is, and
`gx-to-tickets`'s, `gx-tdd`'s, and `gx-resolving-merge-conflicts`' "the model may reach for it" intent
is satisfied whenever the calling skill (e.g. `gx-implement` invoking `gx-tdd`) explicitly invokes it.

## Installation

This directory is embedded in the `gx` binary at compile time (`bundle.go`), so a release, Homebrew,
`go install`, or locally built `gx` all ship the identical bundle. `gx skills install` copies it as
managed files into both Claude's and Codex's user skill roots — `~/.claude/skills` and
`~/.codex/prompts` — under the same relative layout this directory has, so every relative reference
(like `../gx-local-tracker.md`) resolves identically under both agents:

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

`gx server start` and `gx server restart` run the same install before they start the server, so an
upgrade refreshes the bundle on its own. They keep the mode you chose last: a `--dev` install is
re-linked from the same checkout, anything else (or no install yet) gets managed copies. They never
force. A conflict, or a dev checkout that is gone, prints a warning, the server still starts, and
the web UI shows the problem until you close it or an install succeeds.

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
gx skills install --force gx.md --force gx-implement/SKILL.md ...   # one --force per reported conflict
```

## Layout

```
skills/
  gx.md                — this file
  gx-local-tracker.md  — the shared local ticket tracker contract
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
  gx-investigate/
    SKILL.md
    gotchas.md
  gx-cleanup/
    SKILL.md
  gx-merge/
    SKILL.md
  gx-code-review/
    SKILL.md
  gx-changelog/
    SKILL.md
  gx-bump/
    SKILL.md
  gx-one-off/
    SKILL.md
  gx-evolve/
    SKILL.md
  gx-triage-follow-ups/
    SKILL.md
  gx-design/
    SKILL.md
  gx-follow-up/
    SKILL.md
  gx-grilling/            — vendored; each vendored directory also has LICENSE and README.md
    SKILL.md
    LICENSE
    README.md
  gx-domain-modeling/
    SKILL.md
    LICENSE
    README.md
  gx-unslop/
    SKILL.md
    LICENSE
    README.md
  gx-research/
    SKILL.md
    LICENSE
    README.md
  gx-prototype/
    SKILL.md
    LICENSE
    README.md
  gx-wayfinder/
    SKILL.md
    LICENSE
    README.md
  gx-to-spec/
    SKILL.md
    LICENSE
    README.md
  gx-thermo-nuclear-code-quality-review/
    SKILL.md
    LICENSE
    README.md
  gx-verify/
    SKILL.md
```

Every skill references `gx-local-tracker.md` with a relative path (`../gx-local-tracker.md`), so it
must be installed alongside the skill directories, at the bundle root, for those references to
resolve.
