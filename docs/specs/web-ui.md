# Web UI: a read-only phone view of the Queue

## Problem Statement

The person starts epics and then walks away from the desk. They cannot tell from a phone whether the
Queue is moving. Is an epic running, waiting for a slot, or Parked on a question? The only view of
the Queue is the TUI, which needs a terminal on the machine that runs `gx server`.

A second problem sits in the TUI. In server mode the Queue tab never shows a Parked ticket's reason
line, because the snapshot carries no ticket body. The TUI's `parkedStatus` and the server's
`deadlockKind` also disagree about maps and the `error` status. The TUI and the server each derive
epic state on their own, and the two copies have drifted.

## Solution

`gx server` serves a small read-only web page that shows the Queue. The person opens it on a phone
over Tailscale or the LAN. It lists every epic in the Server queue with a state label, a progress
bar, `done/total`, the active tickets and the Parked tickets with their one-line reason. It updates
live and shows a short "Offline" strip when the link drops. It is dark, condensed and built to look
good.

To feed the page, the server computes epic summaries and puts them in `/v1/snapshot`. The TUI Queue
tab reads the same summaries, so the web page and the TUI show the same state words and one parked
rule serves both. This also fixes the two TUI bugs above.

## User Stories

1. As the person running gx, I want to open a URL on my phone and see the Queue, so that I can check progress away from my desk.
2. As the person running gx, I want every epic in the Server queue to appear, so that nothing running, waiting or stuck is hidden.
3. As the person running gx, I want each epic to carry a state label (Running, Waiting for a slot, Parked, Deadlocked, Awaiting merge, Merge blocked or Merging), so that I see at a glance which epics need me.
4. As the person running gx, I want Parked and Deadlocked epics at the top, so that the epics that need my attention come first.
5. As the person running gx, I want epics in the same state to keep the server queue order, so that the order matches what the TUI and the scheduler use.
6. As the person running gx, I want each epic to show its name and project, so that I know what it is.
7. As the person running gx, I want a progress bar with a `done/total` counter, so that I can judge how far an epic has come.
8. As the person running gx, I want to see an epic's active (claimed) tickets, so that I know what the agents work on right now.
9. As the person running gx, I want to see an epic's Parked tickets with their title and one-line Park reason, so that I know what to answer or repair when I get back to a computer.
10. As the person running gx, I want long Park reasons clamped to two lines with an ellipsis, so that the cards stay compact on a small screen.
11. As the person running gx, I want a Deadlocked epic to show a red "Deadlocked" badge and a reason line that names the blocking tickets or the cycle, so that I can tell a dependency error from a Parked epic and see where to look.
12. As the person running gx, I want Waiting epics to list their Parked tickets and nothing else, so that a Park reason is never hidden by the epic state.
13. As the person running gx, I want a "Paused" or "Draining" chip next to the Live dot, so that I know why nothing new starts.
14. As the person running gx, I want Waiting cards to read "Waiting: queue paused" or "Waiting: queue draining" in those modes, so that the cause is on the card.
15. As the person running gx, I want Running epics to keep their label while the queue is Paused, so that I do not think live work has stopped.
16. As the person running gx, I want a "Recently finished" list under the live cards, so that a finished epic does not just vanish. A complete epic that is not yet merged stays among the live cards as Awaiting merge until it merges.
17. As the person running gx, I want each Recently finished row to show the epic name, project, `done/total` and "finished 2h ago" on one dim line, so that history stays out of the way. The relative time refreshes every 60 seconds while the page is visible.
18. As the person running gx, I want map epics never to appear in Recently finished, so that the list shows implementation work only.
19. As the person running gx, I want the number of Recently finished rows to be configurable, with a default of 5, so that I can tune the list.
20. As the person running gx, I want Recently finished to survive a server restart, so that history does not reset.
21. As the person running gx, I want the same Recently finished list on the TUI Queue tab, so that both views agree.
22. As the person running gx, I want an "All quiet" screen with a help line when the Queue is empty, so that an empty page does not look broken.
23. As the person running gx, I want "All quiet" to appear only after a successful snapshot, so that it never lies while I am offline or the page still loads.
24. As the person running gx, I want the page to update by itself, so that I do not have to refresh.
25. As the person running gx, I want a green "Live" dot, so that I know the page is connected.
26. As the person running gx, I want the page to keep its last data on screen when the connection drops, so that a network blip does not blank it.
27. As the person running gx, I want a thin "Offline, reconnecting…" strip after 5 seconds without a connection, so that I know the data may be stale, without a flash on short blips.
28. As the person running gx, I want the page to refresh at once when I wake the phone or the network returns, so that I do not wait for a timeout.
29. As the person running gx, I want the page to detect a silently dead connection, so that it does not show stale data for minutes.
30. As the person running gx, I want the page to recover after a server restart, so that I do not have to reload it.
31. As the person running gx, I want a dark theme with condensed type, so that the page is pleasant on a phone.
32. As the person running gx, I want the page to work offline of the internet, so that it needs no CDN or external font.
33. As the person running gx, I want the web UI off by default, so that gx opens no new port until I ask.
34. As the person running gx, I want to turn it on with a `server.web.listen` address, so that I choose the Tailscale or LAN IP.
35. As the person running gx, I want a warning when I bind `0.0.0.0`, so that I do not expose it by mistake.
36. As the person running gx, I want every request to need a token, so that someone else on the LAN cannot read my Queue.
37. As the person running gx, I want `gx server status` to print the URL with the token, so that I can copy it.
38. As the person running gx, I want `gx server web-url --qr` to print a QR code, so that I can open the page on the phone without typing.
39. As the person running gx, I want the first visit to set a cookie and redirect to `/`, so that the token leaves the address bar and later visits just work.
40. As the person running gx, I want `gx server web-token --rotate` to replace the token and drop all cookies, so that I can revoke a leaked link.
41. As the person running gx, I want a bad or missing token to get a short 401 page, so that I know what went wrong.
42. As the person running gx, I want the web listener to serve only read routes, so that nobody can park, relaunch or edit anything through it.
43. As the person running gx, I want the TUI help dialog to show `Web UI  http://<addr>:<port>` or the bind error, so that I can find the URL and see a failure.
44. As the person running gx, I want a failed web bind not to stop the server, so that a port clash does not take down my queue.
45. As the person running gx, I want `gx server status` to report a web bind error, so that I can fix it.
46. As the person running gx, I want a change to `server.web` to warn "restart needed for server.web" on reload, so that I know it did not apply.
47. As the person running gx, I want Homebrew and release binaries to ship a working UI, so that I need no Node to use it.
48. As the person running gx, I want a `go install` build to serve a page that says it has no UI and where to get one, so that a missing UI is not a mystery.
49. As a gx developer, I want a dev workflow with hot reload against a real server, so that I can iterate on the page quickly.
50. As a gx developer, I want the dev server to run from a canned snapshot with no gx server, so that I can design without a live queue.
51. As a gx developer, I want release CI to build the UI before `go build` and fail if the app entry is missing, so that no release ships the placeholder by mistake.
52. As a gx developer, I want a design system of tokens and primitive components, so that new screens are easy to build and look consistent.
53. As a gx developer, I want a dev-only gallery that shows every component in every state, so that I can review the look without a live queue.
54. As a gx developer, I want a test that fails when a component uses a raw color or font instead of a theme token, so that the system stays the single source of style.
55. As the person using the TUI, I want the Queue tab to show each Parked ticket's reason line in server mode, so that the long-standing blank line is fixed.
56. As the person using the TUI, I want one parked rule for the TUI and the server, so that maps and `error` tickets are treated the same everywhere.
57. As the person using the TUI, I want the Queue tab header to use the same state words as the web page, so that the two views match.
58. As the person using the TUI, I want the Queue tab to read done/total and the active list from the server, so that the TUI keeps no second copy of the logic.
59. As the person using the TUI, I want every ticket status in the TUI to come from the server, so that blocked, waiting-for-children and parked read the same in every tab.
60. As the person using the TUI with no server running, I want the Queue tab to say "server not running", so that it never shows a guess.
61. As the person using the TUI with no server running, I want the Tickets and Archived tabs to show the raw file status, so that I can still browse tickets.
62. As the person using the TUI with an older server, I want an "update the server" notice, so that I know why the Queue tab is empty.

