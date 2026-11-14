# Better Triage 02: inventory of the triage-made area map epics

Research for `gx:better-triage/02`. Sources: the ticket store (`$(gx tickets root)`, read with `rg`) and the Go source in this worktree. Read on 2026-11-14.

## Summary

- 107 epics have "Triaged from follow-ups" in their `ticket.md` body. The count is confirmed. All 107 are `kind: map`, `status: open`.
- They hold 432 tickets: 311 open or claimed (309 open, 2 claimed), 117 done, 4 cancelled. 8 epics have no tickets.
- Ticket types: 354 grilling, 78 research. No `implement` tickets.
- Design progress: 43 epics have no `design_step`, 60 are at `chart`, 3 are at `resolve-tickets`, 1 is at `create-tickets`. Only those 4 have a confirmed Destination (`destination_unconfirmed: false`). The other 103 still say `destination_unconfirmed: true`.
- No epic has a live `<epic>-impl` epic. One `-design` branch exists (`queue-and-epics-design`). One stale `agent-launch-and-recovery-impl` branch exists, but that epic is archived.
- All 432 tickets carry `original_address` (a follow-up address). None has `blocked_by`, a `parent`, or run history. So only the status rule and the map-source refusal block a move today.
- The 107 epics fit 12 areas (table below). The largest is test health (17 epics, 56 open tickets).
- Past charting: 4 epics (`agent-workflow-skills`, `pause-and-recovery`, `pause-cleanup`, `queue-and-epics`). They cannot just be emptied.
- `gx tickets move` refuses a map-epic source with `move-map-source`. The fix belongs in the tickets package, not in the command.

## Per-epic table

Status counts: `open5` means 5 open tickets. "Destination" is `yes` only when `destination_unconfirmed` is not true. Types are grilling and research counts.

