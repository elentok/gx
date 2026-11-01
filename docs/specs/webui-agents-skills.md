## Problem Statement

The person running gx cannot run a command in a session from the web. The Agents page and the design page refuse any text that starts with `/`. A Codex skill typed as `$name` passes, but the person has no list of names to pick from. The New session form's First prompt refuses `/` too. There is no completion anywhere, so the person must remember every skill and command name.

Some commands open a terminal dialog (`/model`, `/resume`, `/permissions`). The session turns Blocked and waits for a key. The web screen view is read-only, and Interrupt refuses a session that is not Working. A Blocked session also has no web way out when it waits on a permission prompt. The person must go back to the terminal. Commands that print local output (`/cost`, `/context`) show nothing in the chat, because the chat hides those lines.

## Solution

The Agents page composer, the design page composer and the New session form's First prompt get a completion menu. Typing `/` as the first character lists commands for Claude and Codex sessions. Typing `$` at the start of any word lists skills for Codex sessions. The menu fills the text and never sends.

The server accepts any `/` command on the session prompt route and on the First prompt. It still refuses text that starts with `!`. A `/` command is sent once, with no nudge and no retype. A local command's output shows in the chat under the command, as a muted plain-text block.

The web can now drive a session without the TUI:

- The screen view gets a key bar (Esc, Enter, Tab, the four arrows, Ctrl+C) and a text field that types literal text. One new keys route serves them in any live state.
- A Blocked session shows Close dialog (Esc). A Blocked session on a permission prompt shows Allow once and Deny instead. Allow once never grants a standing permission.

Answering a session's question prompts stays out of scope.

## User Stories

Completion menu

1. As the person running gx, I want typing `/` as the first character of the Agents composer to open a menu of commands, so that I do not have to remember names.
2. As the person running gx, I want the same menu in the design page composer, so that design sessions are not different.
3. As the person running gx, I want the same menu in the New session form's First prompt, so that I can start a session with a command.
4. As the person running gx, I want each row to show the name, the argument hint, the description and the source (built-in, skill or mcp), so that I know what I pick.
5. As the person running gx with a Claude session, I want the `/` menu to list Claude's skills, custom commands, plugin commands and MCP prompts together with gx's list of built-in commands, so that I see everything the session can run.
6. As the person running gx with a Codex session, I want the `/` menu to list only Codex's built-in commands, so that I do not see names Codex would reject.
7. As the person running gx with a Codex session, I want typing `$` at the start of any word to list Codex's skills, so that I can mention a skill in the middle of a sentence.
8. As the person running gx, I want prefix matches first, then substring matches, alphabetical in each group, over names and aliases, so that the best match is on top.
9. As the person running gx, I want the typed part highlighted in each row, so that I see why a row matched.
10. As the person running gx, I want the menu to close on a space and on Escape, so that it never gets in the way of normal typing.
11. As the person running gx, I want a menu with no match to stay open and show one line, and to leave my text as typed, so that I can still send a command gx does not know.
12. As the person running gx, I want Up and Down to move the selection and Enter or Tab to fill the selected command, so that I can choose by keyboard.
13. As the person running gx, I want a click or tap on a row to fill it, so that I can choose by pointer.
14. As the person running gx, I want a chosen row to fill `/name ` or `$name ` into the text without sending, so that I can add arguments first.
15. As the person running gx, I want a `/` that is not the first character of the text to open nothing, and `$` to open nothing in a Claude session, so that ordinary text is never interrupted.
16. As the person running gx, I want Cmd/Ctrl+Enter and the Send button to keep sending as before, with the menu open or closed, so that the send rule does not depend on the menu.
17. As the person running gx, I want the menu to show the last list at once and refresh it when it is older than 60 seconds, so that it opens fast and still picks up a new skill.
18. As the person running gx, I want one loading line when there is no cached list yet, so that I know a list is coming.
19. As the person running gx, I want a failed list read to show gx's built-in commands and the line "Skills could not load", and sending to keep working, so that a broken read never blocks me.
20. As the person running gx, I want the First prompt menu to list the commands of the worktree chosen in the form, so that a skill that exists only in that worktree shows.
21. As the person running gx, I want the First prompt menu to read the project's base worktree when the form will create a new worktree, so that a menu opens before the worktree exists.
22. As the person running gx, I want Claude's HTML-escaped descriptions shown as plain text, skills that are not user-invocable left out, and a Codex skill with two roots shown once, so that the list reads clean.
23. As a desktop user, I want the menu above the composer with a footer that lists the keys, so that the keys are discoverable.
24. As a phone user, I want two-line rows, no key footer, and the menu shrunk to the space left above the on-screen keyboard, so that it fits.
25. As a phone user, I want the First prompt menu to open below the field, so that the field near the top of the sheet keeps room.
26. As the person running gx with vi mode on, I want the menu to open while I type in insert mode and stay shut for normal-mode keys, so that vi keys keep working.
27. As the person running gx, I want a disabled composer to show no menu, so that I cannot pick a command I cannot send.

Sending commands

28. As the person running gx, I want the server to accept any `/` command on the session prompt route, so that the web can run what the TUI runs.
29. As the person running gx, I want text starting with `!` refused, so that the web cannot run shell commands in a session. (This replaces story 45 of the earlier Agents page spec, which refused `/` and `!`.)
30. As the person running gx, I want a First prompt that starts with `/` accepted and one that starts with `!` refused in the form with my input kept, so that the create route follows the same rule.
31. As the person running gx, I want a `/` command sent once, with no nudge and no retype, so that a command that starts no turn does not run twice.
32. As the person running gx, I want `/compact` (with or without arguments) to keep the nudge path, so that it behaves like a normal prompt, because it starts a turn.
33. As the person running gx, I want "Send failed" with my text kept only when gx could not type the command, so that a command that starts no turn is not shown as a failure.
34. As the person running gx, I want a Codex `$` skill to follow the normal text path with nudges, so that skills still start their turn.
35. As the person running gx, I want a command to follow the same `last_message` stale check as text, with no exemption for `/clear`, so that I read what changed before I run it.
36. As the person running gx, I want the open chat to follow a `/clear` to the new conversation, resetting the chat view, scroll, on-screen Unread and `last_message` and keeping my draft, so that the page never shows the old conversation.
37. As the person running gx, I want my own `/clear` or `/compact` in a design session, and any key from the key bar or text from the Type field in its screen view, to stop the auto-advance countdown and clear the derived Continue, as typed text does, so that gx does not continue over a context I just reset or a dialog I am using. Close dialog, Allow once and Deny do not stop it.
38. As the person running gx, I want Continue to stay refused for a session that is not a design session, so that only the server builds `/clear` and `/gx-design`.
39. As the person running gx with a Codex session, I want the hint "Codex skills start with $. Type $gx-... instead." to stay for a draft starting with `/gx-`, so that I do not type a Claude skill into Codex.
40. As the person running gx, I want a command to send only while the session is promptable, with the usual reason line otherwise, so that my command never lands mid-turn.

