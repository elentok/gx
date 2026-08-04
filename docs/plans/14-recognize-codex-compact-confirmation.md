# Recognize Codex Compact Confirmation

Seam: `recoverSmartZoneBreach`, observed through its dependency boundary (`AgentPrompt`,
`AgentWait`, ticket status/events, and terminal input).

Files to touch:

- `ralphloop/waitforfinish_test.go`
- `ralphloop/waitforfinish.go`

Plan:

- [x] Add a focused failing test for compact confirmation transitioning from blocked to working to
      a valid final state without needs-attention or injected input.
- [x] Implement compact-scoped passive confirmation handling without changing generic blocked or
      quota recovery.
- [x] Run focused checks, then the full test suite.
- [x] Mark ticket done and commit the implementation.
