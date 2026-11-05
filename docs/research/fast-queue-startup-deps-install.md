# Fast queue startup: the deps install and its alternatives

Ticket: `gx:fast-queue-startup/02`. Measured 2026-11-05 on the dev laptop (12 cores, APFS, node 24.22.2, pnpm 12.16.1 installed, `webui/package.json` pins `pnpm@12.8.1`, npm 11.19.1).

## Answer in short

1. The install is not what is slow. On this machine `pnpm install --frozen-lockfile` in a fresh gx worktree takes 0.3s with a warm store and 3.5s with a cold store. The 29s to 150s seen between claim and `deps-installed` is spent before the iteration worktree exists.
2. No reuse method beats a warm pnpm store. Cloning `node_modules` takes about 2s, a real copy 1.8s, and a symlink breaks pnpm 12. The pnpm install that is already there is the fastest correct path.
3. The "deps unchanged" skip rule is worth little for pnpm (a warm re-run costs 0.3s) but it still removes the 3s network step when the policy-verification cache is cold, and it matters for npm, yarn and bun whose installs are slower.

## Where the time really goes (live evidence)

`deps-installed` is logged after the worktree is created and the install is done (`ralphloop/install.go:17-28`, called from `ralphloop/lifecycle.go:134`). The run log has no claim event and no install duration, so "claim to deps-installed" lumps the claim, base resolution, both worktree adds and the install together.

I split the span with file birth times of live worktrees (`/usr/bin/stat -f %SB`), the ticket's `claimed_at` and the run-log event time:

| worktree | claim to worktree dir created | worktree created to `node_modules/.modules.yaml` written |
|---|---|---|
| `webui-paste-image-impl-item-01` (ticket 01, 2026-11-05 23:44) | 148.8s | 0.6s (dir 23:47:13, modules file 23:47:14) |
| `limit-burn-rate-impl-item-26b1` | 2.8s | 1.0s |
| `limit-burn-rate-impl-item-26b2` | 2.1s | 2.0s |
| `limit-burn-rate-impl-item-26b9` | 0.7s | 2.0s |
| `cursor-support-item-03` | 1.1s | 1.0s |

(Older worktrees whose ticket was reclaimed show nonsense deltas and are left out.) In the slow case the install took under a second. The time is spent between the `status: claimed` write and the `git worktree add` of the iteration worktree.

What sits in that span (`server/sched/run.go:30-100`, `ralphloop/lifecycle.go:134-168`): base resolution, `l.Index.Rescan()` (a full re-read of the ticket store, 6,101 `.md` files now; `server/index/watch.go:149-169`), `LaunchModel`, the mode lookups, then `worktree.Lock` plus `addFeatureWorktree` (with the rebase onto base, `ralphloop/feature.go:158-182`) and the iteration add. I did not isolate which of these owns the delay, because the span has no events inside it. A single `git worktree add` of gx costs 0.3s (measured, 2 runs). I could not call `Index.Rescan` outside the server. Ticket 01 (timeline) or the per-step events must pin it down. Do not tune the install for this.

### The 17:00 switch on 2026-11-04 (coordinator's question)

Claim to `deps-installed`, hourly median over `split-server-pkg-impl` (run log + `claimed_at`): 1.7s at 12:00, 2.1s at 13:00, 2.8s at 15:00, 39s at 17:00, then 60 to 110s every hour from 19:00. Across the other 23 epics in the store, the same span is 1.0 to 3.4s median (slow-tests-impl 2.2s over 123 launches, better-tests-impl 1.6s, agent-lifecycle-impl 3.4s, codex-support-impl 1.0s). Only `split-server-pkg-impl` is slow, plus three new epics today (`better-epic-header-impl`, `follow-ups-2026-11-04`, `webui-paste-image-impl`, 25 to 150s).

Checked and ruled out as the cause of the switch:

- Same install command in every row (`webui: pnpm install --frozen-lockfile`).
- `webui/package.json`, `pnpm-lock.yaml` and `pnpm-workspace.yaml` had no commit in 2026-11-04 to 2026-11-05 (`git log origin/main -- webui/...` is empty for that range).
- Store volume: the store (`~/Library/pnpm/store/v11`), the worktrees (`/Users/david/dev/gx`) and the scratchpad share one APFS volume (same filesystem ID, `/dev/disk3s1s1`).
- Install speed itself: 0.6 to 2s in every worktree where I could read it, including a slow-claim one.

Not checkable now: the pnpm store and caches were recreated today (store dir born 23:33, pnpm package-manager store 23:21), so the store state of 11-04 is gone. The log has no event inside the span, so a machine-load or cache cause on 11-04 cannot be proven or excluded from the logs. The pattern that remains: the slow span is correlated with the epic (split-server-pkg-impl is a long-lived epic branch with many commits and many closed tickets; the other epics are small) and not with the install. The 29.x, 49.x, 69.x, 89.x, 109.x, 149.x second values suggest waits in roughly 20s steps (possibly a lock or retry loop). I found no 20s constant in the claim path (`grep` of `ralphloop`, `server/sched`, `git`, `tickets`: only 10s ticket and allocation lock timeouts, 30s polls). Open question for ticket 01 or a new ticket: instrument the claim-to-worktree span.

