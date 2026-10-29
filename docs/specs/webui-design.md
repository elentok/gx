## Problem Statement

A person drives gx-design runs on a map epic from an interactive session in the TUI. A run asks a Grilling round, waits for answers, and ends with a "Next:" line that the person acts on. All of this needs a terminal at the machine. Away from the desk, on a phone or on another computer, the person can't answer a round, can't see where a Design stands, and can't tell that a session crashed, hit a login error or stopped making progress. Rounds wait for hours, and a broken session is found only when the person comes back.

The web UI already shows the Queue, the Epics page and ticket pages, and it can answer Paused tickets. It has nothing for design work.

## Solution

A new **Design** page in the web UI. The UI calls a map epic a **Design**. The page has three parts:

- A **Designs picker** that lists Designs in sections: In progress, Not started, Ready to queue and Recently finished.
- **Progress**, the list of gx-design steps for the Design on screen, with the map's tickets under "Resolve tickets".
- A **chat** with the agent's text and the person's prompts. When a Grilling round is waiting, a docked question sheet shows one card per question, with buttons built from each question's `gx-question` block. One Send answers the whole round.

On desktop the picker is a left column, the chat is in the center and Progress is a right column. On mobile the picker is a dropdown at the top, Progress is a one-line bar that expands, and the chat fills the rest.

The page starts design sessions itself: "Start design session" on a Design with no design session, "New session" to start fresh, "New Design" for an idea that has no map yet, and Restore for an Exited session. Sessions started from the TUI or a terminal show up on the same page. When a session breaks, the page shows a banner with a plain reason and a button named after what the person does. The existing Banner and AFK Chat notifications link to the Design page.

Terminology follows the pause-terminology spec (`docs/specs/pause-terminology.md`). Relaunching an Exited session is **Restore**. **Resume** only lifts a pause. The page never uses park, land or drain.

## User Stories

Navigation and the picker

1. As a person, I want a "Designs" tab in the top bar next to Queue and Epics, so that I can reach design work from any web page.
2. As a person, I want the top bar above the three columns on desktop and above the Design dropdown on mobile, so that the Design page looks like the other pages.
3. As a person, I want the bare Designs route to open the first Design in "In progress", so that I start on live work and not on an empty page. When "In progress" is empty, the route shows the picker and an empty state with a "New Design" button.
4. As a person, I want the bare route never to open a Not started Design, so that an old idea does not hide current work.
5. As a person, I want the picker to list the same maps the Epics page lists (no archived, follow-ups or one-off epics), draft maps included, so that the two pages agree.
6. As a person, I want Designs grouped into In progress, Not started, Ready to queue and Recently finished, so that I see at a glance what needs me.
7. As a person, I want "Not started" collapsed by default with a count, for example "Not started (50)", so that dozens of old ideas don't push current work off the screen.
8. As a person, I want a Not started Design with a live session or a waiting question to show under "In progress", so that a Design I just picked up is not hidden.
9. As a person, I want "Ready to queue" to list Designs whose `<epic>-impl` epic exists and is not queued yet, so that I know which ones only need a Queue click.
10. As a person, I want a Ready to queue row to open the implementation epic page, so that I reach the Queue button in one tap.
11. As a person, I want "Recently finished" to list Designs whose implementation epic was queued in the last 7 days, so that a finished Design does not vanish the moment I queue it.
12. As a person, I want a map that has `completed_at` but no `<epic>-impl` epic to count as finished at `completed_at`, so that old maps that were finished by hand leave In progress.
13. As a person, I want Designs older than the 7-day window to leave the picker and stay on the Epics page, so that the picker stays short.
14. As a person, I want a Recently finished row to open the Design page, so that I can read how the Design ended.
15. As a person, I want Designs with a waiting question or a failure first in each section, then Live ones, so that what needs me comes first.
16. As a person, I want the rest of each section sorted newest first (In progress by latest activity, Ready to queue by when the implementation epic was created, Recently finished by when it was queued, Not started by when the map was created), so that recent work is near the top.
17. As a person, I want a pulsing green dot on a Live Design, so that I know an agent session is running for it.
18. As a person, I want an amber "?" mark on a Design with a waiting round, so that I see which Designs need answers.
19. As a person, I want a red "!" mark on a Design whose session has failed (any failure except a rate limit), so that I see broken sessions without opening each Design.
20. As a person, I want a blue ▶ mark on Ready to queue and a blue ✓ on Recently finished, so that the state reads without text.
21. As a person, I want one filter box at the top of the picker that matches the epic slug and the map title in every section, so that I can find a Design among many.
22. As a person, I want a collapsed "Not started" section to open while my filter has matches in it, and the section counts to show only matches, so that the filter finds old ideas too.
23. As a person, I want clearing the filter to restore the collapsed state, so that the picker goes back to normal.
24. As a person on mobile, I want the Design name as a dropdown button at the top that opens the same picker, so that the chat gets the screen.
25. As a person on mobile, I want a "? N more" count beside the dropdown for the other Designs with a question or a failure, so that I know other Designs need me.

