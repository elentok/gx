---
name: gx-start-design
description: >-
  Start Design: propose one Design theme from an area's open tickets, ask for approval on a question
  card, then create the charting map and move the approved tickets in. Run
  `/gx-start-design <project:area> [hint]`. Manual.
disable-model-invocation: true
---

# gx Start Design

`/gx-start-design <project:area> [hint]` picks one theme from the open tickets of an area and turns
it into a Design. The first word is the area's address. Anything after it is the person's hint for
the theme. This skill never writes tickets itself and never starts the Design.

Read [gx-unslop](../gx-unslop/SKILL.md) before writing the reasons, and
[gx-grilling](../gx-grilling/SKILL.md) for the question-card format.

## Steps

1. **Read the area's open tickets**: the draft and open ones. Use `gx tickets list <project:area>`
   and `gx tickets show <addr>` for the ones you need. Skip done, claimed and paused tickets.
2. **Choose one theme** that fits the hint, if there is one, and a title for it. Check the title's
   map name with `gx tickets epic next-name <project:name>`; use the name it prints.
3. **Pick the tickets**: 8 to 14 that belong to the theme. An area with fewer than 8 open tickets
   proposes all of them.
4. **Run `gx agent actionable`** so the session shows Actionable while it waits. Outside a gx session
   it does nothing.
5. **Ask for approval** on one yes-no question card, "Start this Design?", in the format
   `gx-grilling` defines. Show the title, the map name and the numbered tickets (`1.`, `2.`, ...),
   each with its address and a one-line reason it belongs to the theme. The card is the last thing in
   the turn: no tool call, no text after it.
6. **A typed edit** (drop or add tickets, change the title) is not a Yes. Apply it and print the card
   again **once** for that edit. Never reprint more than once per edit. A further edit gets one more
   reprint.
7. **On Yes**, call the create verb with the map name, the title and the ticket ids:

   ```
   gx tickets start-design <project:area> --map <name> --title "<title>" --ticket <id>,<id>,...
   ```

   If it fails after moving some tickets, run it again with the same `--map` and no `--ticket`: it
   carries on and creates no second map.
8. **Last message**: link the new Design page. Get the web base URL from `gx server web-url --json`
   (its `url` field) and add `/#/d/<project>:<map>`. With no web UI, print `<project>:<map>` only.

## Rules

- One theme per card. Never mix themes to reach 14.
- Never start the Design. The person does that from its page.
- Use the `GLOSSARY.md` words: Design, area, charting map.
