# Hand-driven tickets: map epics and `add --body` frontmatter

Two bugs found while resolving agent-runner ticket 04 by hand.

1. **No CLI path to claim or close a hand-driven ticket.** `tickets set` refuses every orchestration
   status (`cmd/tickets_set.go:157`, ADR 0030), so a wayfinder map's decision tickets can't be
   claimed or closed. Related: a map epic is detected by `map.md` (`tickets/epic.go:11`), but
   `migrate --to-store` folds `map.md` into `ticket.md` and writes no marker, so migrated maps stop
   counting as maps (`gx tickets epics --maps`, `/myway` step 1).
2. **`tickets add --body` writes two frontmatter blocks.** `createTicket` (`cmd/tickets_add.go:85`)
   treats the whole body as text and stamps its own frontmatter (`type: implement`, no `blocked_by`)
   above it. gx-to-tickets' template puts frontmatter in the body, so every published ticket loses
   its `blocked_by` and `type`.

## Decisions

- An epic's `ticket.md` frontmatter gains `kind: map`. `IsMap` reads it; `map.md` stays as a fallback
  for unmigrated trees.
- The server never schedules tickets in a `kind: map` epic. One writer per epic: hand-driven or
  loop-driven, never both (already the rule in gx-to-tickets).
- `tickets set --status claimed|done` (with `--commitless`) is a direct write for tickets in a
  `kind: map` epic. Loop epics still refuse it. `needs-answer`/`needs-repair`/`cancelled` stay
  refused everywhere (no hand-driven need yet).
- `add --body` accepts a leading frontmatter block and merges it: settable fields (`blocked_by`,
  `type`, `expected_context_window`, `commitless`, `parent`) are taken; `id` is ignored (allocated
  id wins, warn on mismatch); `status` may only be `open` or `draft`; read-only or unknown fields
  fail with nothing written.

## Tasks

### `kind: map`

- [x] Add `kind` (enum, only `map` for now) to the epic `ticket.md` schema; `gx tickets schema` and
      `validate` know it
- [x] `IsMap` = `kind: map` in `ticket.md`, else `map.md` exists (`tickets/loader.go:45`); test both
- [x] `migrate --to-store` writes `kind: map` when it folds a `map.md` in (`epicTicketMD`); test
- [x] One-time backfill: add `kind: map` to `agent-runner`, `orchestrator-daemon`,
      `run-budget-limits`, `ticket-land-recovery` (hand edit + `validate`, no new command)
- [x] Server never schedules a map epic: `ralphloop.Frontier` is empty for it (covers the server and
      the old TUI loop), and `gx server queue add|replace` refuses its tickets with `map-epic`; tests
- [x] `tickets set --status claimed|done` writes direct when the ticket's epic is `kind: map`; still
      refused otherwise and on a `ralph-loop/*` branch; tests for both
- [x] ADR 0030 gets a short "hand-driven map epics" exception, next to the repair-verb one

### `add --body` frontmatter

- [x] `createTicket` splits a leading `---` block off the body, merges the settable fields, applies
      the `id`/`status`/unknown-field rules above
- [x] Test: pipe gx-to-tickets' ticket template through `add --body`; `blocked_by`, `type` and
      `expected_context_window` survive, one frontmatter block in the file
- [x] Test: unknown field and `status: done` refuse with no file written

### Docs and skills

- [x] `gx-local-tracker.md`: a map's body is the epic's `ticket.md` with `kind: map` (not
      `map.md`); hand-driven claim/close via `tickets set` on map epics; `add --body` may carry
      frontmatter
- [x] `myway/SKILL.md`: step 1 checks for a `kind: map` epic, not `map.md`; map edits go to
      `ticket.md`
- [x] `wayfinder/SKILL.md` "Chart the map": no change needed; it defers to the tracker doc's
      "Wayfinding operations", now added to `gx-local-tracker.md`
- [x] `gx-to-tickets/SKILL.md`: fix the template's mangled frontmatter (one field per line)
- [x] `CONTEXT.md`: define **Map epic** (hand-driven, never scheduled)
