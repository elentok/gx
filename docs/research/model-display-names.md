# Model display names (gx:better-epic-header/02)

Where a model's pretty name can come from, and how the web UI shows models today. Code line numbers
are from the `better-epic-header-design` branch (31f45830). Harness facts come from the recorded
Claude reply and from `codex app-server` 0.166.0 run on this machine.

## 1. Claude: the `initialize` reply has names, gx ignores them

- The recorded reply is `commandlist/testdata/claude-initialize.jsonl`. Its `response.models` has 13
  entries: `value`, `resolvedModel`, `displayName`, plus effort flags.
- gx does not read `models`. `claudeReply` in `commandlist/claude.go:23-35` decodes only `commands`.
  The probe (`ProbeClaude`, `commandlist/claude.go:56`) runs `claude -p` once per (harness, dir),
  waits up to 15 s because SessionStart hooks run first (`claude.go:17`), and caches the slash-command
  list for 60 s (`commandlist/cache.go:11`). Its only caller is the slash-command menu
  (`server/sessioncommands.go:92-102`). No session or ticket payload carries a name today.
- Catalog (value | resolvedModel | displayName):

| value | resolvedModel | displayName |
|---|---|---|
| `default` | `claude-sonnet-5-5` | `Default (recommended)` |
| `opus` | `claude-opus-5-5` | `Opus 5.5` |
| `fable` | `claude-fable-5-1` | `Fable 5.1` |
| `sonnet` | `claude-sonnet-5-5` | `Sonnet 5.5` |
| `haiku` | `claude-haiku-5-5` | `Haiku 5.5` |
| `claude-haiku-4-5-20251001` | same | `Haiku 4.5` |
| `claude-sonnet-5`, `claude-opus-5`, `claude-fable-5` | same | `Sonnet 5`, `Opus 5`, `Fable 5` |
| `claude-opus-4-8`, `-4-7`, `-4-6`, `claude-sonnet-4-6` | same | `Opus 4.8`, `Opus 4.7`, `Opus 4.6`, `Sonnet 4.6` |

- Coverage:
  - Aliases (`sonnet`, `opus`, `haiku`, `fable`) and the 8 pinned IDs are covered.
  - The dated ID `claude-haiku-4-5-20251001` is covered because it is listed. An unlisted dated ID
    (for example a future `claude-opus-5-5-20270101`) is not.
  - `[1m]` is not covered. No entry has a `[1m]` suffix, and the list is the account's picker list, so
    `sonnet[1m]` or `claude-opus-5[1m]` finds no name.
  - A lookup by `resolvedModel` hits the `default` entry first for `claude-sonnet-5-5`, which reads
    "Default (recommended)". A lookup must skip `value: "default"`.
  - The list depends on the account and the installed CLI version. It is not per directory.
- What model strings gx holds:
  - Launch value from config (`config/agents.go:10-20`): aliases `sonnet`, `haiku`, `opus` for Claude.
  - Reported value: after the first reply, `server/sessionfailure.go:73-89` replaces it with the
    transcript's newest assistant `message.model` (`transcript/transcript.go:106`), the full ID such as
    `claude-sonnet-5-5`. Before a reply the session shows the launch alias.

## 2. Codex: `model/list` has names, gx does not call it

- Codex reports no name in the rollout. `turn_context` has only `model` and `effort`
  (`codexsession/model.go:15-40`). The skills reply has a `displayName` but for skills only.
- The app-server protocol has a `model/list` request (schema from `codex app-server
  generate-json-schema`). Each `Model` has `id`, `model`, `displayName`, `hidden`, `isDefault`.
  gx already starts `codex app-server` for `skills/list` (`commandlist/codex.go:62-66`), so the same
  process can answer `model/list`.
- Live reply on this machine (codex-cli 0.166.0, `includeHidden: true`):

| id | displayName |
|---|---|
| `gpt-6.1-sol` | `GPT-6.1-Sol` |
| `gpt-6-astra` | `GPT-6-Astra` |
| `gpt-6-luna` | `GPT-6-Luna` |
| `gpt-6.1-terra` | `GPT-6.1 Terra` |
| `gpt-6-sol` | `GPT-6-Sol` |
| `gpt-5.6-sol` / `-terra` / `-luna` | `GPT-5.6-Sol` / `GPT-5.6-Terra` / `GPT-5.6-Luna` |
| `gpt-reserve` (hidden) | `GPT-Reserve` |
| `codex-auto-review` (hidden) | `Codex Auto Review` |