| Epic | Proposed area | Tickets | By status | Types | design_step | Destination confirmed | Design or impl branch |
|---|---|---|---|---|---|---|---|
| agent-guidance | agent-guidance-and-skills | 9 | done2 open7 | grilling9 | chart | no | - |
| agent-launch-and-recovery | agent-sessions-and-runners | 5 | open5 | grilling4 research1 | none | no | `agent-launch-and-recovery-impl` branch only; the -impl epic is archived |
| agent-lifecycle-followups | agent-sessions-and-runners | 0 | - | - | none | no | - |
| agent-rules-and-guards | agent-guidance-and-skills | 4 | open4 | grilling4 | none | no | - |
| agent-runner-cleanup | agent-sessions-and-runners | 3 | open3 | grilling3 | chart | no | - |
| agent-session-guards | agent-guidance-and-skills | 6 | done6 | grilling4 research2 | chart | no | - |
| agent-sessions | agent-sessions-and-runners | 7 | open7 | grilling5 research2 | chart | no | - |
| agent-skills | agent-guidance-and-skills | 6 | done5 open1 | grilling6 | chart | no | - |
| agent-verify-and-tooling | agent-guidance-and-skills | 5 | done5 | grilling5 | none | no | - |
| agent-workflow-and-retro | agent-guidance-and-skills | 3 | open3 | grilling2 research1 | none | no | - |
| agent-workflow-skills | agent-guidance-and-skills | 3 | claimed1 done2 | grilling3 | resolve-tickets | yes | - |
| brief-and-check-output | check-and-brief | 4 | done4 | grilling3 research1 | none | no | - |
| brief-tooling | check-and-brief | 8 | done3 open5 | grilling8 | chart | no | - |
| chat-cards | web-ui | 1 | open1 | grilling1 | none | no | - |
| chat-notifications | web-ui | 2 | open2 | grilling2 | chart | no | - |
| check-brief-tooling | check-and-brief | 0 | - | - | none | no | - |
| check-package | check-and-brief | 1 | open1 | grilling1 | chart | no | - |
| chord-dispatch-priority | tui | 1 | open1 | research1 | chart | no | - |
| claude-doctor-live | agent-sessions-and-runners | 2 | done2 | grilling1 research1 | chart | no | - |
| cli-polish | tickets-store-and-cli | 2 | open2 | grilling2 | none | no | - |
| code-review-inputs | agent-guidance-and-skills | 10 | open10 | grilling10 | chart | no | - |
| commitless-close | ralph-loop-and-recovery | 2 | open2 | grilling2 | chart | no | - |
| design-detection | design-flow | 3 | done1 open2 | grilling2 research1 | chart | no | - |
| design-live-verification | design-flow | 0 | - | - | none | no | - |
| design-records | design-flow | 4 | open4 | grilling4 | chart | no | - |
| design-session-claims | design-flow | 1 | open1 | grilling1 | none | no | - |
| design-sessions | design-flow | 8 | done4 open4 | grilling8 | chart | no | - |
| e2e-fixtures | test-health | 2 | open2 | grilling2 | chart | no | - |
| epic-management | epics-queue-and-merge | 3 | done3 | grilling3 | none | no | - |
| epic-merge-flow | epics-queue-and-merge | 10 | open10 | grilling9 research1 | chart | no | - |
| fix-agent-refusals | agent-sessions-and-runners | 2 | done1 open1 | grilling2 | chart | no | - |
| fixture-and-ui-check | test-health | 6 | open6 | grilling5 research1 | none | no | - |
| flaky-session-test | test-health | 1 | done1 | research1 | chart | no | - |
| fork-handling | ralph-loop-and-recovery | 2 | done1 open1 | grilling2 | chart | no | - |
| ghostty-terminal | agent-sessions-and-runners | 2 | done1 open1 | grilling1 research1 | chart | no | - |
| go-flaky-tests | test-health | 8 | open8 | research8 | none | no | - |
| gx-check | check-and-brief | 13 | open13 | grilling13 | chart | no | - |
| gx-check-and-brief | check-and-brief | 5 | open5 | grilling5 | none | no | - |
| gx-cli-commands | tickets-store-and-cli | 3 | open3 | grilling3 | none | no | - |
| gx-ui-check | test-health | 1 | open1 | research1 | chart | no | - |
| gxfixture-harness | test-health | 2 | open2 | grilling2 | none | no | - |
| gxfixture-load-flakes | test-health | 5 | done4 open1 | grilling1 research4 | none | no | - |
| implement-verify-skills | agent-guidance-and-skills | 10 | done5 open5 | grilling10 | chart | no | - |
| loop-health | ralph-loop-and-recovery | 3 | done1 open2 | grilling3 | chart | no | - |
| machine-and-build-tooling | check-and-brief | 4 | open4 | grilling4 | chart | no | - |
| migrations-structure | tickets-store-and-cli | 3 | open3 | grilling3 | none | no | - |
| old-words | tickets-store-and-cli | 2 | open2 | grilling2 | chart | no | - |
| one-off-flow | tickets-store-and-cli | 4 | open4 | grilling4 | chart | no | - |
| pause-and-recovery | ralph-loop-and-recovery | 14 | done10 open4 | grilling13 research1 | resolve-tickets | yes | - |
| pause-cleanup | ralph-loop-and-recovery | 6 | claimed1 done5 | grilling6 | resolve-tickets | yes | - |
| playwright-e2e | test-health | 0 | - | - | none | no | - |
| pr-flow-cleanup | epics-queue-and-merge | 5 | open5 | grilling5 | none | no | - |
| queue-and-epics | epics-queue-and-merge | 4 | done4 | grilling4 | create-tickets | yes | `queue-and-epics-design` branch |
| ralph-recovery | ralph-loop-and-recovery | 3 | open3 | grilling3 | chart | no | - |
| ralphloop-iteration | ralph-loop-and-recovery | 5 | open5 | grilling5 | chart | no | - |
| ralphloop-scheduling | ralph-loop-and-recovery | 4 | done2 open2 | grilling3 research1 | chart | no | - |
| rendered-status-placement | server-performance-and-structure | 1 | open1 | grilling1 | chart | no | - |
| review-and-verify-skills | agent-guidance-and-skills | 9 | open9 | grilling9 | chart | no | - |
| rtk-hook | agent-guidance-and-skills | 1 | open1 | grilling1 | chart | no | - |
| runner-internals | agent-sessions-and-runners | 2 | open2 | grilling2 | none | no | - |
| server-cpu-hotpaths | server-performance-and-structure | 0 | - | - | none | no | - |
| server-cpu-polling | server-performance-and-structure | 6 | done1 open5 | grilling6 | none | no | - |
| server-idle-cpu | server-performance-and-structure | 3 | done3 | grilling3 | chart | no | - |
| server-internals | server-performance-and-structure | 4 | open4 | grilling4 | chart | no | - |
| server-package-structure | server-performance-and-structure | 2 | done2 | grilling2 | none | no | - |
| server-park-recovery | ralph-loop-and-recovery | 4 | done1 open3 | grilling4 | chart | no | - |
| server-performance | server-performance-and-structure | 2 | open2 | grilling1 research1 | none | no | - |
| server-rescan | server-performance-and-structure | 2 | open2 | grilling2 | none | no | - |
| server-store-watch | server-performance-and-structure | 2 | done1 open1 | grilling1 research1 | chart | no | - |
| server-structure-cleanups | server-performance-and-structure | 0 | - | - | none | no | - |
| server-test-flakes | test-health | 10 | done10 | research10 | none | no | - |
| session-lookup | agent-sessions-and-runners | 1 | open1 | grilling1 | chart | no | - |
| skills-sync-install | agent-guidance-and-skills | 5 | done1 open4 | grilling5 | chart | no | - |
| store-index | server-performance-and-structure | 3 | open3 | grilling3 | chart | no | - |
| stuck-ticket-watchdog | ralph-loop-and-recovery | 2 | open2 | grilling2 | chart | no | - |
| system-health | server-performance-and-structure | 5 | open5 | grilling5 | chart | no | - |
| test-determinism | test-health | 1 | open1 | grilling1 | none | no | - |
| test-fixture-failures | test-health | 2 | open2 | research2 | none | no | - |
| test-flakes | test-health | 26 | cancelled4 open22 | grilling11 research15 | chart | no | - |
| test-logging | test-health | 1 | done1 | grilling1 | chart | no | - |
| test-suite-hygiene | test-health | 5 | done4 open1 | grilling5 | none | no | - |
| test-suite-performance | test-health | 1 | done1 | research1 | chart | no | - |
| ticket-authoring | ticket-authoring | 8 | open8 | grilling5 research3 | none | no | - |
| ticket-authoring-checks | ticket-authoring | 6 | done5 open1 | grilling6 | none | no | - |
| ticket-authoring-skills | ticket-authoring | 5 | open5 | grilling4 research1 | none | no | - |
| ticket-finish-and-run-log | ticket-authoring | 5 | done5 | grilling4 research1 | none | no | - |
| ticket-store | tickets-store-and-cli | 2 | open2 | grilling2 | chart | no | - |
| tickets-authoring | ticket-authoring | 10 | done6 open4 | grilling10 | chart | no | - |
| tickets-cli | tickets-store-and-cli | 5 | open5 | grilling5 | chart | no | - |
| tickets-store | tickets-store-and-cli | 3 | open3 | grilling3 | chart | no | - |
| tickets-tab-ux | web-ui | 2 | open2 | grilling2 | chart | no | - |
| tickets-tooling | tickets-store-and-cli | 4 | open4 | grilling4 | chart | no | - |
| tui-and-config | tui | 3 | open3 | grilling3 | chart | no | - |
| tui-menus-hover | tui | 4 | open4 | grilling4 | chart | no | - |
| tui-visual-redesign | tui | 1 | open1 | research1 | chart | no | - |
| usage-limits | agent-sessions-and-runners | 2 | open2 | grilling1 research1 | none | no | - |
| verify-epic | agent-guidance-and-skills | 2 | open2 | grilling1 research1 | chart | no | - |
| verify-estimates | agent-guidance-and-skills | 0 | - | - | none | no | - |
| web-structure | server-performance-and-structure | 2 | open2 | grilling2 | none | no | - |
| webui-dialogs | web-ui | 4 | open4 | grilling4 | chart | no | - |
| webui-flakes | test-health | 3 | open3 | research3 | none | no | - |
| webui-gaps | web-ui | 5 | done2 open3 | grilling4 research1 | chart | no | - |
| webui-pages | web-ui | 11 | done2 open9 | grilling11 | chart | no | - |
| webui-sessions | web-ui | 3 | open3 | grilling3 | chart | no | - |
| webui-shell-and-design-ui | web-ui | 3 | open3 | grilling3 | none | no | - |
| webui-test-flakes | test-health | 7 | open7 | research7 | none | no | - |
| working-row-tool | tickets-store-and-cli | 0 | - | - | none | no | - |

