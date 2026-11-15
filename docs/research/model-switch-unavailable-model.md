# A real model this account cannot use

Ticket: `gx:model-switch/10`. Probed on 2026-11-15 with Claude Code 2.1.319 and codex-cli 0.169.0, through the same holder path as `docs/research/model-switch-bad-model-through-holder.md` (ticket 08). No Settings or defaults were changed and `/model` was never typed.

## Answer

Both harnesses give the same code for a real-but-unusable model as for an unknown name: `model_not_found`. The spec's trigger (`model_not_found` only) is enough for the cases that could be tested. No new code is needed for them.

- Claude: retired real ids (`claude-3-haiku-20240307`, `claude-3-opus-20240229`, `claude-3-5-sonnet-20241022`, `claude-3-7-sonnet-20250219`, `claude-sonnet-4-20250514`) end in a transcript line with `error: "model_not_found"`, `apiErrorStatus: 404`. The pane shows no warning at launch (unlike `nope-9`). Idle is reached, the first message fails, the watch state stays Idle.
- Codex: API-only ids (`gpt-4.1`, `o3`, `gpt-5`, `codex-mini-latest`, `gpt-5.1-codex`) give "The '<id>' model is not supported when using Codex with a ChatGPT account." In the rollout `task_complete` that is `codex_error_info: "model_not_found"`. Watch state goes Blocked (`blocked=error`). This is the exact message ticket 08 saw for `gpt-nope-9`.
- Both harnesses define `model_not_found` as "does not exist OR not available to this account", so one code covers both reasons. By code alone, a model refusal is told apart from a transient error in both harnesses, because transient errors carry their own codes (below).
- Not triggered: a Claude model that needs a higher plan, and the `[1m]` entitlement error. This Max account has access to every Claude model tried, including `opus[1m]`, `sonnet[1m]`, `claude-opus-4-8[1m]`, `claude-fable-5-2` and `claude-opus-4-1-20250805`. Those rows rely on documentation only.
- One trap for the Codex list: a ChatGPT plan that lacks Codex use (`UsageNotIncluded`) writes `usage_limit_exceeded`, not `model_not_found`. That is an account-wide state. The old model fails the same way, so rolling back would not help. Do not roll back on it.

Recommended rollback trigger list:

| Harness | Roll back on | Never roll back on |
| --- | --- | --- |
| Claude (transcript `error`) | `model_not_found` | `authentication_failed`, `authentication_error`, `oauth_org_not_allowed`, `account_on_hold`, `billing_error`, `rate_limit`, `overloaded`, `invalid_request`, `server_error`, `max_output_tokens`, `cloud_credential_error`, `unknown` |
| Codex (rollout `codex_error_info`) | `model_not_found` | everything else, notably `usage_limit_exceeded`, `rate_limit_exceeded`, `server_overloaded`, `unauthorized`, `context_window_exceeded`, `bad_request`, `invalid_prompt`, `http_connection_failed`, `response_stream_*`, `other` |

Caveat on `invalid_request` (Claude) and `bad_request`/`other` (Codex): a plan-tier refusal of a model might use them on some account, but no source says so and none could be triggered. They are shared with context-too-long and malformed requests, so they are not safe triggers. If one shows up, the switch stays and the person sees the error.

## How it was run

Triage first, with one-line prompts:

- Claude: `claude -p --model <id> --output-format json "Reply OK"` over the discovery list plus retired ids. A failure shows `is_error: true`, `api_error_status`, `terminal_reason: api_error` and `total_cost_usd: 0`.
- Codex: `codex exec --skip-git-repo-check -m <id> -c model_reasoning_effort=low "Reply OK"` over API-only ids.

Then the holder probe of ticket 08, reused from its scratchpad (`probe/main.go` and the built `probe` and `gx` binaries there): old launch on a cheap valid model (`claude-haiku-5-5`, `gpt-5.6-luna`), stop through `HolderClient.Stop()`, relaunch on the unavailable model with `harness.Lookup(kind).Resume(...)` and the same extra args as `launchArgs`, read the holder watch stream and live pane, type one line, read again. The conversation had one earlier turn ("Remember the secret word PINEAPPLE. Reply only OK."). Codex ran in this worktree (untrusted-folder dialog, see ticket 08). The probe changed no files in the repo.

Watch states: `state=1` Working, `2` Idle, `3` Blocked.

