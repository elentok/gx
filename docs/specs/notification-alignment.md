## Problem Statement

gx has five places that can tell a person something: the Desktop banner and sound, the web app's toaster, Telegram, Slack and Push. They do not behave alike.

- Push sends 5 event types. Telegram and Slack send many more. Desktop sends session banners and a few system banners. The web toaster shows only system warnings.
- Only Push has an on/off switch. Telegram and Slack can only be turned off by removing their settings, or muted by hand with `gx notify --disable`.
- The person cannot choose which notifications each place sends.
- Long-running-ticket notifications (a ticket past its turn limit) always go out.
- Only Telegram and Slack have a send-rate cap and a global breaker. A flood on Desktop or Push is not limited.

Slack is the work channel. The person may not use Push or Telegram at work, so Slack must keep working on its own.

## Solution

One shared catalog of **notification kinds** and five **channels** (Desktop, Web toast, Telegram, Slack, Push). The terms are in `GLOSSARY.md`.

- The catalog has 16 rows. The person checks any mix of channels for each row, in Settings, Notifications.
- Each channel has its own global switch. Turning a channel off keeps all its other settings, and the Settings page shows them dimmed but editable.
- Only the Long ticket row is unchecked on a new install. An upgrade writes each channel's list from what that channel sends today, with Long ticket off everywhere. Nobody loses a notification they get now, except the long-running one.
- Per channel, a notification passes the channel switch, then the kind list, then that channel's own gates. Each channel has its own send-rate cap and global breaker.
- The Settings page shows a strip of five channel switches, one row per kind with five chips, then a settings section per channel. Push gets a server-wide switch beside its contact email.
- `gx notify --status` lists the five channels with switch and checked-kind count. The TUI config editor shows each switch as a boolean.

### The catalog (16 rows)

1. Session alert (Actionable, Blocked, Idle, failed sessions)
2. Design question
3. Design ready
4. Chat question
5. Paused ticket (a ticket that needs a human, including its escalations)
6. One-off result (the `## Result` of a succeeded one-off ticket)
7. Conflict resolution started
8. Epic complete (including epic merged)
9. Epic paused (merge-blocked epic)
10. Progress update
11. Long ticket
12. Budget and usage (soft and hard limit, extra usage, daily summary)
13. Disk (warning, resume, clean)
14. Project and trunk problems (trunk missing and found, project unavailable)
15. Server started
16. CLI message (`gx notify <message>`)

Every channel can check every row. Only Long ticket is unchecked by default.

## User Stories

