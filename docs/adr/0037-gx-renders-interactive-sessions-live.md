# gx renders interactive sessions live

The agent-runner spec ruled out live terminal attach (story 70). The Agents tab reverses that for
interactive sessions only: the TUI shows the session's PTY live and types into it. Loop sessions
keep read-only snapshots and the one-shot prompt, interrupt and answer verbs.

The TUI dials the session's `holder.sock` directly, not through `gx server`. The server is
loopback-only, so the TUI always runs on the holder's machine, and a hop through the server adds
latency and a streaming route for nothing. A new optional holder op, `attach`, upgrades the
connection to a long-lived two-way stream. The frozen core ops stay as they are, and a holder
without `attach` falls back to a snapshot.

Emulation stays in the holder, as in herdr. The holder sends one full frame on attach, then only
the rows its vt screen marked dirty, as styled ANSI strings. The TUI runs no emulator. Frames are
`[u32 length][u8 type][JSON payload]`, and message types are append-only.

## Considered Options

- **Raw PTY bytes into a vt in the TUI.** Rejected: bytes can't be dropped for a slow client,
  every TUI parses everything twice, and attach still needs a snapshot.
- **Through the server.** Rejected: an extra hop and route, with no gain on one machine.
- **gob, protobuf or per-cell binary frames.** Rejected: rows travel as one ANSI string each, so
  JSON stays small and stays readable with `jq`.

## Consequences

- The holder caps frames at one per 16ms, with extra early frames for about 32ms after a key. It
  keeps a one-frame "latest" slot per client, and when the slot is full it drops the frame and
  later sends a fresh full frame. No output means no frames and no timers.
- Keys travel as key events. The holder encodes them with the vt screen for the modes claude turned
  on, so the vt screen's output now goes to the PTY instead of being discarded.
- The PTY follows the size of the viewer that last focused or typed. With no viewers, it keeps its
  last size. Screen rules must work at widths other than 200.
- `attach` is not tied to interactive sessions, so a later live Watch of loop sessions needs no
  protocol change.
