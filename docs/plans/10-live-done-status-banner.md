# Ticket 10 — Live + done status banner

- [x] Add a failing Queue view test for aggregate running status across multiple epics.
- [x] Implement the running banner with checked-ticket progress, elapsed time, and live context tokens.
- [ ] Add a failing Queue view test for the completed execution summary.
- [ ] Aggregate the landed per-ticket report metrics and implement the completed banner.
- [ ] Run targeted checks, package tests, the full suite, and commit the ticket.

Test seams:

- `QueueModel.View()` with checked queue items still running.
- `QueueModel.View()` after every checked queue item is done.

Planned source files:

- `ui/tickets/queue.go`
- `ui/tickets/queue_test.go`
