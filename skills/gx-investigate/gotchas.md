# gx-investigate gotchas

Running list of previously-diagnosed gx/ralph-loop bugs, newest first. Append one line + a pointer
to the fixing commit or ticket whenever a bug diagnosed via [gx-investigate](SKILL.md) gets fixed
— don't re-explain what the linked commit/ticket already documents.

- **Code-review-spawned tickets show up in the queue tree but never start.** `gx-code-review` set
  `children` on the review ticket but never `parent` on the tickets it spawned; both
  `RunScope.Contains`/`containsChain` (scheduler scope) and the Queue tab's tree nesting walk the
  *child's own* `parent` field, so the new tickets stayed silently out-of-scope until directly
  requested. Fixed in `35c0d2e` (`ralphloop: log scheduler scans; fix review-ticket scoping`),
  `skills/gx-code-review/SKILL.md` step 7.
