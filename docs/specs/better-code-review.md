# Better code review: more reviewers and a security review in the default list

## Problem Statement

`gx-code-review` runs two Quality reviewers by default: `gx-thermo-nuclear-code-quality-review` and
`gx-test-review`. They judge structure and test quality. Nothing in the default run looks for
logic bugs, swallowed errors, weak type design, wrong or stale comments, missing tests or security
problems. A person who wants those checks must write their own `skills.code-review` list and find
reviewer prompts to put in it.

Anthropic publishes good reviewers for this: five agents in the `pr-review-toolkit` plugin and the
`/security-review` prompt. Installing the plugin is not an option for gx. An unattended ralph-loop run
cannot depend on a plugin a person has to install by hand, and the agents read their own `git diff`
instead of the shared review files that `gx-code-review` writes.

## Solution

gx ships six new reviewers as bundled `gx-*` skills and adds them to the default reviewer list:

- Five come from `pr-review-toolkit`: code-reviewer, silent-failure-hunter, type-design-analyzer,
  comment-analyzer and pr-test-analyzer.
- One comes from `/security-review`: a security reviewer.

Each reviewer reads the shared review files that `gx-code-review` writes. Each starts with a "Skip
when" step: when the diff holds nothing it can judge, it says "nothing to review" and stops. That
keeps a diff of markdown or skill text cheap. `gx-code-review` stays simple: no glob table, it
launches every configured reviewer.

Under Codex, `gx-code-review` launches its subagents in waves, following the parallel cap in
`skills/gx-local-tracker.md` "Skill conventions". Triage never defers a confirmed security finding.

The two current reviewers stay in the default list, the ~50-line size gate stays, and
`skills.code-review` still replaces the whole list.

Trial evidence (one finished epic, 1136 changed lines): the six new reviewers cost 61k to 106k tokens
each. Only comment-analyzer and type-design-analyzer added unique findings, both low severity.
code-reviewer, silent-failure-hunter and security-review found nothing. The list stays complete
anyway: one epic and one run are too thin to drop a reviewer, and the self-skip step keeps the cost
down on diffs a reviewer cannot judge.

## User Stories

### A gx user running code review

1. As a gx user, I want the default code review to include a general code reviewer, so that logic
   bugs and convention breaks are checked without my configuring anything.
2. As a gx user, I want the default code review to include a silent-failure hunter, so that swallowed
   errors and misleading fallbacks are caught.
3. As a gx user, I want the default code review to include a type-design analyzer, so that weak
   invariants and leaky type shapes are flagged when a type changes.
4. As a gx user, I want the default code review to include a comment analyzer, so that wrong, stale
   or misleading comments are flagged when a comment changes.
5. As a gx user, I want the default code review to include a test-coverage analyzer, so that missing
   tests are flagged, which `gx-test-review` never does.
6. As a gx user, I want the test-coverage analyzer to report a missing test only when no open verify
   ticket covers that story, so that it does not duplicate the tickets that already own test gaps.
7. As a gx user, I want the default code review to include a security review, so that
   vulnerabilities in the diff are caught before the epic lands.
8. As a gx user, I want `gx-thermo-nuclear-code-quality-review` and `gx-test-review` to stay in the
   default list, so that the review I have today does not shrink.
9. As a gx user with `skills.code-review` set, I want my list to replace the whole default list, so
   that my choice still wins.
10. As a gx user, I want no plugin to install, so that code review works on a fresh machine with only
    gx installed.
11. As a gx user, I want the reviewers to work the same under Claude and Codex, so that the review
    does not depend on my harness.
12. As a gx user, I want a diff of about 50 changed lines or fewer to skip all Quality reviewers,
    so that a tiny epic does not pay for eight reviews.
13. As a gx user, I want a reviewer to say "nothing to review" and stop when the diff holds nothing
    it can judge, so that it does not burn tokens on files it cannot apply to.
14. As a gx user, I want the security reviewer to stop on a diff with no executable code (markdown
    and skill text alone do not count), so that a docs-only epic gets no security pass.
