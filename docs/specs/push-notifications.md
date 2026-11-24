## Problem Statement

gx can tell a person at the machine that a session needs them (the Banner) and a person away from the machine through Chat. A person with only a phone, or a Mac with the web app closed, gets neither. A blocked agent, a Design waiting on a round or a Paused ticket sits unseen until they open gx. The shipped PWA (`docs/specs/pwa.md`) made gx installable, but Push and the app icon badge were cut from it on 2026-11-09. The Design page also cannot notify a phone (`gx:webui-gaps/03`).

## Solution

gx gains **Push**: a Web Push notification the server sends to a subscribed device, even with the web app closed, plus an app icon badge.

- Push is off by default. A new Notifications page under Account in Settings holds one switch, "Notifications on this device". Only a tap on that switch asks the browser for permission. gx never asks by itself.
- Each device has its own switch and its own subscription, tied to its **Web login**. A "Send test push" button pushes to this device only.
- The server pushes what needs the person: Actionable, Blocked and Idle sessions, failed sessions, a Design waiting on a round, Paused tickets and escalations. The text is the Banner's for sessions and Chat's for tickets. Pushes merge for one second and carry one tag per session (one per ticket for ticket events, one per epic for an epic escalation).
- Push is independent of AFK mode, Chat and the Banner. Only the open-on-screen rule (sessions) and a **Rest period** keep it quiet. Held-back events are never sent later.
- A tap opens the session page, Design page, ticket page or epic page. It focuses an open window and navigates it, or opens one. An expired login returns to the target after login.
- The app icon badge is the number of Actionable sessions plus Paused tickets. The server counts it once and carries it in every push and in the snapshot. An open app only displays it.
- iPhone (Home Screen app) and Mac Safari (Add to Dock) are promised. Chrome, Edge and Android are best-effort and not verified. Any origin the device installed from works.

## User Stories

