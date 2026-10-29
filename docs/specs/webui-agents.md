## Problem Statement

The person running gx can only watch and type into their interactive sessions from the TUI Agents
tab. Away from the terminal, on a laptop browser or a phone, the web UI shows design sessions on the
design page but no other session. A person cannot see which sessions finished a turn or wait for an
answer, cannot read a session's chat, cannot send it the next message, and cannot start a new
session. `POST /v1/sessions/prompt` also refuses every session that is not a design session, so the
web cannot type into a plain interactive session at all.

## Solution

A web **Agents** page at `#/agents`, with an Agents tab between Epics and Stats. The tab shows the
number of Actionable sessions.

- A 300px sidebar lists the same sessions as the TUI Agents tab (interactive and design sessions),
  grouped by project, with one-line rows like the TUI: status icon, Design mark, name, `+bg`, session
  ticket, Unread and Actionable marks. Design sessions are hidden by default behind a remembered
  toggle that shows how many it hides.
- Selecting a session opens `#/agents/<label>`. The main column shows a session header and the
  session's rendered chat (not a terminal), with a composer under it. The composer keeps one draft
  per session in localStorage, and Send works only while the session is promptable, with a reason
  line otherwise.
- The session name in the header is a button that opens the session switcher, the design page's
  JumpMenu turned into a general row list. Cmd-K opens the same switcher on the Agents page.
- A "New session" form (from the sidebar, the switcher, Cmd-K or a "+" on a project group) starts a
  Claude session in the project root, an existing worktree or a new worktree.
- On a phone `#/agents` is the session list full-screen. A session opens full-screen with a back
  button and the title button for the switcher.

The Agents page and the design page share one chat view and one composer. The design-only parts
(question sheet, step row, Auto menu, Continue, Interrupt, Restore) sit behind options that the
Agents page leaves off. All composer drafts on both pages move to localStorage.

## User Stories

Navigation

1. As the person running gx, I want an Agents tab between Epics and Stats, so that I can reach my
   sessions from any web page.
2. As the person running gx, I want the Agents tab to show the number of Actionable sessions,
   hidden at zero, so that I see from any page that a session needs me.
3. As the person running gx, I want each selected session to have its own URL, `#/agents/<label>`,
   so that a reload or a shared link opens the same session.
4. As a desktop user, I want `#/agents` to reopen the session I last viewed in this browser, so
   that I return to where I was.
5. As a desktop user with no last-viewed session (or one that is gone), I want a "Pick a session"
   panel with a New session button, so that the empty main column tells me what to do.
6. As the person running gx with no sessions at all, I want a "No sessions yet" panel with a New
   session button, so that I can start my first one.

Sidebar

7. As the person running gx, I want the sidebar to list the same sessions as the TUI Agents tab, so
   that the web and the terminal agree.
8. As the person running gx, I want sessions grouped by project, projects in order of their first
   session and sessions in creation order, so that rows never jump around.
9. As the person running gx, I want each row to show the status icon, the Design mark, the name,
   `+bg` while a background task runs, the session ticket and the Unread and Actionable marks, so
   that I see what the TUI row shows.
10. As the person running gx, I want an Unread session's name in bold, so that new output stands out.
11. As the person running gx, I want a tooltip on the Actionable mark with the Actionable reason, so
    that I know what the session needs without opening it.
12. As the person running gx, I want Exited sessions listed with the faded Exited icon (red for a
    crash), so that I see them as the TUI does.
13. As the person running gx, I want "N live" in the sidebar header, so that I know how many sessions
    run.
14. As the person running gx, I want design sessions hidden by default, so that my plain sessions
    are not buried under design work.
15. As the person running gx, I want a "Design sessions" toggle that remembers my choice in this
    browser, so that I set it once.
16. As the person running gx, I want the toggle to show how many design sessions it hides, so that I
    know they exist.
17. As the person running gx, I want a selected design session to keep its row while the toggle hides
    design sessions, with the hidden count leaving it out, so that the sidebar always shows where I
    am.
18. As the person running gx, I want the sidebar to update live as sessions change state, so that I
    never reload.

Reading a session

19. As the person running gx, I want the session header to show the status icon, the name as the
    switcher button, a status chip, the dir and the session ticket, so that I know which session I
    read.
20. As the person running gx, I want a harness chip in the header for a non-Claude session only, so
    that the usual case stays clean.
21. As the person running gx, I want the selected session's chat rendered as on the design page, so
    that I read prompts, replies, tool calls and results, not a terminal.
22. As the person running gx, I want the chat to open at the newest message and follow new ones until
    I scroll up, so that I can read live or read back.
23. As the person running gx, I want "Earlier messages hidden" when the chat is cut, and the
    background-task strip while tasks run, so that I see what the design page shows.
24. As the person running gx, I want a plain "Working" row while the session works, so that I know it
    is busy.
