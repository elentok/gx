# Run a Codex context-recovery E2E

- [x] Add a production-run E2E combining real-Git iteration infra with ticket 26's fake Codex rollout, using production `codexsession` readers (not a hand-rolled JSONL fixture).
- [x] Assert Herdr/session identity match, exactly one Ctrl-C/compact/finish-up in order, and compact phases complete before finish-up with lowered post-compact occupancy.
- [x] Assert the landed commit carries all three Ralph-loop trailers, ticket frontmatter/lifecycle events are correct with compactions omitted, and worktree/branch/tab/ticket cleanup is complete with no needs-info/needs-attention.
- [x] Run the focused test red, make the scenario pass, then run the full test suite.
- [x] Mark ticket 27 done and commit the completed work.
