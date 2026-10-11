# Config reload

Handoff spec from the `gx:config-reload` map (`config-reload/ticket.md` in the ticket store). The
map's decisions came from one Destination grilling session (2026-10-11); this spec adds the
per-field semantics and the watcher details, researched against the code.

## Problem Statement

The gx server reads the global `~/.config/gx/config.json` once, when it starts. After that, every
edit to that file is silently ignored until the server is restarted. A person who lowers
`max-agents` because their machine is slow, raises a budget limit, or adds a Telegram token
believes the change is live, but the server keeps running with the old values. Restarting the
server to apply a one-line change is disruptive: it interrupts lands in flight, re-indexes the
ticket store, and is easy to forget. Today the person also has no signal at all that their edit was
not picked up, and no signal when the file they saved is broken.

`project.json` does not have this problem: the server reads it on every decision. Only the global
config is frozen.

## Solution

The running server can reload `config.json` without a restart, through one reload path with two
triggers:

- `gx server reload-config` asks the running server to reload now and prints the result.
- A file watcher, on by default, reloads automatically when `config.json` is saved. A new config
  key, `server.watch-config`, turns it off.

Each reload either applies the new values or, when the file is broken, keeps the current config
and says why. Fields that only take effect at startup (`ticket-store.path`, `server.tcp-listen`,
`agent-runner`) never block a reload: the other fields are applied and the result warns "restart
needed for X".

Every reload result — success, success with warnings, or failure — is shown as a macOS
Notification Center banner, from both triggers, whether or not a TUI is open. When a TUI is open it
also shows the result as a toast. Reload results are never sent to Telegram or Slack.

## User Stories

1. As a gx user, I want to run `gx server reload-config` after editing config.json, so that the
   running server uses my new settings without a restart.
2. As a gx user, I want the server to reload config.json automatically when I save it, so that I do
   not need to remember a command.
3. As a gx user, I want a macOS banner saying "gx config reloaded successfully" after a reload, so
   that I know my edit was picked up.
4. As a gx user, I want the banner to list which settings changed, so that I can confirm the server
   saw the edit I meant.
5. As a gx user, I want a banner when my saved config.json is invalid JSON, so that I notice a typo
   right away instead of at the next restart.
6. As a gx user, I want the server to keep running with its current config when the file is
   invalid, so that a typo never breaks a running queue.
7. As a gx user, I want `gx server reload-config` to exit non-zero and print the parse error when
   the file is invalid, so that scripts and I can tell a failed reload from a good one.
8. As a gx user, I want a reload that changes `ticket-store.path`, `server.tcp-listen` or
   `agent-runner` to still apply every other change, so that one startup-only field does not block
   the rest.
9. As a gx user, I want a clear "restart needed for tcp-listen" warning when I change a
   startup-only field, so that I know the change is pending until a restart.
10. As a gx user, I want that warning repeated on every later reload until I restart, so that I
    do not forget a pending startup-only change.
11. As a gx user, I want to lower `execution-queue.max-agents` while agents run, so that the server
    stops starting new agents until the count drops below the new cap.
12. As a gx user, I want lowering `max-agents` to never kill a running agent, so that no work is
    lost. (The one reload that can stop agents is a hard budget limit lowered below today's spend;
    see story 46.)
13. As a gx user, I want raising `max-agents` to start queued work right away, so that I do not
    wait for the next scheduling tick.
14. As a gx user, I want `max-agents-per-epic` to reload the same way as `max-agents`, so that both
    caps behave alike.
15. As a gx user, I want a changed `budget.soft-limit` or `budget.hard-limit` to apply to today's
    budget, so that I can raise or lower my daily spend without a restart.
16. As a gx user, I want a reload to keep a limit I raised with `gx budget increase` unless I edited
    that same limit in the file, so that an unrelated edit (even to the other limit) does not undo
    it.
17. As a gx user, I want a raised budget limit to lift a soft or hard latch that today's spend no
    longer reaches, so that work resumes like it does after `gx budget increase`.
18. As a gx user, I want a lowered soft limit that today's spend already exceeds to latch at the
    next budget check, and a lowered hard limit to follow the grace period in story 46, so that the
    new limit is enforced.