25. As the person running gx, I want viewing a session in a visible tab to clear its Unread for every
    client, the TUI included, so that I do not clear it twice.
26. As the person running gx, I want new output on the session I am viewing not to set Unread, so
    that the mark means I missed something.
27. As the person running gx, I want a session with no messages yet to show "No messages yet", so
    that a new session does not look broken.
28. As the person running gx, I want a non-Claude session to show "Chat works for Claude sessions
    only." with a Show screen button that opens the last screen, so that I can still see what it
    does.

Failure notices

29. As the person running gx, I want a failed session (crashed, ended, auth, rate-limit, stalled, blocked) to
    show the failure label and text above the composer, so that I know why it stopped.
30. As the person running gx, I want a Screen button on a stalled or blocked session's notice, so
    that I can see what it waits on.
31. As the person running gx, I want a Retry button on an auth failure's notice, so that I can send
    "continue" after I log in again.
32. As the person running gx, I want no Restore or New session buttons on the Agents page, so that
    the page offers only what it supports.
33. As the person running gx, I want a crashed or ended session to keep its chat, with a disabled
    composer and the line "Exited. Restore it in gx, Agents tab.", so that I can read it and know
    where to restore it.

Composer and drafts

34. As the person running gx, I want a text box with a Send button beside it under the chat, so that
    I can type the session's next message.
35. As the person running gx, I want Send to work only while the session is promptable (Idle, or
    Working with only background tasks), so that my text never lands mid-turn.
36. As the person running gx, I want a reason line under the composer while Send is off (for example
    "Working. Send turns on when the session is idle."), so that I know why and when it turns on.
37. As the person running gx, I want to keep typing while Send is off, so that I can prepare my next
    message.
38. As the person running gx, I want one draft per session saved in localStorage, so that it
    survives a closed tab and a reload.
39. As the person running gx, I want "Draft saved in this browser" under the composer while a draft
    exists and nothing else needs the line, so that I trust the draft.
40. As the person running gx, I want the draft cleared when the server accepts the send, so that I do
    not send it twice.
41. As the person running gx, I want the draft kept and the refusal reason shown in red under the
    composer when the server refuses a send or the network fails, so that I can fix it and resend.
42. As the person running gx, I want a `stale` refusal to reload the chat and keep my draft, so that
    I read what changed before I resend.
43. As the person running gx, I want Cmd/Ctrl+Enter to send under the same rule as the Send button,
    and Enter to add a newline, so that I can send from the keyboard.
44. As a phone user, I want only the Send button to send, so that the on-screen keyboard's Enter adds
    a newline.
45. As the person running gx, I want text starting with `/` or `!` refused, so that the web cannot
    run slash commands or shell commands in a session.
46. As the person running gx, I want a non-Claude session's composer disabled with "Sending works for
    Claude sessions only", so that I do not type into a session the web cannot check.
47. As the person running gx, I want to send to any Claude interactive session, not only design
    sessions, so that the web is as useful as the TUI for typing.

Design sessions

48. As the person running gx, I want a design session picked here to show its plain chat and
    composer, so that I can follow it from the Agents page.
49. As the person running gx, I want an "Open design page →" link in a design session's header, so
    that I can answer its question cards and see Progress.
50. As the person running gx, I want a design session's draft to be the same on the Agents page and
    the design page, so that text I typed in one shows in the other.
51. As the person running gx, I want the design page's round answers, question index and scroll to
    survive a closed tab, so that a half-answered round is never lost.
52. As the person running gx, I want Cmd/Ctrl+Enter to work in the design page's composer too, so
    that both pages send the same way.

Switcher

53. As the person running gx, I want the session name in the header to open the session switcher, so
    that I can jump to another session.
54. As the person running gx, I want Cmd-K on the Agents page to open the session switcher, and the
    epic jump menu on every other page, so that Cmd-K always jumps within the page I am on.
55. As the person running gx, I want the switcher sorted in Actionable, Unread and Other sections, so
    that the sessions that need me come first.
56. As the person running gx, I want the switcher to filter by name, project, dir and session
    ticket, so that I find a session by whatever I remember.
57. As the person running gx, I want the switcher to always list design sessions, even while the
    sidebar hides them, so that I can reach any session.
58. As the person running gx, I want the current session tagged "current" and a "+ New session" row
    at the end, so that I know where I am and can start a session from it.
59. As the person running gx, I want the switcher to use the same keyboard and search behaviour as
    the epic jump menu, so that both feel the same.

Creating a session

60. As the person running gx, I want a "+ New session" button at the top of the sidebar, so that I
    can start a session.
61. As the person running gx, I want a "+" on each project group that prefills that project, so that
    I start in the right project in one click.
62. As the person running gx, I want "New session" in Cmd-K, prefilled with the selected session's
    project, so that I can start one from the keyboard.
