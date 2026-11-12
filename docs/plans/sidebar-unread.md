# Sidebar unread (bold titles)

Show a bold title in the web UI sidebar when a row changed while nobody looked.

## Rules

- Chat and Design rows: bold when their session has Unread (any change). Done in the web UI.
- Queue rows (implementation epics, one-off cards): bold only when the card is `Needs you` or
  `Merged` and that state differs from the last one a person saw.
- Opening the epic marks it seen.
- Triage and Queue agent rows stay normal weight.

## Tasks

- [x] Split the Unread gate from the Actionable gate (`hasUnreadMark`, `isUnread` in `rules.ts`)
- [x] Chat and Design rows carry `unread`; titles render bold (`sidebarModel.ts`, `SidebarRows.tsx`)
- [x] Test for chat, Design and Queue in `sidebarModel.test.ts`
- [x] Find how "Merged" shows on a card: `State == Finished` and `Section == Finished` on an
      Implementation card (status words "Merged"); Needs you is `Section == NeedsYou`
- [x] Server: seen store, `address -> last seen state` ("needs-you" | "merged"), JSON file in the
      state dir, like `server/designs/drafts`
- [x] Server: `POST /v1/epics/mark-seen {address}` stores the card's current state
- [x] Server: `EpicListRow.unread` = state is needs-you or merged and differs from the seen state
- [x] First run: seed all current cards as seen, so installing does not bold everything
      (same idea as `readySeedMark` in `server/designs/designready.go`)
- [x] `webui/src/generated/server.ts`: edited by hand. `make web-types` also drops `Tip` and
      `QueueMode`, so the committed file is stale against tygo; not fixed here
- [x] Web: call mark-seen when the epic page opens; sidebar reads `row.unread`
- [x] Tests: server (state change raises, mark-seen clears, seed), web (bold, mark on open)
- [x] `GLOSSARY.md`: Unread covers epic cards in two states; drop "no Unread" wording in
      `server/wire/sessiontypes.go` only if it stops being true (it stays true for Queue agents)

## Open

- Does a card that goes Needs you, then Working, then Needs you again count as a new change?
  Plan: yes, because the seen state is cleared when it leaves both states.