Progress

26. As a person, I want Progress to list the eight gx-design steps (Chart, Resolve tickets, Spec, Review, Grill findings, Create tickets, Branch, Done) with ✓, ▶ and ○ marks, so that I know where the Design stands.
27. As a person, I want "Chart · confirm destination" while the Destination is unconfirmed, so that I know the map waits for my confirmation.
28. As a person, I want the map's tickets listed under "Resolve tickets" while it is current, with closed tickets folded into one "✓ N closed" line, so that I see what is left.
29. As a person, I want the current ticket highlighted (the claimed one, else the frontier ticket), so that I know what the agent works on.
30. As a person, I want blocked open tickets to show a lock mark, so that I know why they wait.
31. As a person, I want "Resolve tickets · fog left" when no ticket is open but the map still has unspecified items, so that I know the map is not finished.
32. As a person, I want "Grill findings · 2 of 5" while review findings are open, so that I see how far the grilling got.
33. As a person, I want "Create tickets · N tickets published" while the implementation epic is still a draft, so that I know gx-to-tickets is running.
34. As a person, I want Progress to follow current state only, so that tickets a review sends back show up as open tickets and Spec goes back to ○.
35. As a person, I want an "Open implementation epic" link as soon as `<epic>-impl` exists, in the chat header on desktop and under the Progress bar on mobile, so that I can follow the work.
36. As a person, I want the implementation epic's Queue button off with "Still publishing tickets" while it is a draft, so that I don't queue half a plan.
37. As a person on mobile, I want Progress as a segment bar with a text line such as "Resolve tickets · 03 Chat content (2 of 7)", which expands to the full list on tap, so that it costs one line.

Chat

38. As a person, I want the chat to show the agent's full text, including each question with its options and recommendation, so that I read the same reasoning the terminal shows.
39. As a person, I want the `gx-question` fences stripped from the chat text, so that I read prose and not YAML.
40. As a person, I want my own prompts as bubbles, including the answer lines the page sent, so that I see what I said.
41. As a person, I want slash commands such as `/clear` and `/gx-design <epic>` as small chips, so that they don't read as raw XML.
42. As a person, I want each run of tool calls folded into one muted line such as "· 12 tool calls", naming the latest tool while the session is Working, so that the chat stays readable and I see that the agent is busy.
43. As a person, I want a "Context compacted" divider where the conversation was compacted, so that I know the agent's memory was summarized.
44. As a person, I want a strip above the composer that lists running background shells, Monitors and subagents, so that I know work goes on while the session is Idle.
45. As a person, I want the chat to show only the current conversation of the session, so that a `/clear` between runs starts a clean chat.
46. As a person, I want at most the last 300 messages with an "Earlier messages hidden" line, so that the page stays fast on a long run.
47. As a person, I want the chat to refresh by itself while the agent works, so that I don't reload the page.
48. As a person, I want a Continue button, subtitled with the title of the map's current ticket (or "Review" when none is left), after a run-end line that says `Next: map` or `Next: review`, so that I start the next run with one tap.
49. As a person, I want "Open implementation epic" in place of Continue after `Next: implement`, so that the last step goes to the implementation epic.

Answering a round

50. As a person, I want a question sheet docked above the composer that opens by itself when a round arrives, so that I can start answering at once.
51. As a person, I want to fold the sheet down to its header, so that I can read the chat behind it.
52. As a person, I want a stepper of numbered circles in the sheet header (✓ answered, amber current, outline unanswered), and tapping a circle jumps to that question, so that I can move around the round.
53. As a person, I want one question at a time, switched by swipe or Prev and Next, so that each card fits a phone.
54. As a person, I want the sheet to keep the height of its tallest question, so that it does not jump while I swipe.
55. As a person, I want options as stacked 44px buttons, with "★ recommended" on the option the block recommends, so that I can tap the answer with a thumb.
56. As a person, I want picking an option to move to the next question, so that a round of yes answers is fast.
57. As a person, I want a text box for a text question, so that I can type a free answer.
58. As a person, I want a question whose block is missing or unreadable to show as a free-text card with an "options could not be read" line, so that one bad block does not block the round.
59. As a person, I want a note field on each question, behind a note icon, so that I can add context to one answer.
60. As a person, I want a notes box under the chat, behind a note icon, so that I can add a remark to the whole round or send a plain message.
61. As a person, I want one Send for the whole round, so that the agent gets all answers in one prompt.
62. As a person, I want Send to ask "N questions are unanswered (Q2, Q3). Send anyway?" with "Answer them" and "Send anyway", so that I don't skip a question by mistake.
63. As a person, I want "Answer them" to open the sheet at the first unanswered question, so that I can finish quickly.
64. As a person, I want a blank question sent as "(unanswered)", so that the agent knows I chose not to answer.
65. As a person, I want my draft kept when the page refuses a send because the chat moved on, so that I don't lose my answers.
66. As a person, I want the composer disabled while the session is Working, so that I don't type into a busy agent.

