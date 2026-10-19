# Session prompting: fast, reliable first prompts and Actions

Map: `gx:session-prompting` (decisions 01–09, plus review findings). Sources: `gx:follow-ups/13`, `14`, `15`, `22`.

## Problem Statement

A person who starts a session or runs an Action waits too long, and sometimes gets a broken prompt:

- "Start session…" on a map epic opens the Agents create form and asks for a directory. A design
  session always runs in the project's trunk checkout, so the question is pointless. When a design
  session for that map already exists, the menu still only offers to start another one.
- After the session is created, `/gx-design <epic>` is typed about 10s later. gx waits for the
  first Idle, and Claude reports Idle late.
- An Action of `/clear` then `/gx-design …` pauses about 10s after `/clear`. `/clear` runs locally
  in Claude and never goes Working, so the step only ends when its 10s start timeout fires.
- The first prompt is typed once, with no check that it was sent. A swallowed Enter leaves it in
  the input.
- The nudge's retype clears with one Ctrl+U, which clears only the current line. A multi-line prompt
  (investigate launch prompts have a multi-line "Matched Entry" section) keeps its earlier lines and
  the retype doubles the text.
- With Claude's vim mode on, the input can be in NORMAL mode, for example after gx sends Esc on an
  interrupt or a deny. Typed text then runs as vim commands and the prompt is mangled.
- gx types its prompt into whatever the input already holds, so a person's draft and gx's text mix.

## Solution

Every path that types a prompt into a session goes through the same harness-owned steps. Each step
is fast, checks what it can, and never types over text it didn't write.

- **Start session on a map.** The create form opens with Where already set to the project's trunk
  checkout, and focus on Name. Enter creates at once. When a matching **Design session** exists, the
  menu shows "Go to design session…" above "Start session…".
- **Ready signal.** Each **Harness** reports when its input is ready. For Claude it is the earliest
  of the pid file `idle`, an OSC 7501 report, or a screen match on its input box. The first prompt
  is typed on that signal, or on the first Idle if that comes first, so it never hangs.
- **Nudged first prompt.** A real first prompt goes through the nudge (wait for Working, then Enter
  or retype). A slash first prompt is typed once and ends like an Action slash step, with one extra
  Enter if it is still in the input. Any input from the person stops the remaining nudges.
- **Fast slash steps.** An Action slash step ends on the first of: a full turn, a new conversation
  id, or a 1s Idle settle window, with one extra Enter if it is still in the input. No list of
  commands.
- **Ready to type.** Before any typed text, the harness makes sure its input accepts text. Claude
  with vim mode on and no `-- INSERT --` marker gets `i`, then a short re-check.
- **Draft check.** Before the first prompt and each Action step, gx checks the input is empty. If a
  person's draft is there, gx doesn't type and raises the session as **Actionable** with the unsent
  prompts.
- **Whole-input clear.** The nudge's retype clears the whole input (repeated Ctrl+U, then an empty
  check), never Ctrl+C.
- **Failures are one key away.** Every first-prompt failure raises the prompt as a one-step
  gx-raised Actionable. The person's input never clears a gx-raised Actionable; only `a`, a newer
  Actionable or exit replaces it.
- **Logs.** Each step writes a debug line, so the timing targets can be checked from the server
  log.

Targets, which a person measures by hand once the epic lands (see Measurement):

- A new session gets its first prompt within ~2s of the harness being ready.
- Each Action step, `/clear` included, starts within ~1s of the step before finishing.
- Prompts land in full, never doubled or cut short.

## User Stories

### Start session on a map

1. As a person on the Tickets tab, I want "Start session…" on a map epic to open the create form
   with Where already set to the project's trunk checkout, so that I don't pick a directory a
   design session always uses. [01]
2. As a person, I want the same prefill on the map epic's tickets, so that every design session
   starts the same way. [01]
