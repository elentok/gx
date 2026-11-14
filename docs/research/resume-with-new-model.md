# Resuming a session with a new model and effort

Ticket: `gx:model-switch/01`. Probed on 2026-11-14 with Claude Code 2.1.319 and codex-cli 0.169.0, on real runs with cheap models and one-line prompts. No Settings or defaults were changed, and `/model` was never typed.

## Answer

Both harnesses keep the conversation and answer on the new model when resumed with new launch flags. Both transcripts record the new model, so `Chat.ReportedModelAndContext` and `Chat.ReportedCodex` report it from the next turn on. gx needs no special handling beyond relaunching with the new flags.

## Claude

Probe: `claude -p ... --output-format json`, then `claude -p --resume <id> --model <m> --effort low` three times (haiku, sonnet, opus).

- The session id stays the same across resumes. The transcript is one JSONL file.
- Context is kept. Asked "What was the secret word?" on sonnet and again on opus, both answered `PINEAPPLE` (first said on haiku).
- Newest assistant line `message.model` per turn: `claude-haiku-5-5`, `claude-sonnet-5-5`, `claude-opus-5-5`. The alias `sonnet` and `opus` appear as full resolved ids, which is what `ReportedModelAndContext` reads.
- The `opus` alias resolved to `claude-opus-5-5` at probe time. The alias follows whatever Claude Code 2.1.319 maps it to. This probe cannot show that no newer Opus exists, only that the alias gave this one.
- No refusal, no stderr warning. The transcript gets a new `attachment` line of type `model` ("You are powered by the model named Sonnet 5.5...") on each switch. So the model is told about the change.
- Prompt cache loss is real. Each switch re-created the cache: turn 1 (haiku) created 13,215 tokens, the sonnet turn read 9,403 and created 13,588, the opus turn read 9,403 and created 13,873. The reads are the shared prefix. The conversation part is paid again at the new model's price. This is a cost, not a context loss.
- Interactive `claude --resume <id> --model sonnet --effort medium` (run in tmux) behaves the same. The banner reads "Sonnet 5.5 with medium effort", there is no dialog, and the next reply (`MANGO`) came from `claude-sonnet-5-5` (cache read 23,706, created 18,151). Earlier turn was on `claude-haiku-5-5`.
- Effort is not a transcript field gx reads today. The effort flag was accepted (`--effort max` on haiku gave no error). Effort in the footer must come from `meta.Effort`.
- A bad model name fails fast: `[claude-code:unrecognized_model] {"model":"nope-9"}`. The session file is untouched, so the old model still works on a later resume.

## Codex

Probe: `codex exec -m gpt-5.6-luna -c model_reasoning_effort=low`, then `codex exec resume <id> -m gpt-5.6-terra -c model_reasoning_effort=medium`, then back to luna with `high`.

- The session id stays the same. One rollout file.
- Context is kept. The resumed turns answered `PINEAPPLE`.
- `turn_context` lines, in order: `gpt-5.6-luna/low`, `gpt-5.6-terra/medium`, `gpt-5.6-luna/high`. `ReadTurnModel` reads the newest one, so gx reports the new model and effort.
- Codex prints a warning on stderr and in the TUI warnings list: "This session was recorded with model `gpt-5.6-luna` but is resuming with `gpt-5.6-terra`. Consider switching back ... as it may affect Codex performance." It is a warning only, it does not block. It compares against the model of the last recorded turn.
- Each resume also writes a `thread_settings_applied` event with the new model.
- Token cost: the first turn used 7.4K tokens and a resumed turn 28.5K, because the history is sent again. Codex does not say whether the cache survives.
- Interactive `codex resume <id> -m gpt-5.6-terra -c model_reasoning_effort=xhigh` (in tmux, in a trusted folder) behaves the same. The footer reads "GPT-5.6-Terra xhigh", the reply was `MANGO`, and the rollout shows `gpt-5.6-terra/xhigh`. The warnings counter went from 4 to 5. There is no blocking dialog.
- `codex resume` accepts `-m` and `-c` directly (see `codex resume --help`). `harness/codex.go` already builds `resume <id>` plus the model and effort flags.
- An unknown model fails after the warning: "The 'gpt-nope-9' model is not supported when using Codex with a ChatGPT account." Exit is an error and the rollout keeps its old model.
- In an untrusted folder, interactive `codex resume` shows a trust dialog. It is unrelated to the model, and gx's session directories are already trusted. The probe avoided it by using the design worktree.

## Implications for the design

- The mechanism works as the map assumes: stop the idle session, relaunch with `launchArgs(meta)` after updating `meta.Model` and `meta.Effort`.
- Reported model lags until the first turn after the switch. `ReportedModelAndContext` and `ReportedCodex` read the newest assistant line or `turn_context`, and a resume writes none until the person sends a message. Until then they return the old model. The picker or footer should show the chosen model from meta right after the switch, not wait for the transcript. This is the main design consequence.
- Resume failure is detectable and safe. An invalid model makes the process exit with an error and leaves the conversation file unchanged. The correct fix for a failed switch is to keep the old `meta.Model` until the new launch has started a turn (or at least stays alive), and to relaunch with the old flags on failure. A workaround such as validating names with a regex would be wrong, since the harness is the authority on which models exist (and the model list ticket covers discovery).
- Cache loss and the Codex warning are expected. The switch UI can mention that the next message re-reads the history at the new model's price. Gx does not need to suppress the Codex warning.
- Claude aliases resolve at resume. To resolve "latest of the family", prefer passing the alias or discovered full id the person picked, then read the resolved id from the transcript. Do not assume an alias means a fixed model.

## Not confirmed by a real run

- That the `opus` alias always points to the newest Opus. Only that it gave `claude-opus-5-5` here.
- Whether Codex keeps its prompt cache across a model switch. Only the rise in tokens used is observed.
- Behavior through gx itself (its stop and relaunch path). Probes ran the harnesses directly.
