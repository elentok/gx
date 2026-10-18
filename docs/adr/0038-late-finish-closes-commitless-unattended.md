# A late finish closes commitless tickets unattended

ADR 0034 makes marking a ticket commitless-done a `high`-authority recovery remedy that never
auto-applies. Late-finish adoption still closes a `zero-commit`-parked ticket `done` with no person
when its agent reports `finished` + `commitless` after the park. This is not a remedy: gx is not
deciding the ticket is done, the agent reported it, exactly as an on-time commitless finish does
(ADR 0019: an agent's report can start a landing or a close, gx's own checks conclude it). Tight
conditions keep it from acting on anything a person or an old report put there: status and park kind
unchanged since the park, `finished` set after the park, and only `zero-commit` or `merge-unreported`
parks. Adoption is therefore part of the finish path, not a recovery-catalog entry, and
`commitless-done` stays the manual fallback.