1. As a person, I want to turn push on from a switch on a Notifications page, so that I choose which of my devices buzz.
2. As a person, I want push off by default on every device, so that nothing buzzes until I opt in there.
3. As a person, I want gx to ask the browser for notification permission only when I tap the switch, so that I am never surprised by a permission prompt.
4. As a person, I want a "Notifications" row in the Account group of Settings, between Web logins and Password, showing an On or Off chip, so that I find it and see its state at a glance.
5. As a person, I want the switch to say which web login it is on for ("On for <login name>"), so that I know which device I am looking at.
6. As a person, I want turning the switch off to delete this device's subscription and clear its badge, so that the device stops buzzing at once.
7. As a person whose browser blocks notifications for gx, I want the switch disabled with "Blocked" and a line telling me to allow it in the browser or iOS Settings and reopen the page, so that I know what to fix.
8. As an iPhone person in a plain Safari tab, I want the switch disabled with "Needs the Home Screen app" and the Add to Home Screen steps, so that I know push needs the installed app.
9. As a person in a browser without push support, I want the switch disabled with "Not supported" and a pointer to Safari on iPhone or Mac or a current browser, so that I know why.
10. As a person whose server has no contact email, I want the switch disabled with "Set a contact email in the server config", so that I know push cannot start yet.
11. As a person whose server key changed, I want the switch enabled and Off with "The server key changed. Turn push on again." (kept on this device until I turn push on), so that I know to turn it on by hand.
12. As a person, I want "Send test push" always shown, disabled with "Turn notifications on first." while the switch is off, so that the button is findable and its precondition is clear.
13. As a person, I want "Send test push" to push to this device only and show "Sent" with a hint to check Focus mode and iOS notification settings, so that I can tell whether it works.
14. As a person, I want a failed test push to show the reason right away (for example "gx can't reach the push service. Check the server's outbound HTTPS"), so that I can fix it.
15. As a person, I want the Notifications page to show "Last push failed: <reason>, <time>" after any failed send until the next success, so that a silent failure in the background is visible.
16. As a person, I want each web login row with a subscription to show a "Push on" chip, so that I see which devices get pushes.
17. As a person, I want "Push: On since <date>" with a "Remove" action on a login's page, so that I can stop a lost or old device from the one I hold.
18. As a person, I want Remove to delete only the push subscription and keep the login, and only that device to be able to turn push back on, so that nobody re-enables push for me.
19. As a person, I want a push when a session becomes Actionable, Blocked or Idle, so that I come back when an agent needs me.
20. As a person, I want a push when a session fails, so that I learn about a dead or stuck agent.
21. As a person, I want a push when a Design waits on a round, so that I can answer from my phone.
22. As a person, I want a push for a Paused ticket at the point where Chat sends its message (after the recovery hold, so a pause that recovery fixes buzzes nothing), even when Chat is muted for the project, so that I can resume or answer it.
23. As a person, I want a push when a ticket or epic escalates, at the same point as Chat's message and without Chat's mute gates, so that I know automatic recovery gave up.
24. As a person, I want a session push to carry the Banner's text and a ticket or escalation push to carry Chat's headline and sentence, so that the wording is the one I already know.
25. As a person, I want pushes for one session to merge for one second and carry one tag per session (one per ticket for ticket events, one per epic for an epic escalation), so that a burst is one notification and a newer one replaces the older.
26. As a person, I want push independent of AFK mode, Chat and the Banner settings, so that turning those off does not silence my phone.
27. As a person, I want no push for a session I have open on screen, so that I am not buzzed about what I am looking at.
28. As a person, I want Paused ticket and escalation pushes never skipped by the open-on-screen rule, so that a ticket I look at still reaches my phone.
29. As a person in a Rest period, I want no push, and held-back events never sent afterwards, so that gx leaves me alone and does not nag later.
30. As a person, I want a tap on a notification to focus an open gx window and navigate it to the target, so that I do not get a second window.
31. As a person, I want a tap with no open window to open one on the target (session page, Design page, ticket page or epic page), so that I land where the news is.
32. As a person whose login expired, I want a tap to return me to the target after I sign in, so that the tap still gets me there.
33. As a person, I want the push payload to hold a route path and never a URL or token, so that a push cannot leak access.
34. As a person, I want every push to carry the badge count, so that the icon is right even with the app closed.
35. As a person with the app open, I want the badge to update live from the server's count on every snapshot change and when the app returns to the foreground, so that it never drifts.
36. As a person, I want a closed app to keep the last pushed badge number, so that the icon says what I last knew.
37. As a person whose switch is off, whose browser has no push support or whose permission is not granted, I want gx never to touch the badge, so that gx does not mark an app I did not opt in.
38. As a person, I want the tab title to keep its own Actionable count, so that the browser tab does not change.
39. As a person, I want a subscription to die with its web login on every sign-out path (revoke, password change, authenticator reset, 90 days idle), so that no signed-out device is pushed.
40. As a person, I want a subscription the push service reports gone (404 or 410) deleted at once, so that dead endpoints do not pile up.
41. As a person, I want a device whose browser dropped the subscription (permission revoked) to read Off and the stale server record deleted when the app opens, so that the switch tells the truth and gx never re-subscribes silently.
42. As a person, I want a network error, a 5xx or a 429 retried (3 tries, 1 s then 5 s) in the background and then dropped, so that a hiccup does not lose a push and never delays the Banner.
43. As a person, I want a 403 to keep the subscription and any other 4xx not to be retried, so that a bad `sub` or payload does not delete a good device.
44. As a person with several devices, I want sends to run per device in parallel, so that one slow device never delays another.
45. As a person, I want a push to wait at the push service for up to an hour, and urgent events (Actionable, Blocked, failed, Design question, escalation) sent at high urgency, so that important ones arrive promptly.
46. As a person, I want push to work from any origin the device installed from (hostname or IP), with no hostname special case, so that the way I reach my machine does not matter.
47. As the person, I want the server to make its VAPID key pair on first use and keep it, so that I never manage keys.
48. As the person, I want my own email, set in the server config, as the VAPID contact, never logged and never shown in the web UI, so that my address stays private.
49. As a person, I want Chat to stay exactly as it is, so that adding Push changes nothing about it.
50. As a gx maintainer, I want fast automated tests for the sender, the lifecycle, the badge count and the worker handlers, so that a change cannot break delivery or leak a subscription.
51. As a gx maintainer, I want a real-device check on iPhone and Mac Safari, so that platform behaviour the tests cannot see is confirmed.
52. As a person whose server key changed, I want the app to notice that its browser subscription was made with another key, unsubscribe the browser and show Off with "The server key changed. Turn push on again.", so that I am not left with a device that fails every push forever.
53. As a person whose browser renewed its push endpoint, I want the app to send the new subscription to the server when it opens (and the worker to do the same where the browser tells it), so that the device keeps getting pushes without my turning it on again.
54. As a gx maintainer, I want the server to refuse a subscription whose endpoint is not `https:`, so that the server never POSTs to an arbitrary plain-HTTP address a browser names.
55. As a person with no Chat configured, I want pushes for a Paused ticket, a conflict-resolution child's pause and an escalation anyway, so that push does not depend on Chat being set up.
56. As a person who removed push from a device, I want a late endpoint renewal from that device refused, so that a renewal cannot silently turn push back on.

