# Fork: move topics of a chat or Design session into new sessions

Source: wayfinder map `gx:fork` (tickets 01 to 09). `[NN]` marks a point ticket NN settled. `[map]`
marks a point the map itself set (Notes, Out of scope). `[default]` marks a point a ticket decided by
default. `[impl]` marks an implementer's choice: the map left it open and any sound choice is fine.
`[review]` marks a point the design review settled with the person.
Terms (Fork, Chat session, Workspace, Design, Design session, Promote to Design, Queue agent) are in
`GLOSSARY.md`.
## Problem Statement

A chat or a Design session often deals with several topics at once, for example A, B and C. The
agent's context fills up, and the topics get in each other's way. Today the person opens new chats
by hand, retypes what the agent knows about each topic and moves the new chats into the right
workspace. The decisions already made in the session are lost or have to be written again.

## Solution

The person types `/gx-fork for A and B to new chats and continue with C` in a chat or a Design
session. The `gx-fork` skill (a bundled skill for Claude and Codex) writes one brief per topic for a
cold reader. It shows one approval card per fork. After approval it saves each fork as a draft in
the source session's workspace: a chat draft or a Design draft. The person opens a draft, edits the
brief and presses Start. The source session prints one link per draft and keeps working on the
topics it kept. [01] [05]

## User Stories

1. As a person in a chat, I want to type `/gx-fork` with the topics to move, so that the agent splits the work without me retyping what it knows.
2. As a person in a Design session, I want to fork topics out of it, so that a Design that grew too wide can hand parts to other sessions.
3. As a person using Codex, I want the same skill with the Codex prefix, so that Fork works in both harnesses.
4. As a person, I want to name the topics in the command, so that I decide what moves and what stays.
5. As a person, I want the agent to propose a kind per topic, a plain chat or a new Design, so that I do not have to decide each one from scratch. [01]
6. As a person, I want to change the proposed kind, folder or brief of a fork with a note on its card, so that I can correct a proposal before anything is created. [01] [05]
7. As a person, I want one approval card per fork, so that I can approve some forks and refuse others. [05]
8. As a person, I want a fork I do not approve to leave no draft, so that refusing is free. [05]
9. As a person, I want each card to show the topic, the kind, the folder and the first lines of its brief, so that I see what I approve. [01] [05]
10. As a person, I want the agent to write all briefs before it shows any card, so that every card has real brief text. [05]
11. As a person, I want the agent to ask me to group or pick topics when I name more than 5, so that I do not face a long list of cards. [09]
12. As a person, I want every fork to become a draft, so that I can edit the brief and start it when I am ready. [05]
13. As a person, I want each draft to appear in the source session's workspace, so that the forks sit next to the work they came from. [01]
14. As a person forking out of a Design session, I want the drafts in that Design's workspace, not in Default, so that the lookup works for a Design source too. [06]
15. As a person, I want a chat draft page with the brief, a Start button and a Remove button, so that I control when the chat begins. [05]
16. As a person, I want Start on a chat draft to open a chat on the source's harness, exact model and effort, so that the fork thinks at the same level as the source. [06] [08]
17. As a person, I want the new chat to start with the brief as its first prompt, so that it works from its first turn. [06]
18. As a person, I want a chat fork to run in the source's folder by default, so that it sees the same files. [01]
19. As a person, I want a chat fork to run in a new worktree from trunk when I ask for one, so that parallel edits do not collide. [01]
20. As a person forking a chat out of a Design session, I want the chat to run in the trunk checkout, never in the Design worktree, so that its commits do not land on the Design branch. [01] [04]
21. As a person, I want a Design draft to carry the brief as its idea, so that Start opens a Design whose Chart treats the decisions as settled. [01] [04]
22. As a person, I want a failed Start of a chat draft to leave the draft in place, so that I press Start again and nothing is lost. A Design Start refused before the map exists (slug taken, branch clash) also keeps the draft, and I fix the slug there. A Design Start that fails after the map exists is retried from the new Design page, which shows the start error. [05]
23. As a person, I want a failed draft save to be reported while the other approved forks go on, so that one failure does not stop the rest. [05]
24. As a person, I want the chat draft page to show the error of a failed Start and to name the new worktree it left, and the skill to delete nothing, so that I never lose work I might need. [05] [default]
25. As a person, I want the source session to print one line per draft with a link at the end, so that I open each draft with one click. [01] [05]
26. As a person, I want the source session to work only on the topics it kept afterward, so that the split holds. Pass criterion: the source's next turn touches no forked topic. [01]
27. As a person, I want a Design source's map to list what was forked out, so that the Design record shows where topics went. [09]
28. As a person, I want a chat source to get no such record, so that plain chats stay clean. [09]
29. As a forked agent, I want a brief that makes sense without the source transcript, so that I can start at once. Pass criterion: the brief needs no context that only the transcript has, judged against its Handled elsewhere and Decisions lists. [04]
30. As a forked agent, I want a Handled elsewhere section that lists every other topic and its owner, so that I leave those topics alone. [04]
31. As a forked agent, I want the brief to name the source's branch in full and the files my topic touches, so that I find code I cannot see from my folder. [04] [default]
32. As a forked chat agent, I want a Working folder section and a Start by section, so that I know where I run and what to do first. [04]
33. As a forked chat agent whose source is a Design session, I want to be told that I run in trunk and must not edit the Design branch, so that I do not break the map's branch. [04] [default]
34. As a Design agent, I want a fork brief to begin with a fixed line that says the decisions are settled, so that Chart does not ask the person about them again. [04]
35. As a Design agent running Chart, I want to accept both fixed lines (promoted and forked), so that either path works. [04]
36. As a person, I want sibling forks named by topic and chat name in each brief, with no link, so that a chat link that changes at Start never goes stale. [05]
37. As a person, I want the session to show Actionable while the cards wait, so that I notice it needs me. [09]
38. As a maintainer, I want the skill tests to pin the fixed lines, the brief section order and the skill's key commands, so that edits cannot silently break the flow. [09]
39. As a person running a Design, I want topics forked out of it to count as Out of scope for its Chart and spec, so that the Design stops working on a topic that now lives in another session. [review]
40. As a person, I want a refused or unsaved fork's topic to stay with the source, with the sibling briefs corrected, so that no topic is left without an owner. [review]
41. As a person, I want the skill to refuse with one line outside a gx session and from a Queue agent, so that it never forks into the wrong place. [01] [review]

