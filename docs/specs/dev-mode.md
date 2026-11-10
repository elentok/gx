# Dev mode

Handoff spec from the `gx:dev-mode` map (tickets 01 to 05 in the ticket store). It supersedes the
`server.web.dev-frontend` / `server.web.dev-url` design in `docs/specs/web-ui.md` and the matching
notes in `settings-page.md`, `webui-security.md` and `pwa.md`. Terms: **Dev mode** (the server-wide
switch) and **Dev checkout** (the gx source checkout whose `webui/` Vite serves), as in
`GLOSSARY.md`.

## Problem Statement

A person working on gx's web UI has to rebuild the frontend and restart the server to see each
change. The only faster path is `server.web.dev-frontend`, which needs a Vite server the person
starts by hand in another terminal, a hand-set `server.web.dev-url`, and a server restart to turn on
or off. The key is flagged risky, because a wrong URL leaves a blank page. The server does not know
where its own source checkout is, so it cannot start Vite for the person. The switch is not in the
sidebar menu, and nothing on screen says whether the page comes from Vite or from the built files.

## Solution

**Dev mode** is a switch in the sidebar menu, under Vi mode. When it is on, the gx server runs its
own Vite dev server from the **Dev checkout**, watches it, and forwards the web UI to it, so UI
edits show up by hot reload without a rebuild or a server restart. When it is off, the server stops
Vite and serves the built files. Flipping the switch takes effect at once: the server swaps the
handler that answers UI requests inside the running web server, and every open page (a phone
included) reloads when the UI it is served changes.

gx finds the Dev checkout from a path that `make build` and `make install` stamp into the binary.
The new key `server.web.dev-dir` overrides it, and is the only source in a release build. With no
valid Dev checkout the sidebar hides the switch and the settings page says why Dev mode is off.

A small "Dev" pill in the sidebar footer shows the state: green while Vite runs, grey with a spinner
while gx installs packages or Vite starts, red when Vite is down. When Vite is down (will not start,
crashes, no `pnpm`), gx serves the built files and the pill opens a popover with the reason and the
last Vite log lines, so the switch is always reachable. `server.web.dev-url` is removed: gx always
runs its own Vite.

## User Stories

1. As a gx developer, I want a Dev mode switch in the sidebar menu under Vi mode, so that I can turn
   the live UI on without opening the settings page.
2. As a gx developer, I want the same switch to turn Dev mode off, so that I can get back to the
   built UI quickly.
3. As a gx developer, I want the switch to take effect without a server restart, so that a running
   queue and my agents are not interrupted.
4. As a gx developer, I want every open page, including my phone, to reload when the UI the server
   serves changes, so that no page keeps showing the other UI.
5. As a gx developer, I want the Dev mode row to look and behave like the AFK and Vi mode rows, with
   the same drawn switch, so that the menu stays consistent.
6. As a gx user on a release build, I want the Dev mode row hidden, so that the menu has no switch
   that cannot work.
7. As a gx developer, I want gx to start its own Vite when Dev mode turns on, so that I do not run
   one by hand.
8. As a gx developer, I want gx to find the Dev checkout from the path stamped into the binary by
   `make build` and `make install`, so that I configure nothing in the usual case.
9. As a gx developer with a binary built elsewhere, I want `server.web.dev-dir` to name the Dev
   checkout (a leading `~` is expanded, any other value must be an absolute path), so that Dev mode
   works from any build.
10. As a gx developer, I want an invalid `server.web.dev-dir` (no `webui/package.json`, or a relative path) to count as
    no Dev checkout, never a silent fallback to the stamped path, so that Vite never serves a
    different checkout than the one I configured.
11. As a release-build user, I want the settings page to show the `server.web.dev-frontend` switch
    disabled with "Off: gx has no Dev checkout. Set the Dev checkout path below.", so that I know why.
12. As a gx developer, I want `server.web.dev-dir` to stay editable on the settings page and a valid
    path to enable the switch at once, so that I can set it up without editing a file.