## Claude: `claude-3-haiku-20240307` (retired, real id)

Triage (`claude -p`): the five retired ids above all returned `api_error_status: 404`, `terminal_reason: api_error`, cost 0 and the text "There's an issue with the selected model (<id>). It may not exist or you may not have access to it. Run --model to pick a different model." Available ids (`claude-opus-4-1-20250805`, `claude-opus-4-8[1m]`, `opus[1m]`, `sonnet[1m]`, `claude-opus-5`, `claude-opus-4-6`, `fable`, `claude-fable-5-2`) answered normally. `claude-fable-5-2` also printed `[claude-code:unrecognized_model]` on stderr yet answered.

Holder run (relaunch phase, times from holder start):

1. No exit. Watch: Starting, Starting, Idle (`ready=pid`) at 1.53 s. Same as ticket 08. No exit in 30 s.
2. Pane at idle, before any message: header `claude-3-haiku-20240307 · Claude Max`, the old turn, an empty input box. There is no `isn't recognized` warning line. That warning (option (c) of ticket 08) appears only for names Claude Code cannot recognize, not for a real retired id. The pane scrape is no signal here.
3. After typing "What was the secret word? One word.": Idle, Working (0.3 s after typing), Idle (about 0.9 s). Never Blocked. Pane:

   ```
   ⏺ There's an issue with the selected model (claude-3-haiku-20240307). It may not exist or you may not have access to it. Run /model to pick a different model.
   ✻ Churned for 0s · done 7:30 AM
   ```

4. Transcript line written by the first message:

   ```
   {"type":"assistant","error":"model_not_found","apiErrorStatus":404,"isApiErrorMessage":true,"message":{"model":"<synthetic>", ...}}
   ```

5. Signal gx already reads: `transcript.LastAPIError` (`transcript/api_error.go`) returns `Error`, `Status`, `Message` for the newest assistant line. `chat.transcriptFailure` uses it only for auth (`Status == 401 || Error == "authentication_error"`). Nothing reads `model_not_found` yet: that is the spec's new work.
6. Holder state: Idle with no error. gx must read the transcript, as in ticket 08.

## Codex: `gpt-4.1` (API-only, real id)

Triage (`codex exec`): `gpt-4.1`, `o3`, `gpt-5`, `codex-mini-latest` and `gpt-5.1-codex` all printed `warning: Model metadata for '<id>' not found` and then `ERROR: The '<id>' model is not supported when using Codex with a ChatGPT account.` None of them is in the 10-model `model/list` of `docs/research/model-discovery.md`, so discovery does not offer them.

Holder run:

1. No exit. Watch: Starting, Idle (0.94 s), Working blip (1.01 s), Idle `ready=osc7501` (1.52 s). No exit in 30 s.
2. Pane at idle: the old turn and footer `gpt-4.1 low · 6.58K used ...  ⚠ 6 warnings`. Nothing marks it bad. As in ticket 08, the unknown name shows raw in the footer.
3. After the first message: Idle, Working (about 0.3 s), then `state=3 blocked=error msg="The 'gpt-4.1' model is not supported when using Codex with a ChatGPT account."` about 1.2 s after typing, and stays Blocked. Pane: `■ The 'gpt-4.1' model is not supported ...` and `⚠ 7 warnings`.
4. Rollout (`~/.codex/sessions/2026/11/15/rollout-...-01a1ddae-97c2-72b1-8bf6-1108ebbc5336.jsonl`): `task_complete` with `error: {"message":"The 'gpt-4.1' model is not supported when using Codex with a ChatGPT account.","codex_error_info":"model_not_found"}`. The `turn_context` models read `gpt-5.6-luna` then `gpt-4.1`.
5. Signal gx already reads: `codexsession.LastTurnErrorIn` (`TurnError.Kind` = `model_not_found`) and the Blocked error status (`chat.StatusFailure`, label "Agent hit an error"). Nothing yet maps `Kind == "model_not_found"` to a rollback.

## Error codes each harness can write on a failed turn

### Claude Code

Transcript `error` values. The Agent SDK documents `SDKAssistantMessageError` as: `authentication_failed`, `oauth_org_not_allowed`, `account_on_hold`, `billing_error`, `rate_limit`, `overloaded`, `invalid_request`, `model_not_found`, `server_error`, `max_output_tokens`, `cloud_credential_error`, `unknown` (code.claude.com/docs/en/agent-sdk/typescript, `SDKAssistantMessageError`). The docs say `model_not_found` means "the selected model doesn't exist or isn't available to your account or deployment". The transcript line in the real run carries the same string.