## Implementation Decisions

### Where it runs and how it is configured

- The web UI runs inside `gx server`, as a separate HTTP listener in the same process. It reads the snapshot and epic summaries in memory. There is no `gx web` command.
- A new `server.web` config block turns it on. It is off by default. Its `listen` key is a `host:port` string. Any IP is allowed. `0.0.0.0` logs a warning.
- `server.web.dev-frontend` (dev only, default off) serves the UI from the Vite dev server at `server.web.dev-url` (default `http://127.0.0.1:5173`). The web listener forwards every non-`/v1` GET to it, websockets included, and keeps `/v1` local. A non-loopback URL fails the bind with an error. Both keys are startup-only.
- `server.web` is startup-only, like `tcp-listen`. A reload that changes it warns "restart needed for server.web" and does nothing else.
- A bind failure does not stop the server. The server logs the error, reports it, and does not retry. Restarting the server is the fix. This differs from `tcp-listen`, where a bad bind aborts start.
- The handshake gains a `web_url` field and a web bind error field. `gx server status` prints the URL with the token. The TUI help dialog About section shows `Web UI  http://<addr>:<port>` (no token) when on, `Web UI  error: <reason>` when the bind failed, and no line when off.
- The existing loopback guard on the TCP API listener stays as it is. The web listener is a separate listener and does not go through it.
- The implementer picks the names of the remaining config keys and handshake fields.

