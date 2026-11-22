# Adding a gxfixture knob or scenario

Read this before probing `testutil/gxfixture`. Every path below was checked with `rg`.

## 1. Where a scenario lives

- The data model (`Scenario`, `Epic`, `Ticket`, `SessionSpec`, `CommandStep`, `RestartSpec`) is in
  `testutil/gxfixture/spec/spec.go`. A new knob is a new field on `spec.Scenario`.
- Scenario definitions that need nothing from the fixture program are functions or vars in
  `testutil/gxfixture/spec/*.go` (one file per area: `agents.go`, `recovery.go`, `progress.go`, `stall.go`, ...).
- The registry is `catalog.Scenarios` (a `map[string]scenario`) in `testutil/gxfixture/catalog/scenarios.go`.
  Add the name there. `catalog.Names()` lists the keys, sorted. Bigger groups have their own
  catalog file (`cachegate.go`, `launchstate.go`, `imagesscenarios.go`, `hostfiles.go`).
- `catalog/scenarios.go` and `testutil/gxfixture/scenarios.go` (package `main`) only hold type aliases
  (`scenario = spec.Scenario`, ...) so the code stays short. Do not put data in `main`'s `scenarios.go`.
- Code that acts on a knob (seeding, starting children) is in `main`: `startup.go`, `seed.go`,
  `seedrest.go`, `seedschedules.go`, `serve.go`, `sessions.go`.
- The packages are near their `sizeguard` limits. Prefer `spec` or a new file over growing a big one.

## 2. Env for the fixture's children

- `Scenario.Env` (`[]string` of `KEY=value`) is extra server environment: fake-agent knobs and server test hooks.
  Examples: `Env` in `spec/progress.go`, the `withEnv` helper in `spec/stall.go`.
- `spec.RootToken` (`{root}`, `spec/recovery.go`) in a value stands for the fixture's root dir.
  `startup.go` replaces it with `strings.ReplaceAll(kv, spec.RootToken, f.root)`. Use it for control files.
- `isolatedEnv(root, binDir)` in `testutil/gxfixture/env.go` builds the base env. It drops the real
  `HOME`, `XDG_*`, `GX_*`, `CLAUDE*` and `HERDR_*`, then sets temp ones. It returns `env` (for the server)
  and `cliEnv` (HOME, XDG, caches: what a CLI verb needs). The scenario `Env` is appended after it.
  Change `isolatedEnv` only for a variable every scenario needs; its test is `env_test.go`.

## 3. Extending `Output`

- `Output` is the JSON the fixture prints. It lives in `testutil/gxfixture/output/output.go`
  (apart from `spec` to keep `spec` under its size limit).
- Add a field with a `json:"snake_case"` tag, and a sub-struct for a group (see `HostFilesOutput`, `ModelFakesOutput`).
- Fill it in `startup.go` where `res` is built (see the `sc.ModelFakes` block that sets `res.ModelFakes`).
- Tests read it through `fixturetest.Running` (`testutil/gxfixture/fixturetest/fixturetest.go`); `Start`,
  `StartWith` and `Launch` start a scenario.

## 4. Fake agents and their launch logs

- Each fake records every launch's argv in a per-session file named `<id>.launches`.
- **The id is the conversation id, not the gx session id.** Read it from
  `<root>/data/gx/sessions/<session id>/meta.json`, field `conversation_id`. The helper is
  `(*running).conversationID` in `testutil/gxfixture/modelswitch_test.go`, which is also the worked example.
- Readers:
  - Claude TUI fake: `agentfake.ReadTUILaunches(home, conversationID)` in `testutil/agentfake/tui_control.go`
    (log path from `TUILaunchLogPath`).
  - Codex fake: `codex.ReadCodexLaunches(codexHome, conversationID)` in `testutil/agentfake/codex/codex.go`
    (`codexHome` is `<home>/.codex`).
  - Any log by path: `agentfake.ReadLaunches(path)` in `testutil/agentfake/models.go`.
- Poll for the count you expect (with a timeout); never sleep. See `waitIdle` in `modelswitch_test.go`.
- `Scenario.ModelFakes` makes the fixture print `Output.ModelFakes` (config file, query log, both launch dirs).

## 5. Wire types

- Server wire types are Go structs in `server/wire/*.go` (ops types in `server/wire/ops`), tagged `json:"..."`.
- To find a field: `rg -n 'json:"field_name"' server/wire`. Then `rg` the Go type name in `webui/src`.
- The web UI's copy is generated: `make web-types` runs tygo with `tygo.yaml` into
  `webui/src/generated/server.ts` and `ops.ts`. A file must be in `include_files` of `tygo.yaml` to appear.
  Do not edit the generated files by hand.

## 6. Index of fixture actions and scenario shapes

Fake-agent actions (`testutil/agentfake/claude.go`, constants `Action*`): `ActionFinish`, `ActionBlock`,
`ActionRateLimit`, `ActionStall`, `ActionBackground`, `ActionShell`, `ActionFast`, `ActionDialog`, `ActionBusy`,
`ActionAPIError`, `ActionModel`, `ActionUsage`, `ActionCommandOutput`, `ActionToolUse`, `ActionAskQuestion`,
`ActionPermission` and more. Per-ticket worker behaviour (finish/ask/fail) is set with `servertest.TicketOpts`
in the scenario; the worker file is `.agentfake-worker.json`.

| Shape | Defined in |
| --- | --- |
| Plain scenarios (`empty`, `web-setup`, `web-login`, `web-logins`, `fork-hooks`) | `catalog/scenarios.go` |
| Recovery / hang / classifier outage epics (`RootToken` users) | `spec/recovery.go` |
| Stall and fake-TUI quirks (`StallLeavesI`, `LateRegistry`, `LongVerifyNote`) | `spec/stall.go` |
| Slash-command output (`CommandStep.Output`) | `spec/commands.go` |
| Chat progress interval | `spec/progress.go` |
| TUI redraw delay and other agent knobs | `spec/agents.go` |
| Model switching (`ModelFakes`) | `spec/modelswitch.go`, `modelswitch_test.go` |
| Host files, cache gate, launch state, images | `catalog/hostfiles.go`, `cachegate.go`, `launchstate.go`, `imagesscenarios.go` |
| Context window / usage lines | `spec/contextwindow.go` |

## 7. Check the component before you write a locator

Do not assume a test id exists. Open the component first. Example: `ProgressRing`
(`webui/src/ui/progress-ring/ProgressRing.tsx`) exposes `data-arc`, not a test id, so a flow locates it
with `[data-arc]`.
