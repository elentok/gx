# Repo identity is resolved once per process and cached forever

Identifying a repo (`git.FindRepo` → `git.IdentifyDir` + `detectMainBranch`) costs five `git`
subprocesses, ~20 ms. That is invisible on a one-off command, but gx calls it from code that runs on
a schedule — the Tickets/Queue reload path called it every 2 seconds, on the UI goroutine, forever,
which was the single largest contributor to gx's idle CPU and battery cost. We now memoize
`FindRepo` per cleaned absolute path, for the life of the process, with no TTL and no invalidation.

The trade-off is that repo identity is treated as **immutable for the process lifetime**. A `Repo`'s
`Root`, `WorktreeDir` and `IsBare` genuinely cannot change for a given path, so the only field that
can go stale is `MainBranch`, which changes only if `origin/HEAD` is repointed while gx is running. We accept that staleness: repointing the default branch mid-session is rare, and the
recovery ("restart gx") is trivial, whereas paying 20 ms of process spawns on every periodic reload
is not recoverable at all.

Worktrees created or deleted at runtime are not a problem: a new worktree is a new cache key, and a
deleted one leaves a harmless unreferenced entry.

## Considered options

- **Per-model caching only** (resolve the scratch dir once when the Tickets/Queue model is
  constructed). Fixes the measured hot path with zero semantic change, but leaves the same 20 ms
  trap armed for the next caller — `ralphloop`'s worktree add/remove path already pays it. We do
  this *as well*, but it is not sufficient on its own.
- **TTL-based cache.** Reintroduces periodic subprocess spawning, which is the exact thing being
  removed, in exchange for staleness protection nobody has asked for.

## Consequences

- `git.ResetRepoCache()` exists for tests; production code never calls it.
- Anything that genuinely needs a fresh answer must call the underlying `IdentifyDir` explicitly
  rather than expecting `FindRepo` to re-shell.
- `FindRepo` hands out a copy per call rather than the memoized pointer, so no caller can mutate
  another's view of the repo.
- `IdentifyDir` also batches its `rev-parse` queries: one invocation for `--git-dir`,
  `--is-inside-work-tree` and `--git-common-dir`, plus one for `--show-toplevel` only when inside a
  work tree. `--show-toplevel` cannot be batched with the others, because in a bare repo it fails
  the whole invocation with exit 128 — and gx's own canonical root is a bare repo.