### Network exposure and auth

- The web mux allows only `GET /v1/snapshot`, `GET /v1/events` and the embedded assets. Everything else returns 404. The listener has no write route.
- A token is always required, including on Tailscale. It is 32 random bytes stored in the server state directory with mode 0600. It never expires.
- `gx server web-token --rotate` replaces the token and invalidates all cookies. `gx server web-url --qr` prints a QR code of the URL with the token.
- The first request with a valid `?gx_token=` sets a cookie (`HttpOnly`, `SameSite=Strict`, one year) and redirects to `/`. The SSE stream authenticates with that cookie.
- A bad or missing token gets a 401 with a short plain page. There is no rate limit, because a 32-byte token cannot be guessed.
- Deferred to `gx:web-ui-v2`: a Host header check against DNS rebinding, TLS and a `Secure` cookie, and token expiry.

### Epic summaries on the wire

- `/v1/snapshot` gains an epic summary list, computed by the server from the epics it already loads for the Queue. SSE does not change in shape. Events carry no payload and clients re-fetch the snapshot after each one, so the summaries stay fresh.
- The summary covers every epic that holds a queue item, using the Queue's existing "whole epic" rule. Each summary has: the epic address and name, the project, the state, `done` and `total`, the active tickets, and the Parked tickets.
- State is one of Running, Waiting for a slot, Parked, Deadlocked or Awaiting merge. The server picks the first rule that matches, in this order:
  1. Awaiting merge: every ticket is done and the epic is not yet a Merged epic.
  2. Running: a ticket renders claimed and has a server claim time.
  3. Waiting for a slot: at least one ticket is runnable. A claimed ticket counts as runnable.
  4. Parked: nothing is runnable or running and at least one ticket is Parked.
  5. Deadlocked: nothing is runnable, running or Parked. This matches the server's `checkDeadlock`, which also requires that nothing is running.