3. As a person with a bare repo (for example `~/dev/gx/.bare`), I want Where set to the worktree
   that has trunk checked out (for example `~/dev/gx/main`), so that the design session has a
   working tree on trunk to write the spec, ADRs and glossary in. [01]
4. As a person, I want focus to start on Name, so that Enter creates the session at once. [01]
5. As a person, I want Name and Prompt to stay editable, so that I can still change them before I
   confirm. [01]
6. As a person whose project has no repo root, or no worktree on trunk, I want today's form with
   focus on Where and no error, so that I can still start a session by hand. [01]
7. As a person, I want "Go to design session…" on a map epic's menu when a session is linked to the
   epic or any of its tickets, so that I return to the design work instead of starting a second
   session. [01]
8. As a person, I want "Go to design session…" on a map ticket's menu when a session is linked to
   that ticket, or to the epic while the ticket is `claimed`, so that an epic-level run that works
   this ticket counts too. [01]
9. As a person, I want Exited and Stopped sessions still listed on the Agents tab to count as
   matches, so that going there lets me resume them. [01]
10. As a person, I want removed sessions not to count, so that the item never points at nothing.
    [01]
11. As a person with several matches, I want "Go to design session…" to pick the most recently used
    one, so that I land on the session I last worked in. [01]
12. As a person, I want "Go to design session…" listed above "Start session…", and "Start session…"
    kept, so that I can still start a parallel session for another unblocked map ticket. [01]
13. As a person, I want "Go to design session…" to switch to the Agents tab with that session
    selected, as a new session does after launch. [01]

### First prompt: ready signal

14. As a person who creates a session, I want its first prompt typed within ~2s of the harness
    being ready, so that I don't wait ~10s for `/gx-design` to start. [03]
15. As a person, I want gx to wait while the session is Blocked (for example the trust dialog in a
    directory gx didn't make), and to type the prompt once I answer it, so that the prompt never
    goes into a dialog. [03]
16. As a harness author, I want the ready signal to be a harness method, so that the server holds no
    Claude logic and a new harness only supplies its own rule. [03]
17. As a gx maintainer, I want Claude's ready signal to be the earliest of the pid file `idle`, an
    OSC 7501 report and an input-box screen match, and any signal that adds nothing dropped after
    measuring, so that the signal is as fast as it can be with no dead parts. [03]
18. As a ralph-loop user on the PTY runner, I want the launch wait to use the same ready signal
    through the shared harness code, so that loop launches get faster with no loop-specific work.
    [03]
18a. As a person, I want the first prompt typed on the first Idle when it comes before any ready
    signal, so that a session whose ready signal never fires still gets its prompt. [review]

### First prompt: delivery

19. As a person, I want a real first prompt (no leading `/`) delivered through the nudge (wait for
    Working, then Enter or retype), so that a swallowed Enter doesn't leave it unsent. [03, 08]
20. As a person, I want a slash first prompt (such as `/gx-design …`) typed once, never retyped, so
    that a local command never runs twice. [08]
21. As a person, I want a slash first prompt to end on a full turn, a new conversation id or the
    Idle settle window, as an Action slash step does. [08]
22. As a person, I want gx to press Enter once if the slash first prompt is still in the input when
    the settle window ends, so that a swallowed Enter is recovered. [08]
23. As a person, I want gx to stop without pressing Enter when the input is empty or the empty
    check can't decide, so that gx never submits text it isn't sure about. [08]
24. As a person who starts typing in the new session, I want my input to stop every remaining nudge
    (Enter and retype), so that gx never submits my half-typed text. [08]
25. As a person, I want a nudge cancelled by my input to be no failure (no warning, no Actionable),
    so that typing in my own session is never treated as an error. [08]

### First prompt: failures

26. As a person whose first prompt fails (nudge stuck, nudge start timeout, still Starting after
    30s, or vim NORMAL mode give-up), I want the session raised as Actionable with the prompt as a
    one-step Action, so that one `a` retries it. [09]
27. As a person, I want each failure's reason to name its cause (stuck, agent never started, vim
    NORMAL mode, input not empty), so that I know what to fix before pressing `a`. [09]
