# Recover Codex session on reattach

- [x] Add a read-only Herdr agent-state seam that returns the live native session identity.
- [x] Verify the recovered Codex session against rollout ID and cwd metadata.
- [x] Reattach without relaunching or replaying the implementation prompt, retaining lifecycle attribution.
- [x] Cover working, blocked, already-finished, missing, and mismatched session states.
- [x] Run targeted checks and the full suite.
- [x] Update ticket status and commit.
