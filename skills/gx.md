# gx skill bundle

This directory is gx's canonical, installable skill bundle: the content `gx skills install` copies
into Claude's and Codex's user skill roots.

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

Codex discovers these as skills under `~/.agents/skills`. Both agents receive the same directory
layout and relative references. The invocation rule differs per harness:

- Claude reads `disable-model-invocation` from `SKILL.md`.
- Codex ignores that key. It reads `policy.allow_implicit_invocation` from `agents/openai.yaml`.
- Every skill with `disable-model-invocation: true` also has `agents/openai.yaml` with
  `allow_implicit_invocation: false`, so neither agent starts it on its own.

If a gx skill also sits under `$CODEX_HOME/skills`, Codex finds it twice. `gx skills install` and
the refresh in `gx server start` print a `warning: skills:` line naming each such path. They never
delete it and never fail on it.

## Installation

The `gx` binary embeds this bundle. Ordinary installations copy its files into real directories
under `~/.claude/skills` and `~/.agents/skills`. They need no source checkout. Both roots receive the
same layout, so references such as `../gx-local-tracker.md` resolve in either agent.

```sh
gx skills install                            # install or update bundled copies
gx skills install --dev                      # link this checkout's skill directories
gx skills install --force <relative-path>    # replace one conflicting entry
gx skills install --force-all                # authorize all reported replacements
gx skills uninstall                          # remove owned entries, preserving local edits
gx skills uninstall --force <relative-path>  # remove one edited entry
```

Each target is reported as `installed`, `updated`, `skipped`, `conflicted` or `removed`.
A conflict aborts installation before any destination changes. `--force` names the reported
bundle-relative path. For a directory link or a directory being replaced by a link, name the
skill directory, such as `gx-implement`.

Gx owns directory links as single entries. Updating or uninstalling a dev skill removes the link
itself and preserves its source directory. Copied installations own individual files, so unrelated
files inside their directories remain untouched by normal updates and uninstall.

Discovery roots may be user-managed symlinks. Gx preserves those links and handles roots that point
to the same physical directory once. Force never authorizes writing through a skill-directory link
into the source checkout.

### Development mode

Run `gx skills install --dev` from a gx checkout, including a nested directory or linked worktree.
Each bundled skill directory is linked to its directory in that checkout. Its `SKILL.md` remains a
real source file. The whole directory is visible, including supporting files outside the embedded
bundle. Shared files `gx.md`, `gx-local-tracker.md` and `gx-failing-tests.md` are linked separately
beside the skill directories.

Repeat installation from the same checkout leaves correct links untouched. Changing checkout or
switching between copies and links requires explicit force. For example:

```sh
gx skills install --dev --force-all  # switch to this checkout after reviewing conflicts
gx skills install --force-all        # switch back to bundled copies
```

### Migration and recovery

Gx reads the old file-based manifest and migrates verified, unchanged entries. It can adopt an
existing correct directory link without touching its target. Edited files, wrong link targets and
unrelated directory contents block migration until their reported replacements are forced.
After successful placement, gx removes unchanged owned files from the retired `~/.codex/prompts`
root. Unrelated prompts remain. Modified owned prompts block migration.

Before changing destinations, gx saves recovery snapshots under
`~/.local/share/gx/skills-backups`, honoring `XDG_DATA_HOME`. The command prints the dated backup
location. Backups remain until you remove them. Symlinks are backed up as links, including broken
links. Forced directory replacement backs up the entire replaced directory.

Each backup contains `paths.json`, which maps original absolute paths to numbered snapshots, and
`manifest.json` when an installation already existed. An empty snapshot field means that path was
absent before the operation. To recover manually, stop gx, inspect that mapping, restore the listed
snapshots to their original paths while preserving symlinks, and restore the previous manifest to
`~/.config/gx/skills/gx-canonical.json`. Never copy through an existing directory link.

Install, refresh and uninstall serialize against the same manifest. If a write or manifest
publication fails, gx restores the previous destinations and reports the retained backup location.
If restoration fails too, the error names the affected paths and recovery location.

`gx server start` and `gx server restart` use the same installer. They keep the recorded mode and
dev checkout and never force. A conflict leaves the installation unchanged, lets the server start,
and records a warning naming the path and the `gx skills install` command needed to resolve it.

## Layout

```
skills/
  gx.md                — this file
  gx-local-tracker.md  — the shared local ticket tracker contract
  gx-failing-tests.md  — the shared rule for sorting a failing test
  gx-to-tickets/
    SKILL.md
  gx-tdd/
    SKILL.md
    tests.md
    mocking.md
  gx-implement/
    SKILL.md
    agents/openai.yaml  — Codex invocation policy; every skill with disable-model-invocation has one
  gx-resolving-merge-conflicts/
    SKILL.md
  gx-investigate/
    SKILL.md
    gotchas.md
  gx-cleanup/
    SKILL.md
    agents/openai.yaml
  gx-merge/
    SKILL.md
    agents/openai.yaml
  gx-code-review/
    SKILL.md
    agents/openai.yaml
  gx-changelog/
    SKILL.md
  gx-bump/
    SKILL.md
    agents/openai.yaml
  gx-one-off/
    SKILL.md
    agents/openai.yaml
  gx-evolve/
    SKILL.md
    agents/openai.yaml
  gx-triage-follow-ups/
    SKILL.md
    agents/openai.yaml
  gx-design/
    SKILL.md
    agents/openai.yaml
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
    agents/openai.yaml
  gx-to-spec/
    SKILL.md
    LICENSE
    README.md
    agents/openai.yaml
  gx-thermo-nuclear-code-quality-review/
    SKILL.md
    LICENSE
    README.md
    agents/openai.yaml
  gx-verify/
    SKILL.md
  gx-verify-epic/
    SKILL.md
    agents/openai.yaml
```

Every skill references `gx-local-tracker.md` with a relative path (`../gx-local-tracker.md`), so it
must be installed alongside the skill directories, at the bundle root, for those references to
resolve.