19. As a gx user, I want new Telegram or Slack credentials to take effect on reload, so that I can
    set up chat notifications on a running server.
20. As a gx user, I want removing chat credentials on reload to stop chat messages, so that I can
    silence a noisy channel without a restart.
21. As a gx user, I want a reload that does not touch the chat settings to keep the chat batching
    and throttle state, so that a reload does not cause a burst of messages.
22. As a gx user, I want a changed `ticket-store.commit-debounce` and `ticket-store.push-remote` to
    apply to the running store commit loop, so that commits and pushes follow the new settings.
23. As a gx user, I want a shorter `server.log-retention` to prune old agent logs right after the
    reload, so that I do not wait up to a day for disk space back.
24. As a gx user, I want a changed `server.tab-env` to apply to the next iteration tab the server
    opens, so that I can fix PATH without a restart.
25. As a gx user, I want running iteration tabs to keep the environment they started with, so that
    a reload does not change an agent's shell under it.
26. As a gx user, I want `server.auto-merge-epic`, `evolve`, `recovery.*` and
    `subscription.suppress-extra-usage-warning` to apply at the server's next decision that reads
    them, so that every server-side setting reloads.
27. As a gx user, I want the watcher to handle editors that save by writing a temp file and
    renaming it, so that saves from vim, VS Code and others are all detected.
28. As a gx user, I want one save to cause one reload, so that I get one banner and not several.
29. As a gx user, I want saving the file without changing its content to cause no banner, so that
    a reflexive `:w` is not noisy.
30. As a gx user, I want saving the same broken content twice to report the error once, so that I
    am not spammed while I fix it.
31. As a gx user, I want deleting config.json to keep the current config and warn me, so that a
    stray `rm` or a half-finished save does not reset the server to defaults.
32. As a gx user, I want `server.watch-config: false` to turn the watcher off, so that I can control
    exactly when the server reloads.
33. As a gx user, I want turning `server.watch-config` off or on to take effect at that reload, so
    that the flag itself needs no restart.
34. As a gx user with a TUI open, I want a toast with the reload result, so that I see it in the
    window I am working in.
35. As a gx user with a TUI open, I want a success toast as success, a warning as a warning, and a
    failure as an error, so that the toast's color matches the outcome.
36. As a gx user with a TUI open, I want the TUI to refresh its server state after a reload, so
    that budget limits and the extra-usage warning show the new values.
37. As a gx user on Linux, I want reloads to work without macOS banners, so that the feature does
    not depend on macOS.
38. As a gx user, I want reload results kept out of Telegram and Slack, so that my phone only gets
    messages about agents.
39. As a script author, I want `gx server reload-config --json`, so that I can read the changed
    keys, warnings and refusal reason as data.
40. As a gx user, I want `gx server reload-config` to fail clearly when no server is running, so
    that I know there is nothing to reload.
41. As a gx maintainer, I want every reload logged in the server log with its trigger and result,
    so that I can see when the config changed while I debug a run.
42. As a gx maintainer, I want startup and reload to map config.json to server settings through one
    function, so that a new field cannot be wired at startup and forgotten on reload.
43. As a macOS user with `terminal-notifier` installed, I want gx banners sent through it, so that
    they show a proper sender instead of "Script Editor".
44. As a gx user, I want `gx server status` to show the last reload's result and any startup-only
    changes still waiting for a restart, so that I can see them after the banner is gone.
45. As a gx maintainer, I want every OS notification backend's tests to run on every OS, so that a
    Linux CI or a Mac laptop both catch a broken backend.
46. As a gx user, I want a reload that sets the hard budget limit at or below today's spend to wait
    60 seconds before stopping agents, and to tell me so, so that I can undo a mistaken edit before
    work is lost.
47. As a gx user, I want to cancel that pending stop by raising the hard limit in config.json or by
    running `gx budget increase`, so that fixing the mistake is enough.
48. As a gx user, I want `gx server status` to work and show the config error when config.json is
    broken, so that I can see why the last reload failed.

## Implementation Decisions

### One reload path

- The server gains one reload operation. Both triggers call it; it is serialized by its own mutex,
  so a command and a watcher event never interleave.