Starting and opening sessions

67. As a person, I want opening the page never to start a session, so that browsing Designs costs nothing.
68. As a person, I want a Design with no design session to show Progress, an empty chat and a "Start design session" button, so that I start one when I choose.
69. As a person, I want the map epic page button to read "Start design session" while the Design has no design session and "Open design session" after it, so that the label says what happens.
70. As a person, I want the map epic page button hidden on a Done Design, so that I am not offered work that is finished.
71. As a person, I want "Open design session" only to go to the Design page, never restoring or creating a session, so that a click never starts something I did not ask for.
72. As a person, I want a double click on Start to start one session, not two, so that I don't get duplicate agents.
73. As a person, I want "New Design" to ask for an epic name and a rough idea and start a design session that charts the map, so that I can start a Design from my phone.
74. As a person, I want New Design to refuse a name that is not a valid slug or that already exists in the chosen project, so that I don't overwrite a map.
75. As a person with several projects, I want New Design to have a project select that defaults to the project of the Design on screen, so that the new Design goes to the right place.
76. As a person with one project, I want no project select, so that the form stays short.
77. As a person, I want a new Design to appear under In progress at the Chart step before its map exists, so that I can follow it from the start.
78. As a person, I want sessions started from the TUI or a terminal on the same Design to show here too, so that I can switch device mid-run.
79. As a person, I want a session started on a map ticket (`<epic>/NN`) to count as a design session of that Design, so that I can answer its rounds from the web.
80. As a person, I want the newest live session to drive the chat and Progress when a Design has two live sessions, so that the page has one clear target.
81. As a person, I want a "2 live" mark in the chat header with a menu to switch sessions or stop the other one, so that I can clean up a duplicate.
82. As a person, I want prompts to go only to the session on screen, so that an answer never lands in the wrong agent.

Failures

83. As a person, I want a banner with a plain reason when the session breaks, so that I know something is wrong without reading logs.
84. As a person, I want "Agent crashed" or "Session ended" for an Exited session, with Restore as the main button and New session beside it, so that I can continue the same conversation or start fresh.
85. As a person, I want Restore to keep a half-answered round, so that I don't answer twice.
86. As a person, I want Restore hidden and New session primary when a Restore crashed again within a minute or the server refuses it, so that I am not stuck clicking a button that can't work.
87. As a person, I want New session to start a fresh session that runs `/gx-design <epic>` and rebuilds context from the tracker, so that a broken conversation does not lose the Design.
88. As a person, I want New session to ask "Stop the current session and start a new one?" when the old session is still live, so that one Design keeps one live session.
89. As a person, I want "Claude login needed" with the login command and a Retry button when Claude returned an auth error, so that I can fix it and continue. Retry sends the text `continue`.
90. As a person, I want "Rate limited" with Claude's own limit message (for example "You've hit your session limit · resets 11pm") and no button, so that I know it lifts by itself.
91. As a person, I want "No progress for 15 min" with Screen and New session when a Working session wrote nothing for 15 minutes, so that I find a hung agent.
92. As a person, I want "Waiting at a prompt in the terminal" with Screen when the session is Blocked, so that I know to answer it in gx.
93. As a person, I want a Screen button that shows a read-only plain-text snapshot of the session's terminal, with Refresh, so that I can see what the agent sees.
94. As a person, I want the banner to tell me to "Open it in gx, Agents tab" for things the web can't do, so that I know where to go.
95. As a person, I want a Done Design to hide the composer and the "Session ended" banner and show "Open implementation epic", so that a finished Design does not look broken.
96. As a person, I want a crash on a Done Design still to show its banner, so that I see real failures.
97. As a person, I want the banner to replace the composer for an Exited session and sit above a disabled composer for a live failure, so that the composer is there when it can be used again.

Non-Claude sessions

98. As a person, I want a codex or opencode design session to show Progress and the failures that don't need a transcript, with "Chat works for Claude sessions only. Open it in gx, Agents tab." in place of the chat, and no composer, question sheet or Continue, so that the page says why there is no chat.
99. As a person, I want sessions the page creates to always run Claude, so that the chat works for them.

Notifications

100. As a person, I want a Banner when a session gets a failure kind, including a stall or an auth error, so that I learn about it away from the page.
101. As a person, I want that Banner once per change into a failure, not again while the failure stays, so that I am not spammed.
102. As a person in AFK mode, I want the AFK Chat message for a design session to carry a link to its Design page, so that I can answer from my phone.
103. As a person, I want a Banner click to focus the TUI when one is connected, and to open the Design page when none is, so that the click takes me to a place where I can act.
104. As a person on Linux, I want the Design page link in the Banner text, so that I can still find it.
105. As a person looking at a Design page, I want no Banner and no AFK Chat message for the session I am looking at, so that I am not told about what I already see.
106. As a person, I want a hidden browser tab not to count as looking, so that I still get notified when I switch away.
107. As a person, I want web tabs never to change where a Banner click goes, so that a phone on the table does not steal clicks from my TUI.

