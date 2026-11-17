---
name: gx-promote-to-design
description: >-
  Promote to Design: turn the topic of this plain chat into a new Design. Drafts a slug and a brief,
  asks for approval on a question card, then starts a Design session from the brief. Run
  `/gx-promote-to-design`, no argument. Manual.
disable-model-invocation: true
---

# gx Promote to Design

`/gx-promote-to-design` turns the topic of this plain chat into a new Design. The chat stays a plain
session: this skill does not stop it, link it or convert it. It takes no argument and touches no code.

Read [gx-unslop](../gx-unslop/SKILL.md) before writing the brief, and
[gx-grilling](../gx-grilling/SKILL.md) for the question-card format.

## Steps

1. **Draft the proposal**: a slug, a project and the brief. The project is this session's project
   unless the person named another. The slug is a short kebab-case name for the feature.
2. **Run `gx agent actionable`** so the session shows Actionable while it waits. Outside a gx session
   it does nothing.
3. **Ask for approval** on one yes-no question card in the format `gx-grilling` defines. Show the
   slug, the project and the whole brief in the question. The card is the last thing in the turn: no
   tool call, no text after it. A note instead of Yes means redraft, then ask again with a new card.
4. **On Yes**, pipe the brief into the command. Never pass it as a shell argument: a multi-paragraph
   brief with backticks and quotes breaks there. Write the brief to a scratch file first, then:

   ```
   gx designs open <slug> --idea - --project <project> < <brief file>
   ```

5. **Last message**: link the new Design page. Get the web base URL from `gx server web-url --json`
   (its `base_url` field) and add `/#/d/<project>:<slug>`. With no web UI, print `<project>:<slug>` only.

## Failure handling

- **Refused slug** (taken or invalid): propose another and ask again with a new card.
- **Map created but the session did not start**: the command exits non-zero, printing
  `<project>:<slug>` and the start error. Report the error and the address. Create no second map.
- **Brief over 32 KB**: shorten it before the call. Never let the call reject it.

## The brief

The first line is exactly this sentence, which marks the brief as promoted:

`Promoted from a plain chat: the decisions below are settled.`

The `gx-wayfinder` Chart rule quotes the same sentence; a skills test fails when they differ.

The section order and the rule for each section are in [gx-handoff](../gx-handoff.md). A Promote
brief ends with **Source chat** (from `gx agent transcript --json`) and leaves out the fork-only
sections.

Use the `GLOSSARY.md` words: Promote to Design, Design, Design session.
