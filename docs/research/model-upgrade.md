# Model upgrade warnings: design handoff

## Purpose

Continue the model change design in gx, including warnings when a session uses a model with a known successor. This document records the conversation and preliminary research as of 2026-11-13. No product implementation was requested or made.

The user explicitly requested this repository path for the handoff, overriding the handoff skill's default temporary-directory location.

## User need

The user discovered that substantial Codex work had run on GPT-5.6 Sol despite a newer Sol being available and the saved configuration selecting GPT-6.1 Sol. The earlier usage analysis is recorded in the temporary handoff at `/var/folders/sx/y536dmv55h3bv70g0zytysr00000gn/T/codex-usage-gpt-6-1-sol-handoff.md`. Refer to that artifact for usage findings rather than repeating them here.

The user suggested painting the model indicator in the prompt box footer red, with a tooltip explaining the warning and a link to Settings. They asked how gx could determine whether a model is old, and whether the harnesses expose that information.

## Verified harness sources

### Codex

The documented app-server `model/list` method returns model discovery information. Request `includeHidden: true` to include hidden entries, and follow pagination.

Each model can expose:

- `upgrade`: an optional recommended successor model ID for migration prompts.
- `upgradeInfo`: optional upgrade metadata for migration prompts.
- `hidden`: whether the entry is hidden from the default picker.
- `isDefault`: whether the model is the recommended default.

Source: [Codex app-server model discovery](https://learn.chatgpt.com/docs/app-server#list-models-modellist).

The local `~/.codex/models_cache.json` was inspected on 2026-11-13. It contained both `gpt-5.6-sol` and `gpt-6.1-sol`, but every model's `upgrade` value was null. Both Sol entries were picker-visible. This proves that the inspected cache would not identify the user's case through `upgrade` alone. A live app-server response was not queried, and the cache is an implementation detail rather than a preferred integration contract.

Before implementation, inspect the installed harness's protocol/schema and actual `model/list` response to confirm supported fields and the shape of `upgradeInfo`. Do not assume all documented metadata is populated for every account or harness version.

### Claude Code

The Agent SDK exposes `query.supportedModels()`, returning `ModelInfo[]`. Relevant documented fields include `value`, `displayName`, `description`, and optional `resolvedModel`, alongside capability fields.

`resolvedModel` identifies the concrete model behind an alias such as `sonnet`. The documentation says it requires Claude Code v2.1.197 or later. The documented `ModelInfo` type has no explicit successor or deprecation field.

Sources: [Query methods](https://code.claude.com/docs/en/agent-sdk/typescript#query-object) and [ModelInfo](https://code.claude.com/docs/en/agent-sdk/typescript#modelinfo).

These findings come from the official SDK documentation. The installed Claude harness's discovery response was not queried. Availability through gx's existing transport still needs investigation.

## Proposed approach, not approved design

- Determine the session's effective model from harness/session data. Comparing only the saved Settings value can miss a session override.
- Prefer an explicit harness-provided successor relationship.
- If metadata is absent, consider a small curated successor mapping. Do not derive a definitive warning solely by sorting model version strings.
- For Claude, comparing a pinned model with the same family's alias resolution could support a softer notice such as "Sonnet now defaults to X." This does not establish deprecation or retirement.
- An assistant suggestion was to use amber for a known successor and red for confirmed retirement or unavailability. The user suggested red; the final color and severity policy remain open.
- A possible tooltip is "This session uses GPT-5.6 Sol. GPT-6.1 Sol is available. Change the model in Settings." Provide a real Settings link and verify that changing Settings affects the current session before implying it does.
- Unknown models or stale metadata should not produce a definitive outdated-model warning. Absence from the picker also does not establish retirement.
- Do not claim a successor is cheaper or better without separate evidence. A newer model, a recommended default, and a retired model are different facts.

## Next design work

1. Locate gx's current model discovery, effective-session-model tracking, prompt footer, and model Settings controls.
2. Inspect actual Codex and Claude discovery responses through the transports gx uses. Establish how metadata refreshes and which harness versions support it.
3. Decide whether provider metadata alone is sufficient or whether gx will maintain fallback successor data. The local Codex cache observation shows the coverage gap to address.
4. Define how warnings behave for session overrides, aliases, custom providers, unavailable successors, missing metadata, and Settings changes that apply only to future sessions.
5. Incorporate warning severity, tooltip copy, and navigation into the broader model change design.

No gx source paths for these features were established during this conversation. Do not treat the proposed approach as an implementation plan or approved test seam.

## Suggested skills

The next agent should invoke relevant skills through the available skill mechanism:

- `openai-docs` to recheck Codex protocol and product behavior against current official sources.
- `codebase-design` when defining the model discovery and warning interfaces.
- `gx-domain-modeling` if the design introduces shared model lifecycle terminology or an ADR.
- `react` and `chakra` if inspecting or implementing the web model controls.
- `gx-gopls` for Go symbol navigation and `writing-go` if editing Go code.
- `gx-unslop` before writing design prose or user-facing copy.

The working tree contains unrelated existing changes. This task added only this handoff document.