Command output in the chat

41. As the person running gx, I want a Claude built-in command's non-empty output to show under its command pill, so that `/cost` and `/context` are useful on the web.
42. As the person running gx, I want the output shown as muted, left-aligned plain monospace text with ANSI codes stripped, so that it never renders as Markdown.
43. As the person running gx, I want output over 12 lines cut with a "Show all" button, so that a long report does not push the chat away.
44. As the person running gx, I want the caveat line, empty output and the `/compact` line hidden, so that only real output shows.
45. As the person running gx, I want a Codex `/` message shown as a command pill with no output, because Codex writes no output for it, so that I still see what I ran.
46. As the person running gx, I want a command that only shows a dialog to show the pill alone, so that the chat does not guess.
47. As the person running gx, I want the design page to show the same pills and output, so that both pages agree.
48. As the person running gx, I want output never to count as an Unread reply and never to be sent back as input, so that it does not change what the session does.

Key input and Blocked sessions

49. As the person running gx, I want a key bar (Esc, Enter, Tab, Up, Down, Left, Right, Ctrl+C) in the screen view, so that I can use a terminal dialog from the web.
50. As the person running gx, I want a text field with a Type button in the screen view that types the text without Enter, so that I can fill a dialog's input and press Enter myself.
51. As the person running gx, I want keys and typed text to work while the session is Working, Idle, Blocked or Rate-limited, so that I can act in any live state.
52. As the person running gx, I want a Starting or Exited session to refuse keys with a plain message, so that I do not fight gx's own startup answers or type into a dead session.
53. As the person running gx, I want the screen to refresh after each key or typed text, so that I see the effect.
54. As the person running gx, I want typed text that starts with `!` or holds control characters refused, so that the Type field cannot start a shell command or press keys the key bar does not offer.
55. As the person running gx, I want a Blocked session at a dialog to show a Close dialog button next to Screen that sends Esc, so that a command such as `/model` never traps the session.
56. As the person running gx, I want a Blocked session on a permission prompt to show the prompt text with Allow once and Deny next to Screen, so that I can answer from the web.
57. As the person running gx, I want a permission notice whose prompt cannot be read from the screen to show "The session asks for permission" with the same buttons, so that I can still answer. Deny sends Esc. Allow once is refused with a message that points to the key bar. This covers an unreadable Claude screen, and an unreadable Codex screen with no hook request.
58. As the person running gx, I want the permission buttons (Allow once and Deny) to answer this time only, never to grant a standing permission and never to answer a question prompt, so that the buttons cannot widen what a session may do.
59. As the person running gx, I want Close dialog, Allow once and Deny to do nothing when the session is no longer Blocked, with a refusal shown, so that a late click never interrupts a turn.
60. As the person running gx with a Codex session, I want Allow once and Deny to answer Codex's approval, so that both harnesses get the same buttons. When the Codex screen is unreadable and no hook request exists, Deny sends Esc and Allow once is refused with a message that points to the key bar, as story 57 says.
61. As the person running gx, I want the same buttons on the Agents page and on the design page, so that neither page traps a session.
62. As a phone user, I want key buttons at least 44px tall that wrap, the screen rows to scroll sideways, and the notice buttons to fit, so that the screen view works with a thumb.
63. As the person running gx, I want a session that ended with `/exit` to become Exited with its chat kept, the Agents page showing the disabled composer and "Exited. Restore it in gx, Agents tab.", and the design page keeping Restore and New session, so that nothing about Exited changes.
64. As the person running gx, I want a session Blocked on an error ("Agent hit an error") to keep Screen only, with the Type field and Enter in the screen view as the way out, so that I can get out without a button that does not fit.
65. As the person running gx, I want a Queue agent's session to show Screen only, with no key bar and no Type field, and the keys route to refuse it with `session-read-only`, so that the ticket pause card stays the way to answer it.

## Implementation Decisions

Names, config keys, route paths, CLI verb names and internal markers are the implementer's choice unless a decision below names them. A route follows "Adding a server route" in `.ai/index.md`.

Glossary

- Add the terms **Command**, **Built-in command**, **Command list**, **Completion menu** and **Key input** to `GLOSSARY.md`, in a new "Commands and completion" section before "Skills".
  - **Command** is text at the start of a session's prompt that the harness runs itself instead of sending it to the model. It is `/name` for Claude and Codex, and `$name` for a Codex skill. A skill, a custom command file, a plugin command and an MCP prompt are commands too. Avoid "slash command" alone, because it misses the Codex `$` form.
  - **Built-in command** is a command the harness ships. gx keeps its own list of them per harness. Not a **Bundled skill**, which gx ships.
  - **Command list** is what the Completion menu shows for one harness in one worktree.
  - **Completion menu** is the menu that opens under or over a text field while a person types a command. It fills the text and never sends.
  - **Key input** is the key bar and text field of the screen view, served by the keys route. It sends keys or literal text to a live session.
- Use "session" in all of these. Do not use "chat" or "agent" alone.

Prompt rule and send path

