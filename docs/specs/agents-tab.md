# Agents tab: interactive Claude sessions inside gx

Source: wayfinder map `agents-tab` (tickets 01-10). Decision tags like `[T05]` point to the map
ticket that settled the point. `[map]` marks a point the map itself set. ADR: 0037 (gx renders
interactive sessions live). Builds on `docs/specs/agent-runner.md` (the PTY runner, its holder and
the `charmbracelet/x/vt` screen); its story numbers are written `AR-NN`.

Points the map never decided are marked **Assumption:** where they appear, and listed again at the
end of Further Notes.

## Problem Statement

A person who works with Claude by hand runs each session in some terminal: a herdr pane, a tmux
window, a spare tab. gx knows nothing about these sessions. To see which one finished, which one
waits on a permission prompt and which one hit a rate limit, the person has to look at each terminal
in turn. Some organizations block herdr, so herdr is not always there to help.

The sessions also die with their terminal. Closing the window, or a crash of the multiplexer, ends
the conversation; the person then has to find the right conversation id and run `claude --resume`
in the right dir by hand.

gx already hosts claude on a PTY it owns (the PTY runner), but only for ralph-loop iterations, and
only read-only: a person can look at a snapshot and send one-shot verbs, never type into the live
terminal (AR-70). There is no place in gx to start, watch and type into a session of one's own.

## Solution

A new **Agents tab** in the gx TUI. `gx agents` opens the TUI on it.

- `gx server` owns every **interactive session** through the PTY runner and its holder. Sessions
  outlive any TUI: closing gx, or opening a second TUI, never touches them. They survive a server
  restart, and gx resumes them on its own after a reboot.
- The tab shows a project-grouped sidebar of sessions and a live, typeable terminal panel for the
  chosen one. gx draws the terminal itself (ADR 0037): the TUI attaches to the session's holder and
  draws rows the holder renders.
- A **leader key** (`ctrl+a`, tmux-style) escapes the terminal for gx actions: a fuzzy
  jump-to-agent popup, next/previous, new, stop, remove, resume, copy mode and more.
- gx works out each session's **Session status** itself (Starting, Working, Idle, Blocked,
  Rate-limited, Exited) from the claude pid file, the OSC 7501 status protocol and the transcript.
  No Claude Code hooks. On top of it, two flags: **Unread** (something happened while you were not
  looking) and **Actionable** (the agent needs something specific from you).
- The server sends a macOS **Banner** with a sound when a session becomes Unread or Actionable,
  unless you are looking at it. Clicking the banner opens gx on that session. **AFK mode** also
  sends these events to Chat.
- Claude is the only **Harness** in v1, behind a `Harness` interface so codex or opencode can be
  added later without changing the tab.
- A map epic or map ticket in the Tickets tab can start a session pre-filled with
  `/gx-design <address>`.

## User Stories

### Opening the tab

1. As a user, I want `gx agents` to open the gx TUI on the Agents tab, so that I land on my sessions
   in one command. [map]
2. As a user, I want the Agents tab in the shared tab bar, with a count of Actionable sessions, so
   that I see from any tab that a session needs me. [T05]
3. As a user, I want the tab to open with the terminal focused on the session I viewed last, or
   else on the first session needing attention, so that I continue where I left off. [T06]
4. As a user with no sessions, I want the sidebar focused with a "`c` new session" hint, so that I
   know how to start. [T06]
5. As a user, I want the tab to keep its focus (sidebar or terminal, and which session) when I
   leave it and come back, like other tabs' memory. [T06]
6. As a user, I want the Agents tab to sit beside the other tabs and be reached with a tab key like
   them. **Assumption:** it is tab `8`. [map]

### Creating a session

7. As a user, I want a create form with project, dir, an optional name and an optional first
   prompt, so that I start a session in the right place with one form. [T04]
8. As a user, I want the project to default to the project in focus, so that the common case needs
   no typing. [T04]
9. As a user, I want to pick the dir from the project root, any existing worktree of the project, or
   "new worktree…", so that a session can work beside my other work. [T04]
10. As a user, I want the dir picker to show how many sessions already use each dir ("2 sessions")
    without warning or blocking, so that I share a dir on purpose. [T04]