13. As a gx developer, I want the settings page switch to apply live like the sidebar switch, with no
    restart-pending marker and no risky confirm dialog, so that it behaves like a normal setting.
14. As a gx developer, I want the switch to be saved as `server.web.dev-frontend` in `config.json`,
    so that Dev mode survives a server restart and starts Vite with the server.
15. As a gx developer who edits `config.json` by hand, I want the same live behaviour from the config
    watcher and `gx server reload-config`, so that every way of changing the key works the same.
16. As a gx developer, I want gx to run `pnpm install --frozen-lockfile` before starting Vite when
    `webui/node_modules` is missing or older than `pnpm-lock.yaml`, so that a fresh checkout works.
17. As a gx developer, I want gx to start Vite on a free loopback port, so that it never collides
    with another dev server on my machine.
18. As a gx developer, I want gx to forward to Vite only once Vite is ready, serving the built files
    until then, so that a page never gets a broken response from a Vite that is still starting.
19. As a gx developer, I want hot reload to keep working through the web listener, including over
    HTTPS, with `/v1` kept local, so that the UI behaves as it does against the built files.
20. As a gx developer, I want Vite to stop when Dev mode turns off or the server stops or restarts,
    so that no stray process stays behind after a graceful stop.
21. As a gx developer, I want a Vite that will not start, crashes, or has no `node_modules` to make
    gx serve the built files, so that the UI and the switch stay reachable.
22. As a gx developer without `pnpm` on the PATH, I want gx to serve the built files and say that
    `pnpm` is missing, so that I know what to fix.
23. As a gx developer, I want a crashed Vite not to be restarted automatically, so that a broken
    edit does not loop; turning Dev mode off and on, or the Restart Vite button, retries.
24. As a gx developer, I want no page reload when Vite goes down mid-session, so that I keep the page
    I am on and see the error instead of losing it.
25. As a gx developer, I want a "Dev" pill in the sidebar footer only while Dev mode is on, so that I
    always know the page is a Dev page.
26. As a gx developer, I want the pill green while Vite runs, grey with a spinner while installing or
    starting, and red with "Dev · down" while Vite is down, so that I read the state at a glance.
27. As a gx developer on desktop, I want the pill in the indicator line above the footer row, and on
    phone and tablet in the row left of the menu button, so that it fits each width.
28. As a gx developer, I want a click on the pill to open a popover with the status, one sentence of
    reason, the last Vite log lines and the Dev checkout path, so that I can see why Vite is down.
29. As a gx developer, I want a Restart Vite button in the popover only while Vite is down, so that I
    can retry in one click.
30. As a gx developer, I want a Turn off Dev mode button in the popover at all times, so that I can
    leave Dev mode from where I see the problem.
31. As a gx developer, I want a one-time toast "Vite is down. gx serves the built files." with
    Details and Restart Vite when Vite goes down, so that I notice it without watching the pill.
32. As a gx developer, I want Vite's output written to a log file in gx's log folder, so that I can
    read more than the popover's last lines.
33. As a gx developer, I want a browser that once loaded the built UI not to keep serving its cached
    build while Dev mode is on, so that the pill never lies about what I see.
34. As a gx developer, I want the built page and a working service worker back when Dev mode turns
    off, so that the installed app is whole again.
35. As a gx developer, I want changing `server.web.dev-dir` while Dev mode is on to restart Vite from
    the new Dev checkout and reload pages when it is ready, so that the new path takes effect at
    once.
36. As a gx developer, I want an invalid new `server.web.dev-dir` while Dev mode is on to stop Vite,
    serve the built files and disable the switch, so that gx never serves a stale checkout.
37. As a gx developer with an old `server.web.dev-url` in my config, I want gx to ignore it and log
    one warning at startup, so that an upgrade does not break startup and gx never edits my file.
38. As a gx developer, I want the snapshot to carry the Dev state, the reason, the last log lines
    and the Dev checkout path, so that every page draws the same badge.