15. As a gx user, I want the type-design analyzer to stop when the diff adds or changes no type,
    interface or struct definition, so that it runs only when there is a type to judge.
16. As a gx user, I want the silent-failure hunter to stop when the diff holds no error-handling or
    fallback code, so that it runs only when there is something to hunt.
17. As a gx user, I want the code reviewer to stop only when the diff holds no code files, so that
    it stays the broad default reviewer.
18. As a gx user, I want the comment analyzer to stop when the diff changes no comment, so that it
    runs only when a comment changed.
19. As a gx user, I want the test-coverage analyzer to stop when the diff changes no source file, so
    that a test-only or docs-only diff does not trigger it.

### A gx user on Codex

20. As a gx user on Codex, I want `gx-code-review` to launch its subagents in waves that follow the
    parallel cap in "Skill conventions", so that it respects Codex's subagent limit.
21. As a gx user on Codex, I want every reviewer to still run, so that Codex gets the same review
    coverage as Claude, only slower.
22. As a gx user on Codex, I want the Spec subagents, including the coverage re-dispatch, to count
    in the waves, so that the cap is never exceeded.
23. As a gx user on Claude, I want the single launch message kept, so that Claude's reviews stay
    fully parallel.

### Triage and tickets

24. As a gx user, I want triage never to defer a confirmed security finding, so that a real
    vulnerability is never parked as a follow-up.
25. As a gx user, I want triage to approve a confirmed security finding and reject it only when it
    is not real, so that the finding becomes a fix ticket in the epic.
26. As a gx user, I want the "tiny finding is deferred" rule not to apply to a security finding, so
    that a one-line security fix still becomes a fix ticket.
27. As a gx user, I want security findings to use the same fix-ticket and follow-up flow as other
    findings, so that I learn nothing new.
28. As a gx user, I want every new reviewer's findings joined into the one triage, labeled by
    skill, so that duplicates across reviewers are consolidated.

### A gx maintainer

29. As a gx maintainer, I want each copied reviewer to ship its upstream `LICENSE` and a `README.md`
    that credits the upstream, so that gx meets the Apache-2.0 and MIT notice rules.
30. As a gx maintainer, I want each copied reviewer to have a `port` row in the vendored-skills
    manifest, so that the sync skill can show me upstream changes to it.
31. As a gx maintainer, I want the copies not to sync automatically, so that an upstream change never
    alters the review without my review of it.
32. As a gx maintainer, I want the security reviewer to read the shared review files instead of
    running `!` git lines, so that it judges the same diff as every other reviewer.
33. As a gx maintainer, I want the bundle tests to cover the new skill directories and their
    invocation policy, so that a new reviewer cannot ship uninstallable or half-wired.
34. As a gx maintainer, I want `skills/gx.md` and the README credits to list the new upstreams and
    name Apache-2.0 for `pr-review-toolkit`, so that the bundle's origins stay in one place.

## Implementation Decisions

### The six new reviewer skills

- Each is a bundled skill directory under `skills/`, named `gx-` plus the upstream name:
  `gx-code-reviewer`, `gx-silent-failure-hunter`, `gx-type-design-analyzer`,
  `gx-comment-analyzer`, `gx-pr-test-analyzer`, and `gx-security-review`. Each holds `SKILL.md`,
  the upstream `LICENSE`, a `README.md` and `agents/openai.yaml` (the shape `gx-thermo-nuclear-code-quality-review`
  and `gx-test-review` use).
- Five have a `plugins/pr-review-toolkit/agents/<name>.md` upstream. That agent file, with its
  frontmatter, is a subagent definition, not a skill. The copy turns it into a skill: `name:` becomes the `gx-*`
  name, `description:` says what the skill reviews, and the agent-only keys (`model`, `color`,
  `tools`) go. The body of the upstream prompt stays as written except for the edits below.
- The upstream `/security-review` file is a slash-command definition. Its conversion adds `name:`
  (`gx-security-review`) and `description:` (what the skill reviews), sets
  `disable-model-invocation: true`, and drops the command-only keys (`allowed-tools`,
  `argument-hint` and any other key a skill does not use).