11. As a user, I want "new worktree…" to ask for a name and create the worktree from trunk at
    `<linked worktree dir>/<name>` on branch `<name>` (the Worktrees tab's rule, no `agent/` prefix),
    so that worktrees look the same however they were made. [T04]
12. As a user, I want gx to never delete a worktree it made for a session, so that my work is
    safe. [T04]
13. As a user, I want the session name to default to the worktree name for a new worktree, and
    otherwise to claude's AI title (from the terminal title), then `<project>-<n>`, so that every
    session has a useful name without typing one. [T04]
14. As a user, I want an empty first prompt to start claude Idle, and a typed one to be sent at
    claude's first Idle (waiting out a Blocked dialog, dropped if the session Exits), so that I can
    start work in the same step. [T04, review]
15. As a user, I want sessions to run plain `claude --session-id <uuid>`, with no
    `--permission-mode auto`, no `--settings` TUI override and no `--disallowedTools`, so that the
    session behaves like claude in my own terminal. [T04]
16. As a user, I want the same env cleanup as loop agents (strip `CLAUDECODE` and
    `CLAUDE_CODE_ENTRYPOINT`, set `DISABLE_AUTOUPDATER=1`), so that the transcript is saved and
    claude does not update mid-session. [T04]
17. As a user, I want the trust dialog auto-answered only for a worktree gx just made, so that gx
    never trusts a dir on my behalf that I did not create through it. [T04]
18. As a user, I want one optional config key with extra claude args for interactive sessions, so
    that I can add flags I always use. **Assumption:** the key is `agents.claude-args` (a list of
    strings). [T04]
19. As a user, I want interactive sessions to never count against slot caps or permits, so that my
    own sessions never hold up the queue and the queue never blocks me. [T04]
20. As a user, I want the sidebar to show how many sessions are live, with no cap in v1. [T04, T05]

### Session from a ticket

21. As a user, I want "Start session…" in the Tickets tab `m` menu on a map-epic row and on that
    epic's open tickets, so that I can design a ticket in a live session. [T10]
22. As a user, I want it to open the create form pre-filled (dir = project root, first prompt =
    `/gx-design <project:epic>` or `/gx-design <project:epic/NN>`, name = the short address such as
    `agents-tab` or `agents-tab/10`), so that I only confirm. [T10]
23. As a user, I want every pre-filled field to stay editable before I confirm. [T10]
24. As a user, I want the TUI to switch to the Agents tab on the new session after launch. [T10]
25. As a user, I want "Start session…" offered only on map epics, never on loop-epic tickets, so
    that loop work still runs from the queue. [T10]
26. As a user, I want the session to store its **Session ticket** and show it in the sidebar row and
    the terminal header, so that I know what each session works on. [T10]
27. As a user, I want Tickets-tab rows to show a marker while a live session links to them, so that
    I do not start a second session on the same ticket by mistake. [T10]
28. As a user, I want gx to write no ticket status when a session starts from a ticket, so that the
    skill in the session claims the ticket itself as today. [T10]
29. As a skill author, I want `gx-design` to accept a ticket address and work that ticket instead of
    the first frontier ticket, so that the pre-filled prompt does what it says. [T10]
30. As a user, I want the `m` menu to gain a fuzzy filter, so that I find an item fast as the menu
    grows. [T10]

### Live view and input

31. As a user, I want the terminal panel to show the session's screen live, so that I can watch
    claude work as in my own terminal. [T01]
32. As a user, I want every key I type in terminal focus to reach the session, except the leader
    key, so that claude works as it does anywhere else. [T01, T06]
33. As a user, I want pasted text to arrive as a paste (bracketed only if claude turned that on),
    and a dragged file to arrive as its path, so that pasting behaves as in a normal terminal. [T01]
34. As a user, I want `ctrl+v` passed through so that claude reads an image from the macOS clipboard
    itself (claude shows `[Image #1]`). [T01]
35. As a user, I want a paste over 1 MB refused in the TUI with a message, so that a slip never
    floods a session. [T01]
36. As a user, I want the PTY to follow the size of the viewer that last focused or typed, so that
    the session fits the terminal I am using. [T01]
37. As a user with two TUIs open on one session, I want the other viewer's frames cropped or padded
    instead of resized, so that the two TUIs do not fight over the size. [T01]
38. As a user, I want a session with no viewers to keep its last size. [T01]
39. As a user, I want the TUI to attach only to the session on screen and detach when I switch
    session or tab, so that hidden sessions cost nothing. [T01]
40. As a user of a session hosted by an older holder (no `attach` op), I want a snapshot instead of
    an error. [T01]
41. As a user, I want clicks, drags and releases over the session's screen sent to an agent that
    reports the mouse (such as claude's diff sidebar "x"), and nothing else, so that gx's sidebar,
    border and info rows keep their own clicks. [T01]

### Scrollback and copy mode

42. As a user, I want the mouse wheel over the terminal panel to scroll that view back, even while
    the sidebar has focus, so that I can read earlier output quickly. [T09]
43. As a user, I want any key typed while scrolled back to snap the view to the bottom and go to the
    session, so that wheel scrolling never traps my input. [T09]
44. As a user, I want a "scrolled ↑ N" badge in the header while scrolled, and "+N new" when output
    arrives meanwhile, so that I know I am not at the live bottom. [T09]
45. As a user, I want the view to stay on the same lines while new output arrives, so that what I
    read does not run away. [T09]
46. As a user, I want `<leader> [` to enter keyboard copy mode, so that I can select and copy text
    from the scrollback. [T09]
47. As a user in copy mode, I want vi moves (`h`/`j`/`k`/`l`, arrows, `w`/`b`/`e`, `0`/`$`), half
    pages (`ctrl+u`/`ctrl+d`), full pages (`PgUp`/`PgDn`), `g g` top and `G` bottom. [T09]
48. As a user in copy mode, I want `v` for a character selection and `V` for a line selection. [T09]
49. As a user in copy mode, I want `y` or `enter` to copy to the system clipboard and leave, `q` to
    leave, and `esc` to clear a selection or else leave. [T09]
50. As a user, I want leaving copy mode to return the view to the live bottom. [T09]
51. As a user, I want copied text to have trailing spaces trimmed, so that it pastes cleanly. [T09]
52. As a user, I want 10,000 lines of scrollback per session, with no setting to tune. [T09]

### Leader key and actions

53. As a user, I want `ctrl+a` as the default leader, because herdr catches `ctrl+b` and Claude
    Code uses `ctrl+b` to background a command. [T06]
54. As a user, I want to set `agents.leader-key` in `config.json` to one key or a list of keys,
    each of which enters leader mode. [T06]
55. As a user, I want only the leader key to be configurable in v1; action keys stay hardcoded like
    every other gx key. [T06]
56. As a user, I want leader then one key, with no timeout, so that I never race a timer. [T06]
57. As a user, I want the leader pressed twice to send the leader key itself to the session, so that
    claude's own `ctrl+a` still works. [T06]
58. As a user, I want `esc` to cancel leader mode, and an unbound key to be dropped and end leader
    mode. [T06]
59. As a user, I want the which-key box (as for `g` chords) to list the leader actions while gx
    waits for the second key. [T06]
60. As a user, I want these leader actions: `j` jump popup, `b` toggle sidebar, `tab` move focus
    between sidebar and terminal, `n`/`p` next/previous session in sidebar order, `u` next session
    needing attention (Actionable first, then Unread), `c` new session, `a` run the Action, `x` stop,
    `d` remove, `r` resume, `[` copy mode, `z` toggle AFK mode, `1`-`8` switch gx tabs, `o` back to
    the last active tab, `q` leave the tab like `q` elsewhere, `?` help. [T06, T07, T09]
61. As a user, I want the leader to work in the sidebar too, so that one habit works everywhere.
    [T06]
62. As a user with the sidebar focused, I want plain keys without the leader: `j`/`k`/↑/↓ move,
    `enter` focuses the terminal, `/` opens the jump popup, and `c`, `a`, `x`, `X`, `r`, `b`, `u` do
    what their leader chords do. [T06]
63. As a user with the sidebar focused, I want the gx tab keys (`1`-`8`, `g…` chords) to work as on
    other tabs. The Agents tab is `8`, and the new chord `g a` (goto agents) opens it from any tab.
    [T06, review]
64. As a user, I want no direct shortcuts in terminal focus (no `alt+1..9`), so that claude gets
    every key it may need. [T06]
65. As a user, I want `<leader> x` and `<leader> X` to ask y/n before they act. [T06]

### Sidebar, terminal panel and jump popup

66. As a user, I want the tab to look like the rest of gx: frameless panels with a header row, a
    darker sidebar body and a seam between sidebar and terminal, with focus shown through the panel
    header, never a border. [T05]
67. As a user, I want the sidebar (~30 columns, left) to group sessions under project headers
    (folder icon + project), in creation order inside each group, so that rows never jump. [T05]
68. As a user, I want each sidebar row to show a status icon, the name, then two fixed mark slots
    (Unread, Actionable), with the name bold when Unread and muted otherwise. [T05]
69. As a user, I want to hide the sidebar so that the terminal takes the full width. [T05, T06]
70. As a user, I want the terminal header to show status icon, name and status label on the left,
    and project, dir and time on the right. [T05]
71. As a user, I want the first terminal body row (then a blank row) to show the Actionable reason
    and, when it carries an Action, the prompts `<leader> a` would run; otherwise the status detail
    (blocked kind and message, rate-limit reset), if any. [T05]
72. As a user, I want time to mean time since the last status change ("blocked for 12m"). [T05]
73. As a user, I want the jump popup to be a centered modal up to ~100 columns wide, each row
    showing status icon, marks, name, project, dir, time, status label and the Actionable reason or
    status detail. [T05]
74. As a user, I want the popup to fuzzy-filter (subsequence, case-insensitive) over name, project
    and dir as I type. [T05]
75. As a user, I want the popup to list attention first by default (Actionable, then Unread, then
    the rest, stable inside each bucket), and `tab` to toggle to project order. [T05]
76. As a user, I want the popup border to show "Jump to agent" and, on the right, the order and
    "matches/total" (e.g. "attention · 5/9"), with a hint line at the bottom. [T05]
77. As a user, I want ↑/↓ or `ctrl+n`/`ctrl+p` to move, `enter` to jump and focus the terminal, and
    `esc` to close. [T05]
78. As a user, I want nerd-font status icons in the Tickets/Queue visual language: Starting
    circle-o-notch (subtle), Working the Tickets/Queue progress spinner (blue, a teal `+bg` after the
    label while a background task runs), Idle circle-o (green), Blocked question-circle (yellow),
    Rate-limited pause-circle (mauve), Exited crashed times-circle (red, bold), Exited clean
    stop-circle-o (overlay, faint). [T05]
79. As a user, I want the Unread mark as a mauve dot and the Actionable mark as a red lightbulb,
    each in its own 2-cell slot so both fit without misaligning. [T05]
80. As a user without a nerd font, I want plain ASCII glyphs for every icon. [T05]
81. As a user, I want an "AFK" mark in the tab header while AFK mode is on. [T07]
82. As a user, I want `m` on the sidebar to open a menu with exactly Stop, Resume, Remove, Mark read
    and Run Action, hiding items that do not apply (e.g. Remove while live), so that I can act
    without remembering keys. Mark read clears Unread without viewing. [T10, review]

### Session status, Unread and Actionable

83. As a user, I want each session to show one Session status: Starting, Working, Idle, Blocked,
    Rate-limited or Exited, so that I see what it does at a glance. [T03]
84. As a user, I want Starting to last from process start until the first of: the pid file, an OSC
    7501 report, or a detected pre-session dialog. [T03]
85. As a user, I want Working to include claude's `shell` state (idle with a background task
    running), shown with the `+bg` marker. [T02, T03]
86. As a user, I want Blocked to carry a kind (`permission`, `question`, or `dialog` for
    pre-session dialogs) and claude's own message, so that I know what it waits for. [T03]
87. As a user, I want Rate-limited (from a 429 or API error in the transcript) to hold until the
    next turn starts working. [T02, T03]
88. As a user, I want Exited to say clean or crashed (non-zero exit, holder died). [T03]
89. As a user, I want status changes to show instantly, from claude's OSC 7501 report, with the pid
    file as the base, so that the sidebar is never a poll behind. [T02]
90. As a user, I want Unread raised when a turn finishes, or the session becomes Blocked,
    Rate-limited or Exited, unless I have it on screen in a focused TUI. [T03]
91. As a user, I want an Esc-cancelled turn, or my own Stop, to raise no Unread, banner or sound,
    because I caused it. Only a crash or claude exiting by itself raises them. [T02, T03, review]
92. As a user, I want viewing a session to clear its Unread. [T03, T05]
93. As a user, I want Unread to survive a TUI close and a server restart, and changes that happened
    while the server was down to raise it on the next start. A holder found dead is auto-resumed
    (story 113); it becomes Exited crashed + Unread only when auto-resume is off or unsupported, or
    the resumed claude dies within ~10 s. A holder that dies while the server is up becomes Exited
    crashed + Unread, with no retry. [T03, review]
94. As a user, I want Blocked to make a session Actionable with its reason, and to stop being
    Actionable as soon as the block resolves. [T03]
95. As an agent (or a skill such as `gx-design`), I want `gx agent actionable "<msg>" [--send
    <prompt>]...` to mark my own session Actionable with a reason and an optional Action, finding my
    session from `GX_AGENT_SESSION`, which the holder sets in the child's env. [T03, review]
96. As a user, I want an agent-raised Actionable to stay until my next input (a key or paste sent to
    that session) or its Action runs; the leader key, resize, scroll, copy mode, focus and viewing
    never clear it. [T03, review]
97. As a user, I want Idle, Rate-limited and Exited never to be Actionable on their own. [T03]
98. As a user, I want `<leader> a` to run the Action at once, with no confirm, since the header
    already shows its prompts. [T03, T06]
99. As a user, I want the server to run an Action step by step, a step being finished once the
    session has gone Working and back to Idle, so that it finishes even if I close the TUI. A step
    that sees no Working within ~10 s stops the rest, unless it is a slash command (`/clear`) and the
    session is still Idle, which counts as finished. A step that goes Blocked stops the rest.
    [T03, review]
100. As a user, I want an Action refused (and the session kept Actionable) when the session is not
     Idle, and the refusal shown as a footer message. [T03, T06]
101. As a user, I want a step that finds the session not Idle to stop the rest of the Action. [T03]
102. As a user, I want running an Action to clear the Actionable. [T03]
103. As a user, I want "no action" shown when the session has no Actionable or no Action. [T06]
104. As a user, I want an Action to hold prompts only, never shell commands. [T03]
105. (Dropped in review: mapping Session status down for the loop has no v1 consumer; moved to
     gx:agents-tab-v2.)

### Stop, remove, resume and restarts

106. As a user, I want Stop to kill claude and the holder and keep the session as Exited, so that I
     can still resume it. An Exited session shows its last screen read-only and faint, with a
     "`<leader> r` to resume" hint; scrollback is not kept. [T04, review]
107. As a user, I want Remove, only on an Exited session, to delete the session's state dir. [T04]
108. As a user, I want Resume on an Exited session to run `claude --resume <conversation id>` in the
     same dir with a new holder, keeping the same gx session (name, Unread, Actionable). [T04]
109. As a user, I want the session to follow the newest claude conversation id (e.g. after
     `/clear`), so that Resume always continues the latest conversation. [T04]
110. As a user, I want Resume to fail cleanly ("dir is gone") when the dir was removed. [T04]
111. As a user, I want no automatic retry after a crash. [T04]
112. As a user, I want a server restart to reattach every live session with its saved state. [T04]
113. As a user, I want a session whose holder died while it was live (not Exited, not stopped by
     me), for example after a reboot, to be resumed once on server start, ~100 ms apart, so that my
     sessions come back by themselves. [T04]
114. As a user, I want a resumed claude that exits within ~10 s to stay Exited (crashed) with
     Unread, never retried. [T04]
115. As a user, I want a session that was Working before the restart to come back Idle with a
     "resumed" Unread marker, and gx never to type "continue" for me. [T04]
116. As a user, I want `agents.resume-on-start` (default true) to turn auto-resume off. [T04]
117. As a user, I want sessions I stopped, or that had already Exited, never auto-resumed. [T04]

### Banners, sounds and AFK mode

118. As a user, I want the server, never a TUI, to send a Banner, so that two TUIs never notify
     twice and a banner comes even with no TUI open. [T07]
119. As a user, I want a Banner exactly when Unread is raised or the agent raises Actionable. [T07]
120. As a user, I want no Banner and no sound for a session on screen in a focused TUI, and no toast
     in its place. [T07]
121. As a user, I want each event to wait 1 s, merged with any other event for the same session in
     that window (one banner, the stronger sound, the Actionable reason as text), and dropped only if
     the session went back to a quiet state (e.g. Working), so that a quick flip does not notify and
     a turn end plus Actionable sends one attention banner. [T07, review]
122. As a user, I want one notification group per session, so that a newer banner replaces the older
     one. [T07]
123. As a user, I want two sounds: `attention` for Blocked or agent-raised Actionable, `done` for a
     finished turn, Rate-limited and Exited. [T07]
124. As a user, I want built-in sounds that differ from system sounds (CC0, from Kenney's audio
     packs), so that I tell gx apart from other apps by ear. [T07]
125. As a user, I want `agents.sounds.done` and `agents.sounds.attention` to point to my own sound
     files (relative to the config dir), with a failed file falling back to the built-in sound and a
     logged warning. [T07]
126. As a user, I want `agents.banners.enabled` and `agents.sounds.enabled` (both default true). [T07]
127. As a user, I want the banner title `<project> · <session name>` and a body with the status plus
     claude's message when there is one (e.g. "Blocked: permission — Bash(rm -rf build)"). [T07]
128. As a user, I want a banner click to bring my terminal forward and switch the most recently
     focused TUI to the Agents tab on that session (`gx agents focus <id>`). [T07]
129. As a user with no TUI open, I want a banner click to only bring the terminal forward. [T07]
130. As a user away from the machine, I want AFK mode (`gx afk [on|off]`, no argument toggles, or
     `<leader> z`) to send the same events to Chat through my existing Destinations, with no focus
     check, while banners and sounds keep going. [T07]
131. As a user, I want AFK mode to be one machine-wide switch that survives a server restart. [T07]
132. As a user, I want `gx notify --enable/--disable` and project notification overrides to apply to
     AFK Chat messages, as to every other Chat message. [T07]
133. As a user on Linux or Windows, I accept no banners in v1. [T07]

### Harness seam

134. As a maintainer, I want a `Harness` interface beside the agent runner, so that codex or
     opencode can be added later without touching the tab. [T08]
135. As a maintainer, I want the PTY runner and the holder to take a harness looked up by name (the
     existing `Kind` string, "claude"). [T08]
136. As a maintainer, I want the session state dir to store `harness: claude`, so that a later
     harness loads its own sessions. [T08]
137. As a maintainer, I want a fake test harness to run the tab's integration tests, so that the tab
     never names claude. [T08]
138. As a user, I want no harness picker in the create form in v1. [T08]

## Implementation Decisions

### Vocabulary

Terms from `GLOSSARY.md` ("Agent Runners", "Notification Surfaces", "Leader key", "Copy mode"):
**Interactive session**, **Harness**, **Session status**, **Unread**, **Actionable**, **Action**,
**Resume (interactive session)**, **Session ticket**, **Banner**, **AFK mode**. Loop iterations
are still **sessions** of an agent runner; this spec calls them loop sessions when the difference
matters. Avoid "agent kind", "needs attention" as a status name, "restart".

### Ownership

- `gx server` owns interactive sessions. It creates, stops, removes and resumes them, holds their
  Session status, Unread and Actionable, runs Actions, sends Banners and plays sounds. A TUI only
  shows sessions, sends input and reports what it shows. [T01, T03, T07]
- Interactive sessions run on the PTY runner's holder (the hidden `gx agent-holder`), whatever
  `agent-runner` is set to for loops. [map]
- Interactive sessions are not in `runs.json` and do not touch the reattach scan for tickets. They
  get their own scan of their own state dirs. [T04]
- They count against no slot cap or permit (ADR 0021, 0032). [T04]

### Session state dir

- One dir per session holds everything: meta (harness, project, dir, name, Session ticket), the
  current claude conversation id, the last seen Session status, Unread (with the "resumed" marker),
  Actionable (reason and Action), and `holder.sock` while live. No index file: the server lists the
  dirs. [T03, T04, T08, T10]
- The dirs live at `<gx data dir>/sessions/<short id>/`: apart from the loop agents' `agents/` root,
  short enough for the unix socket path limit, and never pruned by `server.log-retention`; only
  Remove deletes one. [review]
- **Assumption:** a gx session has its own short id, separate from the claude conversation id; this
  is the `<id>` in `gx agents focus <id>` and the banner group `gx-agent-<id>`.

### Server API and CLI

- Status reaches TUIs through the existing snapshot + SSE stream (ADR 0028). The session list uses
  it; the TUI never polls holders for status. [T01]
- New verbs settled by the map: `gx agents` (open the TUI on the tab), `gx agents focus <id>`
  (banner click), `gx afk [on|off]`, `gx agent actionable "<msg>" [--send <prompt>]...`. [T03, T07]
- **Assumption:** per ADR 0028 every new server route gets a CLI verb under `gx agents` with
  `--json` (list, create, stop, remove, resume, run the Action, mark read). Refusals are results
  with stable reasons (e.g. Action refused while not Idle, Remove refused while live, Resume
  refused when the dir is gone or the harness cannot resume).
- **Assumption:** `gx agent actionable` run outside an interactive session prints a short note and
  exits 0, so skills can call it unconditionally.
- **Assumption:** the Agents tab needs a running server; with none it shows an empty state that
  says so, like other server-backed views.
- TUIs report to the server whether their terminal has focus (Bubble Tea focus and blur messages)
  and which session is on screen, plus the terminal app from `TERM_PROGRAM`. The server uses this
  for the Unread rule, the banner skip and banner clicks. [T07] **Assumption:** these reports go
  through a server route and stop counting when the TUI's connection drops.
- `gx agents focus <id>` sends a new server event; the most recently focused TUI switches to the
  Agents tab on that session. [T07]

### Holder and the live-view protocol (ADR 0037)

- The TUI dials the session's `holder.sock` directly, not through the server. The server keeps
  create, stop and status. [T01]
- New optional holder op `attach`, listed by `ping`. The frozen core ops (AR implementation
  decisions) do not change. `attach` upgrades the connection to a long-lived two-way stream; many
  attaches per session. A holder without `attach` gets the snapshot fallback. [T01]
- Framing: `[u32 length][u8 type][JSON payload]`. Message types are append-only. A message over
  2 MB closes the attach with an error. [T01]
- Holder → TUI: `frame` (cursor x, y, visible; rows as ANSI strings from x/vt's line render; scroll
  offset and maximum) and `exit`. TUI → holder: `input` (key events and `paste`), `resize`, `scroll`.
  A `copy` request (start and end as absolute line + column) returns plain text with trailing
  spaces trimmed. [T01, T09]
- Output: the holder keeps the vt. One full frame on attach, then only rows x/vt marked dirty. The
  TUI runs no emulator and draws rows as they come. [T01]
- Pacing: at most one frame per 16 ms, with extra early frames for ~32 ms after a key. One "latest"
  frame slot per client; when full, the frame is dropped and a fresh full frame follows. No output
  means no frames and no timers. [T01]
- Input: key events, encoded by the holder with x/vt for the modes claude turned on. The vt's output
  now goes to the PTY (today it is discarded), so claude gets real answers to terminal queries. The
  leader key is caught in the TUI and never sent. Mouse: the wheel, and button press, drag and release over the screen, only to a child
  that reports the mouse; a left click there also focuses the panel. [T01]
- Paste: Bubble Tea paste → `paste` input → x/vt paste (bracketed only if claude enabled it).
  `ctrl+v` passes through so claude reads the clipboard image. Acceptance check: copy an image,
  press `ctrl+v` in the tab, claude shows `[Image #1]`. No temp-file fallback in v1: if the setsid
  holder cannot see the clipboard, file a follow-up. A paste over
  1 MB is refused in the TUI. [T01]
- Size: the PTY follows the viewer that last focused or typed; other viewers' frames are cropped or
  padded; with no viewers the size stays. Screen rules (pre-session dialogs) must work at widths
  other than 200. [T01] **Assumption:** a new holder (create, Resume, auto-resume) starts at the
  creating TUI's terminal panel size, else the session's last saved size, else the loop default.
- Scrollback: per-viewer scroll offset (from the bottom) in the holder. Source is x/vt's main-screen
  scrollback (claude's default TUI is the main screen), fixed at 10,000 lines. While a viewer is
  scrolled, new output grows its offset so the view stays put; the TUI shows "+N new". [T09]
- `attach` is not tied to interactive sessions, so a later live Watch of loop sessions needs no
  protocol change. [T01]
- New optional holder op `watch`, listed by `ping`: a long-lived stream on which the holder pushes
  Session status and the conversation id on every change. The server holds one per live session;
  it never receives frames, and the frozen `status` op is not polled. The holder also sends an
  `input` event whenever any client sends `input` (key or paste), so the server can clear an
  agent-raised Actionable. The stream closing without an `exit` means the holder died. [review]
- On claude's exit the holder writes its last screen (ANSI rows) to the state dir before it exits;
  the TUI reads that file from the state dir directly (as it finds `holder.sock`) for an Exited
  session. Scrollback is not saved. [review]
- The holder sets `GX_AGENT_SESSION=<gx session id>` in the child's env; `gx agent actionable`
  reads it. Harness-neutral, and stable across `/clear`. [review]
- The holder answers the OSC 7501 probe for every harness (a terminal feature, not a claude one).
  [T02, T08]
- The TUI attaches only to the session on screen. [T01]

### Harness seam

- A `Harness` interface sits in a neutral package beside `agentrunner.Runner`, not inside it. The
  runner decides how a session is hosted; the harness decides what runs in it. The PTY runner and
  the holder take a harness by name (the existing `Kind` string). [T08]
- Five parts, nothing more: [T08]
  - **Launch**: command, args and env for a new session. Claude: `--session-id <uuid>`, the
    `LaunchEnv` cleanup, the extra-args config key.
  - **Resume**: command for a saved conversation id; optional (`ok=false` hides Resume and skips
    auto-resume). Claude: `--resume <id>`.
  - **Watch**: started by the holder for the child pid; reports Session status (Starting, Working,
    Idle, Blocked + kind and message, Rate-limited) and the current conversation id. It merges OSC
    7501 with the harness's own signals. Claude: pid file + transcript. Replaces the pid watcher
    hard-wired into the holder today.
  - **Startup screen**: classifies pre-session dialogs and gives the trust key (claude: the
    existing `claudeui` startup classifier and trust key).
  - **Display name**: what to read from the terminal title (claude: the AI title from OSC 0).
- Exited is the server's, from process death, not the harness's. [T08]
- v1 uses the seam for interactive sessions only: ralph-loop's claude/codex branches, the herdr
  runner's kind switch and the loop PTY launch args stay as they are. [T08]

### Claude status signals (claude harness)

| Signal | Use | Notes |
| :- | :- | :- |
| OSC 7501 (holder answers `ESC]7501;?`) | instant `working`, `done` (turn finished), `idle` (Esc-cancel), `blocked` kind `permission`/`question` + message | claude ≥ 2.1.295; one probe per claude start |
| Pid file `~/.claude/sessions/<pid>.json` | primary base: `busy`, `shell`, `idle`, `waiting` + `waitingFor`; conversation id | `shell` is new to gx: Working + bg |
| Transcript | rate limit (API error, `rate_limit`, 429), errors, background tasks | existing readers |
| OSC 0 title | display name only | spinner/`✳` not used for status |
| Screen rules | pre-session dialogs only (trust and similar) | Blocked kind `dialog` |
| OSC 9/777/BEL | not used | coarse, same text for permission and question |

[T02, T03]

- Precedence: Exited > OSC 7501 > pid file > transcript (rate limit) > screen rules
  (pre-session only). [T03]
- A held rate limit is the exception: it beats an OSC 7501 `done` (and any Idle, or a Working from
  before it) until the next Working, from OSC 7501 or the pid file, clears it. Blocked still beats
  a held rate limit. [review]
- Claude Code hooks are not used (blocked in some orgs). [map]

### Session status model

- Six statuses, interactive sessions only; the loop keeps the runner's state. [T03]
- No separate Done status: done vs Esc-cancel only drives Unread. The mapping down to the runner's
  state (T03) has no v1 consumer and moves to gx:agents-tab-v2. [T03, review]
- Unread: raised on turn finished (OSC 7501 `done`), → Blocked, → Rate-limited, → Exited, unless
  the session is on screen in a focused TUI. Not raised by Esc-cancel or by the person's own Stop.
  Clears on view or on Mark read. Saved in the state dir with the last seen status. On reattach the
  server diffs the current status against the saved one and raises Unread for any of these
  transitions. A dead holder at server start follows the auto-resume rule under Lifecycle; one
  that dies while the server is up → Exited crashed + Unread, no retry. [T03, review]
- Actionable, always with a reason:
  - From Blocked: derived, not stored; clears when the block resolves.
  - From `gx agent actionable`: stored next to Unread; clears on the person's next `input` (key or
    paste) to that session or when its Action runs; the leader key, resize, scroll, copy mode,
    focus and viewing never clear it. May carry an Action (ordered prompts). [T03, review]
- Action run: the server sends each prompt; a step is finished once the session has gone Working
  and back to Idle; no Working within ~10 s stops the rest, except a prompt starting with `/` whose
  session is still Idle then counts as finished (local slash commands like `/clear` never show
  Working); a step that goes Blocked stops the rest. Refused when not Idle (Actionable
  kept); a step that finds the session not Idle stops the rest; running clears the Actionable.
  Prompts only. [T03, review]
- `gx-design` and similar skills call `gx agent actionable` next to their final "Next:" line.
  [T03]

### Lifecycle

- Create form: project (defaults to focus), dir (root, existing worktree, "new worktree…" with a
  per-dir session count), optional name, optional first prompt. New worktree: `git.AddWorktree`
  from trunk, branch `<name>`, under the repo's linked worktree dir. Name fallback: worktree name →
  OSC 0 AI title → `<project>-<n>`. The first prompt is typed through the holder at the first Idle
  after start; while Blocked (e.g. a dialog) it waits; if the session Exits it is dropped. [T04,
  review]
- Launch: `claude --session-id <uuid>` plus the extra-args key; env cleanup as loops; trust dialog
  auto-answered only for a gx-made new worktree. [T04]
- Stop: holder `stop` op; the session stays Exited (clean), with no Unread, banner or sound. The
  tab shows its saved last screen. [T04, review]
  **Assumption:** Stop always asks y/n (ticket 06's `x` confirm), not only when Working as ticket
  04 first said.
- Remove: only when Exited; deletes the state dir. [T04]
- Resume: harness Resume command on the saved conversation id, same dir, new holder (the holder
  exits with claude), same gx session. The saved id follows the newest id from the pid file. No
  automatic retry after a crash. Fails cleanly if the dir is gone. [T04]
- Server start: live holder → reattach, state from disk. Dead holder and saved status not Exited
  (and not stopped by the person) → auto-resume once, ~100 ms apart; a resumed claude that exits
  within ~10 s stays Exited (crashed) + Unread. A session that was Working comes back Idle with
  Unread marked "resumed". `agents.resume-on-start` (default true) turns this off; no per-session
  opt-out. With auto-resume off, or a harness without Resume, a dead holder → Exited crashed +
  Unread. [T04, review]

### TUI: Agents tab

- Layout from the prototype (branch `prototype/agents-tab-layout`, round 4): frameless
  `ui.RenderPanel` panels built with `ui.PanelOptionsFor`, the sidebar with the darker sidebar
  body, split by `ui.RenderSeamColumn` with `ui.SeamColor`. Focused header: Surface1 with an orange
  bold title; unfocused: Surface with a mauve title. Selected rows use `ui.RenderRowHighlight`.
  The jump popup is a `ui.RenderModalFrame` like the Search modal (title in border, border
  `ui.ColorBorder`, blue title). [T05]
- Rejected layouts: a one-line session strip over a full terminal, a Queue-style table with a
  preview, a grid of live tiles. [T05]
- Sidebar: header "Agents" with "N live" on the right; project groups in creation order; row =
  status icon, name, Unread slot, Actionable slot; Session ticket shown on the row. Toggleable. [T05,
  T10]
- Terminal panel: header as in story 70 (plus the Session ticket, scroll badges and the "AFK"
  mark); first body row = Actionable reason + Action prompts, else status detail; then the live
  screen. Viewing clears Unread. [T05, T07, T09, T10]
- Jump popup: as in stories 73-77. The sidebar always stays in project order. [T05]
- Footer tab bar: an `agents` tab with the count of Actionable sessions. [T05]
- Icons (nerd font, `ui.IconSet`, shared with Tickets/Queue where the meaning matches): Starting
  circle-o-notch; Working = `tickets.TicketProgressSpinner`; Idle = TicketOpen; Blocked =
  TicketNeedsAnswer; Rate-limited = TicketPaused; crashed = TicketError; clean exit stop-circle-o;
  Unread dot; Actionable = SuggestedAction; project = FolderClosed; dir = Worktree; search =
  Search. Colors as in story 78. [T05]
- The prototype's status-to-icon mapping, trimmed (decision-rich part):

  ```go
  // from prototype/agents-tab-layout, ui/agents/prototypelayout
  case starting:    fg(ColorSubtle)  iconStarting       // circle-o-notch
  case working:     fg(ColorBlue)    spinner frame      // + teal " +bg" label when shell
  case idle:        fg(ColorGreen)   TicketOpen
  case blocked:     fg(ColorYellow)  TicketNeedsAnswer
  case rateLimited: fg(ColorMauve)   TicketPaused
  case exited:      crashed ? fg(ColorRed).Bold TicketError : fg(ColorOverlay).Faint stop-circle-o
  ```

- **Assumption (ASCII fallback, `ui.Icons(false)`):** reuse the IconSet's existing ASCII glyph
  where the nerd icon is shared (Idle `o`, Blocked `?`, Rate-limited `P`, crashed `!!`,
  Actionable `*`, project `▸`, search `*`); new ones: Starting `~`, Working a `-\|/` spinner,
  clean exit `x`, Unread `+`.
- **Assumption:** the Working spinner ticks only while a Working session is on screen (sidebar or
  popup), so an idle tab costs no ticks (idle-cost spec).
- Keys as in stories 53-65. The leader is matched before any other key handling in the tab; the
  which-key box is `ui.RenderChordOverlay`. Inside copy mode `g g` is the copy-mode top, not a tab
  chord. [T06, T09]
- Copy mode is drawn by the TUI over holder frames: a cursor and selection over rows the holder
  sends; the copy itself is the holder's `copy` request, written with `ui/clip` (pbcopy). No OSC
  52. [T09]
- `m` on the sidebar: exactly Stop, Resume, Remove, Mark read, Run Action; items that do not apply
  are hidden. The `m` menu (suggested actions) gains a fuzzy filter: typing filters at once;
  `ctrl+n`/`ctrl+p` move down/up; `esc` cancels; `enter` picks. Its old `j`/`k`/`q` keys and
  number quick-select are removed; every printable key is filter text. [T10, review]
- An Exited session's terminal panel shows the saved last screen, read-only and faint, with a
  "`<leader> r` to resume" hint. [review]
- Tickets tab: "Start session…" in `m` on map-epic rows and their open tickets; a marker on rows
  with a live linked session. [T10] **Assumption:** the marker glyph is picked at implementation,
  from the IconSet.

### Banners and sounds

- The server sends one Banner per trigger (Unread raised, or agent-raised Actionable), after a 1 s
  delay, skipped with its sound when the session is on screen in a focused TUI. Triggers for the
  same session within the window merge into one banner: the stronger sound (`attention` beats
  `done`), the Actionable reason as text when there is one. Dropped only if the session went back
  to a quiet state (e.g. Working) before the delay ends. terminal-notifier group `gx-agent-<id>` per
  session. [T07, review]
- `osnotify.Notifier` changes from `Notify(ctx, title, body)` to
  `Notify(ctx, Notification{Title, Body, Group, Execute, Activate})`. Config banners keep group
  `gx-config`; the osascript fallback ignores Group, Execute and Activate. [review]
- Click: terminal-notifier `-execute "gx agents focus <id>"` plus `-activate <bundle id>` for the
  terminal the TUI reported (`TERM_PROGRAM`). With no TUI: only `-activate`. The osascript fallback
  has no click action. [T07]
- Sounds: `attention` (Blocked, agent-raised Actionable) and `done` (turn finished, Rate-limited,
  Exited). Built-in CC0 Kenney sounds embedded with `go:embed`, copied to disk once and played with
  `afplay`; the banner itself is silent. The implementation picks one Kenney sound per slot; a
  follow-up lets the person re-pick by ear. Custom paths fall back to built-in on failure with a warning. [T07]
- Config: `agents.banners.enabled`, `agents.sounds.enabled`, `agents.sounds.done`,
  `agents.sounds.attention`. No per-project or per-session mute. [T07]
- Text: title `<project> · <session name>`; body = status + OSC 7501 message. Chat uses the same
  text. [T07]
- AFK mode: one global switch held by the server and saved to disk; `gx afk`, `<leader> z`. While
  on, the same triggers go to Chat through Destinations without the focus check. [T07]
- macOS only; elsewhere `osnotify` stays a no-op. Sound playback follows ADR 0035 (an `argv` through
  an injectable runner, picked by injected `goos`, no build tags). [T07] **Assumption:** the sound
  player is built the same way as `osnotify`'s backends, so it is a no-op off macOS.

### Config keys (all new, under `agents.`)

`leader-key` (string or list, default `ctrl+a`), `resume-on-start` (default true),
`banners.enabled` (default true), `sounds.enabled` (default true), `sounds.done`,
`sounds.attention` (paths, empty = built-in), and the extra claude args key (name assumed
`claude-args`, story 18). [T04, T06, T07] **Assumption:** these keys live in the global (per-machine)
`config.json` and reload like other config keys.

## Testing Decisions

- **What is a good test:** it checks external behaviour only: what the server API and SSE stream
  report, what the holder sends on `attach`, what the tab renders and which requests it makes. Not
  internal state or private helpers.
- **Seams** (the fewest and highest that cover the feature; confirmed by the person):
  1. **Server interactive-session API** (highest). Tests drive a real server in-process
     (`server/servertest`) through its verb routes and SSE, with the fake harness as the session
     program. Covers create (incl. new worktree), first prompt (incl. waiting out Blocked), Session
     status as the fake harness reports it over `watch`, Unread and Actionable rules (incl. focus
     reports, which inputs clear, no Unread on Stop, Mark read), Action run (Working-then-Idle
     steps, ~10 s no-Working stop) and refusals, Stop, Remove, Resume, the reattach diff,
     auto-resume once and the ~10 s crash rule, holder death while the server is up,
     `resume-on-start`, AFK mode, and Banner and sound output through a fake `osnotify.Notifier` and
     a fake sound runner (1 s delay, merge within the window, drop on quiet state, group id, text,
     focus skip, click args).
  1b. **Claude harness Watch.** Tests feed the claude harness's Watch fake pid files, transcripts
     and OSC 7501 events and check the Session status and conversation id it reports. Covers the
     precedence order (OSC 7501 > pid file > transcript > screen rules), `shell`, Blocked kinds,
     rate limit, and following the newest conversation id. [review]
  2. **Holder `attach` protocol.** Tests dial a real holder over `holder.sock` running the fake
     harness. Covers full frame on attach, dirty-row frames, pacing and the dropped-slot full frame,
     key and paste input, resize ownership across two viewers, `scroll` and offset growth on new
     output, `copy` text, `exit`, the 2 MB limit, OSC 7501 probe answer, the snapshot fallback
     when `ping` lists no `attach`, the `watch` stream (pushes on change, closes on death), the
     last screen written on exit, and `GX_AGENT_SESSION` in the child's env.
  3. **Agents tab model.** Bubble Tea tests on the tab with a fake session client (fake server API
     + fake attach stream). Covers layout (golden renders of sidebar, header, popup, ASCII icons),
     leader mode (twice = send, esc, unbound key, which-key box), every leader and sidebar key,
     jump popup filter and order toggle, focus on open and tab memory, copy-mode moves, selection
     and the copy request, wheel scroll and snap-back, paste limit, `m` menus, and the create form
     (incl. pre-fill from a ticket).
- **Harness checks:** the claude harness's Watch runs as table tests on recorded fixtures (pid file
  snapshots, OSC 7501 byte streams, transcript lines). The fake test harness (a script that prints
  OSC 7501 and exits on command) proves the seam: seams 1 and 2 never name claude. [T08]
- **One e2e** (optional, beside the existing `e2e` server + TUI tests): real `gx server`, real TUI,
  fake harness: create a session, type, see the echo, close the TUI, reopen it, see the session.
- **Manual acceptance:** image paste with `ctrl+v` shows `[Image #1]` in real claude [T01]; the
  sound candidates are picked by ear [T07]; banner click focuses the right TUI.
- **Prior art:** `agentrunner/runnertest` (contract suite) and `nativerunner/holdersock_test.go`,
  `pty_test.go`, `reattach_test.go` for the holder; `testutil/agentfake` (the TUI fake of claude
  with a control side channel) as the base of the fake harness; `server/servertest` and the
  server's SSE tests; `osnotify_test.go` (injected runner); `testutil/teatestv2` and the
  Tickets/Queue model tests for the tab; `ui/tickets` golden-style render tests; `e2e/`
  server + TUI tests.

## Out of Scope

- Loop (ralph-loop) sessions in the Agents tab: they stay in the Queue tab. [map]
- A `gx agents attach` CLI that turns the current terminal into one session. [map]
- Real codex/opencode support and a harness picker: v1 ships only the seam (gx:multi-harness).
  [map, T08]
- Loop sessions adopting the harness seam (gx:agents-tab-v2). [T08]
- Configurable action keybindings: only the leader key is configurable in v1. [T06]
- Banners on Linux and Windows: v1 is macOS only. [T07]
- Banner click opening a new terminal, automatic AFK mode, answering sessions from chat
  (gx:future-enhancements). [T07]
- Copy-mode search and mouse drag selection (gx:future-enhancements). [T09]
- OSC 52 clipboard writes. [T09]
- Starting a session from a loop-epic ticket. [T10]
- An app-wide `m` context menu on every focusable thing (gx:future-enhancements). [T10]
- "Investigate" as an interactive session; it stays a herdr tab in v1 (gx:agents-tab-v2). [T10]
- Link or unlink of a Session ticket after create. [T10]
- A cap on interactive sessions, and per-session auto-resume opt-out. [T04]
- Claude Code hooks as a status channel. [map]

## Further Notes

- This spec reverses the agent-runner non-goal "never attach to its terminal" for interactive
  sessions only; loop sessions keep snapshots and one-shot verbs (ADR 0037, AR-70).
- The PTY runner may later replace herdr as the default runner. Holder ops (`attach`, `scroll`,
  `copy`) stay generic so loop sessions can reuse them, e.g. a live Watch. [map, T01]
- herdr (`~/dev/herdr`) was the reference for the leader key, live view, scrollback, auto-resume on
  restore and notifications. gx does not reuse herdr's sounds (no source or credit). [T01, T04, T06,
  T07, T09]