- The one text rule of the session prompt route changes from "refuse `/` or `!`" to "refuse `!`". The refusal keeps the reason `prefix-not-allowed` and its message says text may not start with `!`. The 32 KiB cap stays. The same rule serves the web create route's First prompt and the CLI prompt verb. The CLI create route has no prefix rule, as before.
- `Continue` keeps its rules: design sessions only, built by the server.
- The prompt route keeps the harness check, the empty check, the web stale check on `last_message` and the Promptable check, with no exemption for any command.
- The server picks the send path from the text. Text starting with `/`, except `/compact` with or without arguments, is typed once. gx does not nudge it, does not retype it and does not press Enter again. This is the rule the runners already use (a single-send rule for `/` commands), now also applied to the server's own send. Everything else, a Codex `$` skill included, keeps the nudge path.
- The send raises the "Send failed" Actionable (with the text, so the person can send it again) only when gx could not type the command. A command that starts no turn raises nothing. A command that opens a dialog turns the session Blocked, and the Blocked notice takes over.
- A swallowed Enter on a `/` command is not retried. The text stays in the session's input box and the person sees it on Screen. Accepted cost of the send-once rule.
- A person's `/clear` or `/compact` in a design session follows the typed-text rule: it cancels the auto-advance countdown and clears the derived Continue. An accepted key from the key bar or text from the Type field on a design session does the same (see Key input).
- `/clear` changes the conversation id, not the gx session id. The page resets its chat view, scroll, on-screen Unread and `last_message` when the chat read's `conversation_id` changes. The draft stays, because it is keyed by the session id.

Command list

- One new read serves the menu. It takes either a session, or a project and a dir (for First prompt). A project and dir are checked as the web create route checks them: the dir must be the project root or a worktree the worktree read lists. The server refuses any other dir, because the read starts the harness there and that runs the person's hooks.
- The read refuses with the reasons the prompt route uses: `unknown-session` for an unknown session, `harness-not-supported` for a session whose harness has no command list, and `bad-request` for a bad id or a malformed request. An Exited session is still served, from its dir.
- The response gives each entry: name, argument hint, description, source, and aliases. It also says when the list is degraded, meaning the read failed and only built-ins are returned. Field names are the implementer's choice. The source is `built-in`, `skill` or `mcp`. A custom command file or a plugin command is a `skill`, because Claude lists them together.
- Claude: the server sends an `initialize` control request to `claude -p --input-format stream-json --output-format stream-json` started in the dir, with the environment the server gives every Claude launch, but with no gx session or agent-dir environment variables. The probe runs with `--no-session-persistence` (or the equivalent the implementer verifies against the installed Claude), so it leaves no transcript file and no session record. It merges the reply with gx's built-in list. A name or alias present in both appears once, and the reply's entry wins, because it reflects the installed version. Descriptions are unescaped (`&lt;` and `&gt;`). gx does not filter further: Claude already leaves out skills that are not user-invocable and keeps the ones with `disable-model-invocation`.
- Codex `/`: gx's built-in list only. Codex `$`: the `skills/list` method of `codex app-server` over stdio, with the dir as `cwds` and `forceReload`. A skill name present twice (Codex reads two user roots) appears once. Skills Codex reports as disabled do not show. Codex's `~/.codex/prompts` is not read.
- gx's built-in lists are Go data, one list per harness in one place, each with the version it was copied from. The Claude list holds every command of the docs table, so the fallback is complete. The Codex list is the TUI's command enum at the tag the list names, without the commands Codex hides in release builds (`rollout`, `test-approval`) and without the two debug commands. `app` shows only when the server runs on macOS or Windows. Codex's aliases are kept.
- Cache and freshness: 60 seconds per harness and dir. A read inside 60 seconds is served from the cache. Opening the menu shows the cached list at once. An open on a cache older than 60 seconds, or with no cache, starts a read, and the open menu updates when it ends. Simultaneous reads for one key share one process. A read has a time limit the implementer picks. A hidden server flag sets the cache age, as `--auto-advance-delay` works for auto-advance, so a flow can see a refresh without waiting.
- A failed or timed-out read returns the built-ins with the degraded flag. It is not cached as a success.
- Selecting an MCP prompt fills the typed form Claude accepts. The research listed `server:prompt (MCP)` and recorded the docs form `/mcp__server__prompt`, which was never run. The implementer runs it against the installed Claude and fills the form that works.
- The Claude read starts the user's SessionStart hooks and takes about 3 seconds. The 60-second cache bounds both.

Completion menu in the web

- One menu component and one list hook serve the three fields. A small adapter per field covers the vi-mode editor in the composers (`ExpandableTextField`) and the plain textarea of First prompt. Names and file layout are the implementer's choice.
- The menu opens when the text starts with `/` and holds no whitespace yet (all harnesses), or when the word under the cursor starts with `$` and holds no whitespace yet (Codex only). It never opens in a disabled composer. In vi mode it opens from typed text, not from normal-mode keys.
- Order and match: names and aliases, prefix first, then substring, alphabetical in each group, the typed part highlighted.
- Keys: Up and Down move, Enter and Tab fill, Escape closes. Enter and Tab do not send. Cmd/Ctrl+Enter and the Send button send exactly as the Agents page spec defines them (Enter adds a newline, a phone has no key send), with the menu open or closed. The send rule is unchanged. The composer placeholder keeps today's wording. The mock's "Enter sends" texts (the menu-empty line and the placeholder note) are superseded by this spec. With no match, Enter adds a newline.
- Filling puts `/name ` or `$name ` into the text at the typed token and moves the cursor after the space. It never sends.
- States: closed, open, filtered, no match (one line: "No command matches /x. Cmd/Ctrl+Enter sends it as typed.", with the typed token in place of `/x`, and "Send sends it as typed." on a phone; the menu stays open until a space or Escape closes it), loading (one line, when no cached list exists), list failed (built-ins plus one warning line, "Skills could not load"), keyboard selection and after choosing. The desktop menu has a footer of keys. The phone menu has two-line rows and no footer, and shrinks to the room above the on-screen keyboard. On the phone First prompt sheet the menu opens below the field.
- First prompt always uses a Claude session (the form has no harness field), so it shows `/` only.
- The design page keeps `codexSkillHint`.

