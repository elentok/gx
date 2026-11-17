# gx handoff brief

The shared rules for a brief that starts another agent from this session. Read by the
`gx-promote-to-design` skill, the gx-fork skill and the `gx-wayfinder` skill.

Write for a cold reader: the agent that gets the brief has not seen the transcript, so the brief must
make sense without it. Read [gx-unslop](gx-unslop/SKILL.md) before writing it.

The 32 KiB cap and how to shorten a brief are in each writer skill's own `## Failure handling`, not
here.

## The two fixed first lines

Each is one line, alone, in backticks.

A Promote brief starts with:

`Promoted from a plain chat: the decisions below are settled.`

A Design fork brief starts with:

`Forked from a chat or Design session: the decisions below are settled.`

A chat fork brief has no fixed first line.

## Design fork brief

The fixed fork line, then in order:

## Goal
## Decisions
## Open questions
## Handled elsewhere
## Branch and changed files
## Source

## Chat fork brief

No fixed line, then in order:

## Goal
## Decisions are settled
## Open questions
## Working folder
## Start by
## Handled elsewhere
## Branch and changed files
## Source

## Promote brief

The Promote line, then in order:

## Goal
## Decisions
## Open questions
## Branch and changed files
## Source chat

A Promote brief has no Handled elsewhere.

## Section rules

- **Goal**, **Decisions** (each with its reason) and **Open questions**: only the topic's items, not
  the whole session.
- **Working folder** (chat fork): the source's folder, trunk, or a new worktree from trunk. A chat
  fork whose source is a Design session runs in trunk and must not edit the Design branch.
- **Start by** (chat fork): the first step, so the agent does not wait.
- **Handled elsewhere**: one line listing every other topic with its owner. The owner is the source
  chat or a sibling fork, named by topic and chat name, never by link (a chat link changes at Start).
  The fork leaves those topics alone. A Design fork names a Design sibling by the slug proposed as of
  the save.
- **Branch and changed files**: the Design branch starts from trunk, so it cannot see this session's
  uncommitted or unmerged code. No code moves. Name the source's branch in full and list the
  working-tree changes (`git status --short`) plus the diff against the merge-base with trunk
  (`git diff --name-status $(git merge-base HEAD <trunk>)`). On trunk with a clean tree, say there is
  no unmerged code. A fork lists only the files its topic touches, plus one line saying the other
  changes belong to the source or to sibling forks.
- **Source** (**Source chat** in a Promote brief), last: the harness, the conversation id and the
  transcript path, all from `gx agent transcript --json` (`harness`, `conversation_id`, `path`,
  `found`). When `found` is false, leave the path line out; the brief stays valid. For a Design
  source, add the map address and the `<epic>-design` branch.

Use the `GLOSSARY.md` words: Fork, Design, Design session, Workspace, Promote to Design.
