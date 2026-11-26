## Problem Statement

A person follows a running session in the gx web UI and wants to open its subagents. This is hard today.

- "Back to parent chat" on a subagent page always opens the chat page. A person who came from a Design page loses their place.
- On the chat page the subagent links sit inside the folded "N tool calls" group, so the person does not see them.
- The block below the chat lists each subagent twice or splits it across places: link lines in the log, a Working row that says "waiting for sub-agent", a "N of M tasks done" summary and rows behind a fold.
- A background subagent's row often shows only the word "subagent" and a time, with no name and no link. Claude Code often writes `run_in_background` as the string `"true"`. gx then drops every tool call of that transcript line.
- The current call of a subagent shows only "Read" for a file tool, so the person cannot tell which file it reads.
- The current call is read only when the parent's transcript grows. It stays old while the parent waits for its subagents.

## Solution

One card below the chat, the working card, replaces the Working row and the task list. It has the same look on the chat page and on the Design, Initiative and Triage step pages.

- The header reads "Working: <step> · <title>" with a spinner.
- Each running subagent has two lines. The first has a dot, the subagent's name as a link and the elapsed time. The second has its current call.
- Other tasks use rows in the same card. Finished tasks are grey rows below the running ones.
- The card is at most about 40% of the screen height. Its rows scroll and the header stays.

The log shows a subagent's link only after the subagent finishes. While it runs, the link is only in the card.

"Back to parent chat" returns to the page the link came from. The current call updates while a page of the session is open. Subagent rows have their names again.

## User Stories

Working card

1. As a person watching a session, I want one card below the chat for the Working state and the tasks, so that I read the state in one place.
2. As a person, I want the card header to read "Working", with the step, the title and any other tool in flight, so that I know what the agent does.
3. As a person, I want each running subagent listed once, with its name, its elapsed time and its current call, so that I can follow it without opening it.
4. As a person, I want a running subagent with no current call to show only its first line, so that I see no empty line.
5. As a person, I want the name of a running subagent to be a link, so that I can open its page.
6. As a person, I want shell, Monitor and call tasks in the same card without a page link, so that I see everything the agent waits for.
7. As a person, I want to click a shell, Monitor or call row to see its details and command, so that I keep the information I have today.
8. As a person, I want finished tasks as grey rows with their mark and time below the running ones, so that I see progress.
9. As a person, I want the summary line and its fold gone, so that the rows show without a click.
10. As a person, I want a busy session with no tasks to show only the card header, so that the card stays small.
11. As a person, I want tasks that run while the agent is not busy to show without the Working header and without the moving border, so that I am not told the agent is working.
12. As a person, I want no card when the agent is not busy and has no tasks, so that the chat has the room.
13. As a person on a phone, I want a long card capped with its own scroll, so that the chat stays readable and the header stays in view.
14. As a person, I want the card on the chat page and on the Design, Initiative and Triage step pages, so that every page works the same way.
15. As a person answering questions on a step page, I want the same card in the Questions view, so that I still see the running work.
16. As a person with a Codex session, I want the same card, including the current call of each running subagent, so that both agents work the same way.
17. As a person with an old Codex session that has no task list, I want the card header with the current tool, so that I keep what I see today.

Links in the log

18. As a person, I want a finished subagent's link where the subagent was started, so that I can read it later.
19. As a person, I want a running subagent to have no link in the log, so that it shows once.
20. As a person on the chat page, I want a finished subagent's link below the folded tool group, so that I find it without opening the group.
21. As a person who opens a tool group, I want each subagent link once, so that nothing repeats.
22. As a person who opens a tool group while a subagent runs, I want its call as plain text, so that the list matches the call count.
23. As a person, I want a subagent call that gx has no task record for to show as plain text, so that I never click a link to a "no such subagent" page.

Names

24. As a person with a Claude session, I want every background subagent row to show its description and link, so that I know which one it is.
25. As a person, I want one unreadable tool call input to leave the other calls of the same line intact, so that rows do not vanish.

Current call