The 8 empty epics: `agent-lifecycle-followups`, `check-brief-tooling`, `design-live-verification`, `playwright-e2e`, `server-cpu-hotpaths`, `server-structure-cleanups`, `verify-estimates`, `working-row-tool`. They need no move, only closing.

Source follow-up addresses: every ticket has its follow-up address in `original_address` (432 of 432). Each epic's Notes line also lists them ("Triaged from follow-ups on <date>: gx:follow-ups/NN"). Use `original_address` as the source of truth, not the Notes prose.

## Proposed areas

12 areas, each absorbing whole epics by topic. The count of 12 is the top of the 6 to 12 range. It can drop to 10 if `design-flow` and `tui` fold into `web-ui` and `server-performance-and-structure`, but those mix unrelated work, so 12 is the cleaner cut.

| Area | Epics | Tickets | Open or claimed |
|---|---|---|---|
| agent-guidance-and-skills | 14 | 73 | 47 |
| agent-sessions-and-runners | 10 | 26 | 22 |
| check-and-brief | 7 | 35 | 28 |
| design-flow | 5 | 16 | 11 |
| epics-queue-and-merge | 4 | 22 | 15 |
| ralph-loop-and-recovery | 10 | 45 | 25 |
| server-performance-and-structure | 13 | 32 | 25 |
| test-health | 17 | 81 | 56 |
| ticket-authoring | 5 | 34 | 18 |
| tickets-store-and-cli | 10 | 28 | 28 |
| tui | 4 | 9 | 9 |
| web-ui | 8 | 31 | 27 |

