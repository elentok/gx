# The store watch is an optimization; the store poll is the guarantee

The ticket store is written by processes the server cannot observe directly: agents running
`gx tickets` verbs, the CLI, and hand edits. The server notices those writes in `keepFresh`
(`server/watch.go`), which rescans the store on two triggers:

- **Store watch.** Filesystem events from `NewStoreWatch`. On macOS it uses FSEvents. Elsewhere it
  uses fsnotify, which is not recursive and needs a watch per directory.
- **Store poll.** A periodic rescan. It runs every 2 s with no watch and every 15 s
  (`watchedPollInterval`) with a live watch.

The watch is not a replacement for the poll. Watchers can miss events, for example new directories
on the fsnotify fallback or after fd exhaustion. A watcher can also fail to start, and gx then logs
once and runs on the poll alone.

So the watch decides how fast the server notices a change, and the poll decides whether it notices
at all. The poll is what guarantees that the store view eventually matches the disk. Deleting it
because events exist would turn that guarantee into a staleness bug that only shows up on someone
else's filesystem.

This replaces an earlier design that put a scratch watch and a scratch poll in the TUI's Tickets and
Queue tabs. That design never shipped. The TUI now gets ticket state from the server.

## Consequences

- The 15 s interval is a correctness backstop, not a latency knob. Raising it is safe. Removing it
  is not.
- Each rescan reads and hashes every ticket file, so the backstop interval stays slow while a watch
  is live.
- Correctness tests must pass with the watch disabled (`DisableWatch`). A test that only passes when
  events fire tests the optimization, not the guarantee.