Safety

108. As a person, I want the web to type only into design sessions, so that a phone can't drive any other agent.
109. As a person, I want text that starts with `/` or `!` refused, so that the web can't run slash commands or shell commands in the agent.
110. As a person, I want the server to build the Continue prompts itself, so that the one slash command the page needs does not open a hole.
111. As a person, I want each web send recorded in the server event log without its text, so that I can see who typed what and when.
112. As a person, I want a size cap on web text, so that a bad client can't flood the agent.

Agents and tooling

113. As an agent running gx-grilling, I want a rule that puts a `gx-question` block under every question, so that the web draws buttons for my questions.
114. As an agent running gx-design, I want text after the epic in `/gx-design <epic> <idea>` read as the rough idea for "Chart the map", so that New Design can hand me the idea.
115. As an agent running gx-to-tickets, I want to create `<epic>-impl` as a draft and open it as the last step, so that nobody queues it before every ticket is published.
116. As a person, I want `gx server queue add` to keep refusing a draft epic (`epic-draft`, already on trunk), so that a half-published plan never runs.
117. As a CLI user, I want `gx agents chat <session> --json`, `gx agents screen <id> --json` and `gx designs open <addr> --json`, so that every new web read and write has a CLI twin.
118. As a person, I want `gx tickets migrate` to mark maps whose `<epic>-impl` merged as done, so that old finished maps leave In progress.
119. As a maintainer, I want an unreadable `gx-question` block filed once per round into one aggregated follow-up, so that drift in the skill gets fixed.

## Implementation Decisions

Order and dependencies

- `webui-design-impl` builds on `pause-terminology-impl`, which changed the web views, the generated types and the API version. That epic is merged on trunk.
- The `provisional_destination` to `destination_unconfirmed` rename (gx:follow-ups/77) is on trunk already. This spec uses the new name.

Names

- The UI calls a map epic a **Design**. A **Design session** keeps its glossary meaning: an interactive session with a Session ticket, which can be the map epic or one of its map tickets. A Design is **Live** while one of its design sessions has not Exited, and **Not started** while its Destination is unconfirmed and none of its tickets is resolved. Not started is derived from tracker state. It is not a status, and `draft` keeps its meaning (work set aside deliberately).
- The step overview is **Progress**. A run is one gx-design invocation. "Step" names a Progress step only, which is why the run-end button is "Continue" and not "Next step".

Progress and picker sections (server)

- One function beside `mapListState` returns `{phase, step, section}` for a map. Every `/v1/epics` map row carries it, so the picker needs no request per Design. The epic detail adds the full step list, the map's tickets with their state and the current ticket, and the finding counts.
- The function checks from the last step back and stops at the first match. It must go backwards because the Branch step deletes the spec file from the trunk checkout. Steps and their done rules:

  | Step | Done when |
  | --- | --- |
  | Chart | the map exists and `destination_unconfirmed` is not set |
  | Resolve tickets | no ticket is open or claimed, and `## Not yet specified` is empty |
  | Spec | `docs/specs/<epic>.md` exists in the project's trunk checkout |
  | Review | the map has a `## Review findings` section |
  | Grill findings | every finding is `- [x]` |
  | Create tickets | the `<epic>-impl` epic exists and is no longer `draft` |
  | Branch | `refs/heads/<epic>-impl` exists in the project repo |
  | Done | all of the above |

- Once the branch exists, every step shows done. A map whose status is `done` is Done too, so a map stays Done after its implementation merges and the branch goes away. A map with `completed_at` and no `<epic>-impl` epic (finished by hand) is Done as well.
- The server reads only the trunk checkout and the repo's refs, never worktrees.
- Labels: "Chart · confirm destination", "Resolve tickets · fog left", "Grill findings · N of M", "Create tickets · N tickets published".
- There is no round counter. Progress depends only on current state.
- The current ticket is the claimed ticket, else the frontier ticket (the lowest open ticket whose blockers are all done). Blocked open tickets get a lock mark.
- Phases: Map is Chart and Resolve tickets. Review is Spec through Branch. Implement is Done.
- Sections:
  - **In progress**: the step is Chart through Branch, and the Design is not Not started (or it is Not started but has a live session or a waiting round).
  - **Not started**: the Design is Not started and has no live session and no waiting round.
  - **Ready to queue**: Done, and `<epic>-impl` is not queued and has no later state.
  - **Recently finished**: `<epic>-impl` is queued or later and its `queued_at` is less than 7 days old. A map with `completed_at` but no `<epic>-impl` epic is finished at `completed_at` and stays for 7 days. After the window the Design leaves the picker.
