# Remove the loopback TCP API listener

## Problem Statement

The gx server has two ways to reach its API: the unix socket, and an opt-in loopback TCP listener (`server.tcp-listen`, `127.0.0.1:7421`). Nothing uses the TCP listener. No client, dotfile or config sets it. It serves the full API with no auth, so any local process can drive the server when it is on. It also costs code (a second HTTP server, a browser-refusal wrapper, a handshake field, a status warning, a settings page row with a risk confirmation) and tests that exist only for it.

## Solution

The unix socket becomes the only way to reach the server API. The `server.tcp-listen` setting, the listener, its browser-refusal wrapper, the handshake `tcp_addr` field and the "TCP listener is on" status warning are gone. The web UI listener (`server.web.listen`) is unchanged. A config file that still has `"tcp-listen"` loads without a warning, like any unknown key.

## User Stories

1. As a gx user, I want the server to never open a TCP port for its API, so that no local process can use the API without auth.
2. As a gx user, I want the unix socket (mode 0600) to be the only API entry point, so that file permissions are the one access rule I need to reason about.
3. As a gx user with `"tcp-listen": true` in an old config, I want the server to start normally, so that an old key does not break gx.
4. As a gx user with `"tcp-listen": true` in an old config, I want no warning for it, so that I treat it like every other unknown key (for example the old queue-limit keys).
5. As a gx user, I want the Settings page to have no "Listen on TCP" row, so that I am not offered a switch that does nothing.
6. As a gx user, I want `gx server status` to print no TCP warning, so that every warning it prints is about something that exists.
7. As a gx user, I want `gx server status` to keep printing the web UI listener bind failure, so that removing the TCP warning does not hide a real fault.
8. As a gx user, I want the web UI listener to behave as before (bind, login gate, source-IP rule, Host and Origin checks), so that this removal does not change how I reach the web UI.
9. As a gx user, I want `gx server start` to bind the unix socket and the web address only, so that a start never fails because port 7421 is taken.
10. As a script author, I want the handshake response to have no `tcp_addr` field, so that the response describes only listeners that exist.
11. As a script author, I want the API version unchanged, so that current clients and servers keep negotiating without an upgrade hint.
12. As a CLI user, I want every `gx server` verb to work over the unix socket as before, so that nothing I run depends on the removed listener.
13. As a maintainer, I want `refuseBrowsers` and its browser checks gone, so that no dead security code makes the server look as if it has a browser-facing API listener.
14. As a maintainer, I want the `TCPAddr` field of `server.Config` and the `Server.TCPAddr()` method gone, so that no caller can ask for a listener that no longer exists.
15. As a maintainer, I want the startup config snapshot (the values the server started with, for restart-needed keys) to stop including TCP, so that the restart-needed check covers only real keys.
16. As a maintainer, I want the tests that used `server.tcp-listen` as their example startup-only, restart-needed or risky key to use a key that still exists, so that those tests keep covering the restart-needed and risky-key rules.
17. As a maintainer, I want `servertest.StartWithTCP` and `Harness.TCPAddr` removed, so that test helpers do not offer a listener the server cannot open.
18. As a maintainer, I want `docs/config-schema.json` regenerated without `tcp-listen`, so that the schema matches the registry.
19. As a reader of `docs/web-ui-security.md`, I want the TCP listener bullet gone, so that the doc lists only protections that exist.
20. As a reader of ADR 0028, I want the line about the TCP listener removed, so that the ADR's consequences list matches the server.
21. As a gx user reading the CHANGELOG, I want an entry that says the TCP listener and `server.tcp-listen` are removed, so that I know why the key no longer does anything.
22. As a future maintainer, I want the old specs under `docs/specs/` left alone, so that they keep recording what was built at the time.

## Implementation Decisions

Config
- Remove the `tcp-listen` field from the typed server config and from the raw config decoder, and the line that copies it over. The config loader does not reject unknown keys, so an old `"tcp-listen"` key is ignored with no code for it. Do not add a deprecation warning or a migration. `config_test.go` already pins the "unknown key: no effect, no warning" behavior for the old queue-limit keys.
- Remove the `server.tcp-listen` registry entry and its label. The Settings page reads the registry, so the row disappears with it. The registry's risky-entry helper stays, because `server.web.listen` and `server.web.allowed-hosts` use it.
- Regenerate `docs/config-schema.json` with the repo's generator (`go test ./config/registry -run TestGeneratedSchemas -update`). Do not edit it by hand.