- Every reviewer sets `disable-model-invocation: true`, like the two current reviewers. Callers read
  the file by path.
- **Upstream sources and licenses**:

  | gx skill | Upstream repo | Upstream path | License file | License |
  | --- | --- | --- | --- | --- |
  | `gx-code-reviewer` | anthropics/claude-plugins-official | `plugins/pr-review-toolkit/agents/code-reviewer.md` | `plugins/pr-review-toolkit/LICENSE` | Apache-2.0 |
  | `gx-silent-failure-hunter` | anthropics/claude-plugins-official | `plugins/pr-review-toolkit/agents/silent-failure-hunter.md` | `plugins/pr-review-toolkit/LICENSE` | Apache-2.0 |
  | `gx-type-design-analyzer` | anthropics/claude-plugins-official | `plugins/pr-review-toolkit/agents/type-design-analyzer.md` | `plugins/pr-review-toolkit/LICENSE` | Apache-2.0 |
  | `gx-comment-analyzer` | anthropics/claude-plugins-official | `plugins/pr-review-toolkit/agents/comment-analyzer.md` | `plugins/pr-review-toolkit/LICENSE` | Apache-2.0 |
  | `gx-pr-test-analyzer` | anthropics/claude-plugins-official | `plugins/pr-review-toolkit/agents/pr-test-analyzer.md` | `plugins/pr-review-toolkit/LICENSE` | Apache-2.0 |
  | `gx-security-review` | anthropics/claude-code-security-review | `.claude/commands/security-review.md` | `LICENSE` | MIT |

- `code-simplifier`, the sixth `pr-review-toolkit` agent, is not copied. It edits code, and
  thermo-nuclear covers simplification.
- **Requirement: every vendored skill directory includes the upstream license file, copied verbatim**,
  as `skills/gx-thermo-nuclear-code-quality-review/` does. The five `pr-review-toolkit` directories
  take `plugins/pr-review-toolkit/LICENSE` (Apache-2.0). `gx-security-review` takes the repo
  `LICENSE` of `anthropics/claude-code-security-review` (MIT). If an upstream file is named
  `LICENSE.md`, keep that filename; the file list, the tests and the manifest then use it.
- Each `README.md` follows the thermo-nuclear README: the upstream repo link, the upstream path, the
  author, the license and "see `LICENSE`", and a line saying gx changed the skill and what it changed.
  The author of all six is Anthropic. The README is written once, on first copying. It names the
  upstream by repo and file path (for example `anthropics/claude-code-security-review`,
  `.claude/commands/security-review.md`), never by the bare slash-command token, which
  `TestBundleCallsStayInsideBundle` reads as a call outside the bundle.
- Apache-2.0 asks that changes be marked. Each README states that the skill is changed from the
  upstream agent file and names the changes (skill format, review-file input, skip step).

### Edits every copy makes

- **Input.** The reviewer reads the shared review files whose paths the caller passes (fixed point,
  commit list, changed-file list, diff, search output). It never runs `git diff`, `git log -p` or
  `git status` for the diff. Upstream agents that "run `git diff`" by default lose that step. It may
  Read a changed file for more context.
- **Read-only.** The reviewer edits nothing and runs nothing that changes files.
- **Verify before reporting.** `gx-code-review` already adds the rule "verify a suspicion with one
  read or one grep; drop what you cannot confirm" to every subagent prompt. The skill text must not
  contradict it: an upstream "report possible issues" style is reworded to report only confirmed
  ones.
- **Output.** `gx-code-review`'s brief sets the output form: a list of file, line or hunk, summary,
  severity, suggested fix, under 500 words. Upstream output templates that conflict with it (long
  report headers, ratings tables, "positive observations" sections) are dropped or shortened to
  fit. Upstream's own severity or confidence scale may stay as the `severity` value, mapped to one
  word a triage consultant can rank. A finding below the upstream's own confidence bar stays dropped.