Epics per area:

- **test-health** (17): test-flakes, go-flaky-tests, server-test-flakes, webui-test-flakes, webui-flakes, gxfixture-load-flakes, flaky-session-test, test-fixture-failures, test-determinism, test-suite-hygiene, test-suite-performance, test-logging, e2e-fixtures, gxfixture-harness, fixture-and-ui-check, gx-ui-check, playwright-e2e
- **check-and-brief** (7): gx-check, gx-check-and-brief, brief-tooling, brief-and-check-output, check-brief-tooling, check-package, machine-and-build-tooling
- **agent-guidance-and-skills** (14): agent-guidance, agent-rules-and-guards, agent-skills, agent-workflow-and-retro, agent-workflow-skills, agent-verify-and-tooling, agent-session-guards, rtk-hook, skills-sync-install, implement-verify-skills, review-and-verify-skills, code-review-inputs, verify-epic, verify-estimates
- **ticket-authoring** (5): ticket-authoring, ticket-authoring-checks, ticket-authoring-skills, tickets-authoring, ticket-finish-and-run-log
- **tickets-store-and-cli** (10): tickets-cli, tickets-store, ticket-store, tickets-tooling, cli-polish, gx-cli-commands, migrations-structure, old-words, one-off-flow, working-row-tool
- **agent-sessions-and-runners** (10): agent-launch-and-recovery, agent-runner-cleanup, agent-sessions, runner-internals, session-lookup, ghostty-terminal, usage-limits, fix-agent-refusals, claude-doctor-live, agent-lifecycle-followups
- **ralph-loop-and-recovery** (10): ralph-recovery, ralphloop-iteration, ralphloop-scheduling, loop-health, pause-and-recovery, pause-cleanup, server-park-recovery, stuck-ticket-watchdog, commitless-close, fork-handling
- **epics-queue-and-merge** (4): queue-and-epics, epic-merge-flow, epic-management, pr-flow-cleanup
- **server-performance-and-structure** (13): server-cpu-hotpaths, server-cpu-polling, server-idle-cpu, server-performance, server-rescan, server-store-watch, store-index, server-internals, server-package-structure, server-structure-cleanups, system-health, web-structure, rendered-status-placement
- **design-flow** (5): design-detection, design-live-verification, design-records, design-session-claims, design-sessions
- **web-ui** (8): webui-dialogs, webui-gaps, webui-pages, webui-sessions, webui-shell-and-design-ui, tickets-tab-ux, chat-cards, chat-notifications
- **tui** (4): tui-and-config, tui-menus-hover, tui-visual-redesign, chord-dispatch-priority

