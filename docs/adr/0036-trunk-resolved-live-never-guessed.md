# Trunk is resolved live, in one place, and never guessed

A project's **Trunk** comes from one resolver in `tickets` that reads `project.json` `trunk` on every
use (detecting the remote default branch only when the key is unset) and checks that the branch
exists locally. A configured trunk that names no local branch is an error (**Missing trunk**): the
server stops claiming for the project, a landing parks the ticket `needs-repair`, and every other
caller shows the same error. gx never falls back to the repo default, `main` or `master`, and never
records trunk at claim: a ticket lands on the trunk value read at landing time. `git.FindRepo` no
longer fills `Repo.Trunk`; only `tickets.OpenRepo` does, and `git` helpers refuse an empty trunk.

## Considered Options

- **Fall back to the detected default when the configured trunk is missing** — keeps work flowing,
  but lands a project's work on the wrong branch after a typo or a deleted branch. Stopping is
  visible and clears by itself once the branch exists.
- **Record trunk at claim and land on the recorded value** — protects a running epic from a trunk
  edit, but it is a cache of `project.json` with its own invalidation, and the server reads every
  other project setting live. Landing is ff-only, so a diverged new trunk already refuses.
- **Resolve trunk inside `git`** — `git` cannot read the ticket store (`tickets` imports `git`). A
  caller-side override of `Repo.Trunk` would let a forgotten caller silently use the repo default;
  making `git` refuse an empty trunk makes a missed caller fail loudly.

## Consequences

- Changing trunk while tickets are in flight sends them to the new trunk. If the new trunk is an
  ancestor of the old one, the fast-forward succeeds and carries the old trunk's commits along.
- Only doctor repair and clone may read the repo default branch directly.