- Steps: read the file bytes → parse and validate → build the new reloadable settings → diff them
  against the live ones → apply side effects → swap the live settings → kick the runner → report.
- The server reads the bytes itself (for missing-file detection and content dedupe), so the
  config package exposes a pure parse step (bytes in, `Config` out, same defaults and validation
  as today) that `config.Load` also uses. `config.Load` keeps treating a missing file as defaults.
- The mapping from `config.Config` to the server's reloadable settings moves out of `runServer`
  into the server package, and both startup and reload use it (story 42).
- The server is told which file to reload through a new `server.Config` field (the config path).
  `runServer` passes `config.FilePath()`. An empty path means reload is unavailable: the watcher
  does not start and the route refuses with reason `reload-unavailable`.

### Concurrent-safe settings

- `server.Config` is split. Startup-only and wiring fields stay in it (state dir, build, ticket
  store path, TCP address, runner, test seams such as poll intervals and `Sleep`). The reloadable
  fields move into a new `Settings` struct, embedded in `Config` so callers and tests still set
  them the same way.
- The server holds the live settings in an atomic pointer. Every reader loads the pointer once per
  decision and uses that snapshot, so one decision never sees half of an old config and half of a
  new one. A reload builds a new `Settings` value and swaps the pointer; values are never mutated
  in place.
- The chat notifier is not swapped: a reload replaces its global sink in place under the notifier's
  own mutex, so every send sees either the old sink or the new one.

### Per-field semantics

Reloadable settings, with the moment each new value takes effect:

| Config key | Reload behavior |
| --- | --- |
| `execution-queue.max-agents` | Next scheduling decision. Running agents continue; no new start while running ≥ new cap (ADR 0032). The reload kicks the runner, so a raised cap fills slots at once. Explain verdicts update on the same pass. |
| `execution-queue.max-agents-per-epic` | Same as `max-agents`, per root. |
| `budget.soft-limit`, `budget.hard-limit` | Compared **per field** with that field's value at the last load. Only a field whose file value changed is applied to the ledger's live limit; the other keeps any live `gx budget increase`. When applied, latches are re-evaluated with the same rule as `budget increase` (a latch that spend no longer reaches is lifted; a lowered soft limit already exceeded latches at the next check), budget notes are re-armed, and the runner is kicked. A lowered hard limit at or below today's spend starts the hard-limit grace period (below). The ledger is not rebuilt. |
| `notifications.telegram.*`, `notifications.slack.webhook-url` | When any global chat field changed, only the **global** sink is rebuilt (keeping the test seams of the old chat config) and swapped in. The per-project sinks built from project.json are kept with their batch and throttle state. The old global sink is closed in the background **after** the reload mutex is released, since `Close()` waits on the network and must not block the command. Unchanged chat settings keep the existing notifier. |
| `ticket-store.commit-debounce`, `ticket-store.push-remote` | New storecommit code: `storecommit.Loop` gains `SetSettings(debounce, pushRemote)`, guarded by a mutex (its fields have no lock today), and its ticker is reset to the new interval on the next tick (it is built once today). The loop is not stopped or restarted, so `Immediate` commits keep working during a reload. |
| `server.log-retention` | Live at the next prune. When it changed, the reload runs one prune right away. |
| `server.tab-env` | Applies to the next iteration tab launched. Running tabs keep their environment. |
| `server.auto-merge-epic` | Next epic completion. |
| `evolve` | Next decision that reads the evolve mode. Still validated by the parse step. |
| `recovery.enabled`, `recovery.disabled`, `recovery.follow-ups`, `recovery.notify-hold` | The catalog is rebuilt from the new values. Applies to the next failure matched. A park hold already running keeps the notify-hold it started with. |
| `subscription.suppress-extra-usage-warning` | Next extra-usage check (enqueue, snapshot). |
| `server.watch-config` (new) | Starts or stops the watcher at the end of this reload. The reload never stops the watcher synchronously: it cancels the watcher's own context, and the watcher goroutine exits after the current reload returns. So a watcher-triggered reload that turns the flag off cannot deadlock waiting for itself. Turning the flag on starts a fresh watcher. |