- **Plain tool phrases.** Upstream prompts name Claude tools (for example `Read`, `Grep`, `Bash`,
  `Task`). Each copy replaces them with the plain phrases from the "Tool names" table in
  `skills/gx-local-tracker.md` ("file read", "shell call", "start a subagent"). Each copy must pass
  `TestBundleToolWording` and the retired-terms guard (`TestBundleDropsRetiredTrackerTerms`); both
  are required to pass, for `SKILL.md` and for `README.md`.
- **Hand over.** Each ends with the same hand-over step as `gx-test-review`: return the findings to
  the caller, where they join the consultant triage in `gx-code-review`.
- **No nested subagents.** A reviewer does its own work in one pass. Upstream `security-review` starts
  parallel sub-tasks for exploration and false-positive filtering. The gx copy does that filtering
  itself, in its one pass, against the same exclusion list, and keeps the upstream rule that only
  findings above its confidence bar are reported. Nested subagents would break the Codex cap and
  the cost shape of the run.
- **Skip step.** The first numbered step of every new reviewer is `## 1. Skip when ...`. It reads the
  changed-file list (and the diff when the rule needs it), says "nothing to review" and stops when its
  rule matches. Skip rules:

  | Reviewer | Stops when |
  | --- | --- |
  | `gx-security-review` | no executable code file changed (Go, TS/TSX, bash, SQL, config). Markdown and skill text alone do not count |
  | `gx-type-design-analyzer` | the diff adds or changes no type, interface or struct definition |
  | `gx-silent-failure-hunter` | the diff holds no error-handling or fallback code |
  | `gx-code-reviewer` | the diff has no code files |
  | `gx-comment-analyzer` | the diff changes no comment |
  | `gx-pr-test-analyzer` | the diff changes no source file |

### `gx-pr-test-analyzer`

- It reports a missing test, which `gx-test-review` never does. The rule "open verify tickets own
  test gaps; report a missing test only when no open verify ticket covers that story" is already in
  the Quality brief of `gx-code-review` for every Quality reviewer, and it stays there once. The
  analyzer's skill text does not repeat it.
- Upstream's test-quality ratings stay out of its report. `gx-test-review` owns judging the tests that
  exist.

### `gx-security-review`

- Source: the `/security-review` command prompt from `anthropics/claude-code-security-review`. Upstream
  builds its diff with `!` shell lines against `origin/HEAD`. Those lines are replaced by a read of
  the shared review files: the changed-file list and the diff.
- The rest of the upstream prompt stays: categories to look for, severity and confidence scale,
  false-positive exclusions, and the report form (adapted to the output rule above).
- A security finding carries the severity word and a confidence the triage consultant can read, so
  that it can tell a confirmed finding from one that is not real.

### `gx-code-review` changes

- **Default list (step 2).** The fallback becomes the eight skills: the existing two first, then
  the six new ones, each by its `gx-*` name, in this order: `gx-thermo-nuclear-code-quality-review`,
  `gx-test-review`, `gx-code-reviewer`, `gx-silent-failure-hunter`, `gx-type-design-analyzer`,
  `gx-comment-analyzer`, `gx-pr-test-analyzer`, `gx-security-review`. An empty or absent `skills.code-review` falls back to it; a non-empty value replaces it whole, as
  now.
- **Size gate (step 3).** Unchanged: about 50 changed lines or fewer skips every Quality subagent,
  now eight. The Spec subagent still runs.
- **Launch (step 3).** The parallel cap and the waves rule already live in
  `skills/gx-local-tracker.md` "Skill conventions". `gx-code-review` links to it and does not restate
  the number. It adds only the gx-code-review specifics: the Spec subagents (all ranges, and the
  coverage re-dispatch) count toward the waves, every configured reviewer still runs, and Claude
  keeps the single launch message. Which reviewers fill which wave is the implementer's choice. How
  the skill tells the harness is the implementer's choice, so long as the rule says nothing new for
  a Claude run.
- **Coverage.** The Spec coverage re-dispatch, when a range is only partly covered, follows the same
  wave rule.