28. As a person, I want gx to clear its own leftover text once after a stuck or timed-out nudge, so
    that `a` doesn't then refuse on gx's own text as a draft. [09]
29. As a person, I want the reason to tell me to clear the input when gx's clear left text, or its
    check can't decide, so that I know why `a` would refuse. [09]
30. As a person, I want a Banner and sound (`sound.Attention`) for a gx-raised Actionable, the same
    as for an agent-raised one. [07, 09]
31. As a maintainer, I want a warning log line for every first-prompt failure, so that failures show
    in the server log. [09]

### Actions

32. As a person who runs an Action of `/clear` then a prompt, I want the next step typed within ~1s
    of `/clear` finishing, so that I don't wait 10s. [02]
33. As a person, I want a slash step to end on the first of a full turn, a new conversation id while
    Idle, or the Idle settle window, so that both local and turn-running commands end fast. [02]
34. As a person, I want a real-prompt step unchanged (Working then Idle, 10s start timeout), so that
    a prompt that never started still stops the rest. [02]
35. As a harness author, I want no hard-coded command list (no `/clear`), so that any harness that
    reports a conversation id gets the fast path and any other falls back to the settle window.
    [02]
36. As a person, I want the settle window to default to 1s and be configurable apart from the step
    start timeout, so that it can be raised if a skill command goes Working late. [02]
37. As a person, I want the next step typed at once after a conversation-id change, so that no fixed
    delay is added. [02]
37a. As a person, I want gx to press Enter once if an Action slash step is still in the input when
    the settle window ends, and to stop without Enter when the input is empty or the check can't
    decide, so that a swallowed Enter doesn't make the next step's draft check refuse on gx's own
    text. [08, review]

### Ready to type (vim mode)

38. As a person with Claude's `editorMode: vim`, I want gx to press `i` when Claude's footer has no
    `-- INSERT --`, then re-check for about 1s, so that my prompt is typed as text, not as vim
    commands. [05]
39. As a person, I want gx to read `editorMode` from Claude's config on every check, so that a
    mid-session `/config` toggle is caught. [05]
40. As a person, I want gx never to type into NORMAL mode, so that a prompt is never mangled. [05]
41. As a person, I want the ready-to-type step on every typing path: the first prompt, Action
    steps, `Prompt` and `PromptNudged` in both runners (before the retype's clear), and
    deny-with-reason after its Esc. [05]
42. As a person, I want a vim give-up handled by each path's own failure rule: the first prompt
    raises Actionable; an Action stops its remaining steps, as other step failures do;
    deny-with-reason fails like a deny that can't type its reason; the nudge stops as stuck. [05,
    09]
43. As a Codex user, I want the ready-to-type step to do nothing, since Codex has no modal input.
    [05]

### Draft check

44. As a person with a draft in the input, I want gx not to type its first prompt or Action step
    into it, and never to clear it, so that my text is never lost or mixed. [06]
45. As a person, I want the session raised as Actionable with only the unsent steps and the reason
    "input holds a draft", so that steps already sent never run twice. [06]
46. As a person, I want the same for the first prompt, raised as a one-step Action. [06]
47. As a person, I want my clearing or sending of the draft not to clear gx's Actionable, so that
    `a` still finds its steps. [07]
48. As a person, I want `a` with the draft still there to refuse again and raise the same
    Actionable with the same steps. [07]
49. As a person, I want a gx-raised Actionable replaced only by `a`, a newer Actionable or exit,
    while an agent-raised one is still cleared by my input as today. [07]
50. As a person, I want gx to type as today when the empty check can't decide, so that a missing
    check never blocks every prompt. [06]