63. As the person running gx, I want the form to ask for Project, Worktree, Name (optional) and First
    prompt (optional), so that it matches the TUI form.
64. As the person running gx, I want the Worktree picker to default to the project root and list the
    project's existing worktrees, so that I pick where the session runs.
65. As the person running gx, I want typing a name no worktree has to offer "+ Create worktree
    <name>", so that I can start in a fresh worktree.
66. As the person running gx, I want Name's placeholder to be the worktree name, so that I know the
    name it gets by default.
67. As the person running gx, I want every new session to run claude, with no harness or ticket
    field, so that the form stays short.
68. As the person running gx, I want the page to open the new session after create, so that I see it
    start.
69. As the person running gx, I want a refusal (a bad worktree name, a first prompt starting with
    `/` or `!`) shown in the form with my input kept, so that I can fix it.
70. As the person running gx, I want the form's input kept in localStorage until I create or discard,
    so that a closed tab does not lose it.
71. As a desktop user, I want the form as a modal; as a phone user, as a full sheet, so that it fits
    the screen.

Phone

72. As a phone user, I want `#/agents` to show the session list full-screen, so that I pick a
    session first.
73. As a phone user, I want a session to open full-screen with "‹" back to the list and the title
    button with "project · status" under it, so that the chat gets the whole width.
74. As a phone user, I want the switcher as the JumpMenu's full-screen form with Close, so that I can
    switch sessions with one thumb.

Errors

75. As the person running gx, I want a gone label (removed while viewed, or never existed) to show a
    "Session not found" panel saying "`<label>` no longer exists.", with the sidebar kept, so that I
    know what happened.
76. As a phone user, I want a "Back to sessions" link on that panel, so that I can leave it.
77. As the person running gx, I want the selection never to move by itself, so that no other
    session's Unread is cleared by accident.
78. As the person running gx, I want the page to wait for the first snapshot before it decides a
    label is gone, so that the panel never flashes on load or reconnect.
79. As the person running gx, I want the shared "Offline, reconnecting…" strip while the server is
    down, with the last sidebar and chat kept, so that I can still read.
80. As the person running gx, I want Send and the create submit disabled while offline with
    "Offline. Send turns on when gx reconnects.", my draft kept, so that nothing gets lost.
81. As the person running gx, I want a removed session's draft deleted once a live snapshot no longer
    lists it, so that a later session with the same label starts empty.
82. As the person running gx, I want a chat read that fails with a server error to keep the last
    chat and retry on the next event or reconnect, so that a hiccup does not blank the page.

## Implementation Decisions

Routing and navigation

- New routes `#/agents` and `#/agents/<label>`. The label is the session id. The router redirect
  for unknown hashes stays.
- The top bar gets an Agents tab between Epics and Stats. Its count is the number of sessions in the
  snapshot that are Actionable by the TUI's `isActionable` rule: an Actionable reason, or the
  Blocked state. The count hides at zero, like the Epics tab's count.
- The last viewed session is stored in localStorage. Desktop `#/agents` replaces the URL with that
  session when the snapshot still lists it, else shows the "Pick a session" panel. A phone never
  redirects; it shows the list.
- The page waits for the first snapshot of a connection before it decides anything from the session
  list (last viewed, gone label, draft cleanup).
- The page follows the web rules in `webui/README.md`: it repaints from the pageCache copy after a
  reload and shows the centered Spinner while it has no data.

Session list

- The sidebar reads `sessions` from the snapshot. No new server read. The snapshot already holds
  only interactive sessions (design sessions included), the same list the TUI Agents tab shows.
- Grouping and order port the TUI's `orderedSessions` rule. "N live" ports `liveCount`. The marks
  follow `sidebar.go`. Exact styling follows the approved mock.
- The design-sessions toggle state lives in localStorage, hidden by default. The hidden count
  counts design sessions the toggle hides, minus the selected one, which always keeps its row.
- Viewing reuses `useFocusReport` for the selected session, so the server clears Unread for every
  client and does not set it while the session is on screen in a visible tab.

Shared chat view

The design page and the Agents page use one chat view. Names, props and file layout are the
implementer's choice. The split:

- Shared, on both pages: the chat read through `useSessionChat`, message rendering, "Earlier messages
  hidden", the background-task strip, open-at-newest and scroll-follow, the failure notice (label and
  text), the Screen modal, and the composer slot.
- Options the Agents page turns off: the question sheet and the Questions/Chat view toggle, the step
  Working row (the Agents page shows a plain "Working" row instead), `GxStopped`, the Done state that
  hides the composer, and the failure notice's Restore and New session buttons.
- The question sheet stays a slot in the shared view, so the later question-cards effort can turn it
  on for the Agents page without another split.