1. As a person, I want one list of notification kinds shared by all channels, so that Telegram, Slack, Push, Desktop and Web toast mean the same thing by "a notification".
2. As a person, I want to check any kind for any mix of channels, so that I choose where each notification goes.
3. As a person, I want Push to be able to send every kind, including epic complete, one-off results and merge-blocked epics, so that Push is as capable as Telegram.
4. As a person, I want Telegram and Slack to be able to send every kind, so that they stay aligned with Push.
5. As a person, I want Desktop to be able to send every kind as a banner, so that I can be told at the machine about more than sessions.
6. As a person with the web app open, I want Web toast to show the kinds I checked for it as toasts, so that I see them without Telegram or Push.
7. As a person, I want Web toast to show only while the web app is open and never replay missed ones, so that it behaves like a toaster.
8. As a person, I want `gx notify <message>` to be a kind I can check per channel, and to reach every checked channel through the running server, so that CLI messages go to the channels I pick. With no server running, I want it to still send to Telegram and Slack, and to be told that Desktop, Web toast and Push were skipped.
9. As a person, I want each channel to have its own on/off switch, so that I can silence one channel without touching the others.
10. As a person, I want turning a channel off to keep its bot token, chat id, Slack webhook, contact email and push subscriptions, so that turning it back on needs no setup.
11. As a person, I want every switch on after an upgrade, so that nothing changes by itself.
12. As a person, I want a channel that is off to send nothing, including no sound for Desktop, so that off means off.
13. As a person, I want the `gx notify --enable/--disable` manual mute to stay a separate runtime gate for Telegram and Slack, so that a quick mute from the shell still works.
14. As a person, I want Push to reach a device only when the server switch is on, the contact email is set and the device switch is on, so that each layer keeps its meaning.
15. As a person, I want each device to keep its own "Notifications on this device" switch, so that I choose which devices buzz.
16. As a person, I want system messages (mute notices, test messages, and the config-reload, process-storm and hard-limit grace banners) to ignore the kind list and obey only the channel switch, so that gx can always tell me about the channel itself and about work it is stopping.
17. As a person, I want a notification to be checked in this order: channel switch, kind list, then the channel's own gates, so that the rules are easy to predict.
18. As a person, I want a kind I did not check to cost no send budget, trip no mute and join no batch, so that unwanted kinds never silence wanted ones.
19. As a person, I want Rest period, open-on-screen, AFK mode, the recovery hold and the Desktop focus rule to keep their meaning, so that this change adds no surprises.
20. As a person, I want AFK mode to still gate Chat's session kinds, so that Telegram and Slack get Session alert only while I am away.
21. As a person, I want Push to keep skipping a project's Chat override gates, so that a project override picks only a Telegram or Slack target.
22. As a person, I want a project's `notifications` block to keep setting only its Telegram or Slack target, so that kind lists and switches stay global.
23. As a person, I want the app icon badge to ignore the kind list, so that the number still counts Actionable sessions plus Paused tickets.
24. As a person, I want the badge cleared and not set while the Push server switch is off, so that the icon does not claim pushes that cannot arrive.
25. As a person, I want every channel to have its own send-rate cap and global breaker with today's thresholds, so that a flood on one channel never silences another.
26. As a person, I want the per-source mute and the `needs-repair` parking to stay with Telegram and Slack only, so that they keep today's meaning.
27. As a person, I want a tripped cap or breaker on Desktop, Web toast or Push to send one system notice on that same channel and then stay quiet, so that I know why it went silent.
28. As a person, I want a tripped Desktop, Web toast or Push breaker to clear itself after 5 minutes with no send attempt on that channel, so that I never have to clear it by hand.
29. As a person, I want Chat's global mute to keep needing `gx notify --enable`, so that today's Telegram and Slack behaviour does not change.
30. As a person, I want one Push event, after the one-second merge, to count as one send however many devices it reaches, so that owning several devices does not trip the breaker sooner.
31. As a new user, I want one default list for all five channels with every kind checked except Long ticket, so that a fresh install is aligned.
32. As an upgrading user, I want a one-time config migration to write each channel's list as explicit values from what that channel sends today, so that I lose nothing.
33. As an upgrading user, I want Push to keep Session alert, Design question and Paused ticket (with its escalations), so that Push does not gain or lose events by itself.
34. As an upgrading user, I want Desktop to keep Session alert and Design question, so that my banners and sounds stay as they are. Only those two kinds play sounds, as today.
35. As an upgrading user, I want Web toast to start with no kinds checked, so that its toasts stay system warnings only.
36. As an upgrading user, I want Telegram and Slack to keep every kind they send today, so that Slack keeps working as my work channel.
37. As an upgrading user, I want Long ticket off on every channel after the upgrade, including Telegram and Slack, so that the long-running notice is off by default.
38. As an upgrading user, I want a channel with no settings to still get its list written, inert until I set the channel up, so that setup later needs no migration.
39. As an upgrading user, I want a later change of the default list never to alter my install, so that my lists stay mine.
40. As a person, I want a strip of five channel switches at the top of Settings, Notifications, so that I see every channel's state at once.
41. As a person, I want one row per kind with five chips, on or off, so that I pick channels per kind in one place.
42. As a person, I want a settings section per channel (Telegram, Slack, Push, Desktop) below the rows, so that credentials and options stay next to the channel.
43. As a person, I want Web toast to have only its switch and no section, so that the page shows nothing empty.
44. As a person, I want a channel that is off to show a grey switch and dimmed chips and settings that I can still edit, so that I can set it up before turning it on.
45. As a person, I want the Push server-wide switch beside the contact email, so that the server's Push controls are in one place.
46. As a person, I want the device switch on the Account Notifications page disabled with "Push is off for the whole server. Turn it on in Settings › Notifications › Push." while the server switch is off, so that I know why it does nothing.
47. As a person, I want the device to keep its own switch state while the server switch is off, so that turning the server back on restores my devices.
48. As a person on a phone, I want each kind row to stack with its name above a full-width line of chips and the switch strip in two columns, so that the page fits.
49. As a person, I want the new switches to look like the existing Settings boolean switch, so that the page feels like the rest of Settings.
50. As a person, I want the "Send test push" button disabled while the server switch is off, so that a test never fails for a known reason.
51. As a person, I want `gx notify --status` to list the five channels, each with its switch and number of checked kinds, so that I can check from the shell.
52. As a person, I want `gx notify --status` to stay read-only with no new flags, so that the CLI stays small. The new `--json` flag belongs to the send and test verbs only.
53. As a person, I want the TUI config editor to show each channel switch as a plain boolean, so that I can flip it from the terminal.
54. As a person, I want `gx config test-notifications` to test all five channels through the running server and to skip a channel that is off and say so, so that the test tells the truth. With no server running, I want it to test Telegram and Slack as today and say that the other three were skipped.
55. As a person, I want changes to a switch or a list to apply on the next config reload without a restart, so that Settings feels live.
56. As a person, I want messages already queued for a channel I just turned off to be dropped, so that off takes effect at once.
57. As a person, I want a notification about a ticket, epic, Design or session to open that page when I tap or click it, and server notices and CLI messages to open the app home, so that a tap lands where the news is.
58. As a gx maintainer, I want one notification router that applies switch, kind list and gates for every channel, so that the rules live in one place and no call site decides them. The CLI reaches it through the server, and its in-process fallback applies the switch, the kind list, then today's gate (the manual mute and the cap, `gateCLISend`).
59. As a gx maintainer, I want the catalog kept in one list that the router, the config registry, the Settings page and the migration all read, so that adding a kind touches one place.
60. As a gx maintainer, I want no new pause kind, so that ADR 0042 (every pause is recoverable from the web) stays true.
61. As a gx maintainer, I want fast automated tests for the router, the migration and the Settings page, so that a change cannot break delivery.

