# Tokens and spend come from a token ledger

ADR 0041 rejected a precomputed daily table. For tokens and spend only, we now keep one: the token
ledger, a per-day record of the tokens and dollars each session used, split by agent group (Queue,
Design, One-off, Other). This supersedes only the "no precomputed daily table" part of ADR 0041 for
tokens and spend. Stats still read stamps for throughput, lines of code and friction, and ADR 0041
stands for those.

The ledger feeds live and rebuilt days. Old days keep the stamp rules from ADR 0041. The ledger holds
dollars, so the limit burn view can relate spend to usage limits. The markdown is still the truth for
tickets (ADR 0026); the ledger is rebuilt from transcripts, so it is a cache and not a second source.

Chart totals still do not match the budget ledger. The budget ledger records live cost deltas for
limit checks; the token ledger records what sessions used per day. They count different things on
different days, and we accept the gap.
