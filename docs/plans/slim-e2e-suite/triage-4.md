# e2e triage 4 (migration, restart, restart-run, settings, reload, setup, login)

| file | flow | title | verdict | reason | vitest file |
|---|---|---|---|---|---|
| reload.e2e.ts | - | reload allowance: one allowed | KEEP-E2E | fixture self-test, needs real page.reload | none (could be one combined fixture self-test) |
| reload.e2e.ts | - | second reload fails | MERGE-INTO reload allowance: one allowed | same fixture self-test, same `empty` scenario | none |
| reload.e2e.ts | - | reload fails by default | MERGE-INTO reload allowance: one allowed | same; or drop from CI | none |
| login.e2e.ts | 102 | Login | KEEP-E2E | real TOTP/cookie, hash preserved after login | LoginPage.test.tsx covers UI half |
| login.e2e.ts | 103 | Login rate limit | MERGE-INTO 102 | same web-login scenario; run last (lockout). UI is in LoginPage.test.tsx:54 (server limit of 5 is the only real part) | LoginPage.test.tsx (existing) |
| login.e2e.ts | 104 | Certificate help | MERGE-INTO 102 | same scenario; keep only real downloads + CLI fingerprint match; link/fingerprint UI in LoginPage.test.tsx:137 | LoginPage.test.tsx (existing) |
| setup.e2e.ts | 99 | First-run setup | KEEP-E2E | real setup code from CLI, real TOTP; field validations already in SetupPage.test.tsx and could be trimmed from the flow | SetupPage.test.tsx (existing, partial) |
| setup.e2e.ts | 100 | Setup from the QR link | KEEP-E2E | `gx server web-url` + hash navigation; consumes setup so cannot share 99 | none |
| setup.e2e.ts | 101 | Setup and login with TOTP off | KEEP-E2E | needs server config server.web.totp=false end to end | SetupPage.test.tsx/LoginPage.test.tsx cover UI only |
| restart.e2e.ts | 128 | Restart server in the menu | MERGE-INTO 129 | same menu open; DOM order assertion can be a vitest too | TopBar.test.tsx (existing) |
| restart.e2e.ts | 129 | Restart server keyboard | KEEP-E2E | real focus and arrow keys in a real menu | TopBar.test.tsx could cover |
| restart.e2e.ts | 130 | Restart dialog confirms | KEEP-E2E | real restart, pid changes, reload | RestartDialog.test.tsx covers UI half |
| restart.e2e.ts | 131 | Restart dialog checks | TO-VITEST | Checking.../disabled/versions are pure render states | RestartDialog.test.tsx:39,45 (existing) |
| restart.e2e.ts | 132 | same version | DELETE-DUPLICATE | "Versions match" is RestartDialog.test.tsx:66; restart+pid is flow 130 | RestartDialog.test.tsx (existing) |
| restart.e2e.ts | 133 | other install path | MERGE-INTO 146 | same restart-launchd scenario, same assertions (versions match / enabled) | RestartDialog.test.tsx |
| restart.e2e.ts | 134 | unreadable version | KEEP-E2E | real server error surfacing for failing/missing binary (two server starts; second could merge into first) | RestartDialog.test.tsx:95 partial |
| restart.e2e.ts | 135 | bad config | MERGE-INTO 134 | same restart-projects scenario, alert + disabled button; alert render is vitest | RestartDialog.test.tsx |
| restart.e2e.ts | 136 | old binary | MERGE-INTO 134 | same scenario and alert/disabled assertions | RestartDialog.test.tsx |
| restart.e2e.ts | 137 | running agents | KEEP-E2E | real queue starts agent, server counts it | RestartDialog.test.tsx:45,60 covers text |
| restart.e2e.ts | 138 | dry run | KEEP-E2E | real dry-run failure from migration check, then restart anyway; target for 178 | RestartDialog.test.tsx:71 |
| restart-run.e2e.ts | 139 | Restart with a failed migration | KEEP-E2E | real restart + migration across restarts, CLI retry; target for 140 and 179 | FailedMigrationStrip.test.tsx covers strip UI |
| restart-run.e2e.ts | 140 | Fix agent for a failed migration | MERGE-INTO 139 | same startBroken setup; insert between strip step and retry step | FailedMigrationStrip.test.tsx:99,110 (existing, dialog/toast) |
| restart-run.e2e.ts | 141 | tab reloads | KEEP-E2E | real restart to a new binary, page reload with new build | NewVersionStrip.test.tsx partial |
| restart-run.e2e.ts | 179 | applies pending steps | MERGE-INTO 139 | 139 already does forgetApplied + restart; just assert the list refills | none |
| restart-run.e2e.ts | 142 | other tab | KEEP-E2E | cross-tab new-version strip, draft kept (SSE) | NewVersionStrip.test.tsx partial |
| restart-run.e2e.ts | 143 | server does not return | KEEP-E2E | real restart_log path + recovery via CLI; 60s timeout text is RestartDialog.test.tsx:117. Could be demoted if log-path check dropped | RestartDialog.test.tsx:117 |
| restart-run.e2e.ts | 144 | binary changed | TO-VITEST | dialog re-shows new values after refusal; mock restart API returning refusal | RestartDialog.test.tsx:95 (existing) |
| restart-run.e2e.ts | 145 | two tabs | KEEP-E2E | server-side concurrent restart behavior | none |
| restart-run.e2e.ts | 146 | launchd binary | KEEP-E2E | launchd scenario check on real server; target for 133 | RestartDialog.test.tsx |
| settings.e2e.ts | 105 | Settings and web logins | TO-VITEST | counts, This device chip, days left, rename on blur are all render/callback logic | SettingsPage.test.tsx:44,82,91,146 (existing) |
| settings.e2e.ts | 106 | Sign out another device | KEEP-E2E | other browser gets signed out live, no reload, user-agent naming | SettingsPage.test.tsx:157 UI half |
| settings.e2e.ts | 107 | Sign out of this device | KEEP-E2E | real session cookie revoked; target for 111 | SettingsPage.test.tsx:170 |
| settings.e2e.ts | 108 | Change password | KEEP-E2E | other devices invalidated, new password works from fresh browser | AccountPages.test.tsx:37,58 UI half |
| settings.e2e.ts | 109 | Reset authenticator | KEEP-E2E | real TOTP secret swap, old code rejected (slow, up to 240s) | AccountPages.test.tsx:99,105,141 UI half |
| settings.e2e.ts | 110 | Web reset from the CLI | KEEP-E2E | CLI -> live page flips to setup | SetupPage.test.tsx |
| settings.e2e.ts | 111 | CLI revoke | MERGE-INTO 107 | same web-logins scenario, same "signed out" end state | none |
| settings.e2e.ts | 112 | Add authenticator after CLI reset | KEEP-E2E | CLI reset + login flow + TOTP | LoginPage.test.tsx:108 UI half |
| migration.e2e.ts | 169 | Migrating state holds scheduling | KEEP-E2E | held migration, scheduling, CLI; target for 171 | MigratingStrip.test.tsx covers line |
| migration.e2e.ts | 170 | failed project rescanned | MERGE-INTO 173 | same migrate-broken scenario, assertions are a subset (startup steps) | none |
| migration.e2e.ts | 171 | Writes wait for the migration | MERGE-INTO 169 | same migrate-held scenario; CLI-only, runs after 169's release is awkward: do it before releaseMigration | none |
| migration.e2e.ts | 172 | failed step not recorded | MERGE-INTO 161 | migrate-pending scenario, restartBare chain; strip text is FailedMigrationStrip.test.tsx | FailedMigrationStrip.test.tsx |
| migration.e2e.ts | 173 | Failed start and Migration block | KEEP-E2E | real failed migration, other project keeps working; target for 170, 174-177 | FailedMigrationStrip.test.tsx:44 (strip text) |
| migration.e2e.ts | 174 | Retry button ends block | MERGE-INTO 173 | same migrate-broken; chain as steps (re-break with setInject + restartBare) | FailedMigrationStrip.test.tsx:55 |
| migration.e2e.ts | 175 | retry command by hand | MERGE-INTO 173 | same scenario, one more step | none |
| migration.e2e.ts | 176 | Fix agent route | MERGE-INTO 173 | same scenario; dialog UI already in FailedMigrationStrip.test.tsx:99 | FailedMigrationStrip.test.tsx |
| migration.e2e.ts | 177 | stale failure record cleared | MERGE-INTO 173 | same scenario, one more step | none |
| migration.e2e.ts | 178 | Restart dialog with/without pending | MERGE-INTO 138 | step 1 repeats 138's dry-run assertion; step 2 is RestartDialog.test.tsx:54,71 | RestartDialog.test.tsx (existing) |
| migration.e2e.ts | 161 | Server migrates on start | KEEP-E2E | multi-process migration on real server; target for 163, 168, 172 | none |
| migration.e2e.ts | 163 | Per-project list | MERGE-INTO 161 | same migrate-pending scenario, just another restartBare | none |
| migration.e2e.ts | 168 | Crash restart and login start | MERGE-INTO 161 | same scenario and restartBare chain | none |

## Summary

Total 50 flows (incl. 3 reload smoke tests).
- KEEP-E2E: 25
- MERGE-INTO: 21
- TO-VITEST: 3 (131, 144, 105)
- DELETE-DUPLICATE: 1 (132)

Unique fixture server starts after merges: about 25 vs 50 today (merged ones also drop Chromium start for CLI-only migration flows, but those still use the fixture).

Five largest wins:
1. migration 170/171/172/174-177 + 163/168 (10 flows) merge into 4 targets (173, 169, 161): saves ~10 server starts, all migrate-* scenarios are slow.
2. restart 133/135/136/178 fold into 146/134/138 (4 starts saved, each runs a restart-* scenario).
3. restart-run 140 and 179 merge into 139 (both reuse its startBroken setup with real restarts).
4. 131, 132, 144 (+105 in settings) leave e2e entirely; RestartDialog.test.tsx and SettingsPage.test.tsx already assert the same things.
5. login 103/104 merge into 102 and settings 111 into 107 (same scenarios; run rate limit last).