51. As a person, I want the vim step first, then the draft check, then typing, on each path. [06]

### Nudge retype

52. As a person, I want a multi-line prompt to keep its line breaks, typed as raw LF writes with no
    bracketed paste, so that it lands as written. [04]
53. As a person, I want the retype to clear the whole input (Ctrl+U 2N−1+2 times for N typed lines,
    then the harness's empty check), so that the retype never doubles text. [04]
54. As a Codex user, I want gx never to use Ctrl+C to clear, since on an empty input it quits
    Codex. [04]
55. As a person, I want the nudge to stop as stuck when the input is still not empty after the
    clear, so that gx never types over leftover text. [04]
56. As a person on a runner whose screen read has no styling, I want the clear to rely on the press
    count alone, so that the retype still works. [04]

### Logs

57. As a maintainer, I want a kept debug line for each first-prompt step (start, first status,
    first Idle, ready, typed), so that I can measure the gaps from the server log, including how
    much earlier ready fires than first Idle. [03, review]
58. As a maintainer, I want a debug line per Action step (typed, first status, conversation-id
    change, done) naming the signal that ended it (turn, new-conversation, settle, start-timeout)
    and its elapsed time. [02]
59. As a maintainer, I want a debug line for each draft check, an Info line when gx refuses for a
    draft, a debug line when `i` is pressed, a warning on vim give-up, and a debug line "nudge
    cancelled: person input". [05, 06, 08]

## Implementation Decisions

### Vocabulary

- **Design session**, **Session ticket**, **Actionable** and **Action** as in `GLOSSARY.md`. The
  Actionable entry already covers a gx-raised one (draft or failed first prompt).
- **Typing path**: any code that types text into a session: the first prompt, Action steps,
  `Prompt` and `PromptNudged` in the herdr and PTY runners, and deny-with-reason.
- **Slash prompt**: a prompt whose first character is `/`. The only test gx makes on a prompt's
  text.
- **gx-raised Actionable**: an Actionable gx raised itself (draft or failed first prompt), as
  opposed to one an agent raised with `gx agent actionable` or one derived from Blocked.

### Harness typing seam

- The harness seam (see `docs/specs/agents-tab.md`, "Harness seam") grows a typing part. Each
  harness supplies:
  - **Ready**: when the input is first ready for a prompt after launch.
  - **Ready to type**: a step run before any typed text. It may press keys and re-check; it returns
    ready or give-up.
  - **Input is empty**: from a screen read, one of empty, not empty or can't decide. Can't decide
    when the read carries no styling, since the placeholder is told apart only by its dim (SGR 2)
    style, never by its words.
  - **Clear key**: the key a full clear repeats (Ctrl+U for Claude and Codex).
- The server, the nudge and the runners call these parts only. No Claude logic in the server and
  no hard-coded slash commands.
- Claude and Codex both supply the typing part. Codex needs only the typing part, not the rest of
  the interface, because the herdr runner still launches Codex through its kind switch. Where the
  Codex typing rules live, and how the herdr runner finds them by kind, is the implementer's
  choice.
- Rules found by hand (Claude Code 2.1.303, Codex 0.147.0):
  - Claude empty input: `❯ ` then a dim placeholder. Codex: a bold `›` then a dim placeholder.
  - A raw LF inserts a line break in both, without submitting.
  - Ctrl+U clears N lines in 2N−1 presses. Extra presses on an empty input are harmless.
  - Claude vim INSERT shows `-- INSERT --` in the footer rows below the input box. NORMAL shows no
    marker (the `NORMAL`/`INSERT` in the status row is gx's own statusline, not Claude's). `i`
    returns to INSERT.

### Ready signal

- Claude: the earliest of the pid file `idle`, an OSC 7501 report, or an input-box screen match
  (like the existing `claudeui` screen rules).
- Ready is a separate report from Session status. Session status precedence (OSC 7501 > pid file
  > transcript > screen rules) does not change. How the holder reports ready to the server is the
  implementer's choice.
- First Idle is a fallback: the first prompt fires on whichever comes first, ready or first Idle.
  So a session that goes Idle with no ready signal is still prompted, and never waits forever.
- Ready is ignored while the session is Blocked. gx waits as long as the session stays Blocked.
- The 30s start timeout counts Starting time from launch. A session that went Blocked is no longer
  Starting, so the timeout can't fire while a person answers a dialog.
- The PTY runner's launch wait reads the same ready report through the shared holder and harness
  code. No ralph-loop-specific work.
- The hand measurement (below) checks which of the three Claude signals fires first, and drops any
  that never fires first.

### First prompt (Create session only)

- Replaces today's "type on first Idle with raw `SendKeys`". Fires on ready or first Idle,
  whichever comes first. Order: ready to type, then the draft check, then deliver.
- **Real prompt**: delivered through `claudeui.PromptWithNudge` (default caps) with a Prompter over
  the session's holder. Ralph-loop's `PromptNudged` path is unchanged.
- **Slash prompt**: typed once. Ends on the first of a full turn, a new conversation id, or the
  settle window (same rule and config as an Action slash step). When the settle window ends, the
  empty check runs: the prompt still in the input → one Enter, then stop; empty or can't decide →
  stop.
- **Person input** (`StreamInput`) after gx typed the prompt cancels every remaining nudge, for
  slash and real prompts. Not a failure. Input before typing is the draft check's case.
- The slash rule and the input cancel live in the first-prompt path, not in the shared nudge.
- **Failures**: nudge stuck (`ErrStuckSubmission`), nudge start timeout (`ErrStartTimeout`), still
  Starting after 30s, vim give-up. Each one logs a warning, then raises a gx-raised Actionable with
  the prompt as a one-step Action and a per-cause reason.
  - Before raising after stuck or timeout, gx clears its own text once with the retype's clear.
    Still not empty, or can't decide → raise anyway, with a reason that tells the person to clear
    the input.
  - A turn that starts late after a timeout could make `a` send the prompt twice. Accepted: Actions
    need Idle, and the person chooses when to press `a`.
- A draft at first-prompt time raises the same one-step Actionable with "input holds a draft".

### Actions

- `waitForTurn` keeps its rule for real prompts: Working then Idle, 10s start timeout, the rest
  dropped on no Working.
- A slash step ends on the first of:
  - a full turn (Working then Idle);
  - a new conversation id in a StreamStatus while Idle (Claude's `/clear` changes its pid file
    `sessionId`);
  - the settle window: the session stays Idle for this long after typing.
- When a slash step ends on the settle window, the empty check runs, as for the slash first prompt:
  the step still in the input → one Enter, then the step is done; empty or can't decide → done. One
  shared rule for both.
- Blocked or exit still stop the rest.
- Settle window: default 1s, a new server config field beside `ActionStepStartTimeout`. Its name,
  and whether a user config key sets it, are the implementer's choice.
- The next step is typed at once after a conversation-id change. A delay is added only if the
  measurement finds keys lost while Claude redraws.
- Each step, in order: ready to type, draft check, type. A draft found before step K stops the
  Action and raises a gx-raised Actionable with steps K onward and "input holds a draft". Steps
  already sent never run again. A draft before step 1 keeps the whole Action.
- A vim give-up stops the remaining steps like any other step failure (logged, no Actionable).

### gx-raised Actionable

- The server tells a gx-raised Actionable apart from an agent-raised one. The marker is the
  implementer's choice. It is saved with the session, like the rest of Actionable.
- `StreamInput` keeps clearing an agent-raised Actionable, but never a gx-raised one.
- Replaced only by running it (`a`), a newer Actionable, or session exit.
- Banner and `sound.Attention`, with the reason as text, as for agent-raised.
- No dismiss key (a dismiss key would redesign the Agents tab; out of scope).

### Nudge retype clear

- Prompts keep their newlines. Every typing path writes them as raw text with LF line breaks: no
  bracketed paste, no flattening.
- Only the nudge's full retype clears, plus the one clear after a first-prompt stuck or timeout.
- The clear: ready to type first, then the harness's clear key 2N−1+2 times (N = lines of the text
  gx typed), then the empty check when the runner's screen read carries styling. With no styling,
  the count alone decides.
