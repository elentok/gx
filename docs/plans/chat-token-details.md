# Chat token details

- [x] Verify Codex accounting against upstream source and recorded fixtures.
- [x] Expose first and peak request input, cumulative categories, compactions, and scope.
- [x] Expand the context popover with supported details and clear labels.
- [x] Verify parsing, API integration, and browser rendering.

## Codex evidence

Checked the installed CLI version, 0.169.0, and upstream checkout
`69231e119bfca0b14453515e97d99688ce1c47c9` from 2026-11-13.

- [TokenUsage and TokenUsageInfo](https://github.com/openai/codex/blob/69231e119bfca0b14453515e97d99688ce1c47c9/codex-rs/protocol/src/protocol.rs):
  input includes cached input; output includes reasoning output. Cumulative counters must
  be read as snapshots, not summed across events. The context display uses the last usage's
  total, whereas gx's smart-zone measurement uses request input.
- [Session accounting](https://github.com/openai/codex/blob/69231e119bfca0b14453515e97d99688ce1c47c9/codex-rs/core/src/session/mod.rs):
  usage updates the current session state. Compaction spending can update totals without
  replacing the stored context reading. Recomputed context can be an estimate with zero input.
- [Thread accounting](https://github.com/openai/codex/blob/69231e119bfca0b14453515e97d99688ce1c47c9/codex-rs/core/src/state/session.rs):
  response records carry thread identity and thread totals. gx reads only its selected
  rollout and does not aggregate child rollouts. Label that scope as this thread.
- gx's `codexdoctor/testdata/rollouts/plain-turn.jsonl` records cumulative and latest
  input, cached input, cache writes, output, reasoning output, and model context capacity.

Do not infer a skills/tools/instructions token breakdown from aggregate usage. Reported
capacity is separate from gx's smart-zone ceiling. First request includes the user prompt
and may include inherited history. All readings are best-effort transcript accounting.

## Verification

- Type checking passed.
- Changed-file `gx check` passed formatting, type checking, and 1,852 related web tests.
- Browser checks covered Claude and Codex, stale compaction readings, desktop and phone
  widths, and scrolling the popover within the available viewport.
- Targeted Go tests passed for `codexsession`, `ralphloop/metrics`, and
  `server/sessions/chat` with the required process-count guard.
- Staged `gx check` passed formatting, type checking, and 1,852 related web tests.
  Its Go wire and guard checks remain unverified because the sandbox denied `ps`.
