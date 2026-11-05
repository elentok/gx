# Split the root `server` package further

Goal: root `server` under 50 files, 4,500 src lines, 5,000 test lines, 180 `Test` funcs, no file over 1000 lines, no type over 50 methods.
Now: 43 files, 4,390 src lines, 9,401 test lines, 289 `Test` funcs, `Server` has 129 methods.
Need to move out: about 110 tests, 4,400 test lines, 80 `Server` methods.

Method per slice (same as the Launcher move in 62): new `server/<pkg>` owns the logic behind a small host interface. Root keeps a thin `*Host` adapter in a `*wire.go` file. Tests move with the code (`package <pkg>_test`). Shared helpers go to `servertest`. Each slice checks `go list -deps` for no later-layer import.

## Slices (one ticket each, independent unless noted)

- [ ] S1 web: `web.go`, `webroutes.go`, `webinfo.go`, tests `web_*`, `webhost`, `webroutes`, `origin`, `unmute_origin` (~45 tests, ~18 methods) -> `server/webhttp`
- [ ] S2 session glue: `sessionhost.go`, `sessioninfo.go`, `sessionwire.go`, `readablesession.go`, tests `session*`, `permission_card`, `retry_prompt`, `sessions_afk` (~55 tests, ~10 methods) -> existing `server/sessions` (or `sessions/host`)
- [ ] S3 lifecycle: `startup.go`, `restart.go`, `restartcheck.go`, `configreload.go`, tests `restart*`, `configreload*`, `lock`, `logfile`, `stop`, `shutdown_order`, `migrat*` (~45 tests, ~13 methods) -> `server/lifecycle`
- [ ] S4 reads and verdicts: `verdicts.go`, `reads.go`, `waitinglabel.go`, `claimhold.go`, tests `reads`, `verdicts_epic`, `waiting*`, `snapshot*`, `claimhold*`, `ratelimit_claim_hold` (~30 tests, ~15 methods) -> `server/explain`
- [ ] S5 ticket and project wiring: `ticketopswire.go`, `projectsinit.go`, `ticketwrites.go`, `iterations.go`, `runs.go`, tests `checkstatus`, `checksave`, `retrohook`, `oneoff*`, `rootrefs`, `store_commit` (~45 tests, ~16 methods) -> `server/ticketops` / `server/runs` hosts
- [ ] S6 verify: remaining root tests (`server_test`, `server_notify`, `deadlock`, `runner*`, ...), run the count script, confirm all limits, `gx check`.

Order: S1..S5 in any order (each touches different files); S6 last, blocked by all.
Risk: `Server` has 129 methods because every host interface is satisfied on it. Moving a slice only drops methods if its host adapter moves to a separate small type (like `LaunchHost`), not stays on `Server`. Each slice must do that.
