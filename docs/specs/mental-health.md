# Mental health: Bedtime and Break Rest periods

## Problem Statement

gx is easy to overuse. The person works with agents late into the night and for hours without a pause. Nothing in gx tells them to stop, and every agent event pulls them back with a Banner, a Chat message or a sound.

## Solution

gx gets two kinds of Rest period, both off by default and both owned by the server.

- **Bedtime** covers one daily time range, for example 22:00 to 07:00, in the time zone of the machine that runs the server.
- **Break** starts after a stretch of active work in the web UI (default 55 minutes) and lasts a short time (default 5 minutes).

During a Rest period every open browser and phone shows the **Rest screen**: a mascot image, a message, and either the time Bedtime ends or a countdown to the end of the Break. The Rest screen covers the page and does not unmount it, so drafts survive. Banner, Chat and sound stay quiet, and gx drops what they would have sent. Agents, ralph-loop and the Queue keep working. The TUI is unchanged.

The person can end a Rest period early with "Unlock anyway". It asks them to type "I'm <anything> and I'm addicted to AI". gx counts Breaks and unlocks per day and shows them on the Stats page.

A web toast (the web UI toaster, not the TUI Toast) warns before each Rest period: 10 minutes before Bedtime and 2 minutes before a Break.

A new "Mental health" section of the settings page holds a master toggle "Enable mental health breaks", a Bedtime group and a Break group. Everything is off by default.

## User Stories

1. As a person who overuses gx, I want a master toggle "Enable mental health breaks" that is off by default, so that nothing changes until I opt in.
2. As a person, I want to turn the master toggle off and have every Rest screen, warning and active-time count stop, so that one switch disables the feature.
3. As a person, I want separate Bedtime and Break toggles under the master toggle, so that I can run only one of them.
4. As a person, I want a "Mental health" section on the settings page, so that I find all these settings in one place.
5. As a person, I want to set one Bedtime start and end time for every day, so that the rule is simple.
6. As a person who sleeps past midnight, I want an end time earlier than the start time to cross midnight (22:00 to 07:00), so that I can enter my real hours.
7. As a person, I want equal start and end times refused with a clear message, so that I cannot save a range that means nothing.
8. As a person, I want Bedtime to use the local time zone of the machine that runs the server, so that every device agrees.
9. As a person, I want to edit the Bedtime message, with `{end}` standing for the end time, so that the screen sounds like me. The default is "That's enough AI for today. Go to sleep and come back tomorrow at {end}."
10. As a person, I want a toast 10 minutes before Bedtime that names the exact start time, so that I can finish what I am doing.
11. As a person, I want the Rest screen to appear at once when I turn Bedtime on inside the range, so that the rule is not skipped by timing.
12. As a person, I want the Rest screen to cover the page without unmounting it, so that a draft prompt or an open form is still there when the Rest period ends.
13. As a person, I want nothing under the Rest screen to react to keys or clicks, so that I cannot work behind it by accident.
14. As a person, I want Break to count only my own input (typing, clicks, scrolling) in a visible web tab, so that gx measures real work.
15. As a person with several devices, I want active time to add up across all my browsers and phones, so that switching devices does not reset the count.
16. As a person, I want the TUI not to count as active time, so that Break stays a web UI feature.
17. As a person, I want the browser to stay cheap: flag-only input listeners, at most one small heartbeat per minute, and no timer while the tab is hidden, so that gx does not get slower.
18. As a person, I want to set the Work time and Break length, with defaults of 55 and 5 minutes, so that Break fits my day.
19. As a person, I want idle time equal to the Break length to reset the work count, so that a real pause counts as a Break.
20. As a person, I want a toast 2 minutes before a Break that names the exact start time and the Break length, so that I can wrap up.
21. As a person, I want Bedtime to win an overlap: a Break due or running when Bedtime starts is dropped and the work count resets after Bedtime, so that the two rules never fight.
22. As a person, I want a funny built-in message picked at random for each Break, so that the screen does not get stale.
23. As a person, I want to set one custom Break message that replaces the built-in ones, so that I can choose my own words.
24. As a person, I want the Rest screen to show a countdown in minutes and seconds during a Break, so that I know when it ends.
25. As a person, I want the Rest screen to show "Back at 07:00" during Bedtime, so that I know when to return.
26. As a person, I want a mascot image that matches the Rest period (sleeping in pyjamas for Bedtime, meditating for Break) on a dark page, so that the screen feels calm.
27. As a person on a phone, I want the same Rest screen at 375 px with a smaller mascot, so that it works on any device.
28. As a person with several browsers, I want all of them to show and leave the Rest screen together, so that I cannot dodge it on another device.
29. As a person, I want a dim "Unlock anyway" link that opens an input, so that ending a Rest period takes effort.
30. As a person, I want the unlock button to stay disabled until my text matches "I'm <anything> and I'm addicted to AI", ignoring case and extra spaces and accepting a curly or straight apostrophe, so that a typo does not waste my effort.
31. As a person, I want the server to check the sentence too, so that no client can skip the effort.
32. As a person, I want an unlock to end the current Rest period at once, for Bedtime and Break alike, so that I stay in control.
33. As a person, I want notifications to stay quiet until the original end of the unlocked Rest period, so that unlocking does not flood me.
34. As a person, I want the work count to restart after a Break unlock, so that I get a full work stretch.
35. As a person, I want Banner, Chat and sound quiet during a Rest period, and the held-back events dropped and never sent later, so that I come back to a quiet screen.
36. As a person, I want agents, ralph-loop and the Queue to keep working, and a session that needs me to wait, so that Rest periods cost no work.
37. As a person, I want turning the master toggle off during a Rest period to end it at once, so that the switch is honest.
38. As a person, I want a Stats "Mental health" section with Breaks against unlocks per day, and unlocks split by Break and Bedtime, so that I see how I use the feature.
39. As a person, I want no unlock counter on the settings page, so that the page stays about settings.
40. As a person, I want the server to remember an active Break and its unlock across a server restart, so that a restart is not an escape.
41. As a developer, I want one shared quiet check for every notification path, so that a new path cannot forget it.
42. As a developer, I want the two mascot images and their Codex prompts in the repo, so that the screen has art and the art can be remade.