Local command output

- Claude: the chat reader shows the non-empty `<local-command-stdout>` line of a built-in command as an output entry after the command pill. It hides the caveat line, empty output and the line `/compact` writes. ANSI codes are stripped before the text reaches the page. The server caps one entry's size; the cap is the implementer's choice and a cut entry ends with a line that says so.
- Codex: a user message starting with `/` becomes a command entry, the same as Claude's. Its rollout holds no output, so no output entry exists. `$gx-` pills stay.
- The page renders an output entry as muted, left-aligned, monospace plain text, cut at 12 lines with a "Show all" button. It never renders Markdown. An output entry is not an agent reply: it does not set Unread, it is not read as the session's last agent text by Continue or by the question parsing, and no send path uses it. It does count in the id that `last_message` compares.
- The design page uses the shared chat view, so it gets the same entries.

Key input

- One new keys route, served on the web mux and by a CLI verb. A request names the session and exactly one input.
  - Named keys: Esc, Enter, Tab, Up, Down, Left, Right, Ctrl+C, sent as the usual terminal sequences (the same for both harnesses).
  - Literal text, typed without Enter.
  - A permission answer, Allow or Deny, for the permission buttons.
- A request may carry a guard, "only while Blocked". Close dialog carries it. The permission buttons carry "only while Blocked on a permission". The key bar and Type carry none.
- Refusals:
  - an unknown session is `unknown-session`, and a bad id is `bad-request`;
  - a session that is Starting or Exited is `session-not-live`, and so is a session whose holder gx cannot dial;
  - a Queue agent is `session-read-only` (see Blocked notices);
  - no input, more than one input, an unknown key name, empty text, or text with a control character (anything below space, or DEL) is `bad-request`;
  - text starting with `!` is `prefix-not-allowed`;
  - text over 32 KiB is `text-too-long`;
  - a failed guard is `not-blocked`, and nothing is sent;
  - a request that was sent but left the dialog open (`ErrNotDelivered`) is `not-delivered`;
  - a request whose dialog changed before it was sent (`ErrRequestMismatch`) is `request-changed`, and nothing is sent, as the queue answer verb in `server/agentverbs.go` does;
  - an Allow that fails the guard below is `not-approvable`.
- A notice shows the text of the refusal it gets.
- Working, Idle, Blocked and Rate-limited sessions accept keys. gx's own startup answers (the trust dialog) are why Starting is refused.
- A key reaches the session as a person's input, so it stops gx's own nudging of a send in progress, as the TUI's keys do. The route logs an event with the source and the number of keys or bytes. It never logs the text, because a dialog can hold a secret.
- On a design session, an accepted unguarded keys-route request (a key from the key bar or text from the Type field, one that passed validation) cancels the auto-advance countdown and clears the derived Continue, as typed text does (story 37). Close dialog, Allow once and Deny carry a guard and do not.
- Allow once and Deny reuse the answer logic the PTY runner already holds for the queue (Allow is Enter on Claude and Deny is Esc, with the guard that the session waits on a permission prompt). The implementer shares that code and does not copy it. One refinement for Claude: Allow once re-reads the screen and sends Enter only when the cursor (the `❯` option) is on option 1 and option 1 is a plain "Yes". Otherwise it refuses with `not-approvable` and sends nothing, so Enter can never pick a standing permission. A screen that does not read (no question line with numbered options) is refused the same way. "Reads" means a question line plus numbered options, for any header. It is not the approvable flag of `claudeui.ParseDialog`, which accepts only Bash, Edit and Create headers, and the implementer must not reuse that header limit. Deny is always Esc.
- Plan approval is a permission prompt too (`permissionPrompt` in `nativerunner/pty.go` covers it). The implementer checks plan approval's options in the installed Claude. If option 1 is not a plain "Yes", plan approval shows Screen and Close dialog instead of the permission buttons. Either way, a server test covers it.
- Codex: the bar sends the same sequences. Allow once and Deny use the answer logic of the Codex approval: if the PermissionRequest hook holds a request in the session's agent dir (`GX_CODEX_AGENT_DIR`), the answer is the hook's decision file. Otherwise, with a screen gx can read, the key is `y` for Allow and Esc for Deny. With a screen gx cannot read and no hook request pending, Deny sends Esc and Allow once is refused with `not-approvable`. This differs from the reused `answerCodexByKeys`, which refuses both in that case. The key bar stays the way out. The comment in the Codex dialog code says the dialog text was never captured from a real Codex, so only the fake agents prove this. The live smoke checklist of `gx codex doctor` re-checks it.
- The permission notice shows the prompt text. The server reads it with the existing dialog parsers (Claude's from the screen, Codex's from the hook request or the screen). The page does not parse screens. The page asks for the text only while a permission notice shows, so the session list does not dial a holder for every Blocked session on every update. Where that read lives is the implementer's choice.
- The screen view is the existing Screen dialog. It keeps Refresh and Close, gains the key bar and the Type field, and refreshes the screen after each successful send.

Blocked notices

- The shared failure notice, which both pages use, gets the buttons. A Blocked session that waits on a permission shows Screen, Deny and Allow once with the prompt text. A Blocked session at a dialog or any other terminal prompt shows Screen and Close dialog. A Blocked session that hit an error (the label "Agent hit an error") keeps Screen only, because it is not a dialog and its input is ready. The web way out of an error-blocked session is the Type field in the screen view plus the Enter key. The composer rule does not change.
- A Queue agent's session is read-only here. The keys route refuses it with `session-read-only`. Its notice and Screen dialog keep Screen only, with no key bar and no Type field. The ticket pause card stays the way to answer it. This adds no new way into the blocked-pane pause (ADR 0042).
- ADR 0042: this effort adds no pause kind. A Blocked session is a session status, and a command can now put the web in front of one. Each way into Blocked has a web button that ends it, Close dialog or Allow once and Deny. When the button does not end it (a dialog that ignores Esc), the key bar and Type field in the screen view are the way out. The cursor and plain-Yes check lives only in the Allow path of the keys route. The queue's Approve and the blocked-pane pause are unchanged.
- A dialog that is a real question to the model (a question prompt) also shows Close dialog, which cancels it. Answering a question prompt from the web stays out of scope.
- After `/exit` the session turns Exited and the Exited behavior stays as the Agents page spec defines it.