39. As a gx developer, I want a plain `go build` or `go run .` to have no stamped path and so no Dev
    mode unless `server.web.dev-dir` is set, so that stamping is explicit.
40. As a gx developer, I want the `Makefile`, the `webui/vite.config.ts` comment and the registry
    descriptions to stop pointing at `server.web.dev-url` and an external Vite, so that no doc or
    target names a removed key.
41. As a gx developer whose web listener is off or failed to bind, I want Dev mode to stay off with
    the snapshot saying why, so that gx does not start a Vite that nothing forwards to.
42. As a gx developer, I want Vite limited to the Dev checkout's `webui/` folder, and I want the
    specs to say that Dev mode serves those sources to anyone who can reach the listener, so that
    the exposure is a known, recorded choice.
43. As a gx developer, I want the pill and the settings page to follow Vite's state (installing,
    starting, running, down) without a reload, so that I see each change as it happens.

## Implementation Decisions

**Where it lives**

- The web server owns the UI handler. Today it picks the built-files handler or the Vite proxy once
  at bind time. After this change it holds the handler in force and can swap it while running. The
  swap is the live switch: it is not a restart, and the web listener keeps serving across it.
- A new Vite supervisor module owns the child process: the dependency install step, the start on a
  free loopback port (host `127.0.0.1`, strict port), the readiness check, the log file, the
  last-lines buffer and the stop. It exposes a state: off, installing, starting, running, down, plus
  the reason, the last log lines and the Dev checkout path. The launcher (how a process is spawned)
  is an injectable seam, so tests and the fixture use a scripted fake with no real Vite and no
  `pnpm`. The implementer picks the free-port method, the Vite flags and the readiness signal; the
  signal must be a real check that Vite answers requests, not a timer.
- Effective Dev mode also needs a bound web listener. With `server.web.listen` empty or the bind
  failed, the supervisor stays `off`, Vite never starts, and the snapshot Dev block carries the
  reason ("the web listener is off" or the bind error). The listener keys stay startup-only.
- The Applier runs with the settings reload lock held, so it never does the slow work. It only tells
  the supervisor the desired state (on, off, Dev checkout) and returns at once. The supervisor runs
  the install, the start, the readiness check and the handler swap on its own goroutine, so a
  settings save or reload never waits for Vite.
- Dev mode is a settings Applier, in the same family as the other live-applied settings. It reacts
  to `server.web.dev-frontend` and `server.web.dev-dir`, whichever way they change: the sidebar
  switch, the settings page, a hand edit picked up by the watcher, or `gx server reload-config`.
  Both keys are neither Restart nor risky.
- The sidebar switch and the settings page switch write `server.web.dev-frontend` through the same
  settings writer, and the Applier does the rest. Restart Vite is a small authenticated web route
  (the implementer picks its name); it works only while Vite is down. It is web-only: a
  `wire.WebWrite` route, listed in the web-only route list that the ADR 0028 test checks (see
  `webui-security.md`, story 102), and not on the socket.

**Dev state and Dev checkout**

- The Dev checkout is the `server.web.dev-dir` value when it is set, else the build-stamped path.
  The override always wins. A leading `~` in `dev-dir` is expanded; any other value must be an
  absolute path, and a relative one counts as no Dev checkout. A path counts only if it has
  `webui/package.json`; otherwise there is no Dev checkout. The Dev checkout is the checkout root
  that holds `webui/`. Effective Dev mode is the key on **and** a Dev checkout present. With no Dev
  checkout, effective Dev mode is off, there is no pill, and the key may stay `true` in the file.
- `make build` and `make install` stamp the Makefile's own directory with `-X` in `-ldflags`. Plain
  `go build` and `go run .` stamp nothing. The implementer picks the variable name.
- Turn-on order: install if stale (state `installing`), start Vite (`starting`), and when Vite is
  ready swap the handler to the Vite proxy (`running`). Until ready the built files are served and
  the pill is grey. Server start with Dev mode on runs the same order.