| `error` | Class | Why |
| --- | --- | --- |
| `model_not_found` | safe to roll back | Model refused: not found or not available to the account. Seen for unknown names (ticket 08) and retired real ids (this run). |
| `rate_limit` | transient | A 429 against quota or a usage limit (gx's `RateLimited`). |
| `overloaded` | transient | HTTP 529, server at capacity. |
| `server_error` | transient | Internal API error or a dropped connection (repo test line: "Your computer went to sleep mid-response"). |
| `authentication_failed` | not model related | Login or key problem. gx also tests `authentication_error` and status 401 (`chat.transcriptFailure`; repo test lines use `authentication_error`), so both spellings exist in files. |
| `oauth_org_not_allowed`, `account_on_hold`, `billing_error`, `cloud_credential_error` | not model related | Account, org, billing or cloud-credential state. The old model fails the same way. |
| `invalid_request` | not safe | HTTP 400 `invalid_request_error`: malformed request, context too long, spend limit (platform.claude.com/docs/en/api/errors). Not a model refusal. |
| `max_output_tokens` | not model related | The reply hit the output cap. |
| `unknown` | not safe | Catch-all. The repo has a line with `error: "unknown"` and `apiErrorStatus: 429`. |

Other documented model-access texts, with no documented structured code (Not confirmed):

- "Claude Opus is not available with the Claude Pro plan" (plan does not include the model), "Model ... is restricted by your organization's settings", "Model ... is not a recognized model id" and "Model ... not found" are documented as `/model` or startup messages that keep the current model (code.claude.com/docs/en/errors, "Request errors"). A `--model` launch of a restricted model shows a startup notice and uses another model, so it would not fail on the first message.
- "Usage credits required for 1M context" is an entitlement check on `[1m]` models (same page, "Usage credits required for 1M context"). The docs do not give its `error` value. It would be a plan-based model refusal if it were `model_not_found`, but nothing confirms that.

The API's own error types, for reference (platform.claude.com/docs/en/api/errors): 400 `invalid_request_error`, 401 `authentication_error`, 402 `billing_error`, 403 `permission_error`, 404 `not_found_error`, 413 `request_too_large`, 429 `rate_limit_error`, 500 `api_error`, 504 `timeout_error`, 529 `overloaded_error`. A retired model id came back as HTTP 404 in this run (`apiErrorStatus: 404`), and Claude Code wrote `model_not_found`.

### Codex

Rollout `codex_error_info` names (snake_case), from `CodexErrorInfo` in `codex-rs/protocol/src/protocol.rs` (openai/codex, main at b016cd1) and checked against the installed `codex app-server generate-json-schema` (camelCase there, 0.169.0):

| `codex_error_info` | Class |
| --- | --- |
| `model_not_found` | safe to roll back |
| `usage_limit_exceeded` | not model related. Also written for `QuotaExceeded` and `UsageNotIncluded` ("To use Codex with your ChatGPT plan, upgrade to Plus"), so it covers plan-level lockout. Account-wide, the old model fails too. |
| `rate_limit_exceeded`, `server_overloaded`, `flex_unavailable`, `policy_capacity_unavailable`, `internal_server_error` | transient |
| `http_connection_failed{http_status_code}`, `response_stream_connection_failed{..}`, `response_stream_disconnected{..}`, `response_too_many_failed_attempts{..}` | transient (network or retry limit). `http_connection_failed` is also written for `WorkspaceAccessDenied` and `AccessProgramNotEnabled`, so a 403 here is not safe to read as a model refusal. |
| `unauthorized` | auth (refresh token failed) |
| `context_window_exceeded`, `session_budget_exceeded` | not model related |
| `bad_request`, `invalid_prompt`, `cyber_policy`, `bio_policy`, `misalignment_policy_violation`, `inference_hook_denied`, `too_many_denials`, `sandbox_error`, `active_turn_not_steerable`, `thread_rollback_failed`, `other` | not model related or not safe. `other` also catches an `InvalidRequest` 400 whose body is not one of the recognised codes. |

Mapping source: `to_codex_protocol_error` in `codex-rs/protocol/src/error.rs`. The server-side match is in `codex-rs/codex-api/src/error.rs`: the response body code `"model_not_found"` with HTTP status absent, 400, 403 or 404 becomes `ApiError::ModelNotFound`, which maps to `model_not_found`. The observed `gpt-4.1` message therefore came from a 400-class response carrying that code (the HTTP status itself was not captured; the rollout holds only the message and `codex_error_info`). A test in `codex-rs/core/tests/suite/compact.rs` has the same text for a retired model through `invalid_request_response`, so the server may send it with `invalid_request` status; the `model_not_found` result in the real run shows it was classified by code.

gx's `codexsession.errorKind` also reads object-form infos such as `{"http_connection_failed":{...}}` by their key, so the table's object variants arrive as the key name.

## Can a first-message error be told apart from a transient one by code alone?

- Claude: yes for `model_not_found` against `rate_limit`, `overloaded`, `server_error`, `authentication_failed`. Every transient case has its own `error` value. The one gray value is `unknown` (and `invalid_request`), which should not trigger a rollback.
- Codex: yes. `model_not_found` is exclusive, and the transient cases (`rate_limit_exceeded`, `server_overloaded`, `http_connection_failed`, `response_stream_*`, `unauthorized`, `usage_limit_exceeded`) each have their own value. Never match on the message text: the message is a prose string and the same words appear in the `compact.rs` test for a different path.
- One difference for the first-message rule: Claude writes the error line to the transcript and stays Idle, while Codex goes Blocked. The spec's existing readers already cover both.

## Not confirmed

- A plan-gated Claude model (for example Opus on a Pro plan) and the `[1m]` entitlement error. This Max account could not trigger either. Their `error` value is unknown.
- A Codex model that is listed by discovery but refused at the first call. All 10 discovery models are usable on this account. The refused ids tried were all outside the list.
- Transient errors were not triggered (no way to cheaply force a 429, 529 or network drop). Their classification rests on the docs and source above, not on a run.
- HTTP status of the Codex `model_not_found` response. The rollout does not store it.
- Spelling of the Claude auth `error` value: docs say `authentication_failed`, repo fixtures use `authentication_error`. Not run.
- Full gx server run (`restartConversation`, `watchSession`): the probe reads the holder and files only, as in ticket 08.
- Only one candidate per harness went through the holder (`claude-3-haiku-20240307`, `gpt-4.1`). The other ids were tried with `claude -p` and `codex exec` only. One run each, so there is no spread.
- A resumed Claude run with `--effort` or `agents.claude-args` was not tried.

## Sources

- Real runs: probe output and transcript line in `~/.claude/projects/-private-tmp-claude-501--Users-david-dev-gx-model-switch-design-72a3e083-807d-4e33-940a-b589eb461212-scratchpad-cwd-claude/a0b49530-b86a-485d-8206-648671dd7618.jsonl`, rollout `~/.codex/sessions/2026/11/15/rollout-2026-11-15T07-29-29-01a1ddae-97c2-72b1-8bf6-1108ebbc5336.jsonl`.
- Claude Code docs: https://code.claude.com/docs/en/agent-sdk/typescript (`SDKAssistantMessageError`), https://code.claude.com/docs/en/errors ("There's an issue with the selected model", "Usage credits required for 1M context", "Claude Opus is not available with the Claude Pro plan", "Model is restricted by your organization's settings", "Login expired").
- Claude API errors: https://platform.claude.com/docs/en/api/errors (the old docs.anthropic.com URL redirects here).
- Codex source: https://github.com/openai/codex `codex-rs/protocol/src/protocol.rs` (`CodexErrorInfo`), `codex-rs/protocol/src/error.rs` (`to_codex_protocol_error`, `UsageNotIncluded`), `codex-rs/codex-api/src/error.rs` (`model_not_found` match), `codex-rs/codex-api/src/api_bridge.rs`, `codex-rs/core/tests/suite/compact.rs`. Installed schema: `codex app-server generate-json-schema` for 0.169.0.
- gx code: `transcript/api_error.go` (`LastAPIError`), `server/sessions/chat/failure.go` (`transcriptFailure`, `StatusFailure`), `codexsession/turnerror.go` (`LastTurnErrorIn`, `errorKind`), `docs/specs/model-switch.md` (alive and rollback rules).
- `docs/research/model-switch-bad-model-through-holder.md`, `docs/research/model-discovery.md`, `docs/research/resume-with-new-model.md`.