## Install timings (fresh gx worktree `webui/`, `node_modules` removed before each run)

Method: `CI=true`, temporary store via `--store-dir`, temporary metadata cache via `XDG_CACHE_HOME` (pnpm 12 honours it on macOS; `--cache-dir` is not a flag). Load average is the `uptime` 1-minute value when the run started (other agents were running; 12 cores).

| method | store | result | load at start |
|---|---|---|---|
| `pnpm install --frozen-lockfile` | cold store, cold cache | 3.5s, 3.6s, 3.6s | 12.4, 11.0, 10.5 |
| same | warm store, warm cache | 0.3s, 0.3s, 0.3s | 10.5, 9.9, 9.9 |
| same, `--offline` | warm | 0.3s, 0.3s | 9.9, 9.9 |
| same, `--prefer-offline` | warm | 0.3s, 0.3s | 9.9, 9.5 |
| same | warm store, cold metadata cache | 3.3s, 3.1s | 9.5, 10.2 |
| same, `--offline` | warm store, cold metadata cache | fails (exit 1) in 0.1s and 0.3s | 10.2, 10.2 |
| same, `node_modules` already present | warm | 0.0s, 0.1s | 9.5 |
| same, real default store and cache | warm (live) | 0.3s, 0.3s, 0.4s | 10.0 to 5.6 |
| fresh `git worktree add` + pnpm install | warm | 0.3s + 0.4s | 5.6 |
| `npm ci`, scratch `package-lock.json` | cold cache | 5.8s, 4.3s, 2.6s | 4.2, 4.4, 4.4 |
| same | warm cache | 1.4s, 1.6s, 1.4s | 4.5 |
| same, `--offline` | warm | 1.5s, 1.5s | 4.6, 5.0 |
| same, `--prefer-offline` | warm | 1.5s, 1.4s | 5.0 |
| same, `node_modules` present (npm deletes it first) | warm | 2.0s, 2.3s | 5.5 |
| `npm ci --ignore-scripts` | warm | 1.4s, 1.5s | 7.4 |

Notes on the table:

- The first pnpm runs with a cold store printed `esbuild postinstall: Done`; the cold store runs total 3.5s including that and the download of 405 packages (364 MB store, 186 to 196 MB `node_modules`). Registry network was fast (about 100 MB/s).
- Cold metadata cache costs 3s even with a warm store and `--frozen-lockfile`: pnpm 12 prints `Lockfile passes supply-chain policies (verified 25m ago)` and keeps `lockfile-verified.jsonl` plus per-package metadata in its cache dir. When that verification is not cached it fetches registry metadata for the lockfile. With `--offline` and no cached metadata the install fails. So `--offline` is unsafe as the default; `--prefer-offline` gave no gain over the default.
- Making the npm lockfile (`npm install --package-lock-only`, scratch copy, cold cache) took 31.7s and is not part of a launch. The lockfile stayed in the scratchpad and was deleted.
- npm 11.19.1 does not run install scripts by default (`npm warn install-scripts ... esbuild@0.28.2 (postinstall: node install.js)` is only a warning). pnpm ran the esbuild postinstall because `webui/pnpm-workspace.yaml` has `allowBuilds: esbuild: true`. `npm ci` is therefore not equivalent for gx's webui unless scripts are approved.
- `npm ci` is slower than pnpm with a warm store (1.4s against 0.3s) and needs a different lockfile, so it is not an alternative for gx. gx's existing logic already picks `npm ci` for a project with `package-lock.json` (`ralphloop/deps/install.go:34-36`).
- Yarn 1.22.22 is installed; bun is not. I did not time yarn or bun (the gx webui has no yarn or bun lockfile). Their rows below are from documentation reading and the code, not measured.

## Why pnpm is fast here

- pnpm on macOS imports packages from the store with APFS clones (`clone-or-copy` default). A file in a fresh `node_modules/.pnpm/react@19.3.1/node_modules/react/package.json` has link count 1 (a clone, not a hardlink), and the store and the worktrees are on the same volume, so there is no copy fallback. A copy instead of a clone would not explain slow installs here: I tested a real copy of the whole tree and it took 1.8s.
- With a warm store, 405 packages are linked in about 0.3s total.

## Reuse methods

Source: an installed `webui/node_modules` (196 MB, 17,187 files, 1,460 symlinks, none absolute).