## Implementation Decisions

### Skill `gx-fork`

- A new bundled gx skill, installed and discovered like the other `gx-*` skills, for Claude and Codex. The person runs `/gx-fork for A and B to new chats` (Codex uses its own prefix). The command carries the topics and may carry the kind and the folder. The skill is manual only, like `gx-promote-to-design`. [02]
- Who may fork: a plain chat session and a Design session. A Queue agent may not, since it runs unattended (the `queue` field of `gx agents list --json`). Only a plain chat or a Design session may fork. With no `GX_AGENT_SESSION`, a session missing from `gx agents list --json`, a Queue agent (`queue`), a Fix agent (`fix_of`) or a one-off ticket session, the skill refuses with one line and creates nothing. The skills test pins both refusals. [01] [review] The skill finds its own session with `GX_AGENT_SESSION` and reads the session's project, folder, harness, name, model, effort, and for a Design session its map address, from `gx agents list --json`. [01] [03]
- Steps, in order:
  1. Settle the topics. With more than 5, ask the person to group them or pick 5 before any card. [09]
  2. Propose a kind per topic (chat or Design), a short name per topic, and a folder for each chat fork. The default folder is the source's folder, or the trunk checkout when the source is a Design session. A new worktree from trunk only when the command asks. [01]
  3. Write every brief, following the shared brief doc.
  4. Run `gx agent actionable` so the session shows Actionable while the cards wait. [09] (prior art: `gx-promote-to-design`)
  5. Show one yes-no approval card per fork, in the format `gx-grilling` defines, with a note. Each card shows the topic, the kind, the folder and the first lines of the brief. A note changes that fork, and the agent redrafts it and asks again with a new card. A fork that is not approved gets no draft. No draft exists before its card is approved. [01] [05]
  6. For each approved fork, create the draft with the matching create verb: `gx agents drafts create` for a chat, `gx designs drafts create` for a Design. The brief goes in on stdin, never as a shell argument. A failed save is reported and the skill goes on with the other forks. A refused fork, or one whose save failed, leaves its topic with the source. Before it saves any brief, the skill fixes the Handled elsewhere line in the sibling briefs of refused forks so that it names the source, not a fork that never exists, and tells the person. After a failed save, the skill names each saved draft whose Handled elsewhere line still names the failed fork and gives the line to change on its draft page. [05] [08] [review]
  7. After each draft saves, and only for a Design source, add one line to the map's `## Forked out` section.
  8. Print one line per saved draft with its link. Get the base URL from `gx server web-url --json` (`base_url`, never the setup `url`) and add the draft page path: `#/chat-drafts/<id>` for a chat draft, `#/drafts/<id>` for a Design draft. With no web UI, print the draft id. [05] [07] [review]
  9. Keep working only on the topics the source kept.