- The map's `completed_at` is stamped when `<epic>-impl` is created, before Branch, so it does not decide the section for a map that has an implementation epic.
- The picker lists the maps `epicListed` accepts, draft maps included.

Schema changes

- New server-stamped epic field `queued_at`, written at the first enqueue and never overwritten. Old epics without it fall back to `started_at`, then `merged_at`.
- `gx tickets migrate` gets a step that copies a merged `<epic>-impl` epic's `merged_at` onto its map and sets the map to `done`. It is idempotent, has a dry run like the other steps, and ignores maps without an `<epic>-impl` epic. The epic merge path applies the same rule when an `<epic>-impl` epic merges from now on, the way `BackfillEpicTimes` mirrors the rule that stamps `completed_at` at create time. Without it, a map whose `<epic>-impl` branch is deleted after the merge would fall back from Done to Branch, because the step function checks the branch ref.
- `gx server queue add` already refuses a draft epic (`epic-draft`, `server/queue.go`). This spec adds no code for it, only a test that a draft `<epic>-impl` is refused.

Skill changes

- gx-grilling requires the header `❓ **Qn** - **title**: body` and a `gx-question` block (the `gx tickets ask` YAML) under every question, after the `➡️` line and before `---`. Most questions are `yes-no` with `recommended: yes`. `options` is for real alternatives.
- gx-design reads text after the epic address as the rough idea for "Chart the map".
- gx-to-tickets creates the implementation epic with `gx tickets epic add <epic>-impl --kind loop --status draft`, and its last step, after the Summary and `ensure-code-review`, is `gx tickets set <epic>-impl --status open`.

Parsing a round

- The server takes the last assistant text block of the turn that contains `❓ **Q`, and splits it at each marker into one card per question. A segment ends at the next marker or at a `---` line.
- Headers are read leniently: the number from `Q(\d+)`, the title from the first bold span after it, else the first sentence.
- A card's type and options come from its `gx-question` block, checked by the same rules `gx tickets ask` applies (`applyBlock`). Only the block's `recommended:` drives "★ recommended". The `➡️` text shows as plain text.
- A bad block is anything `gx tickets ask` refuses, or more than one block in one question. A bad or missing block gives a free-text card with an "options could not be read" note. The other cards stay answerable. A bad block is not a failure state.
- The transcript watcher that raises the grilling Actionable files bad blocks, once per round, into one aggregated `gx:follow-ups` bug, "Design round has an unreadable gx-question block". Each case adds a dated "seen again" line to its Evidence with the session, Design, `Qn`, parse error and raw block. If that follow-up is archived `fixed`, a new one opens, per gx-follow-up's dedupe rules. If no `gx` project is registered or the write fails, the server logs a warning and moves on.

Answer prompt

- One line per question: `Qn - <title>: <answer>`. The answer is the option label, "Yes" or "No", or the typed text. A per-question note goes on the next line as `  Note: …`. A blank question reads `(unanswered)`.
- With answers, the notes box text goes after them under a `Note:` line. Alone, it is sent as a plain message.

Chat route

- `GET /v1/sessions/chat?session=<id>`, CLI twin `gx agents chat <session> --json` (ADR 0028). Response:
  - `messages`: `id` (transcript uuid), `role` (agent, person), `kind` (text, command, tools, compacted), `text`, `tool_count`, `last_tool`, `at`.
  - `round`: the parsed cards, or null. It is set only when the last agent turn ended with a round and no person prompt came after it.
  - `background`: `kind` (shell, monitor, agent), `label`, `started_at`. The source is `transcript.ReadBackgroundTasks`, and markers past its aged-out cap drop off. The label is the tool call's description, else the first 60 characters of the command.
  - `next`: `{phase, action}` from the run-end line, or null. The same parser as the TUI Design Actionable (`designActionable`) produces it.
  - `conversation_id`, so the page can tell that a `/clear` happened.
  - `failure`, the same field `/v1/sessions` carries.
- Visible entries: agent text blocks with `gx-question` fences stripped, the person's real prompts, slash commands as a command entry, and the run-end line as a normal agent message. Thinking, system lines, `isMeta` lines, attachments, `isCompactSummary` prompts and sidechain entries are hidden. A `compact_boundary` becomes a `compacted` entry.
- Each run of tool calls between two text blocks folds into one `tools` entry. No tool inputs or results in v1.
- Only the newest conversation of the session loads, capped at the last 300 visible messages. The server rereads the transcript only when its size or mtime changed.
- Claude only. For a codex or opencode session, the page does not load the chat. It shows the "Chat works for Claude sessions only" text and reads the failure from `/v1/sessions`. The implementer picks what the route returns for such a session. The page also hides the composer, the question sheet and Continue for it. Restore, New session and Screen stay.