- New on the Agents page: the "No messages yet" empty state when the chat read returns no messages,
  and, for a non-Claude session, the notice "Chat works for Claude sessions only." with a Show screen
  button that opens the existing `ScreenModal`. The design page keeps its own non-Claude text.
- Failure notice on the Agents page: label and text, read-only. Buttons: Screen for stalled and
  blocked, Retry for auth (a send of "continue" with the stale guard, as today). For crashed and
  ended the chat stays, and the composer stays visible but disabled with the reason line "Exited.
  Restore it in gx, Agents tab." Rate-limit shows the label "Rate limited" and the server's text
  (for example "resets at 5pm"), no buttons, and the not-promptable reason line "Rate limited. Send
  turns on when the session is idle."

Shared composer

- One composer for both pages. Design-only options, off on the Agents page: the Auto menu,
  Continue, Send answers with its popover, Interrupt and the Note toggle layout. On the Agents page
  the text box is always open with Send beside it, as in the mock.
- Send rule, both pages: enabled only while connected, `promptable(session)` holds, the text is not
  empty, and no send is pending. Cmd/Ctrl+Enter runs the same send under the same rule on both
  pages. On the design page the key runs the main button's action only when that action is Send or
  Send answers (the gaps popover included, when questions are unanswered). It never runs Continue
  or Interrupt, and does nothing while the Note toggle hides the text box. Enter adds a newline. A phone (narrow layout) has no key send.