Server
- Remove the TCP address from `server.Config`, the default TCP address constant, the loopback-only bind helper, the TCP listener and its HTTP server fields, and the `TCPAddr()` method.
- `Serve` runs the socket HTTP server and, when bound, the web server. Shutdown joins those two only. The listener list used to wait for serve goroutines shrinks to match. Keep the `errc` capacity right for the remaining goroutines.
- Remove `TCPListen` from the startup config snapshot. `server.web.listen` stays in it.
- Delete the browser-refusal file (`refuseBrowsers`, `browserReason`, the log-rate constant). Verified with ripgrep: its only caller is the TCP HTTP server in the server root. The web listener does not use it and has its own Host, Origin and Content-Type checks in `server/web` and `server/webtls`. `webtls.NormalizeHost` stays, since the web listener uses it in several places.
- Server startup no longer has a bind step that can fail for a TCP address, so the cleanup branch for it (close the socket, release the lock) goes too.

Wire and handshake
- Remove `TCPAddr` / `tcp_addr` from the handshake type and stop setting it in the root handshake route. No API version bump: the field was `omitempty` and no client reads it. An older client against a newer server, or the reverse, sees no difference because the field is only ever absent or ignored.
- `tygo.yaml` does not list `server/wire/server.go`, so `Handshake` is not generated and `webui/src/generated/` has no `tcp_addr`. No generated file changes.

CLI
- `gx server start` no longer reads the TCP setting or passes a TCP address to the server config.
- `gx server status` drops the "TCP listener is on" warning. The other warnings (web UI bind failure, remote check, version hint) stay in the same order.

Docs
- `docs/web-ui-security.md`: delete the TCP listener bullet only.
- ADR 0028: delete the line "The TCP listener has no auth, so it binds to loopback addresses only." No amendment, no new ADR. Adding a listener back later is easy, so the decision is not hard to reverse.
- CHANGELOG: add an entry under the next version using the repo's style (see the `changelog` and `gx-changelog` skills). It says the opt-in loopback TCP listener and `server.tcp-listen` are removed, that the socket is the only API entry, and that an old `tcp-listen` key is ignored.
- Old specs under `docs/specs/` and the research notes under `docs/research/` stay untouched. Earlier CHANGELOG entries stay as they are, since they record past releases.

Test fixtures
- Remove `servertest.StartWithTCP`, the `TCPAddr` field on `servertest.Harness`, and the TCP argument threaded through the harness start helper (which becomes simpler with it gone). Implementer's choice: whether to drop the argument from the internal helper or leave a smaller signature, as long as nothing mentions TCP.

## Testing Decisions

A good test checks behavior a person or client sees: the config a server loads, the settings page content, the handshake body, the status output, and the restart-needed outcome. It does not check that a helper was deleted.

Seams, highest first, all existing:
- The `servertest` harness (a real server on a unix socket plus the API client). It covers the handshake and the "old key is ignored" start.
- The config loader and the registry tests (`config`, `config/registry`). They cover the key's absence and the unknown-key behavior.
- `statusWarnings` in `cmd/serverctl`.
- The settings, settingsapi, lifecycle, serverapi and serverctl tests that use a startup-only key.
- The web unit test for risky and restart settings rows.

No new seam is needed.

Tests to delete: the four `TestTCP_*` tests in `server/server_test.go`, `TestLoadServerTCPListen` in `config/server_test.go`, and the TCP case in the `statusWarnings` test in `cmd/serverctl/server_test.go` (keep its "older build" check, and add a `WebError` case that asserts "web UI listener failed to bind", so story 7 has a test).

Tests to repoint: every test that uses `server.tcp-listen` as its example key.
- Use `server.web.listen` = `"127.0.0.1:0"` in `server/settings/configstatus_test.go`, `server/settingsapi/read_test.go`, `server/lifecycle/configreload_test.go`, `cmd/serverapi/server_reload_config_test.go` and `cmd/serverctl/server_config_status_test.go`. `server/settings/startuponly_test.go` already proves this value is restart-needed under the `servertest` harness. It is restart-needed, risky and has a confirm text, so it also serves the settingsapi test.
- Delete the `tcp-listen` lines in `server/settings/startuponly_test.go` and `config/registry/entries_core_test.go` (the exact key-list test) instead of repointing them.
- `webui/src/settings/SettingsRow.test.tsx`: the `server.tcp-listen` case is the only "risky bool, turning it on asks" case, and no real key can replace it. Keep the case under a neutral test-only key name (the test builds entries inline), so the generic risky-bool branch stays covered.
- Each repointed test must still prove the rule it covered (pending restart list, reload warning text, notification body, risky confirm text).