## Implementation Decisions

### Subscription lifecycle and storage

- A subscription lives on its web login record in `web-auth.json`, in its own optional field on the stored login record (not on the wire `WebLogin`, which the CLI and the web list also send). It is deleted in the same write as the login. At most one per login; a new subscribe or an endpoint renewal replaces it.
- Stored fields: endpoint, the two push keys, the VAPID public key the subscription was made with (the same base64url string the push-state route returns), and creation time. There is no last-successful-push time: it would cost a file write per push and nothing reads it. If a later screen needs it, it is kept in memory like "Last push failed".
- No migration: the new field is optional, a file without it loads with no subscriptions, and a downgraded binary drops the field when it next writes the file.
- The server refuses an endpoint that is not `https:`.
- A device controls only its own subscription (subscribe, turn off, test). The Remove on another login's page is the one exception, and it only deletes.
- The browser is the truth on the device. The switch reads On only when permission is granted, the browser holds a subscription, the server holds a record for this login and the subscription's `applicationServerKey` equals the server's public key. "Matching" means the same login and the same key, not the same endpoint. gx never subscribes again by itself. On every app open the app checks, in this order:
  1. No browser subscription and a server record: delete the record and show Off.
  2. The browser's key differs from the server's key: unsubscribe the browser, delete any record and show the key-changed reason.
  3. Same key, a record, a different endpoint: sync the renewed endpoint (renewal route).
  4. Same key, no record: show Off and leave the browser subscription, which the next turn-on reuses.
- Dies: with the login on every sign-out path; at once on 404 or 410; when the switch goes off; on Remove; when gx makes a new VAPID key pair (the subscriptions made with the old key are deleted in the same write that saves the new pair).
- A login idle for 90 days or more gets no push: before it sends, the sender drops that login and its subscription in one write. The existing rule drops an idle login only when its cookie comes back, which a phone that never opens the app never does.
- Endpoint renewal has its own web-only route. It replaces only an existing record of this login, and only when the old endpoint matches (the worker's `pushsubscriptionchange` handler passes the browser's `oldSubscription` endpoint where it has one). Otherwise it refuses with `push-no-subscription` and stores nothing, so a renewal can never undo a Remove, a switch off, a 404/410 delete or the idle drop. The app calls it in check 3 above; the worker's `pushsubscriptionchange` handler calls it where the browser supports that event. It is a sync of a subscription the person already turned on, not a silent re-subscribe.
- The subscribe, remove and test routes and one `GET` push-state route for this login are web-only (ADR 0028 amendment: account routes and browser-only writes behind the web login). The push-state route returns the VAPID public key as a base64url string, whether a contact email is set (never the email), whether this login holds a subscription and its stored endpoint, and the "Last push failed" line. Refusal reasons are a public contract: `push-no-contact`, `push-bad-subscription` (not `https:`, malformed), `push-not-yours` and `push-no-subscription` (renewal with no record).
- The login list derives a push-on flag and the subscription date for the wire `WebLogin`. `gx server web-logins --json` gains those two fields. The CLI listing text does not change.

### Opt-in page