- Sibling references: a brief names a sibling fork by topic and chat name, never by link. [05]
- Failure handling, in a `## Failure handling` section: a failed draft save is reported, the rest go on, and nothing is left behind. A brief over 32 KiB is shortened before the call, as in `gx-promote-to-design`. Start happens later on the draft page, after the skill's turn, so the skill never sees a Start failure: the draft page shows the error and names any leftover session or worktree. A failed Start of a chat draft is retried by pressing Start on the same draft: once Start has made a new worktree, the server records its path on the draft, a retry starts in that folder, and the draft page names it. A Design Start refused before the map exists keeps the draft, and the person fixes the slug there. A Design Start that fails after the map exists is retried from the new Design page. There is no retry command, and the skill deletes nothing. [05] [review]
- The skill links `gx-grilling` for the card format and the shared brief doc for the brief rules. Its text follows `GLOSSARY.md` (Fork, Design, Design session, Workspace). [09]
- The Codex bundle entry (the agent metadata file other skills ship) is added the way the other skills have it. [impl]

### Trace on a Design source map

- A fork out of a Design session adds one line per fork to the map's `## Forked out` section. The line has the topic, the kind (chat or Design) and the new session's name. It has no link, since a chat link changes when its draft starts. [09]
- The line is added after each draft saves, not at the approval card. A failed save adds no line, and a retry adds it. [09]
- The skill adds its lines so that earlier lines stay, with `gx tickets section --append` on the map. [09] [impl]
- The section sits outside Decisions so far, since a fork is not a decision. A chat source gets no section and no trace. [09] [default]
- The section is free text in the map body, so no schema change and no Migration step is needed. [impl]
- The line names a Design fork by the slug proposed as of the save, and so do sibling briefs. A later slug edit on the draft page does not change them. [review]
- `gx-wayfinder` reads `## Forked out`. Each forked topic counts as Out of scope for Chart and the spec: Right after the fork, the forking Design session itself updates its map for each forked topic: it adds one Out of scope line, closes the topic's open ticket if there is one, removes the topic from Not yet specified and trims the Destination. Later runs read `## Forked out` and repeat these steps for any forked topic still missing an Out of scope line. If the spec already exists, the design review raises a finding for each forked topic still in it. Chart leaves forked topics out of the Destination and the frontier. [review] (stories/scope changed)

### Shared brief doc