Freshness

- A new `session-chat-changed` event, address the session id. While a session is Starting or Working, the server checks the transcript's size every 2 seconds and publishes the event only when it changed. Idle sessions cost nothing.
- The page refetches the chat on `session-chat-changed` and `session-changed`.

Failure field

- The server derives one `failure` field (`kind`, `label`, `text`) on every interactive session. `/v1/sessions` and the chat route return it. It clears when its condition clears.

  | Kind | Detection | Label | Buttons |
  | --- | --- | --- | --- |
  | `crashed` | Exited, exit `crashed` | Agent crashed | Restore, New session |
  | `ended` | Exited, exit `clean` | Session ended | Restore, New session |
  | `auth` | Idle, and the last transcript API error (`transcript.LastAPIError`) has status 401 or `authentication_error` | Claude login needed. Text: "Run `claude /login` on this computer, then Retry." | Retry |
  | `rate-limit` | the existing Rate-limited status | Rate limited. Text: the status message (Claude's limit message, which names the reset time) | none |
  | `stalled` | Working, with no transcript write for 15 min | No progress for 15 min | Screen, New session |
  | `blocked` | the existing Blocked status | Waiting at a prompt in the terminal | Screen |

- `auth` and `stalled` read the Claude transcript, so codex and opencode sessions never get them.
- Retry is a plain text send of `continue` on `sessions/prompt`, with the stale guard and the audit event. It is not the Continue action, which types `/clear`.
- The banner never parses a reset time. When the status message is empty, it shows "Rate limited" alone.
- For `crashed` and `ended`, the banner takes the composer's place. For the live kinds, the banner sits above the composer, which stays visible and is disabled unless the session is Idle.
- Restore relaunches the same conversation through `POST /v1/sessions/resume`. If a Restore crashes again within 1 minute, or the server refuses it (`dir-gone`, `no-resume`), the page hides Restore and makes New session primary. The implementer picks how the server reports the quick re-crash.
- When Progress is Done, the page hides the composer and the `ended` banner and shows "Open implementation epic". A `crashed` banner still shows.
- The banner offers no attach command. It says "Open it in gx, Agents tab" (gx:follow-ups/98).
- A gx-raised Actionable (a Continue that failed, a draft that would not clear) shows as a "gx stopped" note above the composer, with its reason and "Continue tries again." It shows only when no failure banner does. An agent-raised Actionable shows no note.

Screen snapshot

- `GET /v1/sessions/screen?session=<id>`, CLI twin `gx agents screen <id> --json`. It returns plain text rows: the live holder screen (`HolderClient.Screen`), or `screen.txt` once Exited.
- The web shows the rows in a monospace modal with a Refresh button and no auto-refresh. No ANSI colours in v1.

Opening and creating design sessions

- `POST /v1/designs/open`, CLI twin `gx designs open <addr> --json`, a `webWrites` entry. `POST /v1/sessions/create` stays off the web, because it takes a free `dir`, `prompt` and `harness`.
- Body `{epic}`. The server fills every other field the way the TUI "Start session…" prefill does: the trunk checkout (`tickets.TrunkCheckout`) as dir, the short address as name, the map as Session ticket, harness claude, and `/gx-design <epic>` as the first prompt.
- It returns the newest live design session if one exists, else it creates one. Calls are serialized per epic, so a double click starts one session.
- `{epic, new: true}` is New session. It stops a live old session (the page asks for the confirm first), then creates a fresh one. The stopped session stays in the TUI Agents tab as Exited.
- `{epic, idea, new_design: true}` is New Design. The server checks the slug format and refuses `epic-exists` if the chosen project already has that epic. It creates no stub epic. It stores the Session ticket `<project>:<slug>` and sets the session's `Design` flag directly, because `Design` is otherwise computed only from an existing map epic. The first prompt is `/gx-design <slug> <idea>`. The idea has the same size cap as web prompts. The request carries the project. Under the same per-epic lock, when a design session already has that Session ticket, the route returns it and sends nothing, as a double click on Start does.
- The route and the CLI also accept `<epic>/NN`. It matters only on create: the first prompt becomes `/gx-design <epic>/NN`. With a live session, the route returns it and sends nothing. No per-ticket web button in v1.
- Refusals: `not-map-epic`, `unknown-epic`, `epic-exists`, and `server-not-running` from the CLI.
- `designs/open` always creates Claude sessions.
- Until Chart creates the map, a New Design shows under In progress from its newest design session, Live or Exited, with Progress on Chart. If that session exits early, the row stays and the page shows Restore and New session. The row goes when the person stops or removes the session in the Agents tab.
- Opening the Design page never creates or restores a session. "Start design session" calls `designs/open` and shows only while `/v1/sessions` has no design session for the Design. A session removed in the Agents tab no longer counts. The map epic page's button follows the same rule: "Start design session" calls `designs/open` and then goes to the Design page. "Open design session" only goes to the Design page and makes no call. The button is hidden when the Design is Done. When no session is live and the newest one is Exited, the page shows that session with its `ended` or `crashed` banner.
- With more than one live design session, the newest (latest run start) drives the chat and Progress. The picker keeps one row per Design. The chat header shows "N live" with a menu to switch sessions or stop another one.

Sending prompts

- `POST /v1/sessions/prompt`, a `webWrites` entry with a CLI twin. The implementer picks the CLI verb name. It reuses the path that types into a session.
- It works only when the session is Idle, and skips the draft guard in v1 because of the placeholder-as-draft bug (gx:follow-ups/07).
- The route keeps the four web write checks (Host allowlist, `gx_web_<port>` cookie, Origin equal to Host, JSON Content-Type) and adds no new auth.
- Refusals:
  - `not-design-session` when the session's `Design` flag is not set. The flag, not the Session ticket's epic kind, is the check, so a New Design session counts before Chart writes the map.
  - `harness-not-supported` for a non-Claude session.
  - `stale` when the transcript moved past the last message id the send carries. The page reloads the chat and keeps the draft.
  - `prefix-not-allowed` when the trimmed text starts with `/` or `!`.
  - `text-too-long` above the size cap. The implementer picks the number, about 32 KB.
- Continue is its own server-built action on the same route (for example `{"continue": true}` with no text). The server types `/clear`, then `/gx-design <epic>`, the same Action the TUI Design Actionable sends. It carries the stale guard too.
- Each web send adds one event to the server event log: the session, `source: web`, the action (text or continue) and the byte count. Not the text.
- No rate limit and no confirm beyond the round's unanswered confirm.

Web routes

- Reads: `GET /v1/sessions`, `/v1/sessions/chat` and `/v1/sessions/screen` join the web read routes, unfiltered, as the snapshot already carries sessions (`docs/specs/web-ui.md`).
- `webWrites` gains `/v1/sessions/prompt`, `/v1/designs/open`, `/v1/sessions/focus`, and `/v1/sessions/resume` and `/v1/sessions/stop` for design sessions only (the `Design` flag, as for prompts).
- State tokens: `sessions/stop`, `sessions/resume` and `designs/open {new: true}` from the web carry a token built from the session the page saw (its id, its status and when the status last changed). The server refuses `stale` when it no longer matches, as `queueWriteAs` and `repairWriteAs` refuse an empty or old token. Plain `designs/open` and New Design take no token: they are serialized per epic and return the live session when one exists. The implementer picks the token's exact form.

Notifications

- No new channel. Design sessions use the existing Banner and, in AFK mode, Chat. No browser Notification API or Web Push in v1.
- Today's interactive-session triggers stay. One new trigger: a Banner fires when a session's `failure` goes from none to a kind. It fires once per change into a failure and goes through `triggerBanner`, so it merges with other events of the same session.
- A visible Design page sends focus reports to `/v1/sessions/focus`: client is the tab, focused is the tab's visibility, session is the design session on screen. A report counts only while the tab has an open event stream, as a TUI's does. A focused report marks the session read, as it does for a TUI.
- While a web page shows a session, the server skips both the Banner and the AFK Chat message for it. A focused TUI keeps today's rule (it silences the Banner and sound, not Chat).
- Web focus reports never change where a Banner click goes, or the target of `sessions/show`. The server keeps them apart from TUI reports for click targets.
- When the web listener is on, the Banner and the Chat message for a design session carry a link to its Design page, without the token. The link uses the Design's canonical address (ADR 0027), since epic names repeat across projects. The implementer picks the exact route path. A phone that never signed in lands on the sign-in error.
- Banner click: with a TUI connected, the click focuses the TUI, as today. With no TUI connected, it opens the Design page link (on macOS through terminal-notifier). On Linux the link goes in the Banner text only. AFK Chat always carries the link. `osnotify.Notification` gets a link field for this.
- The page follows the machine-wide AFK mode. No AFK toggle on the web in v1.

Web page

- New shared components `Sidebar`, `SidebarSection` and `SidebarItem`. Only the Design page uses them in v1. The sidebar has a raised background and no separator line, and the current item uses the page background. A "+ New" ghost button sits in the sidebar title row.
- The picker builds its rows from `/v1/epics` and the sessions in the snapshot. The "?" mark comes from a session's grilling-round Actionable. The filter runs in the browser over the list the page already loads.
- When more than one project is registered, rows show their project, matching the New Design project select, which shows only then. The implementer picks the exact look.
- Desktop: picker left, chat center, Progress right on a plain background with steps on a vertical line. Mobile: dropdown, segment bar, chat. The implementer picks the breakpoint.
- The question sheet layout, stepper, swipe (horizontal scroll-snap), tallest-question height, option buttons and the composer follow the Design page layout prototype (gx:webui-design/01, branch `prototype/webui-design-layout`, variant D, header H2).
- The page shows the existing disconnected state with buttons disabled when the server goes away.
- No interrupt from the web. The composer stays disabled while the session is Working.

## Testing Decisions

A good test drives the feature from outside and checks what a person or a client sees: HTTP responses, events, Banners, files in the ticket store, and rendered page text. Tests do not reach into parser helpers or React state.

Seams, highest first:

- **Server API through `servertest`.** A server started with `servertest.StartWithStore` and `servertest.WithTestHolder()`, sessions on the fake harness (`agentfake`), and transcripts written into a temp `CLAUDE_CONFIG_DIR`. This covers:
  - `/v1/sessions/chat`: visible entries, tool folding, compaction, the 300 cap, the current conversation only, `round` cards from good, lenient and bad blocks, `next`, background tasks.
  - `session-chat-changed` events while Working and none while Idle.
  - `/v1/sessions/prompt`: each refusal, the answer typed into the session, Continue typing `/clear` then `/gx-design <epic>`, and the audit event without text.
  - `/v1/designs/open`: newest live wins, create, double call makes one session, New session stopping the old one, New Design with the `Design` flag and first prompt, a second New Design returning the first session, `<epic>/NN`, and the refusals.
  - `sessions/stop`, `sessions/resume` and `designs/open {new: true}` refusing a stale state token from the web, and `sessions/prompt` accepting a New Design session before its map exists.
  - The `failure` field for each kind, and the new Banner trigger firing once.
  - Focus reports from a web client silencing the Banner and AFK Chat, and not moving the click target.
  - The bad-block follow-up written to the store once per round.
  - `/v1/epics` rows carrying `{phase, step, section}` for maps in each step, including a map finished by hand, with a real temp git repo for the branch ref.
  - The web mux serving the new reads and writes, and refusing the rest.
  Prior art: `sessions_grilling_test.go`, `sessions_focus_test.go`, `sessions_banner_test.go`, `sessions_afk_test.go`, `sessions_restore_test.go`, `epiclist_state_test.go`, `web_allowlist_test.go` and the `realgit_*` tests.
- **Web page through `appRoutes`.** The Design page rendered with fake fetchers and a fake `Connection`, as `EpicsPage.test.tsx` does. It covers the picker sections, marks, order, filter and the collapsed Not started section; the bare route; Progress labels; the question sheet, unanswered confirm and the exact prompt text sent; the stale refusal keeping the draft; banners and buttons per failure kind and per Done state; Start, Open and New Design; the "N live" menu; the non-Claude text; the map epic page button labels. Prior art: `EpicsPage.test.tsx`, `AnswerForm.test.tsx`, `ActionCard.test.tsx`, `routes.test.tsx`.
- **`gx tickets migrate`** for the merged-impl step, and the merge path setting the map done. `gx server queue add` refusing a draft `<epic>-impl` with `epic-draft`. Prior art: `tickets/migrate_test.go`, `tickets/backfill_epic_times_test.go`.

The CLI twins add no new seam: they call the same server handlers, and a smoke test per verb is enough, as for the other `--json` twins. The skill changes have no automated test.

## Out of Scope

- Implementing this spec is the job of `webui-design-impl`, not this map.
- A full web terminal or live attach. v1 has a screen snapshot only. `gx agents attach` is gx:follow-ups/98.
- Codex and opencode chat: gx:follow-ups/50.
- An "Earlier runs" view of past conversations: gx:follow-ups/75.
- A Screen button on the web ticket page: gx:follow-ups/99.
- Moving other web pages to the shared Sidebar.
- Browser notifications and Web Push, which need HTTPS: gx:follow-ups/109.
- An AFK toggle in the web UI: gx:follow-ups/110.
- Interrupting a Working session from the web: gx:follow-ups/111.
- Transport risks of the web listener (plain HTTP, a year-long cookie, one shared token): `gx:webui-security`.
- The draft guard for web prompts, until gx:follow-ups/07 is fixed.
- A per-ticket "Open design session" button for map tickets.

## Further Notes

- The decisions come from the map `gx:webui-design` and its tickets 01 to 17. Tickets 09 to 15 were folded into 16.
- The Design page layout prototype lives on branch `prototype/webui-design-layout` (commit 02d93a72). Open `#/prototype/design?variant=D&h=2` in `make web-dev` and switch `scenario=` between question, busy, failed and done.
- The round parser rules were checked against 498 past rounds (1,963 questions) in this repo's transcripts: every round sat in one text block, no body held a `---` line, about 1 header in 5 drifted from the strict form, and 9 rounds were followed by a tool call.
- In the store on 2026-10-23, 50 of the 62 maps without an `<epic>-impl` epic were Not started. That is why the Not started section is collapsed and the bare route skips it.
- gx-evolve can't file bad-block reports, because it reads only loop sessions of a finished loop epic.