Startup-only keys: `ticket-store.path`, `server.tcp-listen`, `agent-runner`. They are compared
with the values the server **started** with, not the last reload. While any differs, every reload
applies the rest and warns "restart needed for <key>" (stories 9–10). The socket path is not a
config key (it comes from the state dir), so it never appears here.

`agent-runner` is checked against its allowed values (`auto`, `herdr`, `headless`) during reload
validation, so an invalid value is reported now rather than failing the next restart. `config.Load`
itself is unchanged (it still leaves this check to server start, so a bad value never breaks the
TUI).

Keys that the server does not use (`use-nerdfont-icons`, `image-diffs`, `stage-diff-context-lines`,
`input-modal-bottom`, `name-aliases`, `log`, `skills`, `agents`, `budget.notification-thresholds`,
`evolve.deep-read-cap`, `execution-queue.spin-*`) are parsed and validated but have no effect. They
do not appear in the changed list. `execution-queue.retry-storm-launches` is already read live by
ralphloop and needs nothing.

### Hard-limit grace period

- Applies only when a **reload** sets `budget.hard-limit` at or below today's spend. Spend crossing
  the hard limit during normal work still stops agents at once, as today.
- On such a reload the hard latch is not tripped. Instead a 60-second grace timer starts (fixed, no
  config key; a test seam shortens it). During the grace period no new agent starts, as if the soft
  limit were hit; running agents keep going.
- Cancelled by any later reload (watcher or command) that sets the hard limit above today's spend,
  or by `gx budget increase` raising it above spend. Cancelling clears the pause.
- When the timer ends the server checks again: if spend is still at or above the hard limit, it
  latches hard and stops agents through the existing `killForBudget` path; otherwise nothing
  happens.
- One timer at most. A reload during the grace period that keeps the limit at or below spend does
  not restart the timer.