26. As a person, I want a file tool's current call to show the tool and the file path, so that I know which file it touches.
27. As a person, I want a long file path cut at its start, so that I still see the tool and the file name.
28. As a person, I want the same call text in the card, the Working line and the subagent page, so that one call reads one way.
29. As a person with a chat or step page open, I want a subagent's current call to update while the parent is quiet, so that the card is not stale.
30. As a person who runs gx on a laptop, I want gx to check subagent files only while a page of the session is open, so that gx does no work nobody sees.

Back link

31. As a person who opened a subagent from a Design, Initiative or Triage page, I want "Back to parent chat" to return there, so that I keep my place.
32. As a person who opened a subagent from the chat page, I want the back link to return to the chat page.
33. As a person who opens a pasted subagent URL with no origin, I want the back link to open the chat page, so that it always leads somewhere.
34. As a person who reloads the page or opens the link in a new tab, I want the back link to keep its target, so that it does not change under me.
35. As a person on a nested subagent, I want each back link to go to the parent subagent and the last one to the page I started from, so that I can walk back.
36. As a person on a subagent page that cannot show its content, I want its back link to follow the same rule.

## Implementation Decisions

### Working card

- One card component replaces the bottom block of the shared chat view and of the Questions view. The old task list and its summary are removed.
- The card is the app's existing card with the muted tone. It has the moving border only while it shows the Working header.
- The header shows under the same condition as today's Working row. It reads "Working", then the step and title when the page has them, then the newest other tool in flight with today's "(+N more)" popover. It never reads "waiting for sub-agent".
- Rows follow the task group as the server sends it. Running rows come first and finished rows follow, each in launch order. The elapsed time ticks in the browser.
- A running subagent row has a pulsing dot, the linked name and the time. Its second line is the current call in dim monospace. A subagent with no call id shows its name as plain text.
- A running shell, Monitor or call row has a dot, its label in monospace and the time. It has no kind word.
- A finished row is grey and has the mark, the label and today's time text, including "exit N" and "killed". A finished subagent row has no link.
- Shell, Monitor and call rows open their details and command block on click, as today. An open row stays open across a chat reload. Subagent rows do not open.
- The card, header included, is at most 40% of the viewport height. The rows and any open details scroll inside it. The header does not scroll.
- The card lists only the parent's own tasks. Nested subagents stay on the subagent pages.

### Links in the log

- The chat read tells, for each subagent call of a tools entry, whether the subagent runs, has finished, or has no task record. The server decides this on each read with the same end rules as the task rows, so a call leaves the card and gets its link at the same moment. The cached transcript snapshot holds no verdict. The wire names are the implementer's choice.
- On the step pages, which hide tool calls, a finished call shows its link line, a call with no task record shows the same line as plain text with no arrow, and a running call shows nothing.
- On the chat page, the lines of finished calls show below the folded tool group. When the group is open, the line in the call list is the link and it is not repeated below. Running calls and calls with no task record show in the open list as "Agent · type · description" in plain text with no arrow.
- A decoded archived Codex conversation has no task records, so its subagent calls are plain text.
- The subagent page keeps today's log links, running nested calls included.

### Claude task records

- The reader accepts `run_in_background` as a boolean or as the string form.
- The reader decodes each content block of an assistant line on its own, and one bad input does not drop its siblings.
- A launch result with no readable call line takes its label from the result's description.

### Current-call text

- Read, Edit and Write show the tool name, a space and the file path. Shell calls show the command alone. All other tools stay as today.
- A path inside the session's working directory shows relative to it. Any other path shows as the call wrote it.
- The 60-character limit stays. A file label that is too long loses whole folders from the start of the path, as in "Read …/chat/tasks.go". If the file name alone does not fit, the start of the name is cut. Other labels are cut at the end, as today.
- The same rule holds on a narrow screen. When the browser must shorten a file label, it shortens the path from its start and keeps the tool and file name. How the wire marks the parts is the implementer's choice.
- One place builds the label. The card, the Working line, the popover of other tools and the subagent page's Working row all show it.
- The tool-call lines of the log do not change.

### Codex current call