## Implementation Decisions

Terms follow GLOSSARY.md: Rest period, Bedtime, Break, Rest screen. Nothing in code, docs or UI says "lock".

**Rest engine (server).** One deep module owns all Rest state. A pure core takes the state, the settings, the current time and an event (an active minute, an unlock, a settings change, a tick) and returns the new state. A thin runner holds the state, persists it, owns the timers and publishes a change event. The engine takes an injectable clock, so tests move time without waiting. It has no pause kind (ADR 0042 does not apply: a Rest period stops the person, not agents or the Queue, and the Rest screen's Unlock anyway button ends it from the web).

**State** (persisted in the state dir, written through a rename like AFK, so a restart keeps it):

- the work count and the last active minute;
- the current Rest period, if any: kind, start, end, the Break message chosen at its start, and whether it was unlocked;
- a "quiet until" time, set by an unlock to the original end of that Rest period.

The implementer picks the file name, the field names and the event name.

**Active time.** Time is cut into wall-clock minutes. A minute is active when any device sent a heartbeat in it, so two devices active in the same minute count once. The work count is the number of active minutes since the last reset. The count resets when the gap since the last active minute reaches the Break length, when a Break ends or is unlocked, and when Bedtime ends. No minute counts while a Rest period runs, while an unlocked Bedtime remains, or while the feature, or Break, is off.

**Break.** A Break starts in the same moment the work count reaches Work time. It ends after Break length. The Break message is chosen at the start (random built-in, or the custom message when set) and stored in the Rest period, so all browsers show the same text and a restart keeps it.

**Bedtime.** A Bedtime period exists while the local time is inside the range and the feature and Bedtime are on. It ends at the End time. When Bedtime starts, a Break that is due or running is dropped, the work count resets, and a dropped running Break writes a record that cancels its start record. A Break warning is not raised when the predicted Break start falls at or after the next Bedtime start.

**Warnings.** The snapshot carries at most one warning. A Bedtime warning exists from 10 minutes before the start until it starts. A Break warning exists once the work count is two minutes below Work time and no Rest period runs, and it ends when the count resets. Work time must be at least 3 minutes, so the Break warning always fits; Break length must be at least 1 minute. The web UI shows each warning as a web toast (the web UI toaster, not the TUI Toast) in every open browser, using the existing toaster. The snapshot carries the predicted start time and recomputes it on each active minute, because an idle minute moves the start. The toast updates its text in place when the time moves, and a moved time is not a new warning. The texts are "Break in 2 minutes" with "Your Break starts at 14:55 and lasts 5 minutes.", and "Bedtime starts at 22:00 (in 10 minutes)" with "Wrap up what you are doing." A dismissed toast stays dismissed in that browser until a new warning appears (a warning that ended and came back after a reset). No Banner, Chat or sound goes with a warning.

**Unlock.** The server checks the sentence with the regexp `^\s*i['’]m\s+.+?\s+and\s+i['’]m\s+addicted\s+to\s+ai\s*$` (case-insensitive). A match ends the current Rest period, sets "quiet until" to its original end, restarts the work count, and writes an unlock record tagged Break or Bedtime. An unlocked Bedtime stays off until its end time, even if the range is edited. A wrong sentence is a refusal result with a stable reason, as ADR 0028 says.

**Records.** An append-only store in the state dir holds one record per Break start, one per dropped Break (which cancels that start) and one per unlock, each with a time and the kind. Bedtime starts are not recorded. There is no retention setting: a write prunes records older than 90 days, the longest Stats range, so the store stays bounded as ADR 0041's amendment requires.

**Settings changes.** Settings apply without a restart, on a settings page save and on `gx server reload-config`. A change that ends a condition ends the Rest period at once: the master toggle off, or the Break or Bedtime toggle off for its own kind, or a Bedtime range that no longer holds the current time. Turning Bedtime on inside the range starts it at once. Turning the master toggle off, or only Break off, also clears the work count, so turning Break back on starts a fresh count.

**Config keys and settings page.** The implementer picks the key names. The registry gets a new "Mental health" category and section with three groups: General (master toggle), Bedtime (toggle, Start, End, Message) and Break (toggle, Work time, Break length, Message). Follow the steps in `.ai/index.md` "Adding a config key": registry entries, regenerate the schemas, run the settings guard. Start and End are text in 24-hour `HH:MM` form with a per-key check. The settings write path refuses a save that leaves Start equal to End, through the existing before-write hook, and config load reports equal times as it reports other bad values, naming the keys. The page gets no new control type. All keys are global only, off by default, with defaults 55 and 5 minutes, Start 22:00 and End 07:00, and the Bedtime message above.

**Routes and verbs.** Three routes, each with a CLI verb under a new `gx rest` family (ADR 0028, and the parity table and the web allowlist test from `.ai/index.md` "Adding a server route"). The implementer picks the exact names.

- Read the Rest status as JSON.
- Record one active minute (the browser heartbeat; the verb lets scripts and the verify ticket drive a Break without a browser).
- Unlock with the sentence (the verb takes the sentence as an argument, so it takes the same effort).

Each write answers with the new status. The CLI prints it as a one-line summary for a person (for example "break until 14:05") and as the full status with `--json`. A write the server declines is a refusal result: an error line for a person, a refusal result with `--json`. The routes are web-writable.

**Snapshot and events.** The snapshot gets one Rest block: whether counting is on, the current Rest period (kind, end time, message, whether to show the Break rule note values: Work time and Break length, and the Bedtime range), the warning, and the server's current time, so the countdown does not depend on the browser clock. A change event makes clients refetch, as for AFK. The browser also hides the Rest screen on its own when its countdown reaches zero.

**Notification gate.** One predicate, "quiet", is true while the feature is on and either a Rest period runs or "quiet until" is in the future. It is applied once at each outbound seam, at server construction:

- Banner: a decorator around the OS notifier the server is given. This covers session banners, config reload banners and health banners.
- Sound: a decorator around the sound player.
- Chat: the chat sink checks it when an event enters and when the queue flushes, so events queued before the Rest period are dropped too, and dropped events never reach the notification gate's budget or counters.

A banner or sound trigger that fires during a Rest period is dropped, not rescheduled. gx has no Push sender today (the PWA spec left Web Push out of scope). When it gets one, it must call the same predicate. This epic adds no Push code.

**Web: heartbeat.** A small hook sets a flag from passive input listeners (key, pointer, scroll). Listeners attach only while counting is on and the Rest screen is not shown. A timer runs once a minute only while the tab is visible and sends one heartbeat only when the flag is set, then clears it. The timer stops when the tab hides and restarts when it shows. There is no per-event work and no per-event network call.

**Web: Rest screen.** It mounts above the router and above every modal and sheet, and it uses `inert` on the app underneath plus a capture-phase key guard, so Tab, shortcuts (such as Cmd-K) and Escape reach nothing underneath. It always uses the dark palette (page `#0a0d11`), whatever theme the person chose. The layout is the approved mock, layout A (centred stack): the mascot scene (300 px, 260 px under the phone breakpoint), the message, a sub-message, a label and the time, a small rule note and the dim "Unlock anyway" link. Bedtime shows "Back at" and the end time in the accent blue; Break shows "Break ends in" and a large monospace `mm:ss` countdown. The rule notes read "Bedtime 22:00 – 07:00" and "Break · 5 minutes after 55 minutes of work". The link opens an input with the hint "Type exactly: I'm [anything] and I'm addicted to AI" and a button, "Unlock until the end of this Break" or "Unlock until 07:00", disabled until the same regexp matches. The mascot images are `gx-rest-bedtime.png` and `gx-rest-break.png` (640 px square, near-black navy background), shown with a CSS radial mask so the edges fade into the page, not a pixel match.

**Messages.** A custom message (Bedtime or Break) is split at its first sentence end (`.`, `!` or `?` followed by a space): the first sentence is the message line and the rest is the sub-message. A one-sentence message has no sub-message. The Bedtime `{end}` is the End field. Built-in Break messages are pairs of a message and a sub-message, with `{work}` replaced by the Work time in minutes. The built-in list is:

1. "{work} minutes in a row? The agents will survive without you." / "Stand up, drink some water, look at something far away."
2. "Your chair called. It wants a day off from you." / "Walk to another room and come back slower."
3. "Even the agents get to idle sometimes." / "You can too. Stretch your back and your wrists."
4. "{work} minutes of focus. The mascot is proud and slightly worried." / "Look out of a window for a while."
5. "Nothing here is on fire. Probably." / "The Queue is working. Go get a glass of water."
6. "Your eyes have been staring at a screen for {work} minutes." / "Look at something 6 metres away for 20 seconds."
7. "The code will still be here after a short walk." / "Roll your shoulders and unclench your jaw."
8. "Breaks are not a bug. They are a feature." / "Close your eyes and take five slow breaths."
9. "Hydrate or diedrate." / "Fill a glass and drink it before you come back."
10. "You have been very productive. Do not ruin it by burning out." / "Stand up and stretch for a minute."
11. "The agents ask you to please stop watching them." / "They work better when nobody stares."
12. "Time for a tiny holiday." / "Go outside if you can, or just breathe by the window."

**Stats.** The stats payload gets a "Mental health" section: for each day of the range, the number of Break starts (minus dropped ones), the unlocks of Break and the unlocks of Bedtime. It reads the records store, as ADR 0041's amendment allows for account-wide sections with their own store. It ignores the project filter, like Web performance, and the card's small text says so, as the Web performance card's does. It follows the page's 7/30/90-day range and local days. The web page adds a section with a side-by-side chart card, following the Follow-ups card, placed after Web performance. The wire types are generated like the others.

**Fixtures.** A new fixture scenario `rest` with shapes seeds config and the persisted Rest state (written at start, with times relative to the start). The shapes are listed under Verification.

## Testing Decisions

A good test checks what a person or script sees: the snapshot, the route and CLI result, the notifications sent, the text on the screen. It does not check engine internals or timer calls.

- **Seam 1, server tests with a fake clock (the highest seam).** Drive the routes and the `gx rest` verbs through the server harness with an injected clock and a tick hook. Cover: active minutes across two clients counting once per minute; work count and idle reset; Break start, end, message choice (random and custom); the warnings; Bedtime ranges (crossing midnight, a range edited, enabling inside it); the overlap rule, including the cancelling record; unlock (regexp cases, wrong text refused, quiet-until, work restart); master toggle off ending a Rest period; persistence across a restart; the stats section; equal times refused by the settings write and by config load. Notification gate: the recording `Banners` notifier, a fake sound player and a fake chat transport show nothing sent during a Rest period or after an unlock, and nothing sent late when it ends. Prior art: `server/queue/web_afk_test.go`, `server/sessions/banners/sessions_afk_test.go`, `server/server_notify_test.go`, `server/stats` tests, `server/settingsapi/write_api_test.go`.
- **Seam 2, CLI tests.** The `gx rest` verbs with and without `--json`, and the server-not-running refusal. The route parity test and `TestWeb_EverythingElseIs404` pass. Prior art: `cmd/limits/afk_test.go`.
- **Seam 3, vitest.** The heartbeat hook with fake timers (flag only, one send a minute, nothing while hidden, nothing while the Rest screen shows, nothing while counting is off); the Rest screen render for both kinds with the message split, the countdown from server time, the regexp gate on the button, and the key guard; the toast text and the one-time dismissal; the Stats section; the settings tree entry. Prior art: `afk.test.ts`, `NodeMissingToast`, `StatsPage.test.tsx`, `SettingsPage.test.tsx`.
- **Seam 4, Playwright, two flows.** Only what needs a server round trip and real layout: two pages showing and leaving the Rest screen together, and the layout at 900 px and 375 px. The flows are the new flow R1 (Break) and R2 (Bedtime), numbered from the next free numbers, in a new `rest.e2e.ts`. They extend no existing flow because no existing flow starts the `rest` scenario. `maxFlows` goes up by 2 in the ticket that adds them, and the ticket title says why (server round trip and real layout; Go and vitest tests cover each rule alone). Everything else is covered by seams 1 to 3.

## Verification

Run against a temporary gx server after the epic is built. Scenario `rest` has these shapes, each its own server start (new, listed below): `bedtime` (Bedtime on, range from one hour ago to one hour ahead, master on), `break` (Break on, a Break running that ends in 4 minutes 37 seconds, with the message from the mock, and a Break start record for that Break), `break-warning` (Break on, work count two minutes below Work time, last active minute now) and `bedtime-warning` (Bedtime on, start 10 minutes ahead). Verb checks use `gx rest` verbs with the fixture's `env`.

1. Go test (seam 1): a fresh server has no Rest period and an empty Rest block however long the clock runs. CLI check: `gx rest --json` on a fresh fixture shows the feature off.
2. CLI check (`break` start): set the master toggle off in the config file and run `gx server reload-config`; `gx rest --json` shows no Rest period. Vitest (seam 3): the Rest screen leaves when the snapshot shows no Rest period, including after a reload. Go test (seam 1): no active minute counts while the master toggle is off, and turning only Break off clears the work count.
3. Go test (seam 1): with Bedtime on and Break off, active minutes do not start a Break; with Break on and Bedtime off, the range does not start Bedtime.
4. Settings page in the `rest` scenario: the "Mental health" section has the General, Bedtime and Break groups. `go test ./config -run TestSettingsGuard` passes.
5. Go test (seam 1): a range 22:00 to 07:00 is a Rest period at 23:30 and at 06:00, and not at 12:00.
6. Go test (seam 1): a range 08:00 to 17:00 (no midnight crossing) is a Rest period at 12:00 and not at 20:00. Flow R2 (`bedtime`): the range is in force.
7. Go test (seam 1): the settings write refuses Start equal to End with a "Not saved" message, and config load names the keys. Vitest: the page shows the refusal.
8. Go test (seam 1): the Rest block's time zone is the server's local zone (a fake clock and a fixed zone).
9. Flow R2 (`bedtime`) and Mock states `bedtime-desktop`: the default message appears with the End time. Go test: a custom message with `{end}` is rendered.
10. Go test (seam 1) and Mock states `bedtime-warning`: the warning exists from 10 minutes before the start, with the exact start time, and no Banner, Chat or sound goes with it.
11. Go test (seam 1) and CLI check (own start with Bedtime off and the clock inside the range): turn Bedtime on in the config file and reload; `gx rest --json` shows a Bedtime period with no grace period.
12. Vitest (seam 3): the router and its inputs stay mounted under the Rest screen, and a typed draft is still there after it leaves.
13. Vitest (seam 3): with the Rest screen up, Tab and Cmd-K reach nothing underneath (`inert`, key guard).
14. Vitest (seam 3): the heartbeat hook counts typing, clicks and scrolling and nothing else. Go test: a heartbeat in a minute makes that minute active.
15. Go test (seam 1): two clients active in the same minute count once, in different minutes count twice. CLI check: `gx rest active` twice in one fake minute raises the work count by one.
16. Go test (seam 1): TUI activity (focus reports, session input) does not change the work count.
17. Vitest (seam 3): listeners only set a flag; a fake timer shows at most one request a minute, none while hidden, none while the Rest screen shows, none while counting is off. Manual in the verify ticket: Chrome's performance panel shows no per-event cost.
18. Go test (seam 1): Work time and Break length changes apply at once; defaults are 55 and 5; Work time below 3 is refused.
19. Go test (seam 1): a gap equal to the Break length resets the count, a shorter gap does not.
20. Go test (seam 1) and Mock states `break-warning`: the warning appears at Work time minus 2, with the exact start time and the Break length, and disappears when the count resets.
21. Go test (seam 1): a Break due at Bedtime start is dropped, a running Break is dropped with its cancelling record, the work count is zero after Bedtime, and no Break warning is raised for a start after Bedtime starts.
22. Go test (seam 1): the built-in message is chosen at the Break start from the built-in list (seeded random source), stays the same for all clients, and survives a restart. Mock states `break-desktop`.
23. Go test (seam 1): a custom Break message is used when set and is split at the first sentence end.
24. Flow R1 (`break`): the countdown shows `mm:ss`. Vitest (fake timers): the countdown falls without a reload and uses the server time from the snapshot. Mock states `break-desktop`.
25. Flow R2 (`bedtime`) and Mock states `bedtime-desktop`.
26. Mock states `bedtime-desktop`, `break-desktop`. Vitest (seam 3): each kind shows its own image, and the radial mask style is set.
27. Flow R1 at 375 px: Mock states `break-phone`; flow R2 at 375 px: Mock states `bedtime-phone`.
28. Flow R1 and R2: a second page of the same fixture shows the same Rest screen, and both leave it together after an unlock on the first.
29. Mock states `break-unlock-open`. Flow R1: the link opens the input with its hint.
30. Vitest (seam 3): the button is disabled for non-matching text and enabled for "i'm tired and i'm addicted to ai", "  I’m  very tired  and I'm addicted to AI ", and similar. Flow R1: type a matching sentence and the button enables.
31. CLI check: `gx rest unlock "hello"` exits non-zero with the refusal reason; `gx rest unlock "I'm tired and I'm addicted to AI"` succeeds. Go test (seam 1): the route refuses a wrong sentence.
32. Flow R1: after the unlock the Rest screen leaves on both pages; `gx rest --json` shows no Rest period. Same for `bedtime` (flow R2).
33. Go test (seam 1): after an unlock, the Banner, Chat and sound recorders stay empty until the original end, and nothing is sent when it passes.
34. Go test (seam 1): after a Break unlock the work count is zero and counting resumes with the next active minute.
35. Go test (seam 1): during a Rest period an agent event (session banner, ticket pause for Chat, health banner, config reload banner, a sound) sends nothing, events raised just before the Rest period but still queued for Chat are dropped, and nothing arrives after the Rest period ends.
36. Fixture `break` and `bedtime`: `gx server queue` and a running fake agent keep working during the Rest period; a session that needs the person stays waiting and shows in the snapshot as before.
37. Go test (seam 1): turning the master, Break or Bedtime toggle off ends a matching Rest period at once.
38. Go test (seam 1): the stats section counts Break starts less dropped ones, Break unlocks and Bedtime unlocks per local day. Flow R1 after an unlock: the Stats page shows a "Mental health" section whose day shows one Break and one Break unlock.
39. Vitest (seam 3): the settings page has no counter for unlocks or Breaks.
40. Go test (seam 1): restart the server mid-Break and mid-unlock; the Rest period, its message and the quiet-until time are the same.
41. Go test (seam 1): the Banner, sound and Chat paths all read the one predicate (a test that flips it shows all three go quiet). Go guard test: it fails when `osnotify.NewSystemNotifier`, `sound.NewSystemPlayer` or a chat transport is built outside the server's construction seam.
42. Shell check: `webui/public/gx-rest-bedtime.png` and `gx-rest-break.png` exist and are 640 by 640 PNG files; `docs/mascot/rest-image-prompts.md` exists.

**Mock states**

- `$(gx tickets root)/mental-health/mocks/05-rest-screen-mock.html` · bedtime-desktop · 900 px · `rest` shape `bedtime` · mascot-image, message, submessage, time-label, time, rule-note, unlock, unlock-link
- `$(gx tickets root)/mental-health/mocks/05-rest-screen-mock.html` · bedtime-phone · 375 px · `rest` shape `bedtime` · mascot-image, message, submessage, time-label, time, rule-note, unlock, unlock-link
- `$(gx tickets root)/mental-health/mocks/05-rest-screen-mock.html` · break-desktop · 900 px · `rest` shape `break` · mascot-image, message, submessage, time-label, time, rule-note, unlock, unlock-link
- `$(gx tickets root)/mental-health/mocks/05-rest-screen-mock.html` · break-phone · 375 px · `rest` shape `break` · mascot-image, message, submessage, time-label, time, rule-note, unlock, unlock-link
- `$(gx tickets root)/mental-health/mocks/05-rest-screen-mock.html` · break-unlock-open · 900 px · `rest` shape `break`, after clicking "Unlock anyway" and typing the sentence · mascot-image, message, submessage, time-label, time, rule-note, unlock, unlock-link, unlock-box
- `$(gx tickets root)/mental-health/mocks/05-rest-screen-mock.html` · break-warning · 900 px · `rest` shape `break-warning` · warning-toast
- `$(gx tickets root)/mental-health/mocks/05-rest-screen-mock.html` · bedtime-warning · 900 px · `rest` shape `bedtime-warning` · warning-toast

The mock shows one example Break message and the fixed times 04:37, 07:00, 22:00 and 14:55. The built page shows the fixture's own times and message, so check layout, sizes, colours and part order, not the text.

New flows and scenarios to write (these become implementation tickets):

- The fixture scenario `rest` with shapes `bedtime`, `break`, `break-warning` and `bedtime-warning`, seeded config and persisted Rest state, with times relative to the server start, and its catalog entry in `webui/e2e/CATALOG.md`.
- Flows R1 (Break: show on two pages, 900 px and 375 px, unlock, Stats) and R2 (Bedtime: show, phone layout, unlock) in a new `rest.e2e.ts`, with catalog entries, and `maxFlows` raised by 2.
- The fixture's and servertest's rest-clock seam (an injected clock and a tick hook).

## Out of Scope

- A Rest screen in the TUI.
- Pausing agents, ralph-loop or the Queue during a Rest period.
- A hard lock with no way out. The web API is not blocked during a Rest period, and the CLI or the config file can still change settings.
- A Push sender (the PWA spec left Web Push out). The predicate is ready for it.
- Per-weekday Bedtime times, per-project settings, and a light theme for the Rest screen.
- A retention setting or an unlock counter on the settings page.
- A custom time-picker control on the settings page.

## Further Notes

- The mascot images and the Codex prompts are already in the repo (`webui/public/gx-rest-bedtime.png`, `webui/public/gx-rest-break.png`, `docs/mascot/rest-image-prompts.md`). The glossary already has the Rest period terms.
- The approved mock is stored at `$(gx tickets root)/mental-health/mocks/05-rest-screen-mock.html`. It embeds the images as base64 and does not show the settings layout.
- Because the Rest screen covers the settings page, the person turns the master toggle off from the CLI or the config file during a Rest period, or ends it with Unlock anyway.
- Terms used: Rest period, Bedtime, Break, Rest screen, Banner, Chat, Push, AFK mode, snapshot.
