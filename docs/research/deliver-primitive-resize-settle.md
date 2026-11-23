# Resize settle time for an undecided slash read

Ticket: `gx:deliver-primitive/03`. Question: how long after an `untrusted-read` notice does the screen
read become trusted again? Researched 2026-11-23.

## Answer

The logs cannot give this number. There is no start-to-end gap to measure:

- Count of measurable gaps: 0.
- Median, p95, max: none.
- The logs hold one `untrusted-read` event in total, and it was not a resize.
- No log line marks the read becoming trusted again.

The code has no timed "resize settle" either. A resize read turns trusted on the child's next output,
not after a delay. The wait for `Deliver` must therefore be a poll with a cap, and the cap is a
decision, not a measurement. See "Recommendation".

## How the notice is raised and cleared (code)

- Raised in `ClearDraft`, only when the screen read is `Resized` or the input state is
  `InputUndecided`: `server/sessions/prompt/typing.go:43-47`. It logs `typing on an untrusted screen
  read` (with `resized`, `pty_size`, `attach_sizes`) and calls `RaiseNotice`.
- `RaiseNotice` appends the kind to the session's `Notices` in meta and logs nothing:
  `server/sessions/prompt/notices.go:12-20`.
- Cleared by `ClearNotices` (`notices.go:24-31`), which logs nothing either. It runs from `ClearDraft`
  on a trusted read with the input not holding a draft (`typing.go:49-53`) and after a successful draft
  clear (`typing.go:67-70`). So the notice drops at the next send's `ClearDraft`, not when the screen
  settles. Its lifetime is "time to the next send", which says nothing about resize time.
- The `Resized` flag: a PTY resize sets `resizedSinceOutput` (`nativerunner/holder/holderattach.go:390`)
  and the child's next output clears it (`holderattach.go:120-126`). The field doc says the screen is
  "what the child drew for the old size" until then (`holderattach.go:61-64`).
- `inputRead` returns `InputUndecided` for a resized read, and for an unsupported or failed read it
  returns an empty `ReadState` (zero `PTY`): `typing.go:97-106`. A holder without `OpScreenStyled`
  returns `ReadState{}` (`typing.go:98-100`).
- No code logs a resize. `rg -n -i "resiz|SetSize|winch"` over `nativerunner/holder/*.go` finds no log
  call, and `holder.log` of the sessions holds no `resiz` line (`rg -il resiz ~/.local/share/gx/sessions/*/holder.log`
  returns nothing).

## What the logs hold

State dir: `/Users/david/.local/state/gx` (`gx server logs` prints `server.log`; rotated copies are
`server.log.1` and `server.log.2`). Sessions: `/Users/david/.local/share/gx/sessions/<id>/`.

- Data span: `server.log.2` starts 2026-11-05 22:04:54; `server.log` ends 2026-11-23 15:54:22 (first
  and last lines, +02:00). About 18 days across the three files.
- `rg "untrusted screen read" server.log*` finds exactly one line, in `server.log` (line 37657):

  ```
  2026-11-23T11:02:59.829568+02:00 INFO typing on an untrusted screen read session=036007 resized=false pty_size=[0,0] attach_sizes=null
  ```

  - `resized=false` and `pty_size=[0,0]`: this was an undecided read, not a resize. A zero PTY size
    means the holder returned no read state (unsupported op or a failed call), per `typing.go:98-100`
    and `holdersock.go:678-684`. The logs do not say which of the two it was.
  - It came 48 ms after a `/clear` step finished (`server.log` lines just before 37657, `action step done ...
    signal=new-conversation elapsed=48168084`), and the next step typed at 11:02:59.864 (35 ms later).
- No end marker exists. The session's `meta.json` still lists `"notices": ["untrusted-read"]`
  (`/Users/david/.local/share/gx/sessions/036007/meta.json:28-30`, file written 11:15). The notice
  was never cleared, because no later send ran `ClearDraft`.
- The `draft check` line that marks a trusted read is Debug level, and the server log holds no Debug
  line (`rg -c "draft check"` gives nothing in `server.log*` or `server.stderr`). Only `cleared a draft
  before typing` (Info) is present: 2 lines in `server.log`, 4 in `server.log.1` (plus 2 `draft would
  not clear`). None of those sessions raised the notice.
- `rg -l '"untrusted-read"' ~/.local/share/gx/sessions/*/meta.json` lists only session 036007. Other
  `raw.log` hits for the string are chat text about this feature, not events.

## Time constants in the code (not measurements)

- `draftSettleWait` 1 s, `draftSettlePoll` 25 ms: the re-read loop after clearing a draft
  (`typing.go:30-35`, used at `typing.go:61-75`).
- `settleEnters` 3, `enterGap` 1 s (`typing.go:108-114`).
- `DefaultActionSlashSettle` 1 s (`server/sessions/prompt/prompt.go:28-30`), `DefaultKeysSettleStep`
  100 ms and `DefaultKeysSettleCap` 1.5 s (`prompt.go:31-34`), `sendSettle` 3 s
  (`server/sessions/prompt/send.go:23-24`), `DefaultTurnSettleTimeout` 2 s
  (`server/sessions/prompt/turnsettle.go:12-14`).
- None of these is named for a resize settle, and none was derived from `untrusted-read` data.

## Recommendation

Do not hard-code a measured number: there is none. Poll `StyledScreenRead` and stop on the first read
with `Resized == false`, with a cap:

- Poll every 25 ms (`draftSettlePoll`), cap 1 s (`draftSettleWait`). Reason: it reuses the one
  existing re-read budget in the same file, and the trust signal is event-driven (next child output),
  so a normal resize ends in a few polls.
- Caveat for the design: `Resized` stays set until the child prints something. An idle child that does
  not redraw on resize would stay untrusted for the whole cap. That case, and the case in the one
  logged event (empty read state), both end in "still undecided at the end of the window", which the
  map already routes to the "may not have been sent" card.
- Add the missing evidence in the impl: log the poll count and elapsed time when `Deliver` leaves an
  undecided read (Info, one line), and log when `ClearNotices` actually drops a notice. After a few
  weeks `gx server logs` can then give median, p95 and max, and the cap can be tuned from data.