- An epic leaves the live list once it is a Merged epic. It then appears in Recently finished.
- Each Parked entry carries the ticket address, title, status, recovery state and the one-line Park reason, unclipped. The client clamps the display. A Deadlocked summary carries a reason line that names the blocking tickets by address, or the cycle. The server's current fixed text ("nothing runnable and nothing a person could clear") is not enough, so the server's deadlock record changes to carry the same detail.
- Running and Waiting epics can also list Parked tickets, and Waiting epics list nothing else. Active tickets come first, then Parked ones.
- One shared parked rule serves the summary and `deadlockKind`. It follows the glossary: Parked means `needs-answer`, `needs-repair` or `draft`. Map epics are excluded. An `error` status is Parked, with its reason shown, because a person must act on it. This changes the server's current `deadlockKind`, which does not treat `error` as Parked. The glossary entry for Parked changes to match.
- The summary also carries a separate history list for Recently finished. It holds the last X completed loop epics, read from the epics' `completed_at` stamps in the ticket store. Map epics never count. It survives restarts and needs no new storage.
- X is a config value with a default of 5. Each history entry carries the epic name, project, `done/total` and the completion time.
- The queue mode (running, paused or draining) is already in the snapshot. The web page and the TUI use that field. No new field is needed.
- `TicketInfo` also carries each ticket's rendered status (blocked, waiting-for-children, parked and the rest).
- The web listener serves the full snapshot, including budget, sessions, AFK and extra usage. v1 does not trim it. v2 will show cost and runtime on the page, so trimming now would be undone. The listener is bound to a loopback, Tailscale or LAN address and needs the token.

### Event stream

- The server sends an SSE comment line (heartbeat) every 15 seconds on `/v1/events`. The existing stream has no heartbeat, so this is a new server change. The TUI client ignores comment lines, so it is unaffected.

### The web page

- A new top-level `webui/` directory holds a React, Vite and TypeScript app with Chakra UI v3 for styling. It is separate from the `web/` landing site, which stays on Preact. React and Chakra were chosen over Preact because the UI is expected to grow into a platform, and the component ecosystem targets React. The package manager is pnpm, with `pnpm-lock.yaml` committed, pnpm pinned through the `packageManager` field, and `pnpm install --frozen-lockfile` in `make` and CI. Moving the `web/` site to pnpm is optional and the implementer decides.
- Vite splits React, Chakra and Emotion into a separate `vendor` chunk (`manualChunks`), so app edits leave its hash and bytes alone and browsers keep it cached. File names are hashed.
- `webui/dist` is not committed. `make web-build` produces it, and `make build` and release CI (goreleaser) run it before `go build`. A small committed `webui/placeholder/index.html` is embedded as the fallback. A build without `dist` (for example `go install`) serves a page that says "This build has no web UI. Use Homebrew, a release binary or `make build`." The README `go install` section notes this. `/` falls back to `index.html`.
- `make web-dev` runs Vite with `/v1` proxied to the server TCP listener on `127.0.0.1:7421`. `make web-test` runs the Vitest suite.
- One canned snapshot JSON file feeds both the Vite dev server (usable with no gx server) and the Vitest tests.
- The page re-renders relative times every 60 seconds while the tab is visible.
- Fonts are self-hosted, embedded woff2 files under an OFL licence, Latin subset, never a CDN. Saira Condensed is used for the title, epic names, badges, counters and ids. Saira is used for ticket titles and reasons. The theme is dark only, with no light theme, no toggle and no `prefers-color-scheme` switch.

### Design system