- The names are not uniform: `GPT-6.1 Terra` uses a space where the others use a hyphen.
- Joined with the harness: "Codex GPT-6.1-Sol". It reads as the brand then the model. It is longer than
  "Claude Sonnet 5.5" and says "Codex" and "GPT" in one breath. That is the ticket's wanted shape, so the
  spec should accept it, or show the harness as the dot and the model alone on narrow screens.
- `config/agents.go` defaults: light `gpt-6-luna`, worker `gpt-6.1-sol`, strongest `gpt-6-astra`.

## 3. Fallback rule (a gx rule, pure and synchronous)

Claude:

1. Take a trailing `[x]` out of the ID; keep it as a suffix. `[1m]` reads ` (1M)`; any other
   suffix reads ` (x)`.
2. Drop a leading `claude-` and a trailing `-YYYYMMDD`.
3. Split on `-`. Every token must be a family word (`opus`, `sonnet`, `haiku`, `fable`) or digits, with
   exactly one family word. If not, return the input unchanged.
4. Output: family capitalized, then the digit tokens joined with `.`, then the suffix.

Codex (`gpt-` IDs): uppercase `GPT`, join the version with `-`, capitalize the remaining words, join
them with a space. Anything else is returned unchanged (custom or unknown models, and the
non-model strings `no model` and `a model from its tier` from `server/modelmismatch.go:18-21`).

The harness is joined last: `Claude ` or `Codex ` plus the model name. A name that already starts with
the harness name is not joined twice.

I ran this rule (Python port) against the real catalogs. For Claude it matches the harness `displayName`
for all 12 non-`default` entries when given the resolved ID. For Codex it matches `GPT-6.1 Terra` and
differs from the hyphenated names in the hyphen only.

Output for each ID the ticket names, and a few more:

| Input | Rule output | With harness |
|---|---|---|
| `claude-sonnet-5-5` | `Sonnet 5.5` | `Claude Sonnet 5.5` |
| `claude-haiku-4-5-20251001` | `Haiku 4.5` | `Claude Haiku 4.5` |
| `claude-opus-5` | `Opus 5` | `Claude Opus 5` |
| `claude-opus-4-8` | `Opus 4.8` | `Claude Opus 4.8` |
| `sonnet[1m]` | `Sonnet (1M)` | `Claude Sonnet (1M)` |
| `claude-opus-5[1m]` | `Opus 5 (1M)` | `Claude Opus 5 (1M)` |
| `claude-3-5-sonnet-20241022` | `Sonnet 3.5` | `Claude Sonnet 3.5` |
| alias `sonnet`, `opus`, `haiku`, `fable` | `Sonnet`, `Opus`, `Haiku`, `Fable` | `Claude Sonnet` ... |
| `gpt-6.1-sol` | `GPT-6.1 Sol` | `Codex GPT-6.1 Sol` |
| `gpt-6-luna` | `GPT-6 Luna` | `Codex GPT-6 Luna` |
| `gpt-6-astra` | `GPT-6 Astra` | `Codex GPT-6 Astra` |
| `gpt-5.3-codex` | `GPT-5.3 Codex` | `Codex GPT-5.3 Codex` |
| `my-proxy/llama`, `no model` | unchanged | `Claude my-proxy/llama` |
| empty | none: keep `CLI default` | |

Known limits of the rule:
- An alias carries no version. `sonnet` reads `Sonnet`, not `Sonnet 5.5`. Only the catalog (or the first
  reply, which reports the full ID) knows the version. That is fine: the label upgrades after the first
  reply.
- `gpt-x` reads `GPT-x`. Codex slugs outside the `gpt-<version>-<name>` shape pass through mostly as is.
- The `(1M)` wording for `[1m]` is a gx choice.

## 4. Every place the web UI shows a model today

All paths are under `webui/src/`. Every path is a raw ID, except where noted.

| # | Where | Code | Source field |
|---|---|---|---|
| 1 | `HarnessLabel` pill: `modelText` = model + effort | `ui/harness-label/HarnessLabel.tsx:54`, rendered at `:96` | prop `model` |
| 2 | `HarnessLabel` plain (dense list) line: `shortModel` strips only `claude-` | `ui/harness-label/HarnessLabel.tsx:58`, used at `:61` | prop `model` |
| 3 | Session header on the Agents session view (narrow and wide) | `agents/SessionView.tsx:50`, `:64` via `agents/SessionHarnessLabel.tsx:13` | `SessionInfo.model` (reported, `server/sessiontypes.go:57`) |
| 4 | Design page header | `design/DesignPage.tsx:179` via `SessionHarnessLabel` | `SessionInfo.model` |
| 5 | Epic ticket card, running rows | `epic/TicketCard.tsx:213` via `epic/rows.ts:228` (`rowChips`) | `EpicDetailRow.model` (`server/harnesslabel.go:15`) |
| 6 | New session form, tier picker: `Worker (sonnet)` | `agents/NewSessionForm.tsx:172` | `ProjectWorktrees.tiers` (launch value, `server/projectworktrees.go:33-41`) |
| 7 | Model-mismatch recovery text: `Ran on X, expected Y.` | `ticket/RecoveryActions.tsx:92-94` | `TicketDetail.model_ran`, `model_expected` (`server/detail.go:130-131`); the server can also send the non-ID strings `no model`, `a model from its tier` |
| 8 | Component gallery demo (hard-coded sample IDs, `sonnet-4.6`, `opus-4.7`) | `gallery/registry.tsx:637-640` | literals |