- Still not empty → the existing stuck error plus a debug line. Never Ctrl+C on any harness.
- Applies to both the herdr runner and the PTY runner, through `claudeui.PromptWithNudge`'s
  Prompter.
- `docs/specs/server-recovery.md` (story 52 and the nudge decision near line 406) changes to:
  "clears the whole input (repeated Ctrl+U) before every full retype".

### Deny-with-reason

- After its Esc, deny-with-reason runs ready to type before typing the reason. No draft check: it
  types right after its own Esc. A vim give-up fails like a deny that can't type its reason.

### Start session on a map (Tickets tab)

- "Start session…" on a map epic or map ticket sends Where prefilled with the trunk checkout, and
  the create form starts focus on Name.
- Trunk checkout: the repo root for a normal repo; for a bare repo, the worktree with trunk checked
  out. Trunk comes from the one trunk resolver (ADR 0036). No repo root, no worktree on trunk, or a
  Missing trunk → today's form, focus on Where, no error.
- "Go to design session…" matches sessions by **Session ticket**:
  - epic menu: a session linked to the epic or any of its tickets;
  - ticket menu: a session linked to that ticket, or to the epic while the ticket is `claimed`.
- Any session the Agents tab lists counts, Exited and Stopped included. Several → the most recently
  used one. Which timestamp means "most recently used" is the implementer's choice.
