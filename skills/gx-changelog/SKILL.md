---
name: gx-changelog
description:
  Draft a CHANGELOG.md entry for the next version by summarizing commits since the last version tag.
  Use when asked to update the changelog.
disable-model-invocation: true
---

# gx Changelog

## Two call shapes

This skill is entered two ways, and its last step differs between them:

- **Invoked directly by a user** (`/gx-changelog`): after drafting and reviewing the entry, write it
  into `CHANGELOG.md` yourself.
- **Invoked as a sub-agent by another skill** (e.g. `gx-bump`): do the same drafting and review, but
  return the drafted entry text to the caller instead of writing it — the caller owns the
  review-pause and commit steps in that case.

Everything through "review the summary" below is identical in both cases; only the final "write vs.
return" step differs.

## Workflow

1. **Summarize the commits.** Dispatch a cheap sub-agent (via the `Agent` tool) to:
   1. List every commit from `HEAD` back to the last version tag (`git log <last-tag>..HEAD`).
   2. Summarize the changes in those commits.
   3. Return the summary as its result.
2. **Review the summary** yourself and adjust it — merge multi-commit features into one entry,
   drop noise, sharpen wording — before it goes anywhere near `CHANGELOG.md`.
3. **Write vs. return** (see "Two call shapes" above):
   - Direct invocation: update `CHANGELOG.md` on disk.
   - Sub-agent invocation: return the drafted entry text to the caller; do not touch
     `CHANGELOG.md`.

## Template

```md
# Changelog

## v1.2.3 - {date}

- Added this
- Changed that
```

## Picking the version/section

- If the invoker names the target version or bump type (e.g. `/gx-changelog patch`, or a
  bump type/version passed in from a caller), use it directly.
- Otherwise, infer from the changelog's own state:
  - If the latest section at the top of `CHANGELOG.md` is already tagged (a matching git tag
    exists), create a new `Unreleased` section above it.
  - If it isn't tagged yet, update that section in place instead of creating a new one.

## What to include

- New features — if one feature spans multiple commits (an initial commit plus follow-up fixes),
  collapse it into a single bullet describing the feature as a whole, not one bullet per commit.
- Bug fixes.
- Dependency updates.
