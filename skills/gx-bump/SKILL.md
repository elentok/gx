---
name: gx-bump
description:
  Orchestrate a full release end to end - resolve the bump type, run the test suite while drafting
  a changelog entry, commit it, then create and push the version tag.
disable-model-invocation: true
---

# gx Bump

Orchestrates a full release: bump-type resolution, tests and changelog draft in parallel, commit,
tag+push. Works in any project that uses the `gx` CLI, not only gx's own repo. No preflight
clean-working-tree check - proceed regardless of unrelated uncommitted changes elsewhere in the
tree.

Invoked as `/gx-bump [major|minor|patch] [skip tests]`.

## Step 1: resolve the bump type

Do this first, before anything else runs, so the user isn't left waiting on the rest of the flow
to find out what they're choosing between.

- If `major`, `minor`, or `patch` was passed in the args, use it directly.
- Otherwise, compute the current tag and the three candidate next versions, and ask the user to
  pick, showing the same preview shape `gx bump`'s interactive picker uses:

  ```
  patch v0.28.2 → v0.28.3
  minor v0.28.2 → v0.29.0
  major v0.28.2 → v1.0.0
  ```

## Step 2: draft the changelog entry and run the tests, in parallel

Drafting and testing are both slow and don't depend on each other, so run them at the same time.

First, start [gx-changelog](../gx-changelog/SKILL.md) as a background sub-agent (via the `Agent`
tool), passing it the target version/bump type resolved in Step 1. Per its "sub-agent invocation"
call shape, it returns the drafted entry text instead of writing it to disk.

Then, while it drafts, run the test suite. Skip the tests if the args contain "skip tests" or
`--skip-tests` (e.g. `/gx-bump skip tests` or `/gx-bump patch skip tests`). Otherwise, detect the
project's test command, in this order, and run the first that applies:

1. A `Makefile` with a `test` target - run `make test`.
2. A `package.json` with a `"test"` script - run `npm test` (or the project's package manager
   equivalent).
3. A discernible Go module (e.g. a `go.mod` file) - run `go test ./... -timeout=2m`.
4. None of the above apply - ask the user for the command to run.

Wait for both to finish. If the test suite fails, stop the whole flow immediately and drop the
drafted entry - nothing after this step runs, and nothing was written to disk.

## Step 3: commit the changelog

Don't pause to show the drafted entry - write it into `CHANGELOG.md` straight away, then commit it
directly - this step is not delegated to the `commit` skill:

```sh
git add CHANGELOG.md && git commit -m "changelog: vX.Y.Z"
```

## Step 4: bump and push

```sh
gx bump <type> --yes
```

This creates the tag and pushes the branch and tag to `origin`.