- "Go to design session…" switches to the Agents tab with that session selected, through the
  existing focus-session path.
- No other change to the create form.

### Logging

- All lines below are debug level and kept in the code after the measurement, except where marked.
- First prompt: start, first status, first Idle, ready (naming the signal that fired, or "first
  Idle" for the fallback), typed; nudge cancelled by person input.
- Action slash step: a debug line when the settle-window Enter is pressed.
- Action step: typed, first status, conversation-id change, done (naming the signal: turn,
  new-conversation, settle, start-timeout; with elapsed time).
- Draft check: one debug line per check; Info when gx refuses for a draft.
- Vim: debug when `i` is pressed; warning on give-up.
- First-prompt failure: warning.
- Exact text and keys of each line are the implementer's choice.

### Measurement

A person measures by hand once the implementation epic lands, using the kept debug lines, on real Claude (and Codex for the
herdr runner's typing rules), then records the results in this spec's Further Notes:

- Launch → first status → each Claude ready signal → typed, on a map-epic Create session. Drop any
  ready signal that never fires first. Confirm the ~2s target.
- Typed → Working for a skill slash command (`/gx-design`). If over ~700ms, raise the settle
  window's default above it.
- After `/clear`: whether typing at once loses keys while Claude redraws. Only if it does, add a
  short delay after the conversation-id change.
- Whether any Claude footer notice can hide `-- INSERT --` while Idle. The INSERT row rule and the
  ~1s re-check timeout are then the implementer's choice.
- Confirm the ~1s Action step target and that prompts land in full (multi-line included).

## Testing Decisions

- **What is a good test:** it checks external behaviour only: what the server API and SSE stream
  report, which keys reach the session, what the Tickets tab menu and create form show. Not
  internal state or private helpers. Timing tests use the config knobs (settle window, start
  timeouts) set short, not real-time sleeps of seconds.
- **Proposed seams** (the fewest and highest that cover the feature; for the caller to confirm):
  1. **Server interactive-session API** (highest, existing). Tests drive a real server in-process
     (`server/servertest`) through its verb routes and SSE, with `testutil/agentfake`'s program fake
     (the agentfake harness) as the session program; it grows an input-box model. Covers: first prompt typed on ready (incl. late pid `idle`,
     waiting out Blocked, the 30s Starting failure, first Idle before any ready signal); real prompt nudge (swallowed Enter); slash
     first prompt (typed once, Enter once if still in the input); person input cancelling the
     nudge; every first-prompt failure raising a gx-raised one-step Actionable with its reason,
     Banner and sound; the leftover-text clear; Action slash steps ending on new conversation id or
     settle (with one Enter when still in the input); draft refusals on the first prompt and before step K; gx-raised Actionable surviving
     `StreamInput` while agent-raised doesn't; `a` with the draft still there; vim NORMAL with `i`
     and give-up. The fake grows knobs for: a late ready report, vim NORMAL mode, a seeded draft, a
     swallowed Enter, a local slash command that changes the conversation id, and multi-line input
     with Ctrl+U clearing one line per press.
  2. **Harness typing rules** (new, table tests). Each harness's ready, ready-to-type, input-is-empty
     and clear key, fed recorded screen fixtures (styled and unstyled) and, for Claude, fake pid
     files, OSC 7501 events and config files with and without `editorMode: vim`. Covers Claude and
     Codex empty and draft inputs, can't decide on unstyled reads, the INSERT marker, and Codex's
     no-op ready-to-type.
  3. **Nudge** (existing, `claudeui.PromptWithNudge` with a fake Prompter). Covers the 2N−1+2 clear
     count for multi-line text, the empty check after the clear, stuck when not empty, count-only
     when the read has no styling, ready-to-type before the clear, and never Ctrl+C. Both runners
     reach this through their Prompter, so one suite covers both.
  4. **Tickets tab model** (existing, Bubble Tea tests). Covers the prefilled Where (normal repo,
     bare repo with a trunk worktree, no trunk worktree), focus on Name, "Go to design session…"
     matching rules and order, and the switch to the Agents tab.
- **Manual acceptance:** the hand measurement above on real Claude, Claude with vim mode on, and
  Codex on herdr.
- **Prior art:** `server/sessions_test.go`, `server/sessions_action_test.go`,
  `server/sessions_actionable_test.go`, `server/sessions_banner_test.go` (seam 1);
  `harness/claudepid_test.go`, `harness/osc7501_test.go`, `agentrunner/claudeui/screenmatch_test.go`
  with its `testdata` screens (seam 2); `agentrunner/claudeui/nudge_test.go` (seam 3);
  `ui/tickets/start_session_test.go` (seam 4).

## Out of Scope

- New harnesses (opencode, cursor-cli, a native Codex harness): that is `gx:multi-harness`. The
  typing seam is harness-agnostic so they slot in. Actions and the first prompt run only on native
  sessions, which only Claude supports today; Codex gets them when `gx:multi-harness` adds it.
  Confirmed in review: for now Codex gets only the typing rules (whole-input clear, empty check,
  never Ctrl+C, no-op ready to type), in the herdr runner's nudge.
- Changing harness launch arguments in ways that skip trust prompts, or passing the first prompt as
  a launch argument.
- Redesigning the Agents create form beyond the directory step and the starting focus.
- A dismiss key for a gx-raised Actionable.
- Separate work for ralph-loop ticket launches: they get only what the shared harness and nudge
  code gives them. Ralph-loop's `PromptNudged` keeps its nudge as is (no slash rule, no input
  cancel).
- Stashing and restoring a person's draft.

## Further Notes

- Facts behind the decisions (hand tests in tmux with raw PTY writes) are in the map tickets
  `gx:session-prompting/04` and `05`.
- The hand measurement records its numbers here: which Claude ready signal fires first, typed →
  Working for `/gx-design`, any keys lost after `/clear`, and the footer notices seen near
  `-- INSERT --`.