- Each stage is reported as an OS banner and a `config-reloaded`-style event (TUI toast), no chat:
  - Start (warning): title "gx: agents stop in 60s", body "New hard budget limit ($X) is below
    today's spend ($Y). N agents will stop. Raise the limit in config.json or run `gx budget
    increase` to cancel."
  - Cancelled (success): "gx: agent stop cancelled — hard budget limit raised."
  - Done (error): "gx: N agents stopped — hard budget limit reached."
- `gx server status` shows the countdown while it runs (see Server status).

### Invalid and missing files

- Bad JSON or a validation failure (bad `evolve`, bad `agent-runner`): the live settings stay
  as they are. The result is a refusal with reason `invalid-config` and the parse error as the
  message.
- A missing config.json is not "use defaults" on reload. The live settings stay as they are and
  the result is a refusal with reason `config-missing`. Resetting a running server to defaults
  because a file vanished is never what the person meant.

### Route and command

- New route `POST /v1/config/reload`, served by the new CLI verb `gx server reload-config` with
  `--json` (ADR 0028 parity; add it to the route/verb map in the parity test).
- Result shape (refusals are results, ADR 0028):
  - `changed`: the config keys whose value changed, in config-key spelling.
  - `warnings`: one entry per pending startup-only key ("restart needed for tcp-listen").
  - `refused`, `reason` (`invalid-config`, `config-missing`, `reload-unavailable`), `message`.
- The command always reloads, even if the content is unchanged since the last load (no dedupe).
- Human output: "gx config reloaded successfully", then "changed: …" (or "no changes"), then one
  "warning: …" line per warning. Exit 0 on success and on success with warnings; non-zero on a
  refusal, printing the message. Exit non-zero with "server not running" when no server answers.

### Watcher

- Runs as one more `Serve` goroutine next to `keepFresh`, stopped by the same context.
- Uses fsnotify on the config **directory**, not the file, because editors save by writing a temp
  file and renaming it over the original, which drops a watch on the file. Events are filtered to
  the config file's base name; other files in the directory (for example
  `notifications-state.json.lock`) are ignored.
- If config.json is a symlink at watcher start, the target's directory is watched too and events
  for the target's name count.
- A burst of events becomes one reload after a 300ms quiet period. The debounce is a test seam.
- Dedupe: the watcher remembers the bytes of its last attempt (success or failure). If the bytes
  are equal it does nothing and logs at debug level (stories 29–30).
- If the file is still missing after the debounce, the watcher reports `config-missing` once, and
  does not repeat it until the file has existed again.
- If the config directory does not exist at startup, or it is removed later, the watcher logs a
  warning and stops. `gx server reload-config` still works, and turning `server.watch-config` off
  and on starts it again.

### Reporting

Every reload result goes to three places from the same reporting step:

- **Server log**: one line with the trigger (`command` or `watch`), changed keys, warnings, or the
  refusal.
- **OS notification** (new channel): a new small package sends a banner through a `Notifier`
  interface. It follows ADR 0035 (no build tags for OS-specific behavior):
  - Each backend lives in a plain file that compiles on every OS and only builds an `argv`:
    `terminal-notifier` (`-title`, `-message`, `-group gx-config` so a new banner replaces the
    old one) and `osascript` (title and body passed as `argv` to an `on run argv` script, never
    spliced into the script text, so a quote in an error message cannot break or inject
    AppleScript). Commands run through an injectable runner.
  - One selection function takes `goos` and `lookPath` as inputs: darwin with `terminal-notifier`
    on PATH → terminal-notifier; darwin without it → osascript; any other OS → no-op. The lookup
    happens once, when the notifier is built at server startup.
  - A send runs in the background with a short timeout; a failure is logged and never fails the
    reload. There is no fallback from terminal-notifier to osascript after a failed send.
  - The server uses it through a `server.Config` seam (nil means the real notifier), so server
    tests capture banners and never run a command.
  - Success: title "gx config reloaded successfully", body the changed keys or "no changes".
  - Warnings: title "gx config reloaded with warnings", body "restart needed for …".
  - Failure: title "gx config reload failed", body the message, trimmed to a banner-friendly
    length.
- **Event stream**: a new event kind `config-reloaded`. `server.Event` gains two optional fields,
  `level` (`success`, `warning`, `error`) and `message` (the banner text). Other events leave them
  empty, so the wire format stays compatible.
- No Telegram or Slack message for any reload result.

### Server status

- New route `GET /v1/config/status` with the CLI verb `gx server config-status` (`--json`), added
  to the ADR 0028 parity map. It returns the last reload result, the pending startup-only keys and
  the hard-limit grace countdown (if any).
- `gx server status` and `gx server config-status` call the same client function; status renders
  the result as its Config block.
- `gx server status` no longer fails on a broken config.json: on a config load error it uses the
  default paths to reach the server and prints the config error at the top.
- The server keeps the last reload result (time, trigger, success / warnings / refusal and its
  message) and the startup-only keys a reload found changed; a key stays listed until restart, even
  if the file is set back.
- `gx server status` (and its `--json`) gains a "Config" block with both, plus the hard-limit
  grace countdown while one runs. The block is omitted when no reload has happened since startup
  and nothing is pending.
- A pending startup-only key clears only on restart, matching the repeated reload warning.

### TUI

- `viewmodel.ToastFor` returns a level along with the text, and the shell picks
  `notify.Success`, `notify.Warning` or `notify.Error` from it. Existing toasts (parked) keep
  Warning.
- Reducing a `config-reloaded` event with level `success` or `warning` returns the resnapshot
  effect, so budget limits and the extra-usage flag refresh. An `error` event changes nothing and
  does not resnapshot.
- The usual stream rule still applies: only an applied (not replayed, not gap) event toasts.

### Schema and docs

- `server.watch-config` (boolean, default true) is added to the config struct, the parser (absent
  means true) and the published JSON schema.
- The CHANGELOG entry mentions the command, the watcher, the flag and the macOS banners.

## Testing Decisions

- Good tests here drive the server through its public route with a real config file on disk and
  assert observable outcomes: the result body, the events on the stream, the captured banners, and
  scheduling behavior. They do not reach into the settings pointer or the diff function.
- **Server reload (main seam)**: `servertest.Harness`, extended so each harness writes a temp
  config.json and sets the config path and a capturing notifier by default. Tests call the reload
  route through the API client. Cases:
  - lower `max-agents` while agents run → running agents untouched, no new launch; raise it → a
    queued root starts without waiting for the poll tick;
  - invalid JSON → refusal `invalid-config`, old cap still enforced, one error banner, one
    `config-reloaded` event with level `error`;
  - missing file → refusal `config-missing`, settings unchanged;
  - change `server.tcp-listen` and `max-agents` together → `max-agents` applied, warning for
    `tcp-listen`; a second reload repeats the warning;
  - budget limit changed → budget status reports the new limits and a latch is lifted;
    `budget increase` raising soft, then a reload that edits only `hard-limit` → soft keeps the
    raised value (per-field compare);
  - reload lowers hard limit below spend → no kill, no new start, one "agents stop in 60s" banner
    and event; after the (shortened) grace the agents are stopped and a "stopped" banner sent;
    a second reload raising the limit, or `budget increase`, during the grace → no kill, a
    "cancelled" banner, starts resume;
  - chat config changed while a project.json sink has a pending batch → the project sink's batch
    is still delivered; the reload returns before the old global sink finishes closing;
  - watcher-triggered reload that sets `server.watch-config: false` → returns, and later saves
    cause no event (no deadlock);
  - chat config changed → messages go to the new destination (existing chat test transports);
  - every reload produces exactly one banner and no chat message.
- **Watcher**: same harness with the watcher on and a short debounce seam. Write the file by
  temp-file-and-rename and by in-place write, and assert one `config-reloaded` event each; write
  identical bytes and assert no event; delete the file and assert one `config-missing` report;
  set `server.watch-config: false` and assert later saves cause no event.
- **CLI**: the parity test covers the new route/verb pair. A command test checks exit codes and
  output for success, warnings and refusal, following the existing `gx server` verb tests.
- **config**: parse-step tests in the existing config tests, including `server.watch-config`
  defaulting to true.
- **viewmodel**: `ToastFor` and `Reduce` tests for `config-reloaded` at each level, next to the
  existing parked-toast tests.
- **OS notification package** (runs on every OS, per ADR 0035): each backend's `argv` against a
  fake runner (title and body as separate arguments, a quote in the body stays inside one
  argument); the selection function for each `goos` × `lookPath` case (terminal-notifier found,
  not found, non-darwin).
- **Server status**: after a reload with a warning, `gx server config-status --json` and
  `gx server status --json` show the last result and the pending key; a fresh server shows no
  Config block; with a broken config.json, `gx server status` still succeeds and prints the error.
- **storecommit**: `SetSettings` while the loop runs → the next commit uses the new debounce and
  push remote.
- Prior art: the server tests built on `servertest.Harness` (budget latch, deadlock, agent verbs),
  `servertest.WaitForEvent` for stream assertions, and the store-watch tests for fsnotify timing.

## Out of Scope

- Reloading `project.json` (already read live).
- Applying startup-only fields (`ticket-store.path`, `server.tcp-listen`, `agent-runner`) without
  a restart.
- Reloading the TUI's own cached config (`agents`, `skills`, display settings). The TUI reads
  config.json at its own startup; a server reload does not change it. A "reloaded successfully"
  banner refers to the server only.
- Telegram or Slack messages for reload results.
- OS notifications on Linux or Windows.
- Wiring `execution-queue.spin-cycles` / `spin-window` into the server (the server does not use
  them today, at startup or on reload).
- Using the new OS notification channel for anything other than reload results.

## Further Notes

- ADR 0032 already promises that lowering a cap only stops new starts "without a restart"; this
  feature makes that true for the global caps, not only for project.json's cap.
- Linux gaps (OS notifications are a no-op there; `notify-send` is the likely backend) are tracked
  in the `gx:linux-support` map epic.
- Without `terminal-notifier`, `osascript` banners show as coming from "Script Editor". macOS may
  ask once for notification
  permission, and a server started without a GUI login session (for example over SSH) may not be
  able to show banners. The reload still works; only the banner is lost, and the failure is logged.
- The seams in this spec were not confirmed with the person (the brief asked for no interview).
  The main one is the server route through `servertest.Harness`; the watcher, CLI, viewmodel and
  OS notification tests are thin around it.