| method | time | result | risk |
|---|---|---|---|
| `/bin/cp -cR` (APFS clone) into another worktree | 1.9s, 2.1s, 2.5s (load 4.8 to 5.1) | `tsc`, `vitest --version` and `pnpm run build` pass in the clone | Slower than a warm `pnpm install` (0.3s). The clone carries `.pnpm-workspace-state-v1.json` and `.modules.yaml` with the source worktree's absolute path; a following `pnpm install` in the clone re-ran for 1.1s and rewrote them. A clone made from a `node_modules` that a ticket changed carries that ticket's packages. Note: GNU `cp` is first in PATH here and has no `-c`; use `/bin/cp`. |
| `cp -R` (real copy) | 1.8s, 1.8s | works | Same as the clone but uses 196 MB of disk per worktree. |
| `ln -s` shared `node_modules` | 0.0s | `pnpm run`, `pnpm exec` and `pnpm install` all fail: `workspace hoist directory is not a real directory` (pnpm 12.8.1). Calling `node_modules/.bin/tsc`, `vite build` and `vitest run` directly works (117 test files, 1445 tests passed in 21.9s). | Breaks every `pnpm` command, which is how gx runs the webui scripts. One ticket that runs `pnpm add` or `pnpm install` mutates the `node_modules` of every other worktree. A different `package.json` in one ticket is invisible to the others. |
| pnpm store (already used) | 0.3s warm | correct, per-worktree, isolated | A cold store costs 3.5s once; a cold policy cache costs 3s. |

Recommendation: keep installing per worktree with the manager, and make the install cheap by keeping the store and cache warm. Do not add a symlink or clone layer. It is slower or breaks pnpm, and it adds shared-state risk for no gain.

## "Deps unchanged" check per manager

An earlier install is reusable when all of these match. The main benefit is skipping the install process entirely (including its network and policy checks) for managers that are slower than pnpm.

| manager | files that must match | notes |
|---|---|---|
| pnpm | `pnpm-lock.yaml`, `package.json` (dependencies, `packageManager`), `pnpm-workspace.yaml` (`allowBuilds`, overrides, settings), `.npmrc`, pnpm version, node version and platform | pnpm already records its own state in `node_modules/.modules.yaml` and `.pnpm-workspace-state-v1.json`. A re-run on an installed tree costs 0.0 to 0.1s, and 0.3s on a fresh worktree from a warm store, so the cheapest correct rule is to just run it. |
| npm | `package-lock.json`, `package.json`, `.npmrc`, npm version, node version | `npm ci` deletes `node_modules` first (2.0 to 2.3s with it present), so a skip needs a marker outside `node_modules`. npm also writes `node_modules/.package-lock.json`, a hidden lockfile of what is installed; compare it to the tracked lockfile. |
| yarn (v1 and berry) | `yarn.lock`, `package.json`, `.yarnrc` or `.yarnrc.yml`, `.pnp.cjs` presence, yarn version | Not measured. Berry stores `install-state.gz` in `.yarn`. |
| bun | `bun.lock` or `bun.lockb`, `package.json`, `bunfig.toml`, bun version | Not measured; bun is not installed here. |

One rule for all managers: hash the sorted set of those files plus `manager --version`, `node --version` and the platform, store the hash in a marker file next to `node_modules`, and skip the install when the marker matches and `node_modules` exists. A reuse source is only valid if it was installed by a ticket that did not edit those files.

## Cold and warm summary

| path | cold | warm |
|---|---|---|
| pnpm today | 3.5s | 0.3s |
| npm ci | 2.6 to 5.8s | 1.4s |
| clone of an installed `node_modules` | n/a | 1.9 to 2.5s |
| symlinked `node_modules` | n/a | 0.0s, but breaks pnpm 12 |

## Effect on the 15s target

The deps install cannot be what keeps a launch above 15s on this machine: it costs at most 3.5s cold and 0.3s warm. The 15s target depends on the claim-to-worktree span (up to 150s today, cause not yet isolated) and on the 30s ready wait.

## Cautions

- Load was 4 to 14 during the runs, from other agents. Warm pnpm numbers were stable across loads; cold numbers varied with network.
- The store was recreated at 23:33 today. A fresh machine or CI may show a cold first install of the pinned pnpm version (the `Done in ... using pnpm v12.8.1` line shows pnpm switching to the pinned version from `~/Library/pnpm/package-manager-store`); the download of that version was already cached here and is not measured.
- I used temporary stores and caches only; they are deleted.

## Sources

- `ralphloop/deps/install.go:19-50, 52-77, 195-230`, `ralphloop/install.go:17-28`, `ralphloop/lifecycle.go:134-168`, `ralphloop/feature.go:158-182`, `server/sched/run.go:30-100`, `server/index/watch.go:149-169`.
- Run logs: `~/.local/share/gx/tickets/gx/*/run-log.jsonl` with `claimed_at` from `issues/*.md`.
- Launch-timeline findings: `/Users/david/dev/gx/research-fast-queue-startup-launch-timeline/docs/research/fast-queue-startup-launch-timeline.md`.
- Raw timings: run output of `pnpm install`, `npm ci`, `/bin/cp -cR`, `uptime` as recorded in this session.
