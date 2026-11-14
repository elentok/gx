# Model discovery: what gx can read from Codex and Claude

Ticket: `gx:model-switch/02`. Run on 2026-11-14 with codex-cli 0.169.0 and Claude Code 2.1.319. Builds on `docs/research/model-upgrade.md`, which covers the documented fields. Each fact is marked "real run" (I ran it) or "docs" (taken from the earlier research file or the installed protocol schema).

Probe scripts and raw output are in the scratchpad directory `/private/tmp/claude-501/-Users-david-dev-gx-model-switch-design/7fca6a90-ca7f-4f0d-8a4a-15e004d4492e/scratchpad/probe-discovery/` (`probe.py`, `out.json`, `cl.py`, `claude-init.json`, `schema/`). They are not in the repo.

## Codex

### How to query (real run)

- Transport: `codex app-server` over stdio, JSON lines. Same handshake as `ProbeCodexSkills` in `commandlist/codex.go`: `initialize` (id 1), `initialized` notification, then `model/list` with `{"includeHidden": true}`. Add `cursor` from `nextCursor` until it is null.
- The `initialize` reply needed no extra fields beyond `clientInfo`.
- Latency: process start to full reply in 0.29 s, one page, no pagination. The app-server answered from its local catalog (`~/.codex/models_cache.json`, fetched 2026-11-14T18:49Z, has `fetched_at`, `etag`, `client_version`). I did not observe a network call during the probe, so a refresh is the app-server's own business.
- Schema (docs, from `codex app-server generate-json-schema --out DIR`, files `v2/ModelListParams.json`, `v2/ModelListResponse.json`): params are `includeHidden`, `cursor`, `limit`, `modelProvider`. Response is `{data: Model[], nextCursor}`. `nextCursor` was null on the real run.
- `generate-ts` and `generate-json-schema` exist, so gx can regenerate the protocol shape for the installed version.

### Real response (10 models, `includeHidden: true`, in returned order)

| id | hidden | isDefault | default effort | supported efforts | upgrade / upgradeInfo |
|---|---|---|---|---|---|
| gpt-6.1-sol | no | yes | low | low, medium, high, xhigh, max, ultra | null / null |
| gpt-6-astra | no | no | low | low..ultra | null / null |
| gpt-6.1-terra | no | no | low | low..ultra | null / null |
| gpt-6-sol | no | no | medium | low..ultra | null / null |
| gpt-reserve | yes | no | medium | low, medium, high, xhigh, max | null / null |
| gpt-6-luna | no | no | medium | low, medium, high, xhigh, max | null / null |
| gpt-5.6-sol | no | no | low | low..ultra | null / null |
| gpt-5.6-terra | no | no | medium | low..ultra | null / null |
| gpt-5.6-luna | no | no | medium | low, medium, high, xhigh, max | null / null |
| codex-auto-review | yes | no | medium | low, medium, high, xhigh, max | null / null |

Other fields per model: `id` equals `model`, `displayName` (for example "GPT-6.1-Sol", but "GPT-6.1 Terra" with a space, so not uniform), `description`, `supportedReasoningEfforts[{reasoningEffort, description}]`, `inputModalities`, `serviceTiers`, `multiAgentVersion`, `availabilityNux.message`, `modelSpecialty` (null everywhere), `availableAccessPrograms`.

### Findings

- Both GPT-5.6 Sol and GPT-6.1 Sol are listed, both visible (real run).
- Nothing links them. `upgrade` and `upgradeInfo` are null on all 10 models, including the hidden ones (real run). `upgradeInfo` has `model`, `migrationMarkdown`, `modelLink`, `retirementAt`, `upgradeCopy` (docs, schema), but this account's catalog never fills it. Per `model-upgrade.md`, the on-disk cache showed the same null `upgrade`.
- No release date or version field exists in the response (real run, schema). `id` is the only place the version lives.
- Returned order is not recency. It matches the `priority` field of `models_cache.json` (1 to 10, real run on the cache file): 6.1-sol, 6-astra, 6.1-terra, 6-sol, reserve, 6-luna, 5.6-sol... `priority` is a display rank. It puts `gpt-6-astra` above `gpt-6.1-terra`, and the app-server does not send `priority`. Do not use order as "newer".
- `isDefault` marks one model (gpt-6.1-sol). It is the server's recommended default, not "latest of every family".
- Effort sets differ per model: Luna has no `ultra`, and so the picker must read each model's own list. A switch that keeps the old effort must check membership (real run).
- Naming: family is the last word of the id (`sol`, `terra`, `luna`, `astra`). Version is a number in the id: `5.6`, `6`, `6.1`. gpt-6-astra and gpt-6-sol have no minor part, so parse "6" as 6.0. This is a parse of names, not a protocol guarantee. Note `terra` exists as a family that gx's tiers do not use.

### Latest of a family?

