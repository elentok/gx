## Quiz format (step 5)

Present the proposed breakdown as a numbered list. For each ticket, show:

- **Title**: short descriptive name
- **Blocked by**: which other tickets (if any) must complete first
- **What it delivers**: the end-to-end behaviour this ticket makes work
- **Test seams**: the seams from step 4
- **Estimated context window**: the token estimate behind step 3's split decision — always shown,
  not just when a ticket is borderline, so the user can judge granularity against the same number
  that drove the split

After the implement tickets, show the proposed verify parts (see "Verify tickets" in step 6): per
part its stories, fixture scenario and expected token budget, plus the collect ticket's budget, or
the single lone ticket when everything fits in one part.

Before the list, show the draft epic Summary (written as step 6 describes) so the user can correct
it with the rest of the breakdown.

Ask the user:

- Does the granularity feel right? (too coarse / too fine)
- Are the blocking edges correct — does each ticket only depend on tickets that genuinely gate it,
  and are same-file siblings and cross-cutting tickets chained rather than run in parallel?
- Are the declared seams the right ones to test against?
- Should any tickets be merged or split further?

Ask them as one round in the format [gx-grilling](../gx-grilling/SKILL.md) defines: a `❓ **Qn**`
line, a `➡️` recommendation and a `gx-question` block per question. Only a line that starts with
`❓ **Q` makes the web UI show the round and mark the session as waiting for you; a plain numbered
list goes unseen. Keep the Summary and the breakdown above the round, and make the round the last
thing in the turn: no tool call, and no text, after it.

Iterate until the user approves the breakdown.