- **No glob table.** `gx-code-review` gets no per-reviewer file rule. Skipping is each reviewer's job.
- **Triage (step 4).** The consultant never defers a confirmed security finding. It approves it, and
  rejects it only when it is not real. The "tiny finding is deferred" rule does not apply to it. A
  security finding is one raised by the security reviewer, or by any other reviewer when it
  describes a vulnerability.
- **Steps 5 to 7.** Unchanged. Approved security findings become fix tickets the same way, deferred
  ones (none, by the rule above) would become follow-ups, and the close-out records them like any
  other finding.
- The "default skill sets `disable-model-invocation`" note in step 2 stays true: all eight keep the flag.

### Config and bundle

- The `config` package's code-review default becomes the eight-skill list. Its tests change to match.
  `docs/config-schema.json` names no skill, so it needs no edit.
- The bundle embed list gains the six directories. The bundle tests gain each new file in
  `requiredFiles` (each `SKILL.md`, `LICENSE`, `README.md`, `agents/openai.yaml`) and each skill in
  `wantInvocationPolicy` with `true`.
- `skills/gx.md`: "Origin" gains a section for `anthropics/claude-plugins-official` (five skills,
  Apache-2.0) and one for `anthropics/claude-code-security-review` (one skill, MIT). Its sentence
  that each vendored directory has a MIT `LICENSE` is reworded: five are Apache-2.0. "Layout" lists the
  new directories and files.
- `README.md` credits gain both upstreams, named by repo and file path (never a bare slash-command
  token).
- `docs/ralph-loop-queue-orchestrator.md` names the eight-skill default.
- `scripts/vendored-skills.manifest`: one `port` row per new skill, in the manifest's columns: gx
  skill, upstream repo, upstream file path, upstream license path, copied-from commit, `port`. The
  column header comment is updated: the upstream-path column holds a directory, or a file path for a
  `port` row, and the commit column is the commit the copy was taken from. The
  copied-from commit is the upstream commit the copy was taken from. The copies are not synced
  automatically: the sync script is not run, and the sync skill is run by hand as for the other
  vendored skills. The existing `port` handling works on a file path (path check, LICENSE gate,
  commit count and diff), so the script is not changed.
- The call guard in the bundle tests keeps passing: the new skills name no skill outside the bundle.

### Implementer's choices

- The exact wording of each `SKILL.md` body, `README.md` and the new `gx.md` and README text.
- How each upstream prompt is trimmed to fit the output rule.
- Which reviewers fill which Codex wave, and how the skill text detects Codex.
- The wording of the "nothing to review" line, as long as it contains those words.
- How each skip rule is decided from the changed-file list and diff (extension lists, patterns).
- The upstream commit each manifest row records, and the `agents/openai.yaml` display name and
  short description.

## Testing Decisions

A good test checks external behaviour: what the bundle contains, what the config defaults to, and
what the skill texts require. It does not check how a skill is worded beyond the rule anchors.

- **Seam 1: the `skills` package bundle and rule tests.** Extend `requiredFiles`,
  `wantInvocationPolicy` and the embed coverage for the six directories. Add rule-anchor tests, in
  the style of `TestTestReviewSkill` and the `gx-code-review` rule table, that pin:
  - each new reviewer's `## 1. Skip when` heading, the words "nothing to review", the hand-over link
    to `../gx-code-review/SKILL.md`, and no `git diff` command in the skill text;
  - `gx-security-review` reads the review files and holds no `!` shell line;
  - `gx-code-review` names the eight default skills, links the "Skill conventions" wave rule (and
    does not restate the cap), says the Spec subagents and the coverage re-dispatch count toward
    waves, that every reviewer runs and that Claude keeps one launch message, and holds the
    never-defer-a-confirmed-security-finding rule.
- **Seam 2: the `config` package tests.** The existing default tests expect the eight-skill list. A
  new test reads the list in step 2 of `skills/gx-code-review/SKILL.md` and asserts it equals
  `DefaultSkillsConfig().CodeReview` in `config/skills.go`: same names, same order.