- The chat read fills the current call of each running Codex subagent from its child rollout. It reads only the child's own lines, from the child's history start ordinal, and applies the same open-call rules as the parent's rollout, by the child's Codex version. The inherited parent history never counts.
- It follows the existing file cache, so an unchanged child rollout is not parsed again.

### Live updates while a page is open

- The existing subagent watch gets a second scope, a session. The chat page and the step pages open it for the chat session they show. A subagent page keeps its watch on one call.
- The watch uses today's write route, client stream id, refusal for an unknown client and chat poll interval. An unknown session is refused. A session watch needs no call id.
- On each tick the server finds the session's running subagents with the task rows' end rules, finds their files, and compares size and change time. It does not parse those files.
- When at least one file grew, or a missing file appeared, the server sends one existing subagent-changed event to that client for the session. The page reloads the chat. One tick sends at most one event.
- A file that exists when the watch first sees the subagent at opening is the baseline. A file that appears later counts as growth.
- The watch stays open while nothing runs, so a later subagent is picked up with no new request.
- The watch ends when the page closes or leaves the session, on the page-hide event, and when the client's last event stream closes. A new stream opens it again. A browser tab that is open but hidden counts as open.
- Closing one page's watch never ends the watch of another page, even when two requests for the same client and session arrive out of order. Moving from a Design page to the chat page of the same session keeps a watch running. The way a watch is told apart is the implementer's choice.
- With no page open, gx checks no subagent file. The existing poll of the parent transcript does not change.

### Back link

- Every subagent link carries the address of the page it is on, as a query parameter of the subagent URL. The parameter name and value form are the implementer's choice.
- Links on a subagent page to other subagent pages, which are the log links, the breadcrumb levels and the back link to a parent subagent, pass the origin on unchanged.
- The back link of a nested subagent goes to its parent subagent. The back link of a first-level subagent goes to the origin. The pages for a missing subagent and an unreadable parent follow the origin too.
- With no origin, or with a value that is not a chat, Design, Initiative or Triage page of this app, the back link opens the session's chat page.
- The link text stays "Back to parent chat". The breadcrumb's session name and the "chat page" link for an archived parent keep opening the chat page.
- The page the person returns to opens in its normal state.

## Testing Decisions

- A good test checks what the chat read returns, what the watch sends and what the page shows. It does not check cache shapes or timing.
- Server seam: the chat read over fixture transcripts, sidecar files and rollouts. It covers the three subagent call states, the labels, the Codex current call and the string form of `run_in_background`. Prior art: the chat internal tests for subagent marks, details and Codex children, and the transcript task record tests.
- Watch seam: the subagent watch with a temporary server. Prior art: the existing subagent watch tests. It covers growth, a late file, no event for a quiet or finished subagent, stream close, and two overlapping watches of one client.
- Web seam: the shared chat view with fixture chat responses, and the subagent page. Prior art: the task list, chat log, working row and subagent page tests. It covers card states, log links per state, links with an origin and the watch requests.
- E2E: update flows 208 "Task list" and 216 "Subagent page", and the Working row steps of flows 14 and 75. Add the new flows named below. The repo's e2e skill and flow budget rules apply. Run Go tests with `scripts/proc-cap`.

## Verification

Entries refer to Mock states by state name and do not repeat the look check. "Chat read" is the chat of a fixture session on the temporary server. The CLI check is `go run . agents chat <session> --json`, expected exit code 0 and the same content as the chat read.

Card content (stories 1 to 12)

- Mock states "two-running-one-done-wide", "no-subagents-wide" and "one-subagent-wide".
- Flow 208 "Task list", rewritten for the card, on `bg-progress` shape `claude` on a Design page and on the chat page: all rows show with no click and no summary line, running rows come first, a shell row opens its details and command, a subagent row has a linked name and no details, and the header has no "waiting for sub-agent" and no "running shell:".
- New flow on `subagents` shape `card-two-running`: both names are links, each has a current call, the finished row is grey with no link, and no subagent shows twice on the page.
- Same flow, shape `card-no-tasks`: only the header. Shape `card-one-subagent`: the header and one row. After a result for the open call is appended to one subagent's file, its row shows one line.
- Flow 208, shape `subagent-with-inner-task`: after the turn ends while the subagent runs, the rows show with no Working header and no moving border. After the last task ends, the card is gone.