- Reason line, Agents page: one line under the composer, in this order of precedence: offline
  ("Offline. Send turns on when gx reconnects."), non-Claude ("Sending works for Claude sessions
  only"), Exited ("Exited. Restore it in gx, Agents tab."), not promptable ("<State>. Send turns on
  when the session is idle."), the last refusal reason in red, else "Draft saved in this browser"
  while a draft exists. A refusal on the Agents page shows inline, not as a toast. The design page
  keeps its toasts.
- Draft: the composer owns one draft per session in localStorage, keyed by the session id, used by
  both pages. It clears when the server accepts the send. It stays on a refusal or a network error.
  On `stale` the page refetches the chat and keeps the draft.
- The design page's text box always reads and writes the one per-session composer draft, also while
  a round is open. A round send uses the box text as the round note ("Note: …") and clears the
  composer draft once the server accepts it. On the Agents page the same text goes out as a plain
  message. So text never jumps or disappears when a round opens or closes.
- The design page opens its Note box whenever the session's composer draft is not empty, on load
  and whenever the draft changes, so shared text is never hidden.
- Each composer listens for the browser `storage` event on its session's draft key and updates its
  box, so a clear or an edit in one tab shows at once in another open tab.
- The round draft keeps only what the question cards own (answers, question index, scroll). It
  loses its `notes` field and moves from sessionStorage to localStorage, keyed as today by session
  and round. The new code ignores old round drafts in sessionStorage and does not migrate them; they
  only lived as long as the tab.
- Draft cleanup: on each live snapshot after the first one of a connection, the app-level connection
  (not one page) scans every stored message draft and round draft key and deletes the ones whose
  session the snapshot does not list. This also catches sessions removed while no tab was open.
- No queue in the browser or on the server.

Prompt route

- `POST /v1/sessions/prompt` drops the `session-not-design` refusal for text sends only. A request
  with `continue: true` stays refused for a non-design session, because Continue types `/clear` and
  `/gx-design <ticket>` (`designContinueAction`), which the prefix rule exists to block. It keeps the harness check
  (`harness-not-supported`), the empty-text check, the `/` and `!` prefix block, the 32 KiB cap, the
  web stale check on `last_message`, and the Promptable check. One rule for design and plain
  sessions. The CLI verb follows the same handler.
- Auto-advance cancelling and Actionable clearing in `promptSession` stay as they are.

Session switcher

- `JumpMenu` becomes a general row list. Each row has a title, a subtitle, a link, search text and
  marks. The caller gives the rows and their sections and order. Epics and sessions share one
  keyboard and search behaviour, and the phone full-screen form with Close.
- Session rows: status icon, Design mark, name with a "current" tag, subtitle "project · status ·
  ticket · Actionable reason", Unread and Actionable marks. Sections: Actionable, Unread, Other. The
  search text is the name, project, dir and session ticket. Design sessions are always listed. A
  "+ New session" row ends the list.
- On the Agents page, Cmd-K and the header title button open the session switcher. Every other page
  keeps the epic jump menu.

Create session

- The web mux serves the existing `POST /v1/sessions/create` through a web variant. The web variant
  has its own small request type: project, `dir` or `worktree`, name and first prompt. It never
  takes `made_worktree`, `rebase_note`, `unique_name`, `cols`, `rows`, a harness or a ticket. It
  forces the claude harness, and refuses a first prompt starting with `/` or `!` with the prompt
  route's `prefix-not-allowed` reason. It refuses a project that is not configured, and a `dir` that
  is not the project root or one of the worktrees the worktree read lists. The checks live in the server, not only in
  the form. The CLI route stays as it is.
- No unique-name check, as in the TUI. The label is the session id, so names may repeat.
- A new server read gives each project's root dir and its worktree dirs, because the browser cannot
  run git. Its path, CLI verb name and shape are the implementer's choice. It follows "Adding a server
  route" in `.ai/index.md`: route table, CLI verb, web mux, and the web 404 allowlist test.
- Form fields: Project, Worktree, Name (optional, placeholder is the worktree name), First prompt
  (optional), Cancel, Create session. Worktree is one picker: it defaults to the project root, lists
  the root and the project's worktrees, filters as the person types, and offers "+ Create worktree
  <name>" when the text matches no worktree. Picking a dir sends `dir`; the create row sends
  `worktree`.
- The project prefill comes from the entry point: the group's "+", or the selected session's project
  for the sidebar button, the switcher row and Cmd-K.
- The form keeps its input in localStorage until create or discard, like the New Design form. It is a
  modal on desktop and a full sheet on a phone.
- After an accepted create, the page navigates to `#/agents/<new label>`. A refusal shows in the
  form, input kept. Submit is disabled while offline.

Live updates and errors

- No new live-update work. The snapshot keeps the sidebar live. `useSessionChat` keeps the open chat
  live, with a full refetch on each change, as on the design page.
- Gone label: a "Session not found" panel ("`<label>` no longer exists."), sidebar kept, "Back to
  sessions" on a phone. The selection does not move. The focus report sends no session for a gone
  label.
- Offline: the shared `DetailPage` strip, last data kept, Send and create submit disabled.
- A chat read that fails with a non-404 server error keeps `useDetail`'s behaviour.

## Testing Decisions

- A good test drives the page as a person does (role and visible-text locators, clicks, keys) and
  checks what shows on screen or what the server returns. It does not assert on component props,
  hook calls or storage key names.
- Seams, highest first:
  - The e2e flows against a `gxfixture` server, for the page as a whole.
  - Vitest component tests of the Agents page with canned snapshot and chat fetchers, as
    `DesignPage.test.tsx` and `JumpMenu.test.tsx` do today.
  - Server tests of the prompt route and the web create route through the web mux, built with
    `servertest`.
- Web tests to write: sidebar grouping, order, marks and toggle count; routing (last viewed, Pick a
  session, gone label, waiting for the first snapshot); composer send rule, reason lines, Cmd/Ctrl+
  Enter, draft save, clear on accept, keep on refusal and `stale`; draft cleanup for removed
  sessions; the failure notice button sets; the switcher's sections, search and rows for both epics
  and sessions; the create form's Worktree picker and refusals.
- Existing web tests to keep green: `ChatLog.test.tsx`, `Composer.test.tsx`, `DesignPage.test.tsx`,
  `JumpMenu.test.tsx`, `focus.test.tsx`. Round draft tests change from sessionStorage to localStorage.
- Server tests: prompt to a plain Claude session is accepted; `/` and `!` and non-Claude stay
  refused; `continue: true` to a plain session is refused with `session-not-design`; the web create refuses a ticket, a non-claude harness, a `/` or `!` first prompt, a project that is
  not configured, and a `dir` outside the project root and its listed worktrees; the
  worktree read lists the root and worktrees of a `testutil.TempRepo`; the route parity tests pass.
- Prior art: `server/web_test.go` for web mux writes, `cmd/server_parity_test.go` for route verbs,
  `webui/e2e/design.e2e.ts` and `design-page.e2e.ts` for chat and composer flows, `jump.e2e.ts` for
  the jump menu, `newdesign.e2e.ts` for a form with a localStorage draft, `phone.e2e.ts` for phone
  flows.

## Verification

New scenarios to write (each becomes an implementation ticket):

- `agents`: three projects (`demo`, `other` and `site`) with interactive sessions made by
  `gx agents create` and replayed by fakeclaude. The groups match the mock: four sessions in the first project, two in the second and
  two in the third, with a design session in the first and third projects (rows a3 and c1). The
  sessions below are spread over these rows:
  - an Idle Claude session with a chat in `demo`;
  - a Working Claude session (a long busy turn) in `demo`;
  - an Unread session and an Actionable session in `other`;
  - an Exited session that crashed;
  - two design sessions linked to maps (as `design-two-live` links one), in the first and third
    projects;
  - a Claude session with no messages yet;
  - a live non-Claude session. A build tag that only `gxfixture` uses registers a fake harness with
    no chat, backed by agentfake. The scenario creates the session with `gx agents create --harness
    <fake>`, so it runs, has a screen and goes through the real create path. A faked session meta
    does not work: on server start gx cannot find its harness and marks it Exited;
  - one existing worktree in `demo` besides the root.
- `agents-failures`: sessions with a stalled, an auth, a rate-limit and a blocked failure, for the
  failure notice flow. Rate-limit needs a new `fakeclaude` control that appends a rate-limit API
  error (`isApiErrorMessage`, `error: "rate_limit"`, a "resets at ..." message) to its transcript,
  as real claude does, so the real claude watcher reports Rate-limited. Stalled needs a stall threshold shorter than the 15-minute default: add a
  hidden `gx server` flag for it, passed by a new `gxfixture --stall-threshold` option, as
  `--auto-advance-delay` works today. If `agents` can hold them without breaking other flows, fold them into `agents` instead.
- `empty` already exists (a project, no sessions) and backs "No sessions yet".

New flows to write, numbered after flow 71 (file name is the implementer's choice):

- 72 "Agents tab": the tab order, the Actionable count, `#/agents/<label>` deep link and reload.
- 73 "Agents sidebar": groups, order, row marks, the Actionable tooltip, "N live", live updates.
- 74 "Design sessions toggle": default hidden, the count, remembered after reopening the page, the
  selected hidden design session keeps its row.
- 75 "Read a session": header fields, chat, scroll-follow, Unread clears on view (`gx agents list`
  shows it read), plain Working row.
- 76 "Send and draft": type, reopen the page in a new tab, the draft is back; Send; the draft clears;
  `gx agents chat <id>` shows the text.
- 77 "Send while busy": the Working session's reason line, typing kept, Send off.
- 78 "Send refused": move the chat with `gx agents prompt` from the CLI, then Send; the red stale
  reason, the chat reloads, the draft stays. A `/clear` send shows the prefix refusal.
- 79 "Keyboard send": Cmd/Ctrl+Enter sends on the Agents page and the design page; Enter adds a
  newline; at 390px only the button sends. On the design page: the key runs Send, runs Send answers
  (and opens the gaps popover with unanswered questions), and does nothing when the main button is
  Continue (empty box) or Interrupt.
- 80 "Failure notices": crashed (Exited line, no Restore), stalled and blocked (Screen), auth (Retry),
  rate-limit (label, text, no buttons, the "Rate limited" reason line).
- 81 "Session without chat": the notice, Show screen opens the screen modal, the composer reason.
- 82 "No messages yet".
- 83 "Design session on the Agents page": plain chat, no question sheet, "Open design page →" opens
  `#/d/<map>`.
- 84 "Session switcher": title button and Cmd-K open it; sections; filter by ticket and dir; design
  sessions listed while hidden; "current" tag; pick opens the session; Escape closes. Cmd-K on the
  Epics page still opens the epic jump menu.
- 85 "Create session": from a group's "+", project prefilled, project root, first prompt; the page
  opens the new session; `gx agents list` shows it in `demo`.
- 86 "Create in a new worktree": type a new name, "+ Create worktree", create; the session dir is the
  new worktree. A bad worktree name and a `/` first prompt show the refusal in the form, input kept.
- 87 "Create entry points": sidebar button, switcher row, Cmd-K entry, each with the right prefill;
  the form draft survives reopening the page.
- 88 "Phone, Agents page": list, open, "‹" back, switcher full-screen with Close, create as a sheet.
- 89 "Session not found": deep link to a made-up label, then remove a viewed Exited session with
  `gx agents remove`; the panel, sidebar kept, selection unchanged, "Back to sessions" at 390px.
- 90 "Agents offline": stop the fixture's server; the strip, last data kept, Send and create submit
  disabled with the offline line, the draft kept.
- 91 "Removed session draft": draft on an Exited session, close the tab, remove the session with
  `gx agents remove`, open the Epics page; the draft key is gone (a new session with the same label
  starts empty).
- 92 "Last viewed session": `#/agents` on desktop reopens the last session; with none, "Pick a
  session".
- 93 "Shared draft": with both pages open in two tabs and a round open on a design session, text
  typed on the Agents page shows live in the design page's composer with the Note box open, and the
  reverse; Send answers sends it as the note and both pages'
  boxes are empty after; a half-answered round survives opening the design page in a new tab.

Existing flows to rerun because the shared chat view, composer, round draft or JumpMenu changed:
4, 5, 9, 10, 11, 13, 14, 20, 21, 22, 24, 44, 47, 48, 49, 64, 65, 66, 67, 68.

CLI and server verb checks (on the `agents` scenario):

- `gx agents prompt <plain Idle id> "hello"` exits 0, with no `session-not-design` refusal; then
  `gx agents chat <id>` lists "hello".
- `gx agents prompt <id> "/clear"` is refused with `prefix-not-allowed`.
- `gx agents prompt <non-Claude id> "hi"` is refused with `harness-not-supported`.
- `gx agents create --project demo --prompt "/x"` still works from the CLI (the prefix rule is for
  the web create only).
- The new worktree read's CLI verb, for `demo`, lists the repo root and the seeded worktree.

Story entries:

1. Flow 72: the tabs read Epics, Agents, Stats. Mock state desktop-selected.
2. Flow 72: the Agents count equals the Actionable sessions in `gx agents list`, and hides on
   `empty`.
3. Flow 72: open `#/agents/<label>`, reload, the same session shows.
4. Flow 92: `#/agents` reopens the last viewed session.
5. Flow 92: with no last viewed, "Pick a session". Mock state desktop-no-selection.
6. Flow 72 on `empty`: "No sessions yet" with New session. Mock state desktop-no-sessions.
7. Flow 73: the sidebar labels match `gx agents list`.
8. Flow 73: group and row order. Mock state desktop-selected.
9. Flow 73: the row marks, `+bg` and ticket. Mock state desktop-selected.
10. Flow 73: the Unread session's name is bold.
11. Flow 73: hovering the Actionable mark shows its reason. Mock state desktop-selected.
12. Flow 73: the crashed session shows the red Exited icon.
13. Flow 73: "N live" matches the live sessions.
14. Flow 74: design sessions hidden on first open. Mock state desktop-selected.
15. Flow 74: the toggle state survives reopening the page.
16. Flow 74: the hidden count. Mock state phone-list.
17. Flow 74: a selected design session keeps its row; the count leaves it out.
18. Flow 73: a session that changes state updates its row with no reload.
19. Flow 75: the header fields. Mock state desktop-selected.
20. Flow 81: the harness chip shows on the non-Claude session only. Mock state desktop-no-chat.
21. Flow 75: the chat matches `gx agents chat <id>`. Mock state desktop-selected.
22. Flow 75: the chat opens at the bottom and follows new messages until scrolled up.
23. Flow 75: "Earlier messages hidden" and the background strip, as flow 14 checks on the design page.
24. Flow 77: the plain "Working" row. Mock state desktop-working.
25. Flow 75: after viewing, `gx agents list` shows the session read.
26. Flow 75: output while viewed leaves the session read.
27. Flow 82. Mock state desktop-no-messages.
28. Flow 81. Mock state desktop-no-chat.
29. Flow 80: each failure's label and text.
30. Flow 80: Screen on stalled and blocked opens the screen modal.
31. Flow 80: Retry on auth sends "continue" (`gx agents chat <id>`).
32. Flow 80: no Restore or New session button.
33. Flow 80: the crashed session's chat and the "Exited. Restore it in gx, Agents tab." line.
34. Flow 76. Mock state desktop-selected.
35. Flows 76 and 77.
36. Flow 77. Mock states desktop-working and phone-working.
37. Flow 77: typing stays possible while Send is off.
38. Flow 76: the draft is back in a new tab.
39. Flow 76: "Draft saved in this browser". Mock state desktop-selected.
40. Flow 76: the box is empty after the send.
41. Flow 78: the red reason and the kept draft. Mock state desktop-refused.
42. Flow 78: the chat reloads on `stale`.
43. Flow 79.
44. Flow 79 at 390px.
45. Flow 78 (`/clear`) and the CLI `/clear` check.
46. Flow 81: the composer reason. Mock state desktop-no-chat.
47. Flow 76 and the CLI `gx agents prompt` check.
48. Flow 83. Mock state desktop-design-session.
49. Flow 83: the link opens the design page.
50. Flow 93.
51. Flow 93: the round survives a new tab.
52. Flow 79 on the design page.
53. Flow 84: the title button. Mock state desktop-cmdk.
54. Flow 84: Cmd-K on the Agents page and on the Epics page.
55. Flow 84: section order. Mock state desktop-cmdk.
56. Flow 84: filter by ticket and by dir.
57. Flow 84: design sessions listed while hidden.
58. Flow 84: "current" and "+ New session". Mock state desktop-cmdk.
59. Flows 20, 21 and 22 still pass; flow 84 uses the same keys.
60. Flow 87: the sidebar button. Mock state desktop-create.
61. Flow 85. Mock state desktop-create.
62. Flow 87: Cmd-K "New session" prefills the selected session's project.
63. Flow 85: the form fields. Mock state desktop-create.
64. Flow 85: the default and the list in the Worktree picker.
65. Flow 86. Mock state desktop-create-worktree.
66. Flow 86: the Name placeholder follows the worktree.
67. Flow 85: `gx agents list` shows the new session as claude with no ticket.
68. Flow 85: the page opens the new session.
69. Flow 86: the refusals in the form.
70. Flow 87: the form draft survives reopening the page.
71. Flows 85 and 88. Mock states desktop-create and phone-create.
72. Flow 88. Mock state phone-list.
73. Flow 88. Mock states phone-chat and phone-working.
74. Flow 88. Mock state phone-switcher.
75. Flow 89.
76. Flow 89 at 390px.
77. Flow 89: no other session turns read.
78. Flow 89: no "Session not found" flash on a deep link to a live session.
79. Flow 90.
80. Flow 90.
81. Flow 91.
82. New check in flow 75: hold `/v1/sessions/chat` to fail with 500 after the first read; the chat
    stays and refetches on the next event.

**Mock states**

- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-selected, 1200px, `agents`, parts:
  top-bar, sidebar, new-session, group-gx, row-a1, row-a2, row-a4, group-dotfiles, row-b1, row-b2,
  group-website, row-c2, actionable-tooltip, design-toggle, session-header, switcher-button, chat,
  composer, send.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-working, 1200px, `agents`, parts:
  top-bar, sidebar, new-session, group rows as desktop-selected, design-toggle, session-header,
  switcher-button, chat, composer, send, composer-reason.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-refused, 1200px, `agents`, parts:
  top-bar, sidebar, new-session, group rows as desktop-selected, design-toggle, session-header,
  switcher-button, chat, composer, send, composer-reason.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-design-session, 1200px, `agents`,
  parts: top-bar, sidebar, new-session, group rows as desktop-selected plus row-a3 and row-c1,
  design-toggle, session-header, switcher-button, open-design, chat, composer, send,
  composer-reason.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-no-chat, 1200px, `agents`, parts:
  top-bar, sidebar, new-session, group rows as desktop-selected, design-toggle, session-header,
  switcher-button, chat, no-chat-notice, composer, send, composer-reason.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-no-messages, 1200px, `agents`,
  parts: top-bar, sidebar, new-session, group rows as desktop-selected, design-toggle,
  session-header, switcher-button, chat, no-messages, composer, send.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-no-selection, 1200px, `agents`,
  parts: top-bar, sidebar, new-session, group rows as desktop-selected, design-toggle, pick-panel.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-no-sessions, 1200px, `empty`,
  parts: top-bar, sidebar, new-session, no-sessions.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-cmdk, 1200px, `agents`, parts:
  top-bar, sidebar, new-session, group rows as desktop-selected, design-toggle, session-header,
  switcher-button, chat, composer, send, switcher, switcher rows (a1, a3, c2, a2, a4, b1, b2, c1),
  switcher-new.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-create, 1200px, `agents`, parts:
  top-bar, sidebar, new-session, group rows as desktop-selected, design-toggle, session-header,
  switcher-button, chat, composer, send, create-form, create-project, create-worktree, create-name,
  create-prompt, create-submit.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, desktop-create-worktree, 1200px, `agents`,
  parts: as desktop-create plus create-worktree-options and create-worktree-new.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, phone-list, 390px, `agents`, parts:
  top-bar, sidebar, new-session, group rows as desktop-selected, design-toggle.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, phone-chat, 390px, `agents`, parts:
  top-bar, session-header, switcher-button, chat, composer, send.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, phone-working, 390px, `agents`, parts:
  top-bar, session-header, switcher-button, chat, composer, send, composer-reason.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, phone-switcher, 390px, `agents`, parts:
  switcher, switcher rows (a1, a3, c2, a2, a4, b1, b2, c1), switcher-new.
- `$(gx tickets root)/webui-agents/mocks/06-mocks.html`, phone-create, 390px, `agents`, parts:
  create-form, create-project, create-worktree, create-name, create-prompt, create-submit.

The group and row part names (group-gx, row-a1 and so on) come from the mock's made-up sample
sessions. The look check maps them to the `agents` scenario's groups and rows by position.

## Out of Scope

- Stop, restore and interrupt from the Agents page, and the terminal view. v1 is list, chat, send and
  create.
- Answering question cards (grilling rounds) on the Agents page. The shared chat view keeps a slot for
  the later effort.
- Loop implementation agents' sessions in the list. Wanted once the PTY runner is the default.
- A safe list of slash commands (for example `/clear`) for the prompt route.
- Reading only the new part of a chat on each change. That speed-up applies to both pages and is a
  separate effort.
- Running an Actionable's action from the web.
- Creating a session for a map epic or map ticket from the Agents page. The design page does that.

## Further Notes

- Decisions come from the closed tickets of `gx:webui-agents` (01 to 10). Ticket 06 replaced ticket
  03's three-way Where choice with the single Worktree picker. Ticket 10 (spec round 1) decided the
  failure notices, the shared localStorage drafts, the keyboard send, the hidden selected design
  session, the web create prefix rule and the tab count.
- When choosing between a quick fix and a slower clean fix, take the clean fix: the checks for the
  web create live in the server, the switcher is one general JumpMenu rather than a copy, and drafts
  live in one place per session for both pages.
- The web stale check compares the last chat message id. A non-Claude session has no chat, which is
  why the prompt route keeps its harness refusal.