- The system lives in `webui/src/ui/`, inside the app. It is not a shared package, because the `web/` landing site has a different look and stack.
- The Chakra theme is the single source of style: color tokens (surface, border, text normal/muted/dim, and a state color with a tinted chip background for each tone), font tokens (`condensed` is Saira Condensed, `text` is Saira), a size scale, spacing, radii and the 6px bar height. Components use theme tokens only, never a raw color, font family or pixel value. Fonts load from embedded woff2 files (Latin subset, `font-display: swap`). Saira Condensed weights are 500, 600 and 700, Saira weights are 400 and 500, and the implementer may trim them if size matters.
- A single `Tone` union (`running`, `waiting`, `parked`, `deadlocked`, `muted`; Awaiting merge uses `muted`) drives the color of Badge, ProgressBar and TicketLine. A new state means one new token set and one new union member.
- Primitives, each in its own folder with a render test: `Badge`, `ProgressBar`, `Chip` (the Paused and Draining chip), `LiveDot`, `TicketLine` (id, title, optional clamped reason), `Card`, `Strip` (the Offline strip) and `EmptyState`. Primitives are written as Chakra recipes or wrapped Chakra components and import no snapshot types.
- Domain components (`EpicCard`, `RecentlyFinishedRow`, `QueueScreen`) only compose primitives and hold the snapshot-to-view mapping.
- A dev-only gallery (a second Vite entry, `gallery.html`) shows every primitive in every tone and state, fed by the canned snapshot. The production build leaves it out, so it is not in `dist`.
- Adding a component is documented in `webui/README.md`: a new folder under `ui/`, theme tokens only, an entry in the gallery and a render test. There is no generator.
- Layout, from the approved prototype (variant P of the phone layout, local branch `prototype/web-ui-phone-layout`, never merged):
  - A `Queue` title in uppercase condensed type, with the Live dot on the right. The Paused or Draining chip sits beside the dot and is absent when the queue runs. There is no header summary and no counts row.
  - Epics sort by state: Parked and Deadlocked first, then Running, then Waiting for a slot, then Awaiting merge. Inside a state, epics keep the server queue order.
  - Each epic is a plain rounded card with a hairline border and no coloured stripe. The top row has a small uppercase condensed state badge on the left (Running green, Waiting amber, Parked and Deadlocked red, Awaiting merge muted, on a tinted chip) and the project name on the right. Below it sit the epic name in large condensed type and a smooth 6px progress bar in the state colour, with a small `done/total` counter to its right (total muted).
  - The ticket list sits under the bar as plain lines with no separators and no labels. Active rows show a muted id and the title. Parked rows show a red id, the title, and the Park reason below it in red, clamped to two lines. A Deadlocked card shows its reason line in the same style.
  - Under the live cards, a "Recently finished" heading is followed by dim flat rows: epic name, project, `done/total` and "finished 2h ago". They have no bar, no ticket lines and no badge.
  - The empty Queue shows a large "All quiet" and the line "Nothing is queued. Add an epic in the TUI and it will show up here."
  - Not shown in v1: elapsed time for live epics, cost, agent stats, per-ticket status words and any action.
- Connection behaviour:
  - On any stream drop or error, the page closes the stream, fetches `/v1/snapshot`, then opens `/v1/events?since=<snapshot seq>`. There is no special 410 branch. A restarted server resets the sequence counter, so the old `since` gets 410 and the snapshot-first rule covers it.
  - Backoff is 1s, 2s, 4s, up to 15s. A good snapshot resets it.
  - 45 seconds without any stream data (events or heartbeat) counts as a drop.
  - When the tab becomes visible or the browser fires `online`, the page fetches a snapshot and reconnects at once.
  - After 5 seconds disconnected, a thin "Offline, reconnecting…" strip shows at the top and disappears on recovery. The last data stays on screen the whole time. "All quiet" shows only after a successful snapshot.

### TUI Queue migration

- The TUI Queue tab reads the epic summaries (state, done/total, active list, Parked tickets with title and reason) from the server snapshot. It drops its own derivation of epic grouping, header status and parked status.
- The whole TUI stops calling the client-side rendered-status derivation and reads the status carried on `TicketInfo` instead. This covers Queue rows, preview, the ticket menu, suggested actions, search, checked and clear. The rendered-status rule stays in the `tickets` package as its single home. The server, the ralph loop and repair keep using it.
- The TUI Queue header words follow the server state vocabulary, so TUI and web labels match.
- The TUI Queue tab also shows the Recently finished list, as dim lines below the live epics.
- The TUI re-renders relative times ("finished 2h ago") every 60 seconds, and only while the terminal is visible.
- With an older server that sends no summaries or rendered status, every tab (Queue, Tickets, preview, menu, search) shows the same "update the server" notice and no ticket data. There is no client-side fallback, because the TUI and the server ship in one binary.
- With no server, the Queue tab shows "server not running". The Tickets tab and Archived epics show the raw file status, with no blocked or waiting-for-children overlay.

