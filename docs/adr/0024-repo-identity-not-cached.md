# Repo identity is not cached

`git.FindRepo` runs `git rev-parse` once or twice and `detectMainBranch`, so it costs several `git`
subprocesses, about 20 ms. It has no cache. An earlier plan was to memoize it per path for the life
of the process. That change never shipped, because the caller that motivated it is gone: the TUI no
longer calls `FindRepo` on a repeating timer. Today its callers run once at startup or once per
server request.

`IdentifyDir` does batch its `rev-parse` queries: one call for `--git-dir`, `--is-inside-work-tree`
and `--git-common-dir`, plus `--show-toplevel` only inside a work tree. `--show-toplevel` cannot join
the first call, because in a bare repo it fails the whole invocation with exit 128, and gx's own
canonical root is a bare repo.

## Consequences

- Do not call `FindRepo` or `IdentifyDir` from a timer, a poll loop or a render path. Resolve the repo
  once and keep the result.
- If a hot caller appears, memoize per cleaned absolute path with no TTL. `Root`, `WorktreeDir` and
  `IsBare` cannot change for a path. Only `MainBranch` can go stale, and only if `origin/HEAD` is
  repointed while gx runs. A TTL would bring back the periodic spawning the cache is meant to
  remove.