- **Manifest rows.** `TestEveryVendoredSkillHasManifestRow` in `skills/bundle_test.go` already reads
  `scripts/vendored-skills.manifest`; it covers the six new rows (each directory with a `LICENSE`
  needs a row). No manual check.
- **Sync script fixture.** A new case in `scripts/sync_vendored_skills_test.go` runs the script on a
  fixture manifest holding a `port` row whose upstream path is a file path (not a directory) and
  asserts the path check and the LICENSE gate accept it.
- Reviewer quality (what a reviewer finds) is prose behaviour. The trial in
  gx:better-code-review/01 is the evidence; no further automated test is added. This is accepted.
- Prior art: `skills/bundle_test.go`, `skills/test_rules_test.go`, `skills/check_rules_test.go` and
  `config/skills_test.go`.

## Verification

The epic has no user-visible UI behaviour: no web, no stored mock. It changes skill text, the
bundle, a config default and a manifest. A verify ticket checks the CLI-visible parts against a
temporary gx server.

Reviewer skills and defaults (stories 1 to 11, 29 to 34)

- Flows: none.
- `gx config get skills.code-review` with no override prints the eight-skill JSON list, the two
  current names first. With an override of one name, it prints only that name.
- `gx skills install` into a temp root, then each of the six new directories holds `SKILL.md`,
  `LICENSE`, `README.md` and `agents/openai.yaml`. The five `pr-review-toolkit` `LICENSE` files are
  Apache-2.0, and the `gx-security-review` one is MIT.
- Each of the six new directories (in the repo and in the install root) has a non-empty `LICENSE`
  (or the upstream `LICENSE.md` filename).
- None of the six skills gets a "path no longer exists" or "license changed" line from the sync
  script against its manifest row.
- The bundle tests pass (`go test ./skills/...` under the capped runner).

Size gate and skip steps (stories 12 to 19)

- Flows: none.
- Read each new `SKILL.md`: the first step is `## 1. Skip when ...`, with its rule from the table,
  the words "nothing to review", and no `git diff` command.
- Read steps 3 to 7 of `skills/gx-code-review/SKILL.md` (story 12): the ~50-line gate is unchanged
  and skips every Quality subagent, and the Spec subagent still runs.
- No new flow is needed: skip behaviour is prose and is checked by reading.

Codex waves and triage (stories 20 to 28)

- Flows: none.
- Read `skills/gx-code-review/SKILL.md`: the wave rule (a link to "Skill conventions" with no
  restated number, Spec and coverage re-dispatch counted, every reviewer runs, Claude keeps one
  message) and the security triage rule (approve, reject only if not real, the tiny-finding rule
  does not apply) are each present once.
- Read steps 3 to 7 against story 27: a security finding uses the same fix-ticket, follow-up and
  close-out flow as any other finding, with no security-only branch past step 4.
- Read steps 3 to 7 against story 28: every reviewer's findings are labeled by skill and join the
  one step-4 triage, where duplicates are consolidated.
- No new flow is needed.

## Out of Scope

- Rewriting `scripts/sync-vendored-skills.sh` in TypeScript with a JSON manifest. The new `port` rows
  work with the bash script as it is (gx:follow-ups/573).
- Installing the `pr-review-toolkit` plugin.
- `code-simplifier`.
- Automatic syncing of the copies.
- Changing the ~50-line size gate, or a glob table in `gx-code-review`.
- Dropping or reordering the two current reviewers, or changing how `skills.code-review` overrides.
- Running new trials. The trial in gx:better-code-review/01 stands.

## Further Notes

- Trial caveats (gx:better-code-review/01): one epic, one run, reviewers on the sonnet tier. The trial
  diff had no file kind a reviewer could not apply to, so it does not show how cheap the self-skip
  keeps `gx-security-review` and `gx-type-design-analyzer` on a docs-only diff.
- `pr-review-toolkit` and `/security-review` are copied, not linked. Upstream changes arrive only by
  a manual run of the sync skill with `port` handling: the agent reads the upstream diff and carries
  over what matters.
- Sources: `gx:better-code-review` and its tickets 01 to 03.