- OSC 7501 facts were measured on Claude Code 2.1.300; details on branch
  `research/claude-status-signals` (`docs/research/claude-status-signals.md`). The probe is lost if
  claude runs with `CLAUDE_CODE_DISABLE_TERMINAL_TITLE`; the pid file still covers status. [T02]
- Keybindings are hardcoded today in the app shell's tab handling; this spec adds the leader as the
  first configurable key. `osnotify` has banners but no sounds today. [map]
- Follow-on tickets filed by the map: gx:multi-harness/01 (harness picker), gx:agents-tab-v2/01
  (loops adopt the harness), gx:future-enhancements/04 and /05 (copy-mode search, mouse selection).

### Assumptions (not decided by the map)

- The Agents tab is tab `8`.
- The extra claude args key is `agents.claude-args` (list of strings).
- A gx session has its own short id, used by `gx agents focus <id>` and the banner group.
- Every new server route gets a `gx agents …` CLI verb with `--json` (ADR 0028); refusals are
  results with stable reasons.
- `gx agent actionable` outside an interactive session prints a note and exits 0.
- The Agents tab with no running server shows an empty state that says so.
- TUI focus and on-screen reports go through a server route and expire with the TUI's connection.
- A new holder starts at the creating TUI's panel size, else the last saved size, else the loop
  default.
- Stop always asks y/n.
- ASCII fallback glyphs as listed under TUI.
- The Working spinner ticks only while a Working session is on screen.
- The Tickets-tab linked-session marker glyph is picked at implementation.
- The sound player follows ADR 0035 like `osnotify` and is a no-op off macOS.
- All `agents.*` keys are per-machine `config.json` keys that reload like other config.