### Domain language

- Use the glossary terms Parked, Park reason, Deadlocked, Recently finished, Paused, Server queue, Snapshot and Server indicator. The glossary already defines Deadlocked and Recently finished. Add or update an entry if the epic state vocabulary (Running, Waiting for a slot) is not yet there.

## Testing Decisions

- A good test checks behaviour through the highest seam: what a handler returns, what a snapshot contains, what the page renders. It does not check internals such as which function computed a state.
- There are three layers and no browser end-to-end tests in v1. There are no screenshot tests. The approved prototype plus manual review on a phone cover "beauty". Revisit end-to-end tests if the UI gains actions.
- Go tests (run in CI), at the server's HTTP seam:
  - The web handler serves the embedded assets and `/` falls back to `index.html`. With only the placeholder embedded, it serves the "no web UI" page.
  - The web listener returns 404 for every `POST` pattern in the server route table, and for every `GET` route other than snapshot and events.
  - Token rules: valid token sets the cookie and redirects, a bad or missing token returns 401, the cookie authenticates the event stream, and rotation invalidates old cookies.
  - A config reload that changes `server.web` warns and changes nothing. A bind failure leaves the server running and reports the error in the handshake.
  - The snapshot carries epic summaries, with Parked `title` and reason, Deadlocked epics with their reason, the history list capped at X and excluding map epics, and the queue mode. A table test covers the state order, the shared parked rule (including maps and `error` as Parked) and the Awaiting merge state, and checks that `deadlockKind` agrees with the summary.
  - `/v1/events` emits the heartbeat comment on its interval.
- Vitest in `webui/` (run in CI through `make web-test`): the snapshot-to-view mapping and ordering (Parked and Deadlocked, Running, Waiting), the Waiting label in Paused and Draining, the reconnect and backoff logic with fake timers, the offline strip delay, and component renders: each primitive in each tone and state, plus the epic card, the two-line reason clamp, the Recently finished row and "All quiet". Two guard tests protect the design system: one fails when a component source file contains a raw hex color or a `font-family` literal, and one fails when an exported primitive is missing from the gallery.
- Release check (release CI only): after `make web-build`, fail if `webui/dist/index.html` is missing, so a release cannot ship the placeholder. There is no stale-`dist` check, because `dist` is not committed.
- TUI tests: the Queue tab renders from a server snapshot with summaries, shows the Park reason, shows the "update the server" notice for an older server, and shows "server not running" with no server. The Tickets tab shows the raw status with no server.
- Prior art: the server's `servertest` harness and client-based tests such as the pause and queue-mode tests, the existing deadlock tests, and the TUI Queue model tests that already feed a canned view model.

## Out of Scope

- Any action or write from the web UI: park, relaunch, unpark, queue edits.
- The design-process UI and full queue management (a later effort).
- Public internet access, TLS and multi-user support.
- A Host header check, a `Secure` cookie and token expiry, deferred to `gx:web-ui-v2`.
- Elapsed time, cost and agent stats on live epics.
- A light theme.
- Trimming the web snapshot payload. Cost and runtime arrive in `gx:web-ui-v2`.
- Browser end-to-end tests and screenshot tests.
- A design system shared with the `web/` landing site.

## Further Notes

- The phone reaches the page over Tailscale, and plain LAN also works. The page is never meant for the public internet.
- The research behind the summary design is in the repo's research note on epic summary data.
- The implementer chooses names that the design leaves open: config keys inside `server.web`, the history count key, handshake field names, JSON field names of the summary and the internal layout of the `webui/` source.