- Install is stale when `webui/node_modules` is missing or older than `pnpm-lock.yaml`. It runs
  `pnpm install --frozen-lockfile`. A failed install is `down` with the reason and the output tail.
- Down means the built files are served. Causes: install failure, Vite exits or never becomes ready,
  `pnpm` not on the PATH (the reason says so), no `node_modules` after install. A crash while
  running swaps back to the built files. Vite is never restarted automatically. Turning the switch
  off and on, or Restart Vite, retries. Restart Vite stops any leftover process and goes through the
  turn-on order again.
- Vite is a child of the server. Turning Dev mode off, a Dev checkout change, and a graceful server
  stop or restart (including gx's own restart verbs) stop it. After a hard kill an orphan Vite may
  remain on its own free port. It is never adopted, and cleanup is out of scope.
- Changing `server.web.dev-dir` while Dev mode is on stops Vite and starts it from the new Dev
  checkout. An invalid new path means no Dev checkout: Vite stops, the built files are served and
  the switch is disabled.

**Reload event**

- A new `dev-changed` event on the same stream, like `vimode-changed` and `afk-changed`, fires when
  the supervisor's state or reason changes, never on log lines. Pages refetch the snapshot on it, so the
  pill, the popover, the toast and the settings row follow without a reload. It also fires on a
  crash, where "reload" does not. The snapshot still carries the latest log tail, so the popover
  shows the tail as of the last refetch (it may lag lines written since).
- One new "reload" event on the existing `/v1/events` stream tells every open page to reload. It
  fires whenever the UI the server serves changes between Vite and the built files, except on a
  crash: when Vite becomes ready after turn-on, at once when Dev mode turns off (or the Dev checkout
  becomes invalid), and when Vite becomes ready again after Restart Vite or a Dev checkout change.
  Vite going down mid-session fires no event: the page stays, the pill turns red and the toast shows.
- The event carries no payload a page needs. A page reloads on receiving it. The event is a stream
  event like the others; the reconnect and resume rules of the stream are unchanged.

**Snapshot and pages**

- The snapshot carries a Dev block: the state (off, installing, starting, running, down), whether a
  Dev checkout exists, the reason, the last Vite log lines and the Dev checkout path. The wire
  schema and the generated TypeScript types change with it (the `dev-changed` event too). The pill and the menu read it. The toast
  fires when a snapshot refetch (triggered by `dev-changed`) moves the pill state to down on a page
  that was open before the change. The settings page reads Dev checkout presence from the same
  snapshot Dev block, disables the `dev-frontend` row from it and refetches on `dev-changed`; no new
  `SettingsEntry` field is added.
- The Dev mode row is in the gear menu directly under Vi mode, with the same drawn switch, and is
  absent when the snapshot says there is no Dev checkout. The switch state is the key's effective
  value.
- The pill follows the approved mock (variant A): text pill in the footer, `Dev` plus a state word
  ("Dev · installing", "Dev · starting", "Dev · down", plain "Dev" when running). Tones green, grey
  with a spinner, red. Desktop: the indicator line above the footer row. Phone and tablet: in the
  row, left of the menu button. A click opens the popover: status, one sentence of reason, the last
  Vite log lines, the Dev checkout path, then Restart Vite (only while down) and Turn off Dev mode
  (always). The toast is shown once per transition to down, with Details (opens the popover) and
  Restart Vite.
- Vite's output goes to a log file in gx's log folder (the implementer picks the name and rotation).
  The snapshot carries only the last lines.

**Config**

- Registry: remove `server.web.dev-url`; add `server.web.dev-dir` (string, empty by default,
  editable, no Restart, not risky), described as the Dev checkout path that overrides the one
  stamped into the binary, with `~` expanded and otherwise absolute; `server.web.dev-frontend` loses
  its Restart flag and its risky confirm, and its description is rewritten in Dev mode and Dev
  checkout terms ("Turn on Dev mode: gx runs its own Vite from the Dev checkout and serves the web
  UI from it.").
- The `webui/vite.config.ts` comment that says gx proxies to `http://127.0.0.1:5173` is updated
  with the Makefile `web-dev` comment: gx runs its own Vite, and that file is the config gx's Vite
  reads, and it also sets `server.fs.strict: true` and `server.fs.allow` to the `webui/` folder. The settings page shows `dev-frontend` as a disabled switch with the line "Off: gx has no
  Dev checkout. Set the Dev checkout path below." while there is no Dev checkout, and enables it at
  once when `dev-dir` becomes valid. The settings guard's risky list drops both dev keys.
- Following `.ai/index.md` "Adding a config key": registry entries first, then regenerate the
  schemas with `go test ./config/registry -run TestGeneratedSchemas -update`, then run
  `go test ./config -run TestSettingsGuard`. Removing `dev-url` and the `config` fields, the raw
  config field and the effective-value special case for it goes with the registry change.
- Existing configs: `config.json` is not schema-migrated for this change. The loader already ignores
  keys it does not know, so an old `server.web.dev-url` loads. At startup gx logs one warning that
  the key is ignored and Dev mode now runs its own Vite. gx never edits the file. There is no
  `migrations/` step, because that registry is for the ticket and epic schema.
- Server wiring: the web bind config loses `DevURL`; the web server gets the Dev supervisor and the
  Applier swaps the handler. The `server.go`/`serverctl` plumbing that carried `WebDevURL` goes away.

**Service worker (ticket 02)**

- While Dev mode is on and Vite is running, the server answers `/sw.js` itself with a self-removing
  worker: it deletes only caches whose names start with `gx-shell-`, unregisters itself and navigates
  open windows. It touches no cookies or storage. Dev pages never register a worker. No change to
  `workerCore.ts` or the page code.
- Whichever handler is in force decides `/sw.js`: while Vite is down, or Dev mode is off, the built
  worker is served, and the built page registers a fresh one.
- `docs/specs/pwa.md` carries a pointer to this spec (done with this spec).

**Security**

- Non-`/v1` GETs pass the login gate, so while Dev mode is on Vite's dev server answers anyone who
  can reach the web listener without login, as the built assets do. gx starts Vite with
  `server.fs.strict: true` and `server.fs.allow` set to the `webui/` folder in
  `webui/vite.config.ts` (the config gx's Vite reads, so no launcher argument is needed), so
  `/@fs/` reads stay inside it. Dev mode still serves the UI sources of `webui/` to anyone who can
  reach the listener. This is recorded in `webui-security.md` (story 103). `dev-frontend` has no
  risky confirm: the map decided that the fallback, not a prompt, protects the switch.

**Proxy rules**

- The Vite proxy keeps the existing rules: loopback targets only, Host rewritten so Vite accepts it,
  websocket upgrades passed for hot reload, every non-`/v1` GET forwarded, `/v1` kept local, Vite
  modules served like the built assets, without login (see Security). The target is the supervisor's port, never a config value.
  The 502 text no longer tells the person to start Vite by hand.

## Testing Decisions

- A good test drives behaviour a person sees: what the snapshot says, what `/` and `/sw.js` answer,
  which events arrive, what the menu and pill show. It never asserts on the process spawn arguments
  or internal state-machine names.
- Seam 1 (highest): the existing server test seam (`servertest`) with the fake Vite launcher. It
  covers the Applier, the handler swap, the snapshot Dev block, the reload event, `/sw.js`, the
  proxy rules, and config handling (dev-dir validity, old `dev-url`).
- Seam 2: the gxfixture with a new scenario that scripts the fake Vite through installing, starting,
  running and down, with a reason and log lines. The web mock states and Playwright steps run
  against it. The implementer names the scenario.
- Seam 3: vitest for the self-removing worker's steps and for the menu row, pill, popover and toast
  components against snapshot data.
- Prior art: `server/web/web_devproxy_test.go` for the proxy rules; `server/settings` tests for
  Applier and startup-only reporting (the dev keys leave the startup-only list); the config registry
  and schema generation tests; the PWA steps of the `mixed` flow for the worker; the existing gear
  menu tests for the row.
- No test runs a real Vite or `pnpm`. A real-Vite run is the verify ticket's manual check.
- Default to no new Playwright flow: extend the flow that already exercises the gear menu, put the
  Dev mode PWA step in the `dev-mode` scenario's flow, and add one flow only for the multi-state pill journey that needs
  the scripted fake over a real round trip.

## Verification

1. Dev mode row in the gear menu: Mock states `menu-dev-off` and `menu-dev-on`. Flow: the existing
   gear menu flow extended to flip the row under Vi mode in the new scenario. New: that extension.
2. The same extended flow flips the row back; Mock state `menu-dev-off`.
3. Go test (seam 1): flipping the key on a running test server keeps the same listener; `/` changes
   from the built index to the Vite stub; the test server was never restarted. A settings reload returns while the fake is still `starting`
   (the Applier is off the reload lock). CLI check: `gx server
   status` shows the same PID before and after.
4. Go test (seam 1): the event stream receives one "reload" event on turn-on at ready and on turn-off.
   Playwright: the extended flow with two pages open; both reload (declare the reload count).
5. Mock states `menu-dev-on` and `menu-dev-off` (same drawn switch, row under Vi mode); vitest on the
   menu components.
6. Mock state `menu-no-checkout`; vitest: no row when the snapshot has no Dev checkout. Go test
   (seam 1): a server with no stamped path and no `dev-dir` reports no Dev checkout.
7. Go test (seam 1): turning the key on makes the fake launcher start once, and the supervisor
   state reaches running.
8. Go test (seam 1): with a stamped path pointing at a directory with `webui/package.json`, the
   snapshot reports it as the Dev checkout. Makefile check (story 39): `make -n build` shows the
   `-ldflags -X` stamp with the Makefile directory.
9. Go test (seam 1): `dev-dir` set to a valid directory wins over the stamped path.
10. Go test (seam 1): `dev-dir` set to a directory without `webui/package.json`, or to a relative
    path, reports no Dev checkout even though the stamped path is valid; a leading `~` is expanded.
11. Vitest: the settings row for `server.web.dev-frontend` is disabled with the exact reason line
    while the snapshot Dev block has no Dev checkout. Go test (seam 1): the settings entry carries no
    new disabled field. New: that vitest case.
12. Go test (seam 1): writing a valid `dev-dir` makes the snapshot report a Dev checkout and fires
    `dev-changed`; vitest: the open settings row enables from the refetched snapshot.
13. Go test (seam 1): the registry entry for `server.web.dev-frontend` has no Restart flag and is not
    risky; `TestSettingsGuard`; flow step on the settings page: toggling it opens no confirm dialog.
14. Go test (seam 1): after the key is written, `config.json` has `server.web.dev-frontend` true;
    a new test server over the same config starts the fake Vite.
15. Go test (seam 1): a hand edit of `config.json` picked up by the watcher, and `gx server
    reload-config`, each start Vite. CLI check: `gx server reload-config` exit code 0.
16. Go test (seam 1): with a stale or missing `node_modules` the fake sees an install call before the
    start; with a fresh one it sees none.
17. Go test (seam 1): the proxy target is a loopback port chosen by the supervisor, and two test
    servers get different ports.
18. Go test (seam 1): while the fake is starting, `/` answers the built index; after ready, the Vite
    stub. Mock states `installing` and `starting`.
19. Go test (seam 1, `StartWithTCP`): a module request over HTTPS is proxied without a cookie, an API
    request needs one, `/v1` stays local, and a websocket upgrade passes. Manual on the verify run:
    edit a `webui` file and see hot reload.
20. Go test (seam 1): after Dev mode turns off and after a graceful server stop and restart, the
    fake reports stopped.
21. Go test (seam 1): a fake that fails to start, or exits, leaves `/` on the built files and the
    snapshot `down` with a reason. Mock state `down`.
22. Go test (seam 1): a fake launcher error "pnpm not found" gives a reason that names `pnpm`.
23. Go test (seam 1): after a crash the fake sees no further start until the key is flipped off and
    on, or Restart Vite is called.
24. Go test (seam 1): a crash while running fires no "reload" event but fires `dev-changed`; the
    snapshot turns `down`.
25. Mock states `running`, `running-tablet`, `running-phone`; vitest: no pill when Dev mode is off.
26. Mock states `installing`, `starting`, `running` and `down`; vitest on the tones and state words.
27. Mock states `running` (desktop, indicator line), `running-tablet` and `running-phone` (in the
    row).
28. Mock states `running-popover` and `down-popover`; plus `down-popover-tablet` and
    `down-popover-phone`.
29. Mock state `down-popover`: Restart Vite present; `running-popover`: absent. Go test (seam 1): the
    restart route is refused while Vite is not down, and the ADR 0028 web-only route list test
    lists it as a web-only `WebWrite` route.
30. Mock states `running-popover` and `down-popover`: Turn off Dev mode present; flow step: clicking
    it turns the key off.
31. Mock state `down` (the toast, reached by the scenario moving running to down after the page
    loads); vitest: the toast fires once per transition to down and not on a page that loaded
    already down.
32. Go test (seam 1): after a scripted run, a log file with the fake's output exists in the log
    folder.
33. Go test (seam 1): `/sw.js` answers the self-removing worker while Dev mode is on and Vite is
    running. Vitest: its steps delete only `gx-shell-` caches, unregister and navigate. A PWA step
    run against the `dev-mode` scenario (not `mixed`): a registered worker plus a Dev mode flip ends
    with zero registrations and the shell cache gone.
34. Go test (seam 1): `/sw.js` differs between Dev mode on and off, and while Vite is down it is the
    built worker. The same `dev-mode` PWA step checks a fresh worker registers after turning off.
35. Go test (seam 1): changing `dev-dir` while running stops the fake, starts it again with the new
    path, and fires one "reload" at ready.
36. Go test (seam 1): changing `dev-dir` to an invalid path while running stops the fake, serves the
    built files, and reports no Dev checkout.
37. Go test (seam 1): a config with `server.web.dev-url` loads, and the server log has one warning
    naming it at startup. The file is byte-identical after startup.
38. Go test (seam 1): the snapshot Dev block has the state, the reason, the last log lines and the
    Dev checkout root for each scripted state. Wire schema and generated TypeScript types checks
    pass.
39. Makefile check: `go build ./...` stamps nothing, so a server without `dev-dir` reports no Dev
    checkout (the same test as story 6).
40. `grep -rn "dev-url" Makefile webui/vite.config.ts docs/specs` shows only the superseded
    pointers; the `make web-dev` and `vite.config.ts` comments name no `server.web.dev-url` or
    port 5173 proxy; the registry descriptions use Dev mode and Dev checkout terms.
41. Go test (seam 1): with `server.web.listen` empty, or a bind that failed, turning the key on
    never starts the fake, the Dev state stays `off`, and the snapshot Dev block says why.
42. Vitest: the exported `webui/vite.config.ts` has `server.fs.strict` true and `server.fs.allow`
    set to the `webui/` folder. Go test (seam 1): the registry entry for `server.web.dev-frontend` is not
    risky. Check that `webui-security.md` story 103 states the exposure.
43. Go test (seam 1): `dev-changed` arrives on starting to running and on running to down, and log
    lines written while running publish no `dev-changed`. Vitest:
    the pill and the settings row follow a refetched snapshot. Playwright: the pill flow in the
    `dev-mode` scenario moves running to down with no reload.

**Mock states**

- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, installing, 1040px, scenario
  `dev-mode` (state installing, new, listed below), parts: sidebar-footer, dev-badge. A grey pill
  "Dev · installing" with a spinner in the indicator line.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, starting, 1040px, scenario `dev-mode`
  (state starting), parts: sidebar-footer, dev-badge. A grey pill "Dev · starting" with a spinner.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, running, 1040px, scenario `dev-mode`
  (state running), parts: sidebar-footer, dev-badge. A green pill "Dev".
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, down, 1040px, scenario `dev-mode`
  (the scenario moves running to down after the page loads, so the toast fires), parts:
  sidebar-footer, dev-badge, dev-toast. A red pill "Dev · down" and the toast
  "Vite is down. gx serves the built files." with Details and Restart Vite.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, down-popover, 1040px, scenario
  `dev-mode` (state down), parts: dev-badge, dev-popover. Status, reason, log lines, Dev checkout
  path, Restart Vite and Turn off Dev mode.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, running-popover, 1040px, scenario
  `dev-mode` (state running), parts: dev-badge, dev-popover. As above with Turn off Dev mode only.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, menu-dev-on, 1040px, scenario
  `dev-mode` (state running), parts: gear-menu, dev-mode-row. The Dev mode row under Vi mode with
  the switch on.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, menu-dev-off, 1040px, scenario
  `dev-mode` (state off), parts: gear-menu, dev-mode-row. The row with the switch off.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, menu-no-checkout, 1040px, scenario
  `dev-mode` (no Dev checkout), parts: gear-menu. The menu has no Dev mode row.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, running-tablet, 820px, scenario
  `dev-mode` (state running), parts: sidebar-footer, dev-badge. The pill in the row left of the menu
  button.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, down-popover-tablet, 820px, scenario
  `dev-mode` (state down), parts: dev-badge, dev-popover.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, running-phone, 390px, scenario
  `dev-mode` (state running), parts: sidebar-footer, dev-badge. The pill in the row left of the menu
  button.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, down-popover-phone, 390px, scenario
  `dev-mode` (state down), parts: dev-badge, dev-popover.
- `$(gx tickets root)/dev-mode/mocks/03-dev-badge-look.html`, menu-dev-on-phone, 390px, scenario
  `dev-mode` (state running), parts: gear-menu, dev-mode-row.

The popover text in the mock (for example "Vite stopped after 2 restarts in 30 seconds", "Port 5173
is already in use", "Dev checkout: ~/dev/gx/main/webui") is sample copy only. The real reason
sentence never mentions restarts or a fixed port, and the path line shows the Dev checkout root (the
folder that holds `webui/`).

New flows or scenarios to write (these become implementation tickets): the fixture scenario
`dev-mode` with the scripted fake Vite and its shapes (off, installing, starting, running, down, no
Dev checkout); the extension of the gear menu flow for the Dev mode row; the multi-state pill flow;
the Dev mode PWA step, run against the `dev-mode` scenario and not `mixed`; the settings row vitest
case for the disabled switch reason; the scenario script step that moves running to down after the
page loads (for the `down` mock state); the seam-1 Go tests for the listener rule, the off-lock
Applier, `dev-changed`, the `vite.config.ts` fs limits (a vitest case) and the restart route listing.

## Out of Scope

- A Dev mode switch in the TUI. The TUI can still change the config key.
- Forwarding to an external Vite server. `server.web.dev-url` is removed.
- Automatic Vite restarts after a crash.
- Reaping an orphan Vite left by a hard-killed server.
- Per-user Dev mode: the switch is server-wide and every signed-in web page can flip it, as the
  settings page already allows.
- Config file migration or editing: gx never rewrites an old `server.web.dev-url`.
- Any change to `workerCore.ts` or the service worker registration code.

## Further Notes

- Domain terms are in `GLOSSARY.md`: use **Dev mode** and **Dev checkout**, never "dev frontend" or
  "dev dir".
- The pointer notes in `web-ui.md`, `settings-page.md` (story 31 and the risky list),
  `webui-security.md` (story 103) and `pwa.md` were added with this spec. The old test lines that name
  `server.web.dev-url` (webui-security story 103's test, `web_devproxy_test.go`, `startuponly_test.go`)
  change in the implementation.
- The Makefile `web-dev` target serves the canned snapshot and stays; only its comment about
  `server.web.dev-url` changes.