Not a model label: `TicketInfo` extends `HarnessLabel` (`generated/server.ts:1358`) but no web code reads
its `model`. Stats (`stats/StatsPage.tsx`) shows cost, not model names.

Tests and docs that assert raw IDs and will change:
- Unit: `ui/harness-label/HarnessLabel.test.tsx`, `epic/rows.test.ts:24-28`, `epic/EpicPage.test.tsx:209`,
  `agents/AgentsPage.sessionView.test.tsx:49-71`, `design/DesignPage.test.tsx:121`,
  `ticket/ActionCard.test.tsx:142-149`, `agents/NewSessionForm.test.tsx:98`.
- e2e: `e2e/pause.e2e.ts:202` ("Ran on claude-opus-4, expected claude-sonnet-4."), and the same text in
  `e2e/CATALOG.md:1703-1706`.
- Spec: `docs/specs/design-models.md:187` says "A reported ID is shown as is". This change supersedes
  that line.

## 5. Where effort and tier show next to the model

- `HarnessLabel.tsx` is the one place. Pill layout (`:87-104`): harness pill (dot + "Claude"/"Codex",
  `:89-94`), tier pill (`Worker`, `:95`), then one pill `model · effort` (`:96`). The tier pill's tooltip
  says "<Tier> tier" and sits on the model pill too (`:53`, `:96`).
- With no model yet it shows one pill: `<Tier> · CLI default · effort` (`:97`).
- Plain layout (`:56-85`): harness dot and name, then the words `Tier`, `shortModel`, `note`, joined by
  ` · `, each word kept whole when the line wraps.
- Tier comes from `SessionInfo.tier` through `TIERS` (`agents/SessionHarnessLabel.tsx:7`) or from
  `row.tier` (`epic/rows.ts:218`, labels `Light`/`Worker`/`Strongest`). Effort is `SessionInfo.effort`,
  or the rollout's newest-turn effort for Codex (`server/sessionfailure.go:130-132`).
- `switched by hand` follows the pills (`:99-103`) when `model_switched` is set.
- The reference screenshot of the Design header shows `Claude`, `Worker`, `claude-sonnet-5-5 · medium`: three pills.

## 6. Options and recommendation

| Option | For | Against |
|---|---|---|
| A. Harness-reported only | Right for any new model with no gx release; version known for aliases (`sonnet` is `Sonnet 5.5`) | gx reads nothing today; needs a new server probe (Claude probe is per dir, slow, hook-bound; Codex needs `model/list`) and a new field on session, ticket, tier and mismatch payloads. Breaks the epic's "web only" scope. Misses `[1m]`, unlisted dated IDs and non-ID strings. Name shapes differ (`Opus 5.5`, `GPT-6.1-Sol`, `GPT-6.1 Terra`). Needs a fallback anyway. Async: raw ID flashes first. |
| B. gx rule only | Pure function, instant, no server change, one place, easy table test, covers every ID gx shows | Aliases have no version. A new naming scheme needs a code change. |
| C. Rule now, harness name as an override later | Ships web-only today. The rule is the fallback that option A needs anyway, so no work is wasted. Keeps a single call site for the later override | Two sources to explain once the override lands |

Recommendation: **C**. Build one pure function in the web UI, `modelName(harness, id)`, with the rule
above, and make every place in section 4 go through it. Join it with the harness name in one helper, so
`HarnessLabel` and the mismatch text cannot drift. Do not special-case `claude-` strip in
`HarnessLabel.tsx:58` any more.

For the later harness-reported source, add a `model_name` string next to `model` on the server payloads
(session, ticket row, tier map, mismatch) and let the web prefer it over the rule. Fill it from
`models` in the Claude `initialize` reply (skip `value: "default"`, key by `resolvedModel` and by `value`)
and from Codex `model/list`. That needs a server change, so the parent session decides if it belongs in
this epic or in a follow-up. The rule must exist either way.
