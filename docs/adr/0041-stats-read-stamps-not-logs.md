# Stats read stamped frontmatter, never logs

The web UI's Stats page computes every chart from values gx stamps in ticket and epic frontmatter
when work finishes: `done_at`, `cancelled_at`, `merged_at`, `total_tokens`, `actual_cost`, lines of
code, and a friction summary. It never reads run logs, ticket logs or transcripts. Each value counts
on the day it was stamped. We rejected reading run logs and transcripts on each request: transcripts
age out after 30 days, run logs grow without bound, and the page must load in under 1 second on a
store ten times today's size. We also rejected a precomputed daily table, because the markdown is the
truth (ADR 0026) and a second store would need its own sync.

The costs are accepted. Friction shows on the day a ticket finishes, not the day it happened. A
ticket that is still paused shows nothing until it finishes. Run events that name no ticket are not
counted. Spend on cancelled and started-over tickets never shows, so chart totals do not match the
budget ledger. Charts sum ticket stamps only; epic stamps feed the epic page, so no work counts twice.
This is hard to undo: every finishing path now stamps, and `gx tickets migrate` backfills the past
from the logs once.
