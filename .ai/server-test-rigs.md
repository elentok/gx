# Server test rigs: traps

Facts checked in code. Paths are under `server/servertest/`.

## Fixtures

- `WriteTicketWith` and `WriteTicketFile` (`store.go`) go through `WriteEpic`, which creates
  `project.json` when it is missing. A test for an unregistered project must remove the file after
  the write.
- `SetProjectRepo` and `SetProjectRepoNoAutoMerge` (`projects.go`): a One-off web test needs one of
  them. A cancelled One-off moves to `one-offs-archived` (`server/queue/oneoffsweep.go`).
- `StartWithStore` and `Harness.Restart` (`start.go`): use them to restart on the same store.

## Turn hooks

- `Harness.OnTurn` and `RegisterTurn` (`turn.go`) need a PTY runner or `WithHerdr` (`runner.go`).
  Without one, the test fails with "Harness.OnTurn needs PTY or WithHerdr".

## Rigs (`rigs/`)

- `WithSchedulesClock` (`schedules.go`) puts the scheduler on a fake clock with a short interval.
- `StartPromptSessionWith` starts the fixture's own chat. `StartPromptSessionAs` (`designs.go`)
  seeds a named chat from a `wire.CreateSessionRequest`. The prompt and rename helpers target the
  fixture's own session id.
- A Schedule test with Monday-only days has nothing due, so it waits out its timeout. Use
  every-day schedules.
- List the queue with `rigs.QueueAddresses` (`queue.go`, addresses in order) or `rigs.QueueRows`
  (`queueagents.go`, Queue rows of the session list).

## Seam text

A mutation check in a gx-to-tickets seam must fail for the stated reason, not for a harness error
or a timeout.
