# Plan: Parallelize the Test Suite

## Context

Tests run sequentially within each package. `go test ./...` runs packages in parallel, but the
slowest packages dominate wall time. Current worst offenders:

| Package | Time | Tests |
|---|---|---|
| `ui/status` | **26.6s** | 20 e2e + ~108 unit |
| `git` | **10.8s** | ~60 unit |
| `ui/commit` | **8.8s** | 3 e2e + 29 unit |
| `ui/worktrees` | **12.2s** | ~21 e2e + unit |
| `ui/log` | **3.8s** | 2 e2e + 14 unit |

No `t.Parallel()` calls exist anywhere in the project. All e2e tests are fully isolated:
each creates its own `t.TempDir()`, its own git repo, and its own `teatest.TestModel` with
a private bubbletea program writing to a private `bytes.Buffer`. Safe to run concurrently.

## Changes (priority order)

### 1. Add gc.auto=0 to `testutil/repo.go`

- [x] `TempRepo` doesn't disable Git's background GC. Under parallel load, multiple repos
could trigger GC simultaneously, causing slowdowns or flakiness. Add to `configUser`:

```go
func configUser(t *testing.T, dir string) {
    t.Helper()
    mustGit(t, dir, "config", "user.email", "test@test.com")
    mustGit(t, dir, "config", "user.name", "Test")
    mustGit(t, dir, "config", "gc.auto", "0")
    mustGit(t, dir, "config", "gc.autoDetach", "false")
}
```

File: `testutil/repo.go:185`

### 2. Reduce `CheckInterval` in `testutil/teatestv2/teatest.go`

- [x] Change from 50ms to 20ms. This makes condition detection ~2.5x more responsive
for e2e tests where conditions are typically met in <100ms.

```go
// line 65:
CheckInterval: 20 * time.Millisecond,
```

File: `testutil/teatestv2/teatest.go:65`

### 3. Add `t.Parallel()` to `ui/status/e2e_test.go` — all 20 tests

- [x] Decided NOT to parallelize status e2e tests. Running 20 concurrent git-heavy e2e tests
saturates I/O and causes intermittent timeouts in other packages (worktrees). E2e tests remain
sequential. Unit tests in model_test.go are parallelized instead (see step 4).

Increased `stageLoadWait` from 5s to 10s for extra slack under combined suite load.

File: `ui/status/e2e_test.go`

### 4. Add `t.Parallel()` to `ui/status/model_test.go` — 92 of 103 tests

- [x] **Skip these 11** (they mutate shared state or use `t.Setenv`):
  - 5 with `t.Setenv` (lines 457, 476, 495, 611, 2274): Go panics if `t.Parallel()` precedes `t.Setenv`
  - 6 with `stageClipboardWrite` (package-level var): concurrent mutation causes data races

File: `ui/status/model_test.go`

### 5. Add `t.Parallel()` to `git/*_test.go` — all ~60 tests across 13 files

- [x] No `t.Setenv` anywhere in the git package tests. Each test creates its own isolated repo.

Files:
- `git/branch_test.go`, `git/clone_test.go`, `git/commit_test.go`, `git/doctor_test.go`
- `git/log_entries_test.go`, `git/log_parse_test.go`, `git/log_test.go`
- `git/push_divergence_test.go`, `git/remote_test.go`, `git/repo_test.go`
- `git/stage_test.go`, `git/status_test.go`, `git/worktree_test.go`

### 6. Add `t.Parallel()` to `ui/worktrees/worktrees_test.go` — unit tests only

- [x] E2e tests (those calling `startTUI`, line 79+) remain sequential — they are all git-heavy
and running them concurrently would saturate I/O alongside other packages' parallel tests.
Unit tests (before line 79) are parallelized.

Increased `loadWait` from 5s to 15s for extra slack.

File: `ui/worktrees/worktrees_test.go`

### 7. Add `t.Parallel()` to `ui/log/e2e_test.go` and `ui/commit/e2e_test.go`

- [x] 2 tests in log, 3 tests in commit. All isolated, no `t.Setenv`.

Files: `ui/log/e2e_test.go`, `ui/commit/e2e_test.go`

## Actual outcome

| Package | Before | After |
|---|---|---|
| `ui/status` | 26.6s | ~16-17s |
| `git` | 10.8s | ~3s |
| `ui/worktrees` | 12.2s | ~13s |
| `ui/commit` | 8.8s | ~8s |
| `ui/log` | 3.8s | ~5s |

Wall time dominated by `ui/status` (~17s) vs original ~26.6s. Git package speedup most dramatic.

## Notes on e2e test parallelism

The status and worktrees e2e tests were NOT parallelized because their git subprocess load under
full-suite concurrency caused reliable timeouts. Parallelism within packages creates I/O contention
that exceeds the 5–15s `loadWait` timeouts. Unit tests were safe to parallelize everywhere.

## Verification

```bash
# Check no t.Setenv tests got t.Parallel() (would panic at runtime)
grep -n "t.Parallel\|t.Setenv" ui/status/model_test.go | head -30

# Run the full suite and compare timing
rtk proxy go test ./... -count=1 -timeout 300s 2>&1 | grep "^ok\|^FAIL"

# Run the status package specifically (biggest bottleneck)
rtk proxy go test ./ui/status/... -count=1 -timeout 120s 2>&1 | tail -5

# Confirm no data races
go test ./... -race -timeout 300s
```