Clusters worth noting:

- Test health. The five flake epics named in the map (`test-flakes`, `go-flaky-tests`, `server-test-flakes`, `webui-test-flakes`, `webui-flakes`) overlap, and `gxfixture-load-flakes`, `flaky-session-test` and `test-fixture-failures` hold more of the same. The fixture and Playwright epics join them. `test-flakes` alone holds 26 tickets, and 4 of those are cancelled.
- Check and brief. `gx-check`, `gx-check-and-brief`, `brief-tooling` and `brief-and-check-output` describe the same tools in four names.
- Agent guidance and skills. Verify, code-review and skill epics (`implement-verify-skills`, `review-and-verify-skills`, `code-review-inputs`, `verify-epic`) are one topic: what the agent skills tell agents.
- Ticket authoring. Three `ticket-authoring*` epics and `tickets-authoring` are the same area. `tickets-cli`, `tickets-store`, `ticket-store` and `tickets-tooling` are a second, tooling-side group.
- Server performance. Five CPU and rescan epics (`server-cpu-polling`, `server-idle-cpu`, `server-rescan`, `server-performance`, `server-store-watch`) plus `store-index` are one performance area.
- The split between areas is by topic from epic names and open ticket titles. A few epics could go either way (`commitless-close`, `fork-handling`, `rtk-hook`, `chat-*`). Ticket 03 (what an area is) can adjust the list.

## Epics past charting

Four epics have a Design started and a confirmed Destination:

| Epic | design_step | Tickets | Open or claimed | Notes |
|---|---|---|---|---|
| `agent-workflow-skills` | resolve-tickets | 3 (2 done, 1 claimed) | 1 claimed | Decisions list a dropped item and shipped scope. |
| `pause-and-recovery` | resolve-tickets | 14 (10 done, 4 open) | 4 open | Destination lists ticket numbers 02, 09, 10, 14. Other epics were merged in on 2026-10-27. |
| `pause-cleanup` | resolve-tickets | 6 (5 done, 1 claimed) | 1 claimed | Decisions link done tickets 01 and 05. |
| `queue-and-epics` | create-tickets | 4 (4 done) | 0 | Has a `queue-and-epics-design` branch. Nothing is left to move. |

- Moving their open tickets out would leave a Decisions section that links tickets in an epic that no longer holds them, and a Destination that lists items it no longer owns. Leave these four in place and let their own Design finish. They are not area candidates.
- The two `claimed` tickets cannot move anyway: `move` accepts only draft or open tickets.
- The 43 `none` and 60 `chart` epics are safe to empty. They hold only open, unconfirmed tickets. Check any `Decisions so far` text first. Only the four above have real decisions.

## What `gx tickets move` refuses

Entry: `cmd/tickets/tickets_move.go`. The core is `tickets/ticketmove/move.go`.

