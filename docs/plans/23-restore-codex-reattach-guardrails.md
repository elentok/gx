# Restore Codex guardrails after reattach

- [x] Confirm smart-zone observation, Codex quota recovery, and metrics/trailer
      attribution already flow through the shared `waitForFinish`/`landCherryPick`
      code paths for both fresh and reattached iterations (ticket 22 wired
      `reattachIteration` to call them with the recovered live session).
- [x] Add a Run()-level test proving a reattached iteration's wait still detects
      and recovers from a smart-zone breach (proactive observation survives restart).
- [x] Add Run()-level tests proving a reattached iteration's wait still recovers
      from a structured Codex quota exhaustion and from a pane-text-only quota hit.
- [x] Confirm reattachment never replays the launch prompt and performs
      worktree/tab/branch cleanup exactly once (already covered by ticket 22's
      tests; verified, not duplicated).
- [x] Run targeted checks and the full suite.
- [x] Update ticket status and commit.