- A "Notifications" row in the Account group of Settings home, between Web logins and Password, with an On or Off chip. The chip reads On only when the switch is On.
- It opens a Notifications page: the switch "Notifications on this device" and a "Send test push" row. The page never asks for permission on load.
- Switch states: Off ("Off. Turning it on asks this browser for permission."), On ("On for <login name>"), and disabled with a reason line: Blocked, Needs the Home Screen app (plain iPhone Safari tab), Not supported, and "Set a contact email in the server config". The key-changed state is not disabled: the switch is enabled and Off, with the reason "The server key changed. Turn push on again." The device keeps that reason in `localStorage` until the person turns push on again, so it survives the next open.
- "Send test push" is disabled with "Turn notifications on first." while the switch is off. The test push goes to this device only, is not an event (the open-on-screen rule and the Rest period do not apply to it) uses a 60 s TTL and makes one attempt with no retries, so the page shows the result at once. After the tap the page shows a result message: sent (with the Focus and iOS settings hint) or the failure reason.
- "Last push failed: <reason>, <time>" shows on the page after any failed send, kept in memory and cleared by the next success. The wording of failures is the Delivery rules wording below, not the mock's longer sentence.
- A login row with a subscription shows a "Push on" chip. Each login's page has a Push row ("On since <date>") with "Remove". Remove keeps the login.

### VAPID keys and the contact claim

- One key pair per server, made on first use and saved as `web-push/vapid.json` (mode 0600) under the state directory, beside `web-tls`. No config key for it.
- There is no rotate action. A key file that is lost, unreadable or bad is treated as lost: gx makes a new pair, writes one log line, and writes `vapid.json` first. It then deletes the subscriptions made with the old key in one `web-auth.json` write (two files, so two writes). The loader and the sender also treat a subscription whose stored key differs from the current key as gone, so a crash between the two writes is harmless. The app compares its browser subscription's `applicationServerKey` with the server's public key (from the push-state route) in check 2 above on every open. gx never re-subscribes silently. The server loads the key once, at start.
- The `sub` claim is the person's own email from the server config as one `mailto:<email>` (Apple answers 403 unless it holds exactly one). It is required. The implementer picks the config key name and validates it as one plain email address. With none set, the switch is disabled and "Send test push" stays disabled. The email is sent only in the claim, never logged and never shown in the web UI.

### What is pushed

- Events: Actionable, Blocked and Idle sessions, failed sessions, a Design waiting on a round, Paused tickets, escalations. Session pushes hang off the same triggers as the Banner and use its 1-second merge and its text. Paused ticket and escalation pushes use the Chat message's headline and plain sentence (`docs/specs/better-notifications.md`), with the ticket title. They fire where Chat sends its message, which is after the recovery hold (a pause that recovery fixes sends nothing), and skip Chat's mute and project-override gates. Chat sends them from three call sites, and the push hook sits at each one, beside the Chat call and not inside `ServerChat` (whose sinks apply the mute and quiet gates and which returns early when no Chat is configured): `Writer.sendPause` (unheld pauses and a recovery hold's release), `finishSink.TicketNeedsHuman` (a conflict-resolution child's pause, which never goes through `Writer.Pause`) and `Recovery.escalate` (ticket and epic escalations). `Merger.pauseRoot` (a merge-blocked epic) gets no push, as Out of Scope says. The badge still counts every `paused` ticket.

_Superseded in part: a merge-blocked epic root pause now pushes when the Push kind list has Epic paused (`docs/specs/notification-alignment.md`)._
- Push is independent of AFK mode, Chat and the Banner settings. Session pushes are skipped by the existing open-on-screen rule (a visible web tab, or a focused TUI when AFK mode is off) and by nothing else. The rule is global, not per device. Paused ticket and escalation pushes have no session, so the rule never skips them. An epic escalation opens the epic page.
- A Rest period keeps Push quiet and never sends held-back events later, as the glossary says. The sender runs the same quiet check the Banner uses, at each send attempt, so a retry that falls inside a Rest period is dropped. The check stays true after "Unlock anyway" until the period's original end. The test push skips it. The snapshot's badge field keeps updating.
- Chat is unchanged.

### Delivery rules

