# Codex fixtures

Recorded with `codex-cli 0.166.0` by a hand-run script on 2026-10-30.
Streams are raw PTY output frames (`t` seconds, `b64` bytes). Rollouts are Codex's own files.

| Scenario | Frames | OSC 7501 states | Rollout |
|---|---|---|---|
| plain-turn | 210 | state=clear, state=done:app=codex, state=idle:app=codex, state=working:app=codex | plain-turn.jsonl |
| compaction | 414 | state=clear, state=done:app=codex, state=idle:app=codex, state=working:app=codex | compaction.jsonl |
| approval | 329 | state=clear, state=done:app=codex, state=idle:app=codex, state=working:app=codex | approval.jsonl |
| background | 516 | state=clear, state=done:app=codex, state=idle:app=codex, state=working:app=codex | background.jsonl |
| error-bad-model | 156 | state=clear, state=error:app=codex, state=idle:app=codex, state=working:app=codex | error-bad-model.jsonl |

`rollouts/rate-limits-sample.jsonl` is one real `token_count` event with `rate_limits`.
The 100% usage-limit case is not capturable on demand: edit `used_percent` to 100 by hand.
Launch shape: -c check_for_update_on_startup=false --sandbox workspace-write --ask-for-approval on-request

## Gaps (synthetic in tests)

- No real `state=blocked` frame. With the launch shape's automatic reviewer, an out-of-workspace
  `touch` ran with no dialog (`streams/approval.jsonl` shows idle/working only). `fakecodex` builds
  `blocked` frames from the documented OSC 7501 format; the story 80 smoke checklist checks a real one.
- No real Background flag (`working` with title `Ready` while a background command runs). The stream
  shows the title `Run background sleep command | gx-codex-rec` during the run. `fakecodex` builds the
  flag; the story 80 smoke checklist checks it.
- No real usage-limit error: edit `used_percent` to 100 in `rollouts/rate-limits-sample.jsonl`.