Card size and pages (stories 13 to 15)

- Mock states "two-running-one-done-phone", "no-subagents-phone" and "one-subagent-phone".
- New flow on `bg-progress` shape `many` at 390px: the card is at most 40% of the viewport height, its rows scroll, the header stays in view, the log is visible and the page has no sideways scroll.
- New flow: the card shows on a Design page, an Initiative page, a Triage page and the chat page, and in a step page's Questions view. New scenario to write: sessions with a running subagent on an Initiative and a Triage page, and one with an open question round.

Codex card (stories 16 to 17)

- New flow on a new `subagents` shape `codex-running-call`: the card shows the child's current call, and no call from the inherited history. The chat read's task has that current call.
- Flow 208, shape `old-codex`: the card shows the header with the current tool and no rows.

Links in the log (stories 18 to 22)

- Flow 216 "Subagent page", updated, on `subagents` shapes `card-two-running` and `finished-handback`, on a Design page and on the chat page: a running call has no link in the log, a finished call has one below the folded group, an open group shows it once, and a running call in it is plain text.
- Same flow: end a running subagent through the fixture files. With no reload, its row turns grey or leaves the card and its log link appears.
- CLI: the chat read names the state of each subagent call.

No task record (story 23)

- New flow on a new scenario, an old Codex session with a subagent call: the call is plain text on the chat page, in the folded and the open group, and as a plain line on a step page. The chat read marks it as having no record.

Names (stories 24 to 25)

- New `subagents` shape `string-background`, with one assistant line holding a background Agent call with the string `"true"` and a second tool call: the card shows the description as a link, and both rows are in the chat read.

Current-call text (stories 26 to 28)

- New `subagents` shape `file-call`, a running subagent whose open call reads a file in the session's directory, and a second whose open call edits a long path outside it: the chat read, the card and the subagent page show the relative path for the first and the cut path with the file name for the second. Check at 920px and 390px.
- Flow 75, updated: a parent's file tool in flight shows tool and path in the Working header, a search still shows its pattern.

Live updates (stories 29 to 30)

- New flow on `subagents`, with a chat page and then a Design page open: append a new open call to a running subagent's file while the parent writes nothing. The card shows it with no reload. Repeat on a Codex child.
- Go server tests: with no watch, growth of a subagent file sends no event. With a session watch it sends one. After a close request or a closed stream it sends none. A late close of an old watch leaves the new one running. An unknown client and an unknown session are refused.

Back link (stories 31 to 36)

- Flow 216, updated: from the card and from a log link on a Design, an Initiative and a Triage page, the back link returns to that page. From the chat page it returns there. A URL with no origin, and one with an outside address as origin, go back to the chat page. A reload keeps the target (declare the reload).
- Same flow, shape `nested`: from a Design page open the nested subagent and its grandchild. Back goes to the parent subagent, then again, then to the Design page. The breadcrumb levels keep the origin.
- Same flow, shapes `file-gone` and `parent-gone`: the back link follows the origin.

New flows and scenarios to write are those named above: the `subagents` shapes `card-two-running`, `card-no-tasks`, `card-one-subagent`, `codex-running-call`, `string-background` and `file-call`, the Initiative, Triage and question round sessions, and the old Codex subagent call. These become implementation tickets.

**Mock states**