- Retries (not for the test push): a network error, a 5xx or a 429 gets 3 tries in total (1 s, then 5 s apart), in the background so the Banner is never delayed. `Retry-After` is obeyed when it is 30 s or less. After the last try the push is dropped.
- A 404 or 410 deletes the subscription at once. A 403 never deletes it (Apple also answers 403 for a bad `sub`). Any other 4xx is a failure with its reason, keeps the subscription and is not retried.
- Failures: every failed send writes a log line and sets the device's "Last push failed" line. Nothing on the Health tray. With no outbound HTTPS the reason is "gx can't reach the push service. Check the server's outbound HTTPS" and the same rules apply.
- TTL 1 hour for every event, 60 s for the test push. Urgency `high` for Actionable, Blocked, failed sessions, Design questions and escalations; `normal` for Idle sessions and Paused tickets. The `Topic` header is a short URL-safe hash of the tag (the header allows 32 characters of the URL-safe base64 alphabet; ids and addresses hold `:` and `/`).
- Payload: JSON with `title`, `body`, `path`, `badge` and `tag`. `path` is a route path inside the app, never a URL or token. The body is cut at 200 characters; if the payload still does not fit the 3993-byte limit, the server cuts the body to 100. The tag is one per session, one per ticket for ticket events, one per epic for an epic escalation. The implementer picks the tag and path formats.
- Sends run per device in parallel, in the background.

### Badge

- The server counts the badge once: Actionable sessions plus Paused tickets. It carries the number in every push and as one `badge` field in the snapshot on the event stream.
- The open app only displays it with `setAppBadge`, on every snapshot change and on return to the foreground. It never counts itself. A closed app keeps the last pushed number.
- The app sets the badge only while this device's switch is On. Turning the switch off clears the badge, and the app does not touch it again until the switch is turned on. With no push support or no permission the app never touches the badge.
- The tab title keeps its own Actionable count.

### Worker and tap