Fixtures and fakes

- `fakeclaude` answers the `initialize` probe with a list the scenario sets, and can fail or delay it. `fakecodex` does the same for `codex app-server` `skills/list`. `fakeclaude` writes `<local-command-caveat>` and `<local-command-stdout>` lines for a built-in command the scenario names, and `fakecodex` writes a rollout user message for a `/` command. The fakes already model a dialog (`dialogNext`) and a permission dialog. `fakeclaude` also gets an option that ignores the Allow key, so the permission dialog stays open and the keys route answers `not-delivered`.
- Scenarios `agents-commands`, `agents-commands-slow` and `agents-commands-failed` extend `agents`. See Verification.

## Testing Decisions

- A good test drives the page as a person does (role and visible-text locators, clicks, keys) or calls the route and reads the result. It checks what shows on screen or what the server returns. It does not check component props, hook calls or storage key names.
- Seams, highest first.
  - The e2e flows against a `gxfixture` server, for the pages as a whole.
  - Vitest component tests of the Agents page and the design page with canned fetchers, as the existing `AgentsPage.*.test.tsx` and `DesignPage.test.tsx` do.
  - Server tests through the web mux, built with `servertest`: the prompt route, the create route, the new command list read and the keys route.
  - Parser tests on recorded replies: Claude's `initialize` reply, Codex's `skills/list` reply, Claude `<local-command-*>` transcript lines and a Codex rollout with a `/` message. The recorded text is real output from the research.