- Map-epic source: `cmd/tickets/tickets_move.go:53-55` calls `tickets.IsMapEpic(srcEpicPath)` and returns `MoveRefusal(tickets.CodeMoveMapSource, "epic %q is a map; its tickets cannot be moved out", ...)`. The code `move-map-source` is at `tickets/address.go:32`. `IsMapEpic` reads the epic's `ticket.md` for `kind: map` (`tickets/epic.go:49-55`).
- The refusal runs before the epic locks and before `PlanMove`, so nothing in `ticketmove` knows about maps. It is a command-level guard.
- Why: `docs/specs/gx-evolve.md:356-363` lists a map-epic source as a refusal and allows a map target ("A map-epic **source** still refuses"). The spec gives no reason. It was added with the core in commit `bb5e7c00a`. A test pins it: `cmd/tickets/tickets_move_test.go:169-178`.
- Other refusals that still apply to a map ticket (`tickets/ticketmove/move.go`):
  - status other than draft or open: `:96-98` (`move-status`, code at `:97`), so claimed, done and cancelled tickets stay;
  - run history (`session_ids`, `resolved_base`, `iteration_status`): `:99-101`;
  - fork parent or fork children: `:120-127`;
  - queued or live on the server: `cmd/tickets/tickets_move.go:63-65` via `refuseIfActive`;
  - same epic, other project, missing target: `cmd/tickets/tickets_move.go:41-52`;
  - a rewritten project that fails reference validation: `tickets/ticketmove/move.go:339-341`.

## What a move out of a map epic would touch

- `blocked_by` edges. `PlanMove` renumbers the ticket in the target (`NextTicketID`, `:129`). It rewrites the ticket's own `blocked_by` and `base` (`:148-160`). `planInbound` (`:239-343`) rewrites every ref to it in every ticket and every epic `ticket.md` of the project, then validates the result in memory with `ValidateRefs` (`:329-342`). Siblings moved to the same target become bare ids, others become `epic/NN` refs, matched by sibling key (`respeller`, `:209-231`). Today none of the 432 tickets has `blocked_by`, and no other epic's `blocked_by` points at them, so these edges cost nothing for this one-time move. Map tickets in general may have them, and the logic already handles them.
- `original_address`. `PlanMove` does not touch it: the ticket is parsed and written back with the field intact (`tickets/schema/ticket.go:357-360`, comment "OriginalAddress survives a move to another epic"). So the follow-up address stays on the moved ticket. The old map address is not recorded. `MoveOpts.RecordOriginal` and `AppendOriginal` (`move.go:70-75`) only write a `## Resolution` line `- original: <address>` (`tickets/original.go:19`), and only the one-off sweep and `tickets close` use them. For a map move the follow-up address is the useful one, so no change is needed, unless the person wants the old map address kept too.
- Prose is never rewritten. `proseMentions` (`move.go:364-402`) only warns about files that still spell the old address. Each map epic's Notes ("gx:follow-ups/NN -> gx:<epic>/NN") and its Decisions links will go stale. `docs/specs/gx-evolve.md:363` says `move` never touches a map's Decisions or Notes. For the 103 epics with no decisions this only affects the Notes line.
- Map-only state: `design_step`, `destination_unconfirmed` and `design_started_at` live in the source `ticket.md` and are left alone. An emptied map epic still says it is open and in `chart`. Something must close or archive it (a separate epic close step) and update the map's own record. That is for ticket 07.

## The clean fix

Lift the refusal from the command into the tickets package as a rule about what a map epic's tickets may do:

- Move the map check out of `cmd/tickets/tickets_move.go:53` into `ticketmove.PlanMove`, where the other source rules live. Allow a map source when the ticket is open or draft (the existing status rule already keeps claimed and done tickets in place). Keep refusing a map source once the Design has left charting: check `design_step` and `destination_unconfirmed` through `tickets.LoadEpicTicketMD` and refuse for a map past `chart` (code `move-map-source` stays, with the reason in the message). That protects the four epics above without a workaround flag.
- Keep `MoveRefusal` and the stable code. Change the message to say why (the Design has started), and update `cmd/tickets/tickets_move_test.go:169-178` to test both the allowed case (open ticket, charting map) and the refused case (map past charting).
- Update `docs/specs/gx-evolve.md:356-363`, which states the old rule.
- Do not add a `--force` flag or a one-off script that edits files. The rewrite of `blocked_by` and the validation in `planInbound` are what make a move safe, and a script would skip them.
- Open question for ticket 07: whether the move should also close the emptied map epic. That is a separate step, since `move` works one ticket at a time.