- The service worker gains a `push` handler (shows the notification with the tag, sets the badge from the payload) and a `notificationclick` handler (focuses an open window and sends it the path with `postMessage`, and the app's router moves to it; `navigate()` is the fallback only for a window with no listener, because it reloads the app; with no window, `openWindow` opens `/#<path>`) and a `pushsubscriptionchange` handler (calls the renewal route). The worker always shows a notification for a push, because iOS requires it. The server decides what to send.
- An expired login returns to the tap's target after login. The existing signed-out behaviour is otherwise unchanged.

## Testing Decisions

A good test checks external behaviour: what the person sees, what the push service receives, what is stored. It does not check internals.

- **Go.** The sender takes an HTTP client; tests point subscriptions at an `httptest` server that records requests. They check the VAPID header (`sub` is exactly one `mailto:`), the payload (decrypted with the subscription's own keys), TTL, urgency and topic; 404/410 delete the subscription; 3 tries then drop; 403 keeps it; other 4xx are not retried; a refused connection gives the "can't reach the push service" reason. More: key file made on first use with mode 0600; a lost or bad key file gives a new pair and devices read Off with the reason; a missing email disables the switch; the subscription is deleted with its web login on every sign-out path; the badge is one count shared by the push and the snapshot; Rest period sends nothing and sends nothing later; the open-on-screen rule skips session pushes and not ticket pushes; a Paused ticket or escalation push waits for the recovery hold and ignores Chat's mute; a login idle 90 days or more gets no push and is gone afterwards; a new key pair deletes old-key subscriptions; the Rest period check runs at each attempt; the test push makes one attempt; a non-`https:` endpoint is refused; the Topic header is at most 32 URL-safe characters; an old `web-auth.json` with no subscription field loads and a subscription survives a write and reload. Prior art: the existing `server/sessions` banner tests and `server/webauth` tests.
- **Chat call sites.** Go tests with no Chat configured: a paused ticket, a conflict-resolution child's pause and an escalation each still send one push.
- **Vitest.** The worker's `push` handler (shows the notification, sets the badge, one tag per session) and `notificationclick` handler (focuses and navigates an open window, else opens one); the Notifications page states; the app setting the badge from the snapshot field only while the switch is on, and clearing it when the switch goes off; one test per open-time check (no browser subscription with a record, key mismatch, renewed endpoint, no record with a browser subscription); the key-changed reason kept in `localStorage` until the switch is turned on again; the worker's `pushsubscriptionchange` calling the renewal route; the app's router moving to a path sent by `postMessage`. Prior art: `webui/src/pwa/workerCore.test.ts`.
- **Playwright.** No new flow. The Notifications page steps join the existing flow 2, "Epic looks right" (`epics.e2e.ts`), on scenario `mixed`, before its server-stop step. They fake push support and permission in the page and check the page states. Real delivery is never tested in Playwright. The fixture gains a fake push service that a faked subscription points at and a control to make it fail; production accepts only `https:` endpoints, so the fixture reaches its fake service through a test-only setting that changes no production behaviour, and the implementer picks that setting.
- **Real device.** A human-in-the-loop verify ticket on iPhone (Home Screen app) and Mac Safari (Add to Dock). It needs the person's email in server config and the server reachable over the origin the device installed from.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built. Checks use the `gx-ui-check` flow runner on the fixture, plus a real-device pass on iPhone and Mac Safari. One entry per user story, numbered as the story is. "Steps" are the new Notifications steps in flow 2 "Epic looks right" (scenario `mixed`, push support and permission faked in the page, fake push service in the fixture, contact email set).

1. Story 1: steps tap the switch and check it reads On and the login holds a subscription. Real-device: iPhone and Mac Safari turn push on. Mock states line `on`.
2. Story 2: steps open the page and check the switch is Off and no permission request happened. Mock states line `off`.
3. Story 3: vitest component test (no permission call on page load, one on the switch tap). Steps check the faked permission request count.
4. Story 4: steps check the Account group order (Web logins, Notifications, Password) and the chip. Mock states line `settings-home`.
5. Story 5: Mock states line `on` (names the login).
6. Story 6: steps turn the switch off and check the login row loses "Push on" and the badge is cleared. Go test: the subscription is deleted on switch off.
7. Story 7: Mock states line `blocked`.
8. Story 8: Mock states line `iphone-tab`.
9. Story 9: Mock states line `unsupported`.
10. Story 10: no stored mock state: built from the `blocked` reason style. New Go test (no email: not startable) and vitest page test showing "Set a contact email in the server config" with the test button disabled.
11. Story 11: no stored mock state: built from the `blocked` reason style, with the switch enabled. New Go tests (lost key file: `vapid.json` is written first, then old-key subscriptions are deleted in one `web-auth.json` write; the loader and the sender treat a different-key subscription as gone) and vitest page test (a key mismatch shows an enabled Off switch with the reason, kept in `localStorage` across a reopen until the switch is turned on). Real-device: delete `vapid.json` and restart the server; the device reads Off with the reason.
12. Story 12: Mock states line `off`. Steps check that "Send test push" is disabled with "Turn notifications on first." while the switch is off.
13. Story 13: Mock states lines `on` and `test-sent`. Real-device: the test push arrives on both devices.
14. Story 14: Mock states line `test-failed`. Go test: a refused connection gives the reason text.
15. Story 15: no stored mock state: built from the `test-result` warn style. Steps make the fake push service fail, run the test, then check "Last push failed" and that a later success clears it.
16. Story 16: Mock states line `logins-list`.
17. Story 17: Mock states line `login-detail`.
18. Story 18: steps click Remove and check the login stays and "Push on" goes. Go test: Remove deletes only the subscription. Real-device: after Remove the device gets no push and must turn it on itself.
19. Story 19: Go tests (Actionable, Blocked and Idle each send one push with the Banner's text). Real-device: one real Blocked session.
20. Story 20: Go test (failed session sends one push, a repeated kind sends none).
21. Story 21: Go test (a Design round sends one push to the Design page path). Real-device: a Design question; a tap opens the Design page.
22. Story 22: Go tests (a Paused ticket sends one push with Chat's headline and ticket title at the point Chat sends, after the recovery hold; a pause that recovery fixes sends none; a project that mutes Chat still sends the push; the badge counts every `paused` ticket).
23. Story 23: Go tests (a ticket escalation and an epic escalation each send one push with Chat's sentence at high urgency, after the hold and without Chat's mute gates; the epic push has its own tag and the epic page path).
24. Story 24: Go tests comparing the push title and body with the Banner's (sessions) and Chat's (tickets).
25. Story 25: Go tests (two triggers within 1 s give one push; the tag is per session, per ticket for tickets).
26. Story 26: Go test (AFK on, Chat off and Banners off still send a push).
27. Story 27: Go test (a session open in a visible web tab sends none).
28. Story 28: Go test (a Paused ticket push is sent while that ticket's page is the open one).
29. Story 29: Go test (during a Rest period nothing is sent, and after it ends nothing held back is sent).
30. Story 30: vitest worker test (open window focused and sent the path by `postMessage`, no new window; `navigate()` only for a window with no listener) and a vitest router test. Real-device: a tap on an open window navigates it.
31. Story 31: vitest worker test (no window: `openWindow` on `/#<path>`; the path covers session, Design, ticket and epic pages). Real-device: a tap opens the session, Design or ticket page.
32. Story 32: vitest test (login screen keeps the target and opens it after sign-in). Real-device: an expired login returns to the target.
33. Story 33: Go test (the decrypted payload holds a route path, no URL, no token).
34. Story 34: Go test (every push carries the shared badge count).
35. Story 35: vitest test (`setAppBadge` called on snapshot change and on return to foreground). Steps check the fake `setAppBadge` calls match the snapshot field.
36. Story 36: vitest worker test (the push handler sets the badge from the payload). Real-device: the closed app keeps the last number.
37. Story 37: vitest tests (switch off, no support and no permission: `setAppBadge` never called).
38. Story 38: the existing tab-title vitest tests still pass.
39. Story 39: Go tests (the subscription is deleted on revoke, password change and authenticator reset; a login idle 90 days or more gets no push and is deleted with its subscription in one write before sending).
40. Story 40: Go tests (404 and 410 delete it).
41. Story 41: vitest test (a stale server record is deleted when the app opens and the switch reads Off). Real-device: revoke permission in iOS Settings, reopen, the switch reads Off.
42. Story 42: Go tests (3 tries then drop, `Retry-After` obeyed up to 30 s, the Banner not delayed; the test push makes one attempt; a retry inside a Rest period is dropped).
43. Story 43: Go tests (403 keeps the subscription; other 4xx are not retried).
44. Story 44: Go test (one slow device does not delay another).
45. Story 45: Go tests (TTL 1 hour and 60 s for the test push; urgency per event; the `Topic` is a hash of the tag, at most 32 URL-safe characters).
46. Story 46: Real-device: iPhone and Mac Safari turn push on from a hostname origin and an IP origin.
47. Story 47: Go test (key file created on first use, mode 0600, same pair on restart).
48. Story 48: Go tests (`sub` is exactly one `mailto:`, an invalid email is refused, the email is never in logs or in any web response).
49. Story 49: the existing Chat tests and the Chat flows still pass.
50. Story 50: `go test` for the server packages and `pnpm exec vitest run` for the worker and page tests pass.
51. Story 51: the real-device verify ticket runs: turn push on, "Send test push", one real event (a Design question), a tap opens the right page, a tap on an open window navigates it (this checks the `postMessage` path in the iOS Home Screen app), the badge matches Actionable plus Paused, an expired login returns to the target, Remove stops pushes, a deleted `vapid.json` plus a server restart shows Off with the reason.
52. Story 52: vitest tests, one per open-time check, including a mismatching `applicationServerKey` unsubscribing the browser, deleting any record and showing the reason. Go tests as in story 11. Real-device: as in story 11.
53. Story 53: vitest tests (check 3: an open app with a differing endpoint calls the renewal route; the worker's `pushsubscriptionchange` does the same with `oldSubscription`). Go test: a renewal replaces the record when the old endpoint matches.
54. Story 54: Go test (a subscribe with an `http:` endpoint is refused with `push-bad-subscription`).
55. Story 55: Go tests with no Chat configured (a Paused ticket from `Writer.sendPause`, a conflict-resolution child's pause from `finishSink.TicketNeedsHuman` and an escalation from `Recovery.escalate` each send one push; a merge-blocked epic root pause sends none).
    _Superseded in part: a merge-blocked epic root pause sends a push when the Push kind list has Epic paused, and none when it lacks it (`docs/specs/notification-alignment.md`)._
56. Story 56: Go tests (a renewal after Remove, switch off, a 404/410 delete or the idle drop is refused with `push-no-subscription` and stores nothing; a renewal with a wrong old endpoint is refused).

New flows or scenarios to write: no new Playwright flow and no new scenario. The steps join flow 2 on `mixed`, and the fixture gains a fake push service with a fail control. The vitest and Go tests above, the fixture's fake push service and the real-device verify ticket are implementation tickets.

**Mock states** (mock file `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html`, layout A). The stored mock is ten states. The `logins-list` and `login-detail` states use `mixed`'s one login, which stands in for the device rows. The unsupported, blocked and iPhone states are faked in the page by permission, feature and user agent.

- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `settings-home` · width 390 · scenario `mixed` · parts `account-group`, `notifications-row`.
- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `off` · width 390 · scenario `mixed` (permission default) · parts `switch-card`.
- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `on` · width 390 · scenario `mixed` (push faked as granted and subscribed) · parts `switch-card`, `test-card`.
- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `test-sent` · width 390 · scenario `mixed` (fake push service accepts) · parts `test-result`.
- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `test-failed` · width 390 · scenario `mixed` (fake push service unreachable; the reason text is the Delivery rules wording) · parts `test-result`.
- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `blocked` · width 390 · scenario `mixed` (permission faked as denied) · parts `reason`.
- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `iphone-tab` · width 390 · scenario `mixed` (iPhone Safari user agent faked, not standalone) · parts `reason`.
- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `unsupported` · width 390 · scenario `mixed` (no `PushManager` in the page) · parts `reason`.
- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `logins-list` · width 390 · scenario `mixed` (the login holds a subscription) · parts `push-chip`.
- `$(gx tickets root)/push-notifications/mocks/02-opt-in-ui.html` · `login-detail` · width 390 · scenario `mixed` (the login holds a subscription) · parts `push-row`.

Three states have no stored mock state: "Last push failed", "Set a contact email in the server config" and "The server key changed". They are built from the existing `test-result` and `reason` message styles, and the steps and the vitest page tests check them.

## Out of Scope

- Buttons on a notification that answer an agent without opening the app. It needs action payloads and posting from the worker, and iOS supports notification actions poorly. A later effort.
- Chrome, Edge and Android are best-effort and not verified. Push may work there, but no test or device check covers it.
- A server-wide push config key. The per-device switch is the only push switch.
  _Superseded: Push now has a server-wide switch beside the contact email, and a kind list (`docs/specs/notification-alignment.md`)._
- A rotate action for the VAPID key.
- Any change to Chat, the Banner or AFK mode.
- Several subscriptions per web login.
- Re-sending events held back during a Rest period.
- Pushes for merge-blocked epics (`merge-error`, `dirty-trunk`, `trunk-missing`, `needs-rebase`). They need the person, but the map's list names Paused tickets only.
  _Superseded: the Epic paused row of the kind list covers them (`docs/specs/notification-alignment.md`)._
- Editing `gx:webui-gaps/03` (this spec covers it, the ticket stays as it is).

## Further Notes

- Builds on `docs/specs/pwa.md` and the worker in `webui/src/pwa/`. The parked push tickets are in the archived `gx:pwa` map (02, 03, 04, 07).
- Device test (archived `gx:pwa/07`, 2026-11-09): the iPhone Home Screen app and Mac Safari Add to Dock passed on `https://mini.elentok.com:8443` and on the IP origin. Chrome on a Mac failed only because that Mac did not trust gx's root. Apple answered 403 until the VAPID `sub` held exactly one `mailto:`.
- The Banner lives in `server/sessions/sessionbanner.go` and web focus in `server/sessions/sessionfocus.go`.
- The mock's test-failed wording names Apple's host. The Delivery rules wording above replaces it.
