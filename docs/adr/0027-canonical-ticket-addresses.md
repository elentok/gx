# Tickets have one canonical address: `project:epic/06`

A ticket is named `project:epic/06` — project name, epic directory, ticket number — everywhere gx
stores or exchanges a name: commit trailers, the API, event logs, queue keys, iteration prompts.

**Short forms are input only.** `06` and `epic/06` are accepted on the command line and resolved to
the full address before anything is written. A stored short form would be ambiguous the moment a
second project exists.

## Why

- **Readable history.** Rejected an opaque id: it is unreadable in `git log`, and `gx-investigate`
  reads trailers to find a ticket's commits.
- **Derivable.** The address is the store path (`<store>/<project>/<epic>/issues/06-*.md`), so there
  is no registry to keep in sync with the files.

## Consequences

- Renaming an epic or project changes addresses. That is rare; a later `gx tickets mv` rewrites
  references.
- The `Ralph-Loop-Ticket` trailer parser keeps accepting the old `<featureBranch>/<id>` form so
  existing history still resolves.
- A ticket's file path is an implementation detail. Agents get an address in their prompt and use
  `gx tickets` verbs; they never need filesystem access to the store.