- Server tests to write:
  - the prompt route accepts `/cost`, `/clear` and `/compact foo`, refuses `!ls`, and keeps `continue: true` refused for a plain session;
  - `/cost` types once (a holder double counts the typing) and `/compact` keeps the nudge path;
  - a holder that cannot type raises "Send failed" with the text, and a command that starts no turn raises nothing (no fixture can force a typing failure, so this is a server test only);
  - the web create route accepts a `/` First prompt and refuses `!`;
  - the command list read merges and dedupes, unescapes, falls back to built-ins, caches 60 seconds, shares one process and refuses a dir outside the project;
  - the keys route covers every refusal above (including `session-read-only` for a Queue agent, `not-delivered` for `ErrNotDelivered`, `request-changed` for `ErrRequestMismatch` with nothing sent, and `session-not-live` for a holder gx cannot dial), the guard, Allow and Deny on Claude and Codex (hook decision and keys), the `not-approvable` guard (cursor off option 1, option 1 not a plain "Yes", a header other than Bash, Edit and Create that still reads, an unreadable Claude screen, and the unreadable Codex screen with no hook request, where Deny sends Esc), plan approval (permission buttons or Screen and Close dialog, as the installed Claude's option 1 decides), and a design session losing its countdown and Continue on an accepted unguarded keys request (key bar or Type) but not on Close dialog, Allow once or Deny;
  - the command list read refuses `unknown-session`, `harness-not-supported` and `bad-request`, and serves an Exited session from its dir;
  - the Claude list probe leaves no transcript file and no session record;
  - route parity tests, the web 404 allowlist test and the web write list.
- Web tests to write: the menu's open and close rules, matching and order, keys, fill, the states, vi mode, the First prompt adapter, `$` for Codex only, output entries (cut, Show all, hidden lines), the chat reset on a conversation change, the notices' button sets and the guard refusal text, the key bar and Type field.
- Existing tests to keep green: `AgentComposer.test.tsx`, `Composer.test.tsx`, `NoteBox.test.tsx`, `ChatLog.test.tsx`, `FailureBanner.test.tsx`, `NewSessionForm.test.tsx`, the `AgentsPage.*` tests, `sessionprompt_test.go`, `web_sessions_create_test.go`. The test in `AgentsPage.composer.test.tsx` that stubs the `/` refusal changes to `!`.
- Prior art: `webui/e2e/agents-composer.e2e.ts` and `agents-session.e2e.ts` for composer flows, `webui/e2e/codex.e2e.ts` for Codex, `webui/e2e/phone.e2e.ts` for phone flows, `server/web_test.go` for web mux writes, `cmd/server_parity_test.go` for route verbs, `nativerunner/pty_codexapproval_test.go` for the Codex approval.

## Verification

New scenarios to write (each becomes an implementation ticket). All three extend `agents`, so they keep its projects, sessions, design sessions, Codex session and the `feature` worktree in `demo`.

- `agents-commands`:
  - `fakeclaude` and `fakecodex` answer the list probes with a fixed list. Claude's list holds built-ins, skills (one with `<` in its description), a plugin command and an MCP prompt. Codex's `skills/list` holds skills, one name twice. The `feature` worktree holds one extra skill that no other worktree has.
  - An Idle Claude session whose chat holds a `/cost` pill with 14 lines of output, a `/model` pill with no output, an empty-output command, a caveat line and a `/compact` line.
  - An Idle Codex session whose chat holds a `/status` pill.
  - A Claude session Blocked at a dialog after `/model`.
  - A Claude session Blocked on a Bash permission prompt whose text is readable, and one whose screen cannot be parsed.
  - A Claude session Blocked on a permission whose first option is not a plain "Yes".
  - A Claude session Blocked on a permission with the `fakeclaude` option that ignores the Allow key, so the dialog stays open.
  - A Codex session Blocked on a permission through the hook, one with only the screen dialog, and one whose screen gx cannot read with no hook request.
  - A Claude session Blocked on an error ("Agent hit an error"), and a Queue agent session.
  - A Rate-limited Claude session and a Working Claude session.
  - A Claude design session with auto-advance on, and a Codex design session.
  - An Exited session.
- `agents-commands-slow`: as `agents-commands`, with the list probes delayed past the time the flow waits.
- `agents-commands-failed`: as `agents-commands`, with the list probes failing.

New flows to write, numbered after flow 146 (the catalog repeats some numbers; the implementer gives these unique ones, and the file names are the implementer's choice):

- 147 "Command menu opens and filters". `agents-commands`, Claude session. Type `/`: the menu opens with name, hint, description and source on each row, footer on desktop. Type `/co`: prefix matches first, then substring, alphabetical, typed part highlighted. A space and Escape close it. A no-match keeps the menu open with one line ("No command matches /x. Cmd/Ctrl+Enter sends it as typed.") and the text kept. A `/` after other text opens nothing.
- 148 "Command menu keys". Down, Up, Enter and Tab fill `/name ` and send nothing (`gx agents chat` shows no new message). A click on a row fills. Cmd/Ctrl+Enter sends the typed text with the menu open or closed. Enter with no match adds a newline. With vi mode on, typing in insert mode opens the menu and a normal-mode key does not. A disabled composer (the Exited session) shows no menu.
- 149 "Command list freshness and failure". The first open on `agents-commands-slow` shows the loading line. On `agents-commands-failed` the menu shows built-ins and "Skills could not load", and a typed command still sends. On `agents-commands`, add a skill file to the worktree, then reopen the menu after the (flag-shortened) cache age: the new skill shows after the refresh and the cached rows showed at once. Escaped text shows plain, a skill with `user-invocable: false` does not show, one with `disable-model-invocation` does.
- 150 "Codex command menu". `/` lists only built-ins, no skills. `$` at the start and in the second word lists skills, a doubled name once, and filling puts `$name ` at that word. `$` in a Claude session opens nothing. A Codex draft starting with `/gx-` shows the Codex hint.
- 151 "Command menu on the design page and First prompt". The design composer opens the same menu for a Claude and a Codex design session. In the New session form, First prompt lists the chosen worktree's commands (the `feature` skill shows only for `feature`), and the project's base worktree for "+ Create worktree".
- 152 "Phone, command menu". At 390px: two-line rows, no footer, a tap fills, the viewport shrunk to 390x420 to stand in for the on-screen keyboard (the menu scrolls inside the room left), First prompt opens below the field, and the design page menu.
- 153 "Send a command". `/cost` sends once (the fake records one typing), shows the pill and output, and leaves no "Send failed". `/compact foo` takes the nudge path. A Codex `$` skill takes the nudge path. A stale `last_message` refuses `/clear` like text. `/clear` on the Idle session: the chat resets to the new conversation, on-screen Unread and scroll reset, the draft stays. `!ls` is refused in the composer and in First prompt with the input kept, and `/cost` as a First prompt is accepted. The Working session shows its reason line and Send stays off.
- 154 "Commands in a design session". With the auto-advance countdown running, the person sends `/clear` or `/compact`: the countdown stops and Continue is cleared. The same happens when the person sends a key from the key bar or typed text from the design session's screen view. Close dialog, Allow once and Deny do not stop the countdown: with it running, click Close dialog on a design session Blocked at a dialog and check that the countdown keeps running. Continue still works on the design session when the countdown is off.
- 155 "Command output in the chat". `/cost` shows the pill and a muted monospace block cut at 12 lines with "Show all", which expands. ANSI codes are gone. The caveat, empty output and `/compact` lines are hidden. `/model` and the Codex `/status` show the pill alone. The design page shows the same. The session stays read in `gx agents list` and no output text is sent back.
- 156 "Key bar and Type field". Screen opens with the key bar and Type field. Each key reaches the fake (it records the sequence). Type sends the text with no Enter, and Enter is a separate key. Keys work in the Working, Idle, Blocked and Rate-limited sessions and are refused with a message on the Exited one. `!x` and text with a newline are refused. The screen refreshes after each send. On the Queue agent session the screen view shows no key bar and no Type field, and a keys request is refused with `session-read-only`. On the error-blocked session, the Type field plus Enter gets the person out, and the composer rule is unchanged.
- 157 "Close dialog". On the Agents page and the design page, the Blocked-at-dialog session shows Close dialog next to Screen. Clicking it sends Esc and the session turns Idle. A second click after the session left Blocked sends nothing and shows the refusal.
- 158 "Allow once and Deny". The notice shows the prompt text, Allow once and Deny. Allow lets the Bash command run and Deny declines it, on Claude and on Codex (the hook decision and the screen-dialog key). An unreadable Claude screen shows "The session asks for permission" with the buttons. Deny works there, and Allow once is refused with `not-approvable`. The session whose first option is not a plain "Yes" refuses Allow once with a message and sends nothing. So does a session where the person pressed Down in the key bar and then clicks Allow once (the cursor is off option 1). The Codex session whose screen gx cannot read, with no hook request, sends Esc on Deny and refuses Allow once with `not-approvable`. The session whose fake ignores the Allow key keeps the dialog open: Allow once shows the `not-delivered` text from the notice. The fake records that no standing option was picked. Both pages show the buttons.
- 159 "Exit with /exit". `/exit` turns the session Exited. The Agents page keeps the chat, a disabled composer and "Exited. Restore it in gx, Agents tab.". The design page keeps Restore and New session.
- 160 "Phone, key input and Blocked notices". At 390px: key buttons are at least 44px tall and wrap, the screen rows scroll sideways, and the Blocked notices fit with their buttons.

Existing flows to rerun because the shared composer, chat view, failure notice, Screen dialog or New session form changed: 4, 5, 14, 44, 47, 49, 73 (Note box), 75, 76, 77, 78, 79, 80, 81, 82, 83, 85, 86, 87, 88, 93, 123 to 127.

CLI and server verb checks (on `agents-commands`):

- `gx agents prompt <idle id> "/cost"` exits 0, and `gx agents chat <id>` lists the `/cost` pill and its output.
- `gx agents prompt <idle id> "/clear"` exits 0 (the earlier check that refused it is gone), and the conversation id changes.
- `gx agents prompt <idle id> "!ls"` is refused with `prefix-not-allowed`.
- `gx agents prompt <non-chat harness id> "/cost"` is still refused with `harness-not-supported`.
- `gx agents create --project demo --prompt "/cost"` works from the CLI.
- The new command list verb, for a Claude session and for `demo` with the `feature` dir, returns the fixed list merged with built-ins, aliases included. For a dir outside `demo` it is refused. On `agents-commands-failed` it returns the built-ins and the degraded flag. For the Codex session, `/` returns built-ins and `$` returns skills.
- The new keys verb: `Esc` to the Blocked-at-dialog session exits 0 and the session turns Idle. `Esc` to the Exited session is refused with `session-not-live`. `--text '!x'` is refused with `prefix-not-allowed`. A guarded Esc to the Working session is refused with `not-blocked`. A permission Allow to the non-Yes session is refused with `not-approvable`.

Story entries:

1. Flow 147. Mock state agents-open.
2. Flow 151. Mock state design-open.
3. Flow 151. Mock state first-prompt-open.
4. Flow 147. Mock state agents-open.
5. Flow 147, and the CLI command list check for a Claude session.
6. Flow 150. Mock state agents-codex-slash.
7. Flow 150. Mock state agents-codex-dollar.
8. Flow 147. Mock state agents-filtered.
9. Flow 147. Mock state agents-filtered.
10. Flow 147.
11. Flow 147. Mock states agents-no-match, design-no-match and phone-no-match.
12. Flow 148. Mock state agents-keyboard.
13. Flow 148.
14. Flow 148. Mock state agents-after-fill.
15. Flows 147 and 150.
16. Flow 148, and flows 76 and 79 rerun.
17. Flow 149.
18. Flow 149. Mock state agents-loading.
19. Flow 149. Mock states agents-list-failed, first-prompt-failed and phone-list-failed.
20. Flow 151.
21. Flow 151.
22. Flows 149 and 150.
23. Flow 147. Mock state agents-open.
24. Flow 152. Mock states phone-open and phone-keyboard.
25. Flow 152. Mock state phone-first-prompt.
26. Flow 148, and flows 123 to 127 rerun.
27. Flow 148.
28. Flow 153, and the CLI `/cost` and `/clear` checks.
29. Flow 153, and the CLI `!ls` check.
30. Flow 153, and the CLI create check. The web create refusal is in the server tests.
31. Flow 153.
32. Flow 153.
33. Flow 153 shows no failure for a command that starts no turn. The "Send failed" case is a server test, because no fixture can force a typing failure.
34. Flow 153.
35. Flow 153.
36. Flow 153.
37. Flow 154.
38. Server test (the CLI has no continue verb), and flow 154.
39. Flow 150.
40. Flow 153.
41. Flow 155. Mock state output-collapsed.
42. Flow 155. Mock state output-expanded.
43. Flow 155. Mock states output-collapsed, output-expanded and phone-output.
44. Flow 155.
45. Flow 155. Mock state output-pills.
46. Flow 155. Mock state output-pills.
47. Flow 155. Mock state design-open for the page, and output-collapsed for the block.
48. Flow 155.
49. Flow 156. Mock state screen-keys.
50. Flow 156. Mock state screen-keys.
51. Flow 156, and the CLI keys check.
52. Flow 156, and the CLI keys checks for `session-not-live`.
53. Flow 156.
54. Flow 156, and the CLI `--text '!x'` check.
55. Flow 157. Mock states blocked-dialog and phone-blocked-dialog.
56. Flow 158. Mock states blocked-permission and phone-blocked-permission.
57. Flow 158.
58. Flow 158.
59. Flows 157 and 158, and the CLI guarded Esc check.
60. Flow 158.
61. Flows 157 and 158.
62. Flow 160. Mock states phone-screen-keys, phone-blocked-dialog and phone-blocked-permission.
63. Flow 159.
64. Flow 156.
65. Flow 156, and the keys-route server test for `session-read-only`.

**Mock states**

- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-closed, 1200px, `agents-commands`, parts: top-bar, sidebar, chat, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-open, 1200px, `agents-commands`, parts: top-bar, sidebar, chat, composer, completion-menu, menu-item-1 to menu-item-10, menu-footer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-filtered, 900px, `agents-commands`, parts: composer-strip, composer, completion-menu, menu-item-1 to menu-item-3, menu-footer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-no-match, 900px, `agents-commands`, parts: composer-strip, composer, completion-menu, menu-empty, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-loading, 900px, `agents-commands-slow`, parts: composer-strip, composer, completion-menu, menu-loading, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-list-failed, 900px, `agents-commands-failed`, parts: composer-strip, composer, completion-menu, menu-item-1 to menu-item-6, menu-notice, menu-footer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-keyboard, 900px, `agents-commands`, parts: composer-strip, composer, completion-menu, menu-item-1 to menu-item-4, menu-footer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-after-fill, 900px, `agents-commands`, parts: composer-strip, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-codex-slash, 900px, `agents-commands`, parts: composer-strip, composer, completion-menu, menu-item-1 to menu-item-5, menu-footer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, agents-codex-dollar, 900px, `agents-commands`, parts: composer-strip, composer, completion-menu, menu-item-1 to menu-item-3, menu-footer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, design-open, 1200px, `agents-commands`, parts: top-bar, design-page, chat, composer, completion-menu, menu-item-1 to menu-item-6, menu-footer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, design-no-match, 900px, `agents-commands`, parts: composer-strip, composer, completion-menu, menu-empty, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, first-prompt-open, 1200px, `agents-commands`, parts: top-bar, first-prompt-field, completion-menu, menu-item-1 to menu-item-6, menu-footer, first-prompt-text.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, first-prompt-failed, 1200px, `agents-commands-failed`, parts: top-bar, first-prompt-field, completion-menu, menu-item-1 to menu-item-4, menu-notice, menu-footer, first-prompt-text.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-closed, 390px, `agents-commands`, parts: chat, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-open, 390px, `agents-commands`, parts: chat, composer, completion-menu, menu-item-1 to menu-item-6, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-keyboard, 390px, `agents-commands`, parts: chat, composer, completion-menu, menu-item-1 to menu-item-3, composer-text, send-button, on-screen-keyboard.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-no-match, 390px, `agents-commands`, parts: chat, composer, completion-menu, menu-empty, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-list-failed, 390px, `agents-commands-failed`, parts: chat, composer, completion-menu, menu-item-1 to menu-item-3, menu-notice, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-first-prompt, 390px, `agents-commands`, parts: first-prompt-field, completion-menu, menu-item-1 to menu-item-4, first-prompt-text.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-design-open, 390px, `agents-commands`, parts: chat, composer, completion-menu, menu-item-1 to menu-item-3, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, screen-keys, 1200px, `agents-commands`, parts: top-bar, screen-dialog, screen-rows, key-bar, text-field.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-screen-keys, 390px, `agents-commands`, parts: screen-rows, key-bar, text-field.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, blocked-dialog, 1200px, `agents-commands`, parts: top-bar, sidebar, chat, command-pill, blocked-notice, screen-button, close-dialog-button, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, blocked-permission, 1200px, `agents-commands`, parts: top-bar, sidebar, chat, blocked-permission-notice, permission-text, screen-button, deny-button, allow-once-button, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-blocked-dialog, 390px, `agents-commands`, parts: chat, command-pill, blocked-notice, screen-button, close-dialog-button, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-blocked-permission, 390px, `agents-commands`, parts: chat, blocked-permission-notice, permission-text, screen-button, deny-button, allow-once-button, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, output-collapsed, 1200px, `agents-commands`, parts: top-bar, sidebar, chat, command-pill, command-output, show-all, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, output-expanded, 1200px, `agents-commands`, parts: top-bar, sidebar, chat, command-pill, command-output, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, output-pills, 1200px, `agents-commands`, parts: top-bar, sidebar, chat, command-pill-1, command-pill-2, composer, composer-text, send-button.
- `$(gx tickets root)/webui-agents-skills/mocks/06-menu-mock.html`, phone-output, 390px, `agents-commands`, parts: chat, command-pill, command-output, show-all, composer, composer-text, send-button.

## Self-decided design questions

The map left these open. Each is decided here, with the reason.

- Which states accept keys? Choice: Working, Idle, Blocked and Rate-limited accept keys, and Starting refuses. The map said Working, Idle and Blocked. Reason: a Rate-limited session is live and may sit at a dialog, and Starting is where gx answers the trust dialog itself.
- What if Allow once could pick a standing permission? Choice: the `not-approvable` guard refuses unless the cursor is on option 1 and option 1 is a plain "Yes". Reason: the permission buttons must never widen what a session may do, and an unreadable screen gives no proof. The key bar stays free: a person can pick any option by hand.
- How does a permission answer travel? Choice: as an input of the keys route, next to named keys and literal text. Reason: one route with one set of refusals and one guard is easier to keep correct than a second route.
- What does Close dialog do on a question prompt? Choice: it cancels the prompt. Reason: a Blocked session must always have a web way out, and answering a question stays out of scope.
- What does an error-blocked session show? Choice: Screen only. Reason: it is not a dialog and its input is ready, so the Type field plus Enter is the way out.
- How does a flow see a cache refresh? Choice: a hidden server flag sets the cache age, as `--auto-advance-delay` does. Reason: a flow cannot wait 60 seconds.

## Out of Scope

- `!` shell commands from the web. The prompt route, First prompt and the Type field keep refusing text that starts with `!`. A later effort can open this.
- A safe list of `/` commands. The server accepts any `/` command.
- Answering a session's question prompts (a session asking the person to choose) with a button. Close dialog can cancel one, and the permission buttons never answer one. The key bar can still pick an option by hand. A later effort can open a button for this.
- A standing permission ("don't ask again") from the permission buttons. Allow once is the only allow button. The key bar stays free, so a person can still pick any option by hand.
- Deny with a reason from the web.
- Sending a command while the session is Working. Commands follow the Promptable rule of text.
- Listing the commands of a live session. Both harnesses can only list for a fresh process, so a skill added after a session started may show in the menu and still be unknown to that session.
- Retrying a `/` command whose Enter was swallowed.
- Choosing a harness in the New session form.
- Fixing the stale `~/.codex/prompts` check in `ralphloop` (`verifySkillWith`). The research found it. Codex 0.166.0 does not read that path.

## Further Notes

- Decisions come from the closed tickets 01 to 06 of `gx:webui-agents-skills` and the research in `docs/research/webui-agents-skills-commands.md` on the branch `research/webui-agents-skills-commands` (Claude Code 2.1.316, codex-cli 0.166.0). The map's Decisions so far are authoritative.
- Where tickets disagreed with the Agents page spec, this spec follows the Agents page spec. Tickets 02 and 06 say "Enter sends" in a few notes, and the Agents page spec says Enter adds a newline and Cmd/Ctrl+Enter sends. The send rule did not change, so the notes are read as that rule.
- When a choice was between a quick fix and a clean one, this spec takes the clean one: the permission buttons reuse the PTY runner's answer logic and refuse a standing first option, the Codex permission goes through the hook decision when there is one, the guards stop a late click from sending keys into a running turn, and the list read checks its dir before it starts a harness there.
- Known side effects. The Claude list read runs the person's SessionStart hooks. Ctrl+C on an idle Claude prompt twice ends the session, and the session then turns Exited. Esc sent to a Working session interrupts its turn, like the Interrupt button. The key bar sends what the TUI would get, so a person can pick any option by hand, even "Yes, and don't ask again", or answer a question prompt. Only the permission buttons are bound to this-time-only answers.
- The built-in lists are copies and age with the harnesses. Each list names the version it came from, so a later update knows what to compare.
- A command not available on the person's plan still shows in the menu from gx's built-in list, and typing it prints "Unknown command" in the session.
- The Codex approval dialog text and keys were never captured from a real Codex (the comment in the Codex dialog code says so). The fake agents prove the buttons. The live smoke checklist of `gx codex doctor` re-checks the real dialog.