- The Promote to Design brief rules move into one shared doc in the skills folder, named `gx-handoff.md`, read by `gx-promote-to-design`, by `gx-fork` and by `gx-wayfinder`. It is not a slash command. [01] [map]
- Shared section order: Goal, Decisions, Open questions, Handled elsewhere, Branch and changed files, Source. A chat fork adds two sections after Open questions: Working folder (the source's folder, trunk, or a new worktree) and Start by (the first step, so the agent does not wait). [04]
- Handled elsewhere is in every fork brief, chat and Design. It lists every other topic in one line, with its owner: the source chat, or a sibling fork by topic and chat name. The fork leaves those topics alone. [04] [05]
- Decisions and Open questions hold only the topic's items, not the whole session. [04] [default]
- Branch and changed files lists the files the topic touches, plus one line that says the other changes belong to the source or to sibling forks. It names the source's branch in full. [04] [default]
- Source names the source's harness, conversation id and transcript path from `gx agent transcript --json`. When the source is a Design session, it also names the map address and the `<epic>-design` branch. [04] [default]
- A chat fork from a Design source is told that it runs in trunk and must not edit the Design branch. [04] [default]
- A chat fork brief marks its decisions as settled in the Decisions heading text, since it has no fixed first line. [04] [default]
- Fixed first lines:
  - A Design fork brief starts with exactly `Forked from a chat or Design session: the decisions below are settled.` [04]
  - A Promote to Design brief keeps `Promoted from a plain chat: the decisions below are settled.` unchanged. [04]
  - A chat fork brief has no fixed first line. [04]
- The `gx-wayfinder` Chart rule treats a brief with either line as settled. Both lines live in the shared doc, the skills that write them and the Chart rule. [04]
- The Promote to Design skill keeps its behaviour. Only its brief rules move to the shared doc. A Promote brief leaves out Handled elsewhere and keeps the heading "Source chat". The skills test pins this. When the rules move, its link is built from `base_url`, not the setup `url`. [01] [review]

### Chat drafts (new)

- A chat draft is a server record, like a Design draft. It holds the project, the folder choice (source folder, trunk checkout, or a new worktree from trunk), the harness, the exact model, the effort, the name and the brief. The workspace is a membership entry, as for a Design draft. Exact field names are [impl]. [05] [06] [07]
- Chat drafts get new server routes to create, edit, start and remove them, a new `gx agents drafts create` verb, a sidebar row in the draft's workspace and a draft page. Existing commands do not change, and `gx agents create` stays as it is. [05] [08]
- `gx agents drafts create` reads the brief from stdin (cap 32 KiB, like `gx designs open --idea -`) and takes the project, folder choice, name, harness, model and effort. It sends `from_session` from `GX_AGENT_SESSION`, so the server puts the draft in that session's workspace. Flag names are [impl]. [08] [default]
- The workspace member key of a chat draft is `chat-draft/<id>`. The two places that today treat any `draft/` key as a Design draft stay as they are, so a chat draft is never treated as a Design draft. The workspace store learns the chat draft key kind: `joinable` accepts it (sidebar Move works), `pruneEpicMembers` drops it when its draft is gone, Hold treats it like a draft row, and Move publishes a drafts event. A `chat-drafts-changed` event and chat draft rows in the snapshot carry the changes; Create, Move, Start and Remove publish the event. Names are [impl]. Server tests cover each. [08] [review]
- Create allows an empty brief (the 32 KiB cap and the `!` prefix rule still apply to a non-empty brief), and Start of an empty draft is refused. Create takes an optional `workspace` name: with no `from_session` the draft joins that workspace, with neither it lands in Default, and with `from_session` the source's workspace wins. Folder `source` with no `from_session` resolves to the project root. Missing harness, model and effort take the defaults of the web New chat form. [21] [23]
- A chat draft page route is `#/chat-drafts/<id>`. After Start the server keeps a redirect record to the new chat, as Design drafts keep one for 7 days, so a printed link keeps working. [review]
- The sidebar row and the page chrome (Start and Remove buttons, autosave, revision flow) are shared code with Design drafts. Each draft kind keeps its own verbs. [08]
- The draft page shows the project, folder, harness, model, effort and name, and lets the person edit the brief. The page edits all of them (project, a folder choice with a worktree name, harness, model, effort, name) in a settings form. Edits autosave with the brief through the save route, under one revision; a field the save omits stays unchanged, and a save never touches the recorded worktree path or start error. Once Start has made a worktree the folder shows as that worktree and is read-only. The person can still change a fork with a note on the approval card, before the draft exists. An empty draft is a normal state: the brief shows its placeholder and Start stays disabled with its blocker text. [22] After a successful Start the page goes to the new chat, as a Design draft goes to its Design. [05] [impl]
- A Start request that creates a chat is a server call. The web chat create request gets optional `model` and `effort` fields, and Start uses them, so the chat starts on the source's exact model and effort from its first turn. No `gx agents switch` follows. `gx agents create` takes no new option. [08]
- Start places the new chat in the draft's workspace, runs it in the chosen folder, types the brief as the first prompt and deletes the draft. A failed Start keeps the draft. A chat started with an explicit model or effort is marked as switched by hand (`SwitchedByHand = true`, set by `applyChosenModel` in `server/sessions/sessions.go`), so the chosen model is not overridden later. [05] [06]
- The brief cap on Start is the web create route's existing 32 KiB cap. [03] [06]

### Design drafts

- A new `gx designs drafts create` verb reads the idea from stdin (cap 32 KiB, like the chat verb) and sends `from_session` from `GX_AGENT_SESSION`, as `gx designs open` does. The draft joins the source session's workspace. It takes a project and a slug, which the skill proposes for a Design fork like Promote to Design does. The person can edit both on the draft page. Flag names are [impl]. [08] [impl]
- Start of a Design draft is the existing path. The brief becomes the map's `## Initial prompt`. [05]

### Workspace lookup fix

- The `from_session` lookup of `gx designs open` and of the new draft verbs resolves a Design session to its workspace through its map address. Today a Design session id is not a workspace key, so a Design started from a Design session lands in Default. This is a server change. It fixes Design forks and Promote to Design from a Design session, and chat drafts use the same lookup. [03] [06]

### Web + New menu

- The sidebar `+ New` menu lists New design, New chat, New chat draft, then a separator and workspace. New chat draft creates an empty chat draft in the active workspace (default project, harness Claude, folder `source`) and opens its page; on an empty chat draft page it only focuses the brief. A failed create shows an error toast. The New chat dialog stays. No command palette entry or chord. [23]

### Not changed

- `gx agents create` and its server routes keep their options. [08]
- No parent edge between the source and a fork. The brief names the source. No "forked from" link in the web sidebar. [02] [01]
- No web button in the session header. The command needs topic text, so the person types it. [impl]

## Testing Decisions

- A good test checks external behaviour: what the person sees on the web, what a verb prints and returns, what a route stores. It does not check component internals or skill wording beyond the pinned rules below.
- Seams, highest first:
  - Web e2e flows through the fixture server, for the chat draft page and row, and for a draft created from a Design session. Extend the Workspaces flow for "Fork drafts in the source workspace". "Chat draft page" is a new flow, since it needs a server round trip.
  - CLI and server verb tests for `gx agents drafts create` and `gx designs drafts create`, with the server route table parity entries, and a server test for the `from_session` lookup (a plain chat source, and a Design source).
  - A server test that Start of a chat draft passes the model and effort to create, places the chat in the workspace, deletes the draft on success and keeps it on failure.
  - The skills package tests (bundle, rules and duplicate checks) for `gx-fork`, the shared brief doc and the edited wayfinder and promote skills.
- The skills package tests pin: [09]
  - both fixed first lines (Design fork and Promote to Design) in the `gx-fork` skill and the shared brief doc, and the two lines the wayfinder Chart rule accepts, plus a test that fails when any copy drifts;
  - the brief doc's section order, with Handled elsewhere, and Working folder and Start by for chat forks;
  - that the `gx-fork` text names `gx agent actionable`, the approval card before any draft, both draft create verbs, `gx server web-url --json`, the 5-topic cap, the `## Forked out` section, a `## Failure handling` section, and links to `gx-grilling` and the brief doc;
  - that the `gx-wayfinder` and `gx-promote-to-design` skills link the shared brief doc;
  - that `gx-wayfinder` names `## Forked out`, that `gx-fork` refuses with one line outside a plain chat or Design session, and that it fixes sibling Handled elsewhere lines of refused forks.
- The web unit tests cover the chat draft row and page states (editing, Start, Remove, a failed Start) with a draft fixture.
- The skill's conversation (the cards, the redraft, the failure paths) cannot run in CI, since the agent is a model. Verification runs it by hand against a temporary server.
- Prior art: `promote_to_design_rules_test.go` and the `gx-promote-to-design` skill test, the Design draft store, route and page tests, the `gx designs open` tests, the web chat create route tests, and the flow catalog's Design draft flows.

## Verification

No stored mock exists, so there is no Mock states list. A verify ticket runs these against a temporary
gx server. New flow numbers: the implementer takes the next free numbers in `webui/e2e/CATALOG.md`.
Each missing fixture below is a new scenario, listed outright.

Plain chat: cards and the topic cap (stories 1 to 1, 4 to 11, 37 to 37)

Manual run in a fixture plain chat. Type `/gx-fork for A and B to new chats`. Expect one card per fork with the topic, kind, folder and brief lines, Actionable while waiting, a redraft after a note, no draft for a refused card. Name 6 topics: expect a request to group or pick 5 before any card.

- New flows or scenarios: none (needs a real agent).

Design source and Codex (stories 2 to 3)

Repeat the manual run in a Design session and in a Codex chat. Expect the same cards and drafts, and the Codex prefix in Codex.

- New scenarios: a fixture Design session and a Codex plain chat.

Drafts in the source workspace (stories 12 to 14)

CLI checks. `gx agents drafts create` and `gx designs drafts create` with a brief on stdin inside a fixture chat, then `gx server snapshot --json`. Expect each draft in the source's workspace. Repeat inside a Design session. Expect the Design's workspace, not Default. Outside a gx session, expect no workspace (Default). A Design draft over 32 KiB is refused by the verb.

- Extend the Workspaces flow with "Fork drafts in the source workspace": a workspace holding a plain chat and a Design session. Create drafts from each. The sidebar shows the draft rows under that workspace, and Move on a chat draft row works.
- New tests: a server test for the `from_session` lookup with a Design source, a verb test per create verb, the parity entries, and workspace store tests for join, prune, hold and Move events of a `chat-draft/<id>` key.

Chat draft page and Start (stories 15 to 17, 22 to 22)

New flow "Chat draft page". Scenario: a chat draft with a Claude model and effort. Open `#/chat-drafts/<id>`. Edit the brief. Press Start. Expect the page to go to a new chat on that exact model and effort whose first prompt is the edited brief, and the draft gone. The old draft link redirects to the new chat. Make Start fail after the new worktree exists (new fixture hook): the draft stays, the page names the worktree, and Start works again in that folder. For a Design draft, a Start refused for a taken slug keeps the draft; a Start that fails after the map exists leaves the new Design with its start error and the retry is on the Design page.

- Same flow with a Codex chat draft.
- New scenario: a chat draft in a plain workspace, with a Claude and a Codex variant, and a Start-failure hook.
- Remove: press Remove on a draft. Expect the row and the draft gone and the chat never created.

Folder choice (stories 18 to 20)

CLI check of the folder choice. Create a chat draft with the default folder, then with a new worktree, then from a Design source. Start each. Expect the chat's folder to be the source's folder, a new worktree from trunk, and the trunk checkout (never the Design worktree).

- New tests: a server test per folder choice.

Design draft as idea (stories 21 to 21)

CLI check. Create a Design draft, Start it, and read `gx tickets show <project:epic>`. Expect the brief under `## Initial prompt` starting with the Design fork fixed line.

Failures and links (stories 23 to 26)

Manual run in a fixture chat with one forced draft-save failure and one forced Start failure. Expect the failure reported, the other forks saved, one printed line with a link per saved draft, the source working only on its kept topic afterward (its next turn touches no forked topic), and the skill deleting nothing. Open each printed link and expect the draft page. The draft page of the failed Start shows the error and names any leftover.

- New scenario: a hook that forces a draft-save failure in the fixture.

Forked out trace (stories 27 to 28)

Manual run, then `gx tickets show <project:epic> --section "Forked out"` on a Design source map. Expect one line per saved fork (topic, kind, name), none for a failed save, and earlier lines kept after a second `/gx-fork`. Repeat from a plain chat: expect no `## Forked out` section anywhere.

Brief content (stories 29 to 33, 36 to 36)

Manual run, then read each draft's brief on its draft page. Expect the shared section order, a Handled elsewhere section that lists the other topics with their owners and names siblings by topic and chat name with no link, the source branch in full, and Working folder and Start by in chat briefs. For a Design source, expect the map address, the `<epic>-design` branch and the trunk warning. Pass: the brief needs no context that only the transcript has.

Fixed first lines (stories 34 to 35)

Skills test (see Testing Decisions) plus manual. A Design draft brief starts with the Design fork line. In the new Design session, Chart treats the decisions as settled. A promoted brief still works the same way, with no Handled elsewhere and the heading "Source chat".

Skill tests (stories 38 to 38)

Run `go test ./skills`. Expect the pinned rules to pass, and the drift test to fail when one copy of a fixed line changes.

Wayfinder reads Forked out (stories 39 to 39)

Manual run in a Design session whose map has an open ticket and a Not yet specified item for the forked topic. Fork it. Expect the map to get one Out of scope line, the ticket closed, the item removed and the Destination trimmed at once. Run the Design again with a `## Forked out` line that lacks an Out of scope line: expect it added. A skills test pins that `gx-wayfinder` names `## Forked out`.

Refused and failed forks (stories 40 to 40)

Manual run with three topics: refuse one card and force a save failure on the second save. Expect the first brief to name the source, not the refused fork, in its Handled elsewhere line. Expect the person told which saved draft still names the failed fork and which line to change. The refused and failed topics stay with the source.

Refusals (stories 41 to 41)

Run the skill with `GX_AGENT_SESSION` unset, from a Queue agent, from a Fix agent and from a session missing from the list. Expect one refusal line and no draft each time. A skills test pins both rules.

## Out of Scope

- Messages between the source session and its forks. `gx agents prompt` types only into an Idle Claude session, so reliable messaging needs new server work. Each brief names the source, so the person relays by hand. [map]
- A standalone `/gx-handoff` slash command. The brief rules live in the shared doc only. [map]
- Forking from Queue agents. They run unattended. [map]
- A "Fork" button in the web session header. The command needs typed topics. [impl]
- Any TUI view of chat drafts. The web is where a draft is edited and started, and the sidebar already lists chat drafts. [review]
- A "forked from" link or a parent edge in the sidebar or the tickets. [01] [02]
- A retry command for a failed Start. The person presses Start again. [05]
- Deleting leftovers of a failed Start. [05]

## Further Notes

- Name: the action keeps the word "fork" because a ticket that has too much and a session that has too much are one idea. `GLOSSARY.md` has one Fork concept with two kinds. [02]
- Drafts, not direct starts: the person asked for drafts so that every fork can be edited before it costs a session. This replaces the earlier idea of new `--from-session`, `--model` and `--prompt -` options on `gx agents create`. [05] [06]
- The wayfinder Chart rule edit is small: it accepts a second fixed first line. [04]