## Implementation Decisions

### Catalog and ids

- The 16 rows above are one catalog list in Go. The router, the config registry, the config parser, the migration, `gx notify --status` and the web page read it. The web page gets it from the settings snapshot, not from a copy in TypeScript.
- The implementer picks the stored id of each kind and the display text. Ids are config values that the migration writes, so they are stable and unique once chosen.
- Row membership (which code kinds belong to which row):
  - Session alert: the banner, sound, Push and AFK Chat alert of Actionable, Blocked, Idle (Done), Rate-limited, Exited and failed sessions, except a Design waiting on a round.
  - Design question: a Design waiting on a round on every channel (Banner and sound, Push, Chat's Question message, toast).
  - Design ready, Chat question, Conflict resolution started, Progress update: as the Chat kinds named by those rows.
  - Paused ticket: a ticket pause, a conflict-resolution child's pause and a ticket or epic escalation.
  - One-off result: the Result message of a succeeded one-off ticket. A failed ticket notifies through Paused ticket.
  - Epic complete: epic merged and epic complete.
  - Epic paused: a merge-blocked epic root pause (`merge-error`, `dirty-trunk`, `trunk-missing`, `needs-rebase`). This adds a notification only. It adds no pause kind and no new way into one, so ADR 0042 is untouched.
  - Long ticket: the long-ticket notice, its Desktop banner and Chat notice.
  - Budget and usage: soft and hard limit, extra usage and the daily summary notices.
  - Disk: disk warn, resume and clean.
  - Project and trunk problems: trunk missing, trunk found and project unavailable.
  - Server started and CLI message: one each.
- Config-reload banners (non-web trigger), process-storm banners and the hard-limit grace banners (countdown, agents stopped, stop cancelled) are system messages. They are not kinds. They obey only the Desktop switch. Their TUI toast is unchanged.
- System messages (mute notices, "globally muted", test messages, and the banners above) skip the kind list on every channel and obey the channel switch only.

### The notification router

- A new module is the one place that decides, per channel, whether and how a notification goes out. It takes a kind, text (title and body), a target (a ticket, epic, Design, session or none), a source for the gate, and flags (system message, session-scoped). It fans out to the five channel adapters. The implementer picks its name and interface shape.
- Order per channel: channel switch, kind list, then the channel's own gates. The first no ends it. System messages skip the kind list.
- The existing call sites stop deciding for themselves. These hand the router a notification instead of calling a transport or the OS notifier: the Chat event sink and server notices, the session banner firing path, the Push hooks beside the three pause call sites and the Design-wait path, the config-reload, budget grace and process-storm banners, and `gx notify`. This replaces the separate Push wiring that bypasses Chat's gates. Push's behaviour at those points is kept (after the recovery hold, no project-override gates).
- The Telegram and Slack adapters keep destinations, the project override targets, the 6-second batch queue, the dedup and the manual mute exactly as today.
- The Desktop adapter wraps the shared OS notifier and the sound player. The Rest period quiet wrapper stays under it. Only Session alert and Design question play sounds, as today. Every other kind on Desktop is a banner only.
- The Web toast adapter publishes one new event on the server's event stream that carries a title, body and path. Every open page shows a toast and nothing is stored or replayed. The implementer picks the event name and the toast's look, using the web app's existing toaster. Existing toasts (Rest warning, node missing, dev toasts, "Force refreshed") are not kinds and stay as they are.
- Text for the new channel and kind pairs: Session alert and Design question keep the Banner's text. Every other kind uses Chat's headline as the title and Chat's plain sentence as the body, as Push does today for tickets. The body is cut as Push cuts it today. Telegram and Slack text is unchanged.
- Tap or click target: a ticket kind opens the ticket page, an epic kind the epic page, Design kinds the Design page, session kinds the session page, and server notices, Budget, Disk, Project and trunk, Server started and CLI message the app home. A Push payload keeps a route path and never a URL or token. Desktop uses the web link where it has one.
- Push urgency is high for the same events as today. Every kind new to Push is normal urgency.
- Batching, dedup, Push's one-second merge and tag formats stay. The implementer picks tags and paths for the new kinds.

### Gates after the kind list

- Rest period keeps Desktop (banner and sound), Push and Chat quiet, as today. Web toast is not covered, as today.
- Open-on-screen skips session kinds on Desktop and Push, as today. It never skips a ticket or epic kind.
- AFK mode gates Chat's Session alert only. Design question and Chat question are not AFK gated, as today.
- The recovery hold applies to Paused ticket on every channel it reaches.
- The Desktop focus rule (a focused TUI while AFK mode is off) keeps applying to session kinds on Desktop.
- The manual mute (`gx notify --enable/--disable`, `--project`) blocks Telegram and Slack after the switch, as today. It never touches Desktop, Web toast or Push.
- The per-source storm mute and `needs-repair` parking stay Telegram and Slack only.
- The per-channel send-rate cap and breaker (next section) are the last gate. Only a notification that passed every other gate counts as an attempt.
- The app icon badge ignores the kind list. It counts Actionable sessions plus Paused tickets. While the Push server switch is off the snapshot carries no badge, open apps clear theirs and do not set it, and it returns when the switch is turned on. The implementer picks how "no badge" is carried.

### Send-rate cap and breaker per channel

- Desktop, Web toast and Push each get the cap and global breaker Telegram and Slack have: about 20 sends per trailing minute, counted per channel. The thresholds are today's. A flood on one channel never affects another.
- The counters live in the notification state file beside Chat's, under a new entry per channel. The implementer picks the entry names.
- A send is one delivery to that channel, counted at dispatch. Push and Desktop count after the one-second merge: one send per merged push or banner, and one per event however many devices it reaches. A Desktop banner and its sound are one send. Telegram and Slack count as today.
- The cap is the last gate. A notification dropped by the switch, the kind list, Rest period, open-on-screen, AFK, the recovery hold, the focus rule or the manual mute is not an attempt and cannot trip or hold a breaker.
- On a trip the channel sends one system notice on that same channel, then drops further sends on it. The implementer picks the wording. The notice skips the kind list and is the last send allowed.
- The breaker clears by itself once the channel has had no send attempt for 5 minutes, the notification state file's existing series retention. Notifications that pass every other gate and are dropped by the tripped breaker count as attempts, so a flood keeps the breaker tripped until it stops. No manual clear and no new flag.
- Chat is unchanged: its global mute stays until `gx notify --enable`. Ticket 06 said Chat clears by itself after a quiet window. The code and `docs/specs/notification-throttle.md` show it does not, so this spec states the rule per channel.

### Channel switches (config)

- Five new boolean config keys, one per channel, all default on. The implementer picks the key names and puts them in the Notifications category of the registry.
- The Desktop switch sits above `agents.banners.enabled` and `agents.sounds.enabled`. Those two keep their meaning. Desktop off sends no banner and no sound.
- Telegram and Slack: the new switch is separate from the manual mute. `gx notify --status` shows both.
- Push: a server-wide switch beside `server.web.push-contact-email`. Push reaches a device only when the server switch is on, the email is set and the device switch is on. The push-notifications spec's "no server-wide push config key" is superseded by this epic. Its other out-of-scope lines (no buttons on notifications, no Chrome or Android promise) stand. Its line that leaves merge-blocked epics out is superseded by the Epic paused row.
- Web toast: a global switch with no other setting.
- A switch change applies on the next config reload with no restart. A message queued for a channel that is now off is dropped at flush.

### Kind lists (config)

- One config key per channel holds that channel's checked kind ids, as a list of strings. The implementer picks the key names and how the registry describes a list whose allowed values are the catalog ids.
- An unknown id in a hand-edited list is handled like other invalid config values: the implementer matches the existing parse behaviour and the Settings page shows the same "Not saved:" refusal on a web save.
- The list is global. A project may not override it.
- New install default: one shared list per channel with every kind except Long ticket. The `settings` scenario shows these defaults: every channel has 15 kinds checked and Long ticket unchecked.
- A later release that adds a kind brings its own migration for existing installs. The registry default never alters an upgraded install.

### Config migration

- A one-time config migration step writes each channel's kind list as explicit values from what that channel sends today. It runs in whichever loader (the CLI, the TUI or the server) loads the config first. It is idempotent and never overwrites a list key that is already present.
- It records that it ran with a marker key in `config.json`, which the implementer picks and adds to the registry or its exclusions as the settings guard needs. Deleting a list key later does not run it again.
- Whenever gx creates a new `config.json`, it stamps the marker into it, so a new install never migrates. A `config.json` without the marker is an upgrade. A start with no `config.json` is a new install: the migration writes nothing and the registry default applies.
- The step runs wherever any process loads the config (the CLI, the TUI or the server), the first one to load it. It then writes the lists and the marker in one atomic write.
- Test fixtures and rigs preset the marker in the config they write, so they count as new installs. One rig scenario writes an unmarked config to exercise the upgrade lists.
- Lists written by an upgrade:
  - Push: Session alert, Design question, Paused ticket.
  - Desktop: Session alert, Design question.
  - Web toast: none.
  - Telegram and Slack: every kind they send today. That is Session alert, Design question, Design ready, Chat question, Paused ticket, One-off result, Conflict resolution started, Epic complete, Epic paused, Progress update, Budget and usage, Disk, Project and trunk problems, Server started and CLI message. Session alert is included because Chat sends it in AFK mode, and the AFK gate still decides.
  - Long ticket is off on every channel.
- A channel with no settings still gets its list. It stays inert until the channel is set up.
- No switch is written. The registry default (on) applies.

### Settings page (web)

- Settings, Notifications opens with `channel-switches`: five labelled switches (Desktop, Web toast, Telegram, Slack, Push). Below, `kind-rows`: 16 rows, each with the kind name, a short hint and five chips in the order Desktop, Web toast, Telegram, Slack, Push. A chip toggles one cell. Then `channel-settings`: a section each for Telegram, Slack, Push and Desktop. Web toast has none.
- A channel that is off shows a grey switch and dimmed chips and settings. They stay editable and the checked kinds are kept.
- The Push section holds the server-wide switch beside the contact email. The helper line reads "Off here stops Push for every device. Device switches keep their state."
- The Account Notifications page's device switch is disabled while the server switch is off and shows the line "Push is off for the whole server. Turn it on in Settings › Notifications › Push." The device's state is kept. Its "Send test push" is disabled too.
- On a phone each kind row stacks: the name on top, the chips on a full-width line below. The switch strip is two columns.
- The new switches use the existing Settings boolean switch component (`BoolControl`, blue `tone.running.fg`, dark knob). Chips use the existing status-tag look. The Push device switch on the Account page is not restyled.
- The mock (`$(gx tickets root)/notification-alignment/mocks/05-settings-page-look.html`) is a layout and behaviour reference only. Its CSS is hand-written. Agents use gx's real components and tokens and never restyle gx to match it.
- The page shows a save error the way other settings do ("Not saved:").

### CLI and TUI

- `gx notify <message>` goes to the running server on a new socket route, and the router sends it as a CLI message to every channel that is on and has the kind checked. `gx config test-notifications` gets its own socket route. Each verb gains a `--json` flag. This follows ADR 0028 (the server API mirrors the CLI), so each route gets its CLI verb and the parity entries the repo's parity tests require. Both routes are socket only and not on the web mux, and both are added to the 404 allowlist in `TestWeb_EverythingElseIs404`. A channel that is off or skipped is a field in the result, never an HTTP error. With no server running, or when an old server answers 404 or an unknown route, the CLI keeps today's direct Telegram and Slack send. That send applies the switch, the kind list, then today's gate (the manual mute and the cap, `gateCLISend`) in-process, and the CLI prints that Desktop, Web toast and Push were skipped.
- `gx notify --status` also prints one line per channel: its switch (on or off), its number of checked kinds, and "tripped since <time>" while that channel's breaker is tripped. It stays read-only with no new flag. The existing destination, mute and ticket lines do not change. `gx notify --enable/--disable` stays the manual mute for Telegram and Slack.
- `gx config test-notifications` sends its test messages as system messages. With the server up it goes through the same new route and tests all five channels. A channel whose switch is off is skipped and the output says it is off. With no server it tests Telegram and Slack as today and prints that the other three were skipped.
- The TUI config editor shows each switch as a plain boolean, with no dimming. Kind lists show as the generic list editor does. The web page is the rich view.

### Docs

- `GLOSSARY.md` already has Channel and Notification kind. The Push entry is updated: Push also has a server switch and a kind list, and sends all kinds. The Notification Surfaces intro and the Chat and Banner entries note that each is now a Channel with a kind list.
- `docs/specs/push-notifications.md`, `better-notifications.md` and `notification-throttle.md` are history. This spec supersedes their lines named above and leaves the rest. In the push spec, the Implementation Decisions line that says a merge-blocked epic root pause gets no push (the `Merger.pauseRoot` sentence) and its story 55 are superseded: a root pause pushes when the Push list has Epic paused. The test `TestRootPause_MergeBlockedSendsNoPush` changes to two cases: no push when the Push list lacks Epic paused, one push when it has it.

## Testing Decisions

A good test checks what a channel receives, what is stored and what the person sees. It never checks which helper decided. The Go tests use the existing server-level rigs.

- **Seam 1, the server test rig** (the one seam for behaviour): a real event goes in, and the Chat recorder (fake Slack webhook, fake Telegram Bot API), the recording OS notifier and sound player, the fake push service and the event stream are read. It covers the order switch, kind list, gates; every row's membership; the three adapters' text; the per-channel cap and breaker and the 5-minute clear; system messages; the Rest period, open-on-screen, AFK, recovery hold, focus rule and manual mute still holding; the badge. Prior art: `server_notify_test`, `configreload_chat_test`, the session banner tests, `server/webpush` sender tests and the chat recorder rigs.
- **Seam 2, migration unit test**: a config in, the written config out, for each channel, plus an unmarked `config.json` being an upgrade and no `config.json` being a new install that writes nothing, a present list not overwritten, and a second run writing nothing. Prior art: `config/migrate_test.go` and the ticket migration tests.
- **Seam 3, Vitest on the Settings page**: the strip, rows and chips, channel off, push off with the Account switch disabled, phone stacking, and the registry-driven list. Prior art: `SettingsPage.test.tsx`, `NotificationsPage.test.tsx`.
- **Seam 4, CLI**: `gx notify --status` lines and `gx config test-notifications` skipping an off channel. Prior art: the notify CLI tests.
- **Playwright**: the Settings steps join flow 200 (`settings-page.e2e.ts`, scenario `settings`). One short Account device-switch step joins flow 2 on `mixed`. Flow 2 has a tight time budget, so the step only flips the server switch and checks the device switch is disabled. The disabled line and the other Account states are checked in `NotificationsPage.test.tsx`. OS banners cannot be seen by Playwright, so Desktop is covered by Go tests. No new flow or scenario.
- Tests follow `gx-go-tests`, `gx-vitest-tests` and `gx-playwright-tests`: no sleeps, a short knob for the 5-minute clear and the 6-second tick, capped test runs.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built. Settings steps use flow 200 on scenario `settings`. The Account step uses flow 2 on `mixed`. Go and CLI checks run against the fixture's server and rigs.

Catalog and channels (stories 1 to 8)
- Flows: flow 200 (the strip and the 16 rows show). Mock states line `desktop`.
- Checks: Go tests, one per row, that the row reaches each of the five channels when checked (the recorder, the OS notifier, the fake push service and the event stream each receive it). A Web toast test that two open pages each get the toast and a page opened afterwards gets none. A test that `gx notify "hi"` with the server up reaches only the channels with CLI message checked. With no server it sends to Telegram and Slack, applies their switch and list, and prints that Desktop, Web toast and Push were skipped. CLI verb and parity tests pass for both new routes, `TestWeb_EverythingElseIs404` lists them, a channel that is off is a result field and not an HTTP error, and an old server's 404 triggers the direct fallback.
- New: the Web toast event is an implementation ticket. The fixture needs no new scenario.

Switches (stories 9 to 16)
- Flows: flow 200 turns Telegram off and checks the token and chat id remain in config.json and in the row. Mock states line `channel-off`.
- Checks: a config parse test that a kind list or a switch in `project.json` is rejected (story 22). Go tests (a channel off sends nothing, and Desktop off plays no sound; the manual mute still blocks Telegram and Slack and does not touch the others; Push needs server switch, email and device switch; a system message is sent with the kind list empty and not sent with the switch off). `gx server settings-set` of a switch applies with no restart.

Gates and order (stories 17 to 24)
- Checks: Go tests (an unchecked kind adds nothing to the channel's event or send series, trips no mute and joins no batch; Rest period, open-on-screen, AFK, recovery hold, the focus rule and the manual mute each still hold on the channels they cover; Push ignores a project's override gates; the badge is unchanged by the kind list and absent while the Push switch is off). Vitest (the app clears the badge and does not set it while the snapshot has none).

Caps and breakers (stories 25 to 30)
- Checks: Go tests per channel (21 sends in a minute trip the breaker and one system notice goes out on that channel; a flood on Desktop leaves Push and Telegram sending; the per-source mute exists on Telegram and Slack only; with a short test knob the breaker clears after no attempts, and attempts while tripped keep it tripped; Chat's global mute stays until `gx notify --enable`; one event to three devices is one send, counted after the one-second merge; a notification dropped by Rest period, the kind list or open-on-screen adds no attempt; only Session alert and Design question play a sound on Desktop).

Defaults and migration (stories 31 to 39)
- Fixtures: the rigs preset the marker. The new rig scenario writes an unmarked config with today's behaviour and checks the upgrade lists.
- Checks: migration unit tests for every list in Implementation Decisions (Push 3 kinds, Desktop 2, Web toast none, Telegram and Slack 15, Long ticket nowhere), idempotence, never overwriting, a new install writing nothing and getting the shared default, a channel with no settings getting its list, no switch key written (story 11), and a config without the marker upgrading in the CLI, the TUI and the server loaders. A Go test that a changed registry default does not alter an upgraded config. CLI: after the migration, `gx config get` prints each list.

Settings page (stories 40 to 50)
- Flows: flow 200 steps on `settings` at 900 px and 390 px. Mock states lines `desktop` (stories 40 to 43, 45), `channel-off` (story 44), `phone` (story 48), `push-off` (stories 46, 47, 50) and `account-device-switch` (stories 46, 47). Story 49 is the mock-fidelity rule: a Vitest test that the switch is the existing `BoolControl`. The flow also toggles a chip and checks config.json, and checks the "Not saved:" refusal on a bad list.
- Flow 2 step on `mixed` (short, for the time budget): turn the Push server switch off at 390 px, check the Account device switch is disabled, turn it on and check it is enabled again. `NotificationsPage.test.tsx` checks the disabled line "Push is off for the whole server. Turn it on in Settings › Notifications › Push.", that the device's own state is kept, and that "Send test push" is disabled.

CLI and TUI (stories 51 to 57)
- CLI: `gx notify --status` prints five channel lines with "on" or "off" and the checked count and still prints the destination lines; `gx notify --status --enable` still fails as today; `gx config test-notifications` with the server up tests all five channels, and with Slack off prints that Slack is off and sends nothing to it; with no server it tests Telegram and Slack and prints the other three as skipped. `gx notify --status` shows "tripped since <time>" for a tripped Desktop, Web toast or Push breaker. TUI: a Go test that the config editor lists each switch as a boolean. Story 55: a config reload test. Story 56: a Go test that a queued batch for a channel turned off is dropped. Story 57: Go tests for the target path per kind, and a Push payload test (route path, no URL).

Maintainer stories (stories 58 to 61)
- Story 58 and 59: a Go test that every catalog id is in the registry, the migration table, the web snapshot and the router, and that no call site calls a transport directly (the guard test the implementer writes), allowing exactly one exception: the CLI's direct fallback path. Story 60: the existing pause-table test passes with no new row. Story 61: `go test` for the touched packages and `pnpm exec vitest run` pass.

**Mock states** (mock file `$(gx tickets root)/notification-alignment/mocks/05-settings-page-look.html`, layout C). Each state is checked in the real UI with gx's own components, as a layout check and not a pixel match.

- `$(gx tickets root)/notification-alignment/mocks/05-settings-page-look.html` · `desktop` · width 900 · scenario `settings` (all five switches on, the new-install default lists) · parts `channel-switches`, `kind-rows`, `channel-settings`.
- `$(gx tickets root)/notification-alignment/mocks/05-settings-page-look.html` · `phone` · width 390 · scenario `settings` · parts `channel-switches`, `kind-rows`, `channel-settings`.
- `$(gx tickets root)/notification-alignment/mocks/05-settings-page-look.html` · `channel-off` · width 900 · scenario `settings` (the Telegram switch turned off in the flow) · parts `channel-switches`, `kind-rows`, `channel-settings`.
- `$(gx tickets root)/notification-alignment/mocks/05-settings-page-look.html` · `push-off` · width 900 · scenario `settings` (flow 200 turns the Push server switch off) · parts `channel-switches`, `kind-rows`, `channel-settings`.
- `$(gx tickets root)/notification-alignment/mocks/05-settings-page-look.html` · `account-device-switch` · width 390 · scenario `mixed` (flow 2 turns the Push server switch off, so the Account page's device switch is disabled) · parts `account-device-switch`. The mock draws this part at 900 px, inside its `push-off` state. The check runs at 390 px, the phone Account page.

New flows or scenarios to write: one rig scenario for the upgrade lists (an unmarked config). No new Playwright scenario. New steps in flow 200 (scenario `settings`) and flow 2 (scenario `mixed`), as above. The Go router, migration and breaker tests, the new upgrade rig scenario (an unmarked config, with the upgrade lists checked), the new server route and its CLI verb, the Web toast event and its page handler, the CLI tests and the guard test are implementation tickets.

## Out of Scope

- Per-project switches or kind lists. A project override sets only its Telegram or Slack target.
- Per-device Push kind lists.
- New delivery mechanics: notification action buttons, new transports, changes to batching or to the throttle thresholds.
- Any new pause kind, or any new way into a pause (ADR 0042).
- Replaying a Web toast to a page that was closed, or storing toasts.
- Changing the existing toasts (Rest warning and the others), the TUI toast or the Queue tab.
- A CLI to change a switch or a list, or to clear a Desktop, Web toast or Push breaker.
- A rich TUI view of switches or lists.
- Restyling the green Push device switch on the Account page.
- Re-sending events held back by a Rest period.

## Further Notes

- Builds on `docs/specs/push-notifications.md`, `docs/specs/better-notifications.md` and `docs/specs/notification-throttle.md`. The map is `gx:notification-alignment`; its tickets 01 to 07 are the decisions (07 settled the breaker clear and the Desktop banners).
- Ticket 04 listed "Desktop keeps Session alert"; ticket 07 settled that Desktop's upgrade list also holds Design question. A Design waiting on a round banners today, and Push already treats it as Design question, so Desktop's upgrade list also holds Design question. Desktop then keeps exactly what it shows today.
- A kind row can be checked for a channel that has no settings. The notification is then dropped at the adapter with a log line, as Telegram and Slack do with no token today.