Tests to add:
- A server start test with an old `"tcp-listen": true` in the config file: the server starts, the raw handshake JSON has no `tcp_addr` key (a typed decode cannot show this once the field is gone), and the config status has no warning and no pending restart for it. Put it in `server/lifecycle/configreload_test.go`.
- A registry test, or an extension of the registry's key list test, that no entry for `server.tcp-listen` exists. The existing exact-key-list test in `entries_core_test.go` already fails if the entry stays, so extending it is enough.

Prior art: `server/settings/configstatus_test.go`, `server/lifecycle/configreload_test.go`, `config/config_test.go` (unknown keys), `cmd/serverctl/server_test.go` (`statusWarnings`).

Run per package with the cap wrapper, as `.ai/index.md` requires: `config/...`, `server` (scoped with `-run`), `server/settings`, `server/settingsapi`, `server/lifecycle`, `server/servertest/...`, `cmd/serverctl`, `cmd/serverapi`, plus `make web-test` for the one web test. Then `gx check` on the changed files.

## Verification

A verify ticket runs these against a temporary gx server after the build. This change has no new web page, so no flow-catalog flow is needed beyond the Settings checks below.

1. Story 1, 2, 9: start a temporary server, then run `ss -ltnp` (or `lsof -iTCP -sTCP:LISTEN -a -p <server pid>`) and expect no listener on 127.0.0.1:7421. With the web listener off, expect no TCP listener at all. `curl -sS http://127.0.0.1:7421/v1/handshake` exits non-zero (connection refused).
2. Story 3, 4: put `{"server":{"tcp-listen":true}}` in the temporary config, start the server, and run `gx server status`. Expect exit 0, no warning that mentions TCP or `tcp-listen`, and no `restart needed for` line. Start it again with `server.watch-config` on and edit the key while it runs. Expect no reload warning for it.
3. Story 5: open the Settings page (use the settings flow in the flow catalog, or `gx server settings` with `--json`). Expect no `server.tcp-listen` entry and no "Listen on TCP" row.
4. Story 6, 7: `gx server status` prints no TCP line. With a web listen address that fails to bind, it still prints "web UI listener failed to bind".
5. Story 8: run the existing web login and Host/Origin flows from the flow catalog against the fixture server. They must pass with no edit.
6. Story 10, 11: fetch the handshake over the socket, for example `curl --unix-socket <state-dir>/server.sock http://x/v1/handshake`. Expect JSON with no `tcp_addr` and the same `api_version` as before the change.
7. Story 12: run `gx server status`, `gx tickets epics` and one write verb against the server. Expect the same output and exit codes as on main.
8. Story 13, 14, 15, 17: `rg -n -i 'tcp-?listen|TCPAddr|tcp_addr|listenLoopback|tcpHTTP|DefaultTCPAddr|StartWithTCP|refuseBrowsers|7421'` over the repo, excluding `docs/specs/`, `docs/research/` and earlier CHANGELOG entries. Expect no hit except the new CHANGELOG entry and the new old-key start test in `server/lifecycle/configreload_test.go`. Do not add a `tcp-listen` case to `config/config_test.go`: `TestLoadOldCapKeysAreUnknown` already pins the rule. `net.TCPAddr` hits in `webtls`, `web`, `devvite`, `vitefake` and `gxfixture` are unrelated and stay.
9. Story 16: the repointed tests pass (`go test` for the packages above and `make web-test`).
10. Story 18: `go test ./config/registry -run TestGeneratedSchemas` passes without `-update`, and `rg tcp-listen docs/config-schema.json` has no hit.
11. Story 19, 20, 21, 22: `rg -n -i tcp docs/web-ui-security.md docs/adr/0028-server-api-mirrors-the-cli.md` has no hit. The CHANGELOG has the new entry. `git diff --stat -- docs/specs docs/research` is empty.

New flows or scenarios to write: none. The checks above are commands and existing flows.

Also run `gx check --full` once in the background, as `.ai/index.md` says for a verify ticket.

## Out of Scope

- Adding a TCP listener back, with auth or without it. That would be a new effort.
- Any change to the web UI listener (`server.web.listen`), its source-IP rule, Host and Origin checks, or login.
- A deprecation warning, a migration step or a config rewrite for an old `tcp-listen` key.
- An API version bump.
- A new ADR.
- Edits to old specs, research notes or earlier CHANGELOG entries.

## Further Notes

- The change is one session of work. It touches no ticket schema, so no Migration step and no schema snapshot update.
- The unix socket is the only API entry after this. Its security rests on mode 0600 and the state-dir lock, which this change does not touch.
- Charting found no use of `tcp-listen`, port 7421 or `tcp_addr` in the dotfiles, the gx config or any client.
- Dead-code sweep: after the deletions, run `gx brief go-vet` and check for unused imports (`net`, `slog`, `atomic`) and unused test helpers.
