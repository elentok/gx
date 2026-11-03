# Slim the web e2e suite

The Playwright suite has about 310 flows and one full run takes about 35 minutes. Every flow starts
its own fixture server and Chromium. Goal: about 100 flows and a full run of 11 to 15 minutes, with
no coverage lost.

Triage tables (one row per flow, with the verdict and the vitest file) are in
`docs/plans/slim-e2e-suite/triage-1.md` to `triage-5.md`. They were written by reading the flows
and searching `webui/src`; **no test was run**, so every verdict is a proposal to check.

| Verdict | Rows | Meaning |
|---|---|---|
| KEEP-E2E | 101 | Needs a real server, browser layout, restart, auth or SSE |
| TO-VITEST | 104 | Render or component logic; a vitest test covers it |
| MERGE-INTO | 98 | Same scenario as another flow; assert it as one more step there |
| DELETE-DUPLICATE | 13 | Same path as another flow or test |

## Rules for every ticket

- **Replacement first.** Write or confirm the vitest test (or the merged e2e step) and see it pass.
  Only then delete the flow. Never delete an assertion without a passing replacement.
- **Never run the whole e2e suite.** No `make web-e2e`, no `gx check --full`, no
  `GX_TEST_E2E=1` run without a file filter. Run only the e2e files the ticket touched, with
  `gx brief -C webui playwright <file> <flow numbers>`, and only the vitest files it touched
  (`make web-test` scoped to the file, or `npx vitest run <file>` from `webui`). Finish with plain
  `gx check`, which checks only the changed files.
- **Check the Go side first** when a verdict says the rule is server logic (flow 8 empty epic,
  11b cancel, stats shaping, long-running marking, waiting label). If no Go test covers it, add one
  in the same ticket.
- **Do not edit `webui/e2e/CATALOG.md`.** Parallel tickets would conflict. The last ticket updates
  it from the tickets' results.
- **Keep the numbers.** Flows keep their plan numbers. A merged flow lists the numbers it now
  covers in its title or a comment.
- If a flow cannot move to vitest because jsdom behaves differently (Chakra menu focus timing,
  media-query font sizes, Escape over the question sheet, vim motions), keep it as e2e or merge it
  into a flow that already runs. Say why in the commit message.
- Each commit records, per flow: kept, moved (to which test), merged (into which flow) or deleted.

## Tasks

- [ ] 1. `slim-vi-editor`: `vi-editor.e2e.ts`, 45 flows down to about 7 (triage-1)
- [ ] 2. `slim-small-flows`: `checks`, `disk`, `dirty-trunk`, `health`, `modes`, `stats`, `titles`,
      `long-running`, `check-lock` (triage-1). Delete the "coarse pointer" flow.
- [ ] 3. `slim-design-flows`: `design`, `design-page`, `newdesign` (triage-2)
- [ ] 4. `slim-jump-phone-epics`: `jump`, `phone` (triage-2), `epics` (triage-3)
- [ ] 5. `slim-menu-flows`: `menu` (triage-3)
- [ ] 6. `slim-united-flows`: `united` (triage-3)
- [ ] 7. `slim-pause-codex`: `pause`, `codex` (triage-3)
- [ ] 8. `slim-restart-migration`: `migration`, `restart`, `restart-run` (triage-4)
- [ ] 9. `slim-settings-login`: `settings`, `login`, `reload`, `setup` (triage-4)
- [ ] 10. `slim-agents-permission-commands`: `agents-permission`, `agents-commands`,
      `agents-commands-send`, `agents-commands-output` (triage-5)
- [ ] 11. `slim-agents-session-composer`: `agents-session`, `agents-composer`, `agents-keys`
      (flow 156 becomes a Go server test) (triage-5)
- [ ] 12. `slim-agents-sidebar-misc`: `agents-sidebar`, `agents-create`, `agents-tiers`,
      `agents-switcher` (triage-5)
- [ ] 13. `slim-e2e-catalog`: update `CATALOG.md`, the flow counts and the budget notes; blocked by
      1 to 12. Run `go test ./playwrightguard` only.

Tickets 1 to 12 run in parallel (they edit different files). Ticket 55a4 is blocked by all of them:
it fixes the flows that still fail, and it should only fix flows that survive.
