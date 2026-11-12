#### Prefactor tickets

A prefactor or make-room ticket (move code out of a package, free N lines or files) states numbers
and candidates, so check them before drafting. Never write them from recall.

1. Measure current sizes with `sizeguard` or `wc`, not from memory or an older ticket.
2. Check with gopls or `rg` that every named candidate still exists where you say it does, and that
   the promised line or file count is reachable under the stated constraint. Count the lines that
   are free to move, not the lines in the package.
3. If a constraint (for example "do not export X") could make the goal unreachable, because the
   large files are all methods on or users of X, put the export decision in the ticket, or ask the
   person while authoring. Never leave it for the implementer to find out.