- Not from the protocol data alone as a stated fact, because no field says "successor". gx can derive it by parsing `(family, version)` from each id and taking the highest version. On the real data this gives Sol: 6.1 > 6 > 5.6, Terra: 6.1 > 5.6, Luna: 6 > 5.6, Astra: 6 only. That is a heuristic over names.
- The correct fix is to treat an explicit `upgrade` / `upgradeInfo.model` as authoritative when present, and otherwise fall back to the parsed-version rule, limited to ids that match `gpt-<major>[.<minor>]-<family>` exactly. An unparseable id gets no warning (matches the map's "unknown model never shows red"). A gx-shipped successor table is the wrong default here: it goes stale the day a new model ships, while the catalog already lists new models.
- Smallest extra source, if the version rule is judged too loose: a small gx-shipped list of known family names (sol, terra, luna, astra), so a new unrelated suffix is not read as a family.

## Claude

### Without the Agent SDK (real run)

- `claude --help` has no models command and no list flag. The commands are agents, auth, doctor, mcp, plugin, and others. `--model <alias or full name>` and `--effort <level>` are launch flags.
- The stream-json control protocol works directly. I ran `claude -p --input-format stream-json --output-format stream-json --verbose` in `/private/tmp`, wrote one line `{"type":"control_request","request_id":"r1","request":{"subtype":"initialize"}}`, and read the `control_response`. No user message was sent, so no model call was made. Reply in 0.20 s.
- The reply has a `models` array. The SDK's `supportedModels()` (docs, `model-upgrade.md`) is the same data. gx needs the process, not the SDK. The reply also contains an `account` object and the full command list, which gx should ignore and not store.
- Doing this means spawning a short-lived `claude` child. It touches no settings files. I did not change `~/.claude`, and I did not type `/model`.

### Real response (`models`, 14 entries)

Fields: `value`, `resolvedModel`, `displayName`, `description`, `supportsEffort`, `supportedEffortLevels`, `supportsAdaptiveThinking`, `supportsFastMode`, `supportsAutoMode`, `supportsThinkingOff`. No successor, deprecation, date or hidden field.

| value | resolvedModel | efforts |
|---|---|---|
| default | claude-sonnet-5-5 | low, medium, high, xhigh, max |
| opus | claude-opus-5-5 | low..max (5 levels) |
| claude-fable-5-2 | claude-fable-5-2 | 5 levels |
| sonnet | claude-sonnet-5-5 | 5 levels |
| haiku | claude-haiku-5-5 | 5 levels |
| fable | claude-fable-5-1 | 5 levels |
| claude-haiku-4-5-20251001 | same | none listed (`supportsEffort` absent) |
| claude-sonnet-5 | claude-sonnet-5 | 5 levels |
| claude-opus-5 | claude-opus-5 | 5 levels |
| claude-fable-5 | claude-fable-5 | 5 levels |
| claude-opus-4-8 | same | 5 levels |
| claude-opus-4-7 | same | 5 levels |
| claude-opus-4-6 | same | low, medium, high, max (no xhigh) |
| claude-sonnet-4-6 | same | low, medium, high, max (no xhigh) |

- Aliases resolve: `opus` to `claude-opus-5-5`, `sonnet` to `claude-sonnet-5-5`, `haiku` to `claude-haiku-5-5` (real run). `default` is an extra alias (here Sonnet 5.5).
- The alias `fable` resolves to `claude-fable-5-1`, yet `claude-fable-5-2` is also listed. So an alias can lag the newest listed model. Resolving the alias is not enough to find the latest of a family (real run).
- Effort sets differ per model (4.6 models have no `xhigh`, Haiku 4.5 has no effort), so the picker needs the per-model list (real run).
- The list is the account's available models for the installed build. It is not the full Anthropic catalog, and it likely varies by account or plan (not tested on a second account).
- Minimum version: `resolvedModel` needs Claude Code 2.1.197 or later (docs, `model-upgrade.md`). The `initialize` control request itself worked on 2.1.319 (real run). I did not test older versions, so the minimum for `initialize` plus `models` is not confirmed. gx should treat a missing `models` array, or a missing `resolvedModel`, as "no data" and show no warning.

### Latest of a family?

- Closer than Codex, still not a stated fact. Family is the middle word of `resolvedModel` (`claude-<family>-<major>[-<minor>]`, with an optional date suffix). Version is the number run. Parsing the real list gives Opus 5.5 (alias), Sonnet 5.5, Haiku 5.5, Fable 5.2 (not the `fable` alias, 5.1).
- Correct approach: take the highest parsed version among listed `resolvedModel` values per family. Do not trust aliases alone (shown by `fable`). Ids with no parseable version, and `default`, are skipped. Dated ids like `claude-haiku-4-5-20251001` need the date stripped (`modelName.ts` already strips `-\d{8}`).
- Smallest extra source, if needed: none for Claude beyond the family-name list.

## Cost and change rate

- Codex `model/list`: one `codex app-server` process (about 0.3 s total, real run), local catalog, no observed network. The catalog behind it refreshes on the app-server's own schedule (cache has `fetched_at` and `etag`). It changes when OpenAI ships a model or the account changes, so hours to weeks. Reusing the skills probe's 15 s timeout and kill-on-reply pattern is enough.
- Claude `initialize`: one `claude -p` stream-json process (about 0.2 s, real run), no model call. Changes when Claude Code updates or the account changes.
- Both are cheap enough to run once at server start and refresh on demand (for example when the picker opens), with the result cached in memory. A stale list only means a missing newer model, so the warning shows later, never wrongly. Refresh frequency beyond that is a design choice for ticket 03.
- Not measured: cold-cache Codex start with a network refresh, and the behavior when the user is logged out.

## Summary

- Both harnesses give gx the model list and per-model effort lists by a real, cheap query. Neither gives a successor link (Codex `upgrade` is null for all 10 models, Claude has no such field).
- gx can compute the latest model per family from parsed ids and versions. Prefer the correct fix: trust `upgrade` / `upgradeInfo.model` when set, otherwise parse `(family, version)` from the live list. Do not hard-code a successor table, and do not use list order.