- `$(gx tickets root)/subagent-links-working-card/mocks/01-choose-bottom-block-layout.html`, two-running-one-done-wide, 920px, fixture scenario `subagents` (shape `card-two-running`); parts: card, working-line, subagent-row-1, subagent-row-2, done-row-1.
- `$(gx tickets root)/subagent-links-working-card/mocks/01-choose-bottom-block-layout.html`, two-running-one-done-phone, 358px, fixture scenario `subagents` (shape `card-two-running`); parts: card, working-line, subagent-row-1, subagent-row-2, done-row-1.
- `$(gx tickets root)/subagent-links-working-card/mocks/01-choose-bottom-block-layout.html`, no-subagents-wide, 920px, fixture scenario `subagents` (shape `card-no-tasks`); parts: card, working-line.
- `$(gx tickets root)/subagent-links-working-card/mocks/01-choose-bottom-block-layout.html`, no-subagents-phone, 358px, fixture scenario `subagents` (shape `card-no-tasks`); parts: card, working-line.
- `$(gx tickets root)/subagent-links-working-card/mocks/01-choose-bottom-block-layout.html`, one-subagent-wide, 920px, fixture scenario `subagents` (shape `card-one-subagent`); parts: card, working-line, subagent-row-1.
- `$(gx tickets root)/subagent-links-working-card/mocks/01-choose-bottom-block-layout.html`, one-subagent-phone, 358px, fixture scenario `subagents` (shape `card-one-subagent`); parts: card, working-line, subagent-row-1.

## Out of Scope

- Changes to the subagent page other than its back link, the origin on its links and the current-call text. It keeps today's Working row.
- The TUI.
- Links for Codex grandchildren.
- Task records for Codex versions that have none today.
- A page for shell and Monitor tasks.
- Live updates of shell output or other task details beyond today's behaviour. A chat reload caused by a subagent file also refreshes them.

## Further Notes

- This spec replaces parts of two earlier specs. From "Background progress": the closed summary line and the list below the Working row. From "Subagents": the "Open ↗" link on a task row, the details of a subagent row, the log link of a running subagent and the fixed target of the back link.
- The glossary term Task group keeps its meaning.
- The prototype branch `prototype/subagent-links-bottom-block` is a reference for the look only and is never merged.
- The cause of the lost names is written up in `docs/research/subagent-task-name-lost.md` on the branch `research/subagent-task-name`.

## Self-decided design questions

1. **How does the page-scoped check start and stop?** Choice: a session scope on the existing subagent watch. Reason: it already has the client stream lifecycle, the page-hide close and the poll interval. One request per page avoids the browser tracking each subagent.
2. **Do overlapping page changes end each other's watch?** Choice: no, each page's watch is told apart. Reason: a late close request must not stop the next page's updates.
3. **Is a hidden tab "open"?** Choice: yes. Reason: the subagent watch works this way, and the card is current when the person returns.
4. **Which event reloads the page?** Choice: the existing subagent-changed event, sent only to the watching client, at most once per tick. Reason: the chat already reloads on events for its session.
5. **What decides "finished" for a log link?** Choice: the task rows' end rules, not the sidecar's finished marker. Reason: the card and the log must switch together.
6. **Which tools are file tools?** Choice: Read, Edit and Write. Reason: these are the ones the person was asked about. Other tools stay as today.
7. **How is a long file label cut on a narrow screen?** Choice: also at the start of the path. Reason: an end cut by the browser would hide the file name.
8. **What do calls with no task record show on the chat page?** Choice: plain text in the open group, nothing below the folded group. On step pages, a plain line. Reason: only links were added below the folded group.
9. **What about a decoded archived Codex conversation?** Choice: plain text. Reason: it has no task records, and the subagent page could show nothing.
10. **What do non-subagent rows look like?** Choice: dot or mark, label, time, no kind word. Reason: this is the card row in the mock.
11. **What is the height cap exactly?** Choice: 40% of the viewport height, header included. Reason: it is the agreed "about 40%" as one number that a test can check.
12. **Which origin values count?** Choice: only chat, Design, Initiative and Triage pages of this app, else the chat page. Reason: a URL parameter must not send the person elsewhere.
13. **Do breadcrumb links keep the origin?** Choice: subagent levels do, the session name still opens the chat page. Reason: back links from an upper level must still end at the origin, and the session name already means the chat.
14. **Is a step page's picked session restored on return?** Choice: no, the page opens in its normal state. Reason: the browser Back button behaves the same way today.
15. **How far does the lost-name fix go?** Choice: all three parts from the research: tolerant boolean, per-block decoding and the description fallback. Reason: each removes a way to lose a name.
