## Problem Statement

The gx web UI is a web page. On an iPhone it cannot be added as a proper app: there is no web app manifest, so the Home Screen icon opens a Safari-style page, not a standalone window. There is no service worker, so a cold open while the gx server is unreachable shows the browser's error page, not gx. A person who wants gx on their phone, Mac or desktop has no "Install app" entry and no instructions: Chrome and Edge show their own install prompt only when the page is installable, and Safari has no prompt at all. After gx is upgraded there is also no way to be sure an installed copy runs the new build.

## Solution

The gx web UI becomes an installable PWA, still a web app served by the gx server (no app store listing, no native wrapper).

- A web app manifest and generated icons make the UI installable: iOS Home Screen (the main target), Android Chrome, desktop Chrome and Edge, and macOS Safari ("Add to Dock"). Each installed copy opens in a standalone window.
- A service worker caches the app shell. A cold open while the server cannot be reached shows the shell with the existing Offline strip and an empty state, not a browser error.
- The gear menu gets two rows at the bottom, under their own separator, after Tips: "Install app" and "Force refresh". "Install app" opens the browser's own install prompt where one exists and a steps sheet on Safari, and is hidden when the app already runs installed. "Force refresh" unregisters the worker, clears the app shell caches and reloads from the network.
- Updates need no prompt: the app shell is fetched network first, so a new gx build shows on the next open.

Web Push and the app icon badge are out of scope (see Out of Scope).

## User Stories

1. As an iPhone user, I want gx to be addable to my Home Screen with its own icon and name, so that I can open it like any other app.
2. As an iPhone user, I want the Home Screen app to open without Safari's address bar and toolbar, so that the whole screen shows gx.
3. As an iPhone user, I want "Install app" in the gear menu to show me the Share steps (Share, Add to Home Screen, Add), so that I do not have to know where Safari hides the option.
4. As a macOS Safari user, I want "Install app" to tell me File, then Add to Dock, so that I can install gx in Safari, which has no install button.
5. As a Chrome or Edge user (desktop or Android), I want "Install app" to open the browser's own install prompt, so that I install gx in one click.
6. As a person running the installed app, I want the "Install app" row hidden, so that the menu does not offer something already done.
7. As a person in a browser that has neither an install prompt nor steps, I want no "Install app" row, so that the menu has no entry that does nothing.
8. As a person who installs gx, I want the app icon to be the gx mascot at every size, with no clipped ears on a platform that masks icons, so that it looks right on my Home Screen, dock or launcher.
9. As a person who installs gx, I want the app named "gx" in the launcher and the window title, so that it is easy to find.
10. As a person who runs several gx servers on one host, I want each server (each port is its own origin) to install as its own app, so that they do not overwrite each other.
11. As a person who installs gx, I want the window and splash colours to match the UI's dark background, so that opening the app shows no white flash.
12. As a person who opens the installed app while the gx server is unreachable, I want to see the gx shell with the Offline strip and a short empty state ("Can't reach gx. Retrying…"), so that I know gx is the app and the server is the problem.
13. As a person who opens the installed app while offline, I want it to reconnect by itself when the server comes back, and show the sign-in screen if my login has expired meanwhile, so that I do not have to reopen it.
14. As a person who opens the installed app while offline, I want no stale data shown as if it were live, so that I do not act on an old snapshot of agents and epics. (A reload in the same tab may still show that tab's own session-storage copy under the Offline strip; a closed and reopened app has none.)
15. As a person who upgrades gx, I want the installed app to open on the new build the next time I open it with the server reachable, so that I never run an old build by accident.
16. As a person on a slow link to the server, I want the app shell to open from the cached copy after 1.5 seconds, so that a slow server does not hold the app on a blank screen.
17. As a person whose slow request for the shell eventually lands, I want it to refresh the cached copy, so that the next open is current.
18. As a person who signs in, I want the login flow, the event stream and every `/v1/` call to go to the network untouched, so that the worker cannot break sign-in or show cached data.
19. As a person whose web login has expired, I want the same sign-out behaviour as today (a 401 signs the page out), so that nothing about the installed app changes security behaviour.
20. As a person signed out, I want the login page to come from the server, never from a cached copy, so that I always see the current sign-in state.
21. As a person whose installed app seems stuck on an old build, I want a "Force refresh" row in the gear menu that clears the cache and reloads from the network, so that I can recover without removing the app.
22. As a person using "Force refresh", I want it to run at once with only a toast ("Clearing the cache and reloading…"), so that a recovery tool does not ask me to confirm.
23. As a person in a plain browser tab, I want "Force refresh" to be there too, so that I can clear a bad cache before installing.
24. As a person using the installed app, I want "Force refresh" to keep showing there, so that I can recover without a browser.
25. As a person who installs gx, I want the app to open at the app root and stay inside gx's scope, so that following links stays in the app window.
26. As a person who uses gx over an IP origin or a hostname origin, I want the service worker and install to work on both, so that the way I reach my machine does not matter.
27. As a person whose browser does not trust gx's root CA, I want gx to keep working as a normal web page, so that a blocked service worker only costs me the install, not the UI. (Chrome on a Mac without the root trusted fails to register the worker; the person trusts the root, as the existing root trust notes describe.)
28. As a person on iOS, I want the existing Home Screen behaviour (standalone detection, edge swipe, the Apple touch icon, the app title) to stay as it is, so that the PWA work does not regress it.
29. As a gx maintainer, I want the manifest, worker and icons embedded in the binary and served by the existing web server, so that there is nothing extra to deploy.
30. As a gx maintainer, I want the worker's cache and fetch rules covered by fast automated tests, so that a change cannot make it cache an authenticated response.
31. As a gx maintainer, I want a real-device check of install on iPhone, macOS Safari and desktop Chrome and Edge, so that platform behaviour the automated tests cannot see is confirmed.
32. As a person in Chrome or Edge whose browser does not trust gx's root CA, I want a one-line hint in the gear menu that links the certificate download or trust help, so that I learn why "Install app" is missing and how to fix it.
33. As a person who dismissed the browser's install prompt, I want the "Install app" row to hide until the browser offers installation again, so that the row never opens a prompt that cannot show twice.

## Implementation Decisions

### Manifest and icons

- One static web app manifest embedded in the binary. Fields: `name` and `short_name` "gx"; `id`, `start_url` and `scope` all `/`; `display` `standalone`; `theme_color` and `background_color` `#0a0d11`. `id` is `/` because each port is its own origin, so it is already unique per server.
- The index page links the manifest. The existing `apple-touch-icon` (180 px), `theme-color` and `apple-mobile-web-app-title` tags stay. No `apple-mobile-web-app-capable` tag is added.
- Icons are generated from the brand source image (`docs/brand/app-icon-1024.png`, full-bleed mascot on navy). Nothing is scaled up from the 180 px touch icon. The manifest lists `any` icons at 192 and 512 px, resized from the source, and one separate `maskable` 512 px icon whose art fills about 70% of the canvas on the source's navy (the mascot's ears reach the edges, so a mask crop would clip the full-bleed art). The implementer picks the generation method (a one-off script run by hand with the PNGs committed, or a build step) and the icon file names. Committed PNGs are preferred: the build then has no image tooling dependency.
- The manifest's `Content-Type` is `application/manifest+json`. The server sets it explicitly for the manifest file, because Go's `mime` table has no entry for `.webmanifest` and the result would depend on the host's mime database. The implementer picks the manifest file name (`.webmanifest` or `.json`); the `Content-Type` rule holds for either.

### Serving, login gate and caching headers

Facts verified in the code, which fix the design:

- Static assets are served without a login. The login gate lets any GET or HEAD not under `/v1/` through to the static handler when there is no valid web login cookie, so the manifest, `/sw.js`, icons and `index.html` all load for a signed-out page. No new public route is needed, and nothing needs a behavioural choice.
- Because those fetches need no login, it does not matter that the browser fetches the manifest without credentials by default, and that a service worker script fetch sends credentials only per its own rules. The manifest link carries no `crossorigin` attribute. A signed-in page's requests still renew the login cookie as today; the worker adds no new renewal path.
- Every `/v1/` route bypasses the worker (see Service worker). Non-public `/v1/` routes such as the snapshot and the event stream answer 401 when signed out. The public routes (`GET /v1/web/auth`, the login and setup POSTs, `POST /v1/client-errors`) are served ahead of the gate and are unchanged.
- `Cache-Control` in the static handler does not change. Files under `assets/` keep their immutable year-long header. Every other root file, including `sw.js`, the manifest and the icons, gets `no-cache` from the existing non-`assets/` rule. The index page keeps its ETag and `no-cache`.
- The service worker is served from `/sw.js` at the root, so its scope is `/` with no `Service-Worker-Allowed` header. The server must serve it unhashed at that path from the embedded build. The worker is not registered in dev (`import.meta.env.DEV`), so the dev-frontend proxy mode needs no `/sw.js` and a caching worker never fights hot reload.
- The worker script's `Content-Type` is a JavaScript type. The existing static handler already gives `.js` files that type.

### Service worker

- Registration happens from the web UI at start-up in a secure context only, and registration failure is silent: a browser that cannot register a worker (for example one that does not trust gx's root CA) runs gx as a plain web page. The implementer picks where registration lives.
- Strategy for `index.html`: network first, fetched with `cache: "no-cache"` so the HTTP cache cannot hold back a new build, with a 1.5 s timeout. After the timeout the worker answers from the cached copy. A network error (connection refused, as when the server is down) or a non-OK answer (for example the dev proxy's 502) falls back to the cached copy at once, and only a 200 answer refreshes the cached `index.html`. The slow request is not cancelled: when it lands with a 200, it refreshes the cached copy for the next open. No update prompt and no waiting-worker flow. The worker calls `skipWaiting` and `clients.claim` so a new worker takes over at once, which is what removes any need for a prompt; the verify ticket confirms this on iOS.
- Cached: `index.html`, everything under `assets/`, the manifest and the icons. `sw.js` carries a build id, so every build installs a new worker. At install the worker precaches `index.html` plus the build's asset list, and the cached `index.html` is replaced only together with the assets it references, so an offline open never loads an index whose scripts are missing. `assets/` are cache-first (they are hashed); the manifest and icons are stale-while-revalidate. Old hashed files and old caches are pruned when the new worker activates. The implementer picks how the asset list reaches the worker.
- Never cached and never answered by the worker: every `/v1/*` request (the snapshot, the event stream, the login routes and the writes), and any non-GET request. For these the worker does not call `respondWith` at all, so the request goes straight to the network and streaming, auth and cookies behave exactly as today. The worker never stores an authenticated response. The login page is the app shell itself (a signed-out page renders the sign-in screen from the same `index.html`), so it needs no special case: it comes from the shell, never from a cached `/v1/` answer.
- There is no conflict with the login flow or the event stream: both live under `/v1/` and bypass the worker. The event stream is a long-lived streaming GET and must never be intercepted or buffered.
- The worker file's layout, its cache names, its versioning scheme (for example a name derived from the build), and how Vite builds it (a plugin that emits an unhashed `sw.js`, or a second build entry) are the implementer's choices, with two constraints: the file lands at the root with no hash in its name, and the build keeps `webui/dist` as the only embedded tree (see the existing embed rules). The cache logic must be importable by vitest without a browser.

### Offline behaviour

- The sign-in check gate (`AuthGate`) mounts the app only after `GET /v1/web/auth` succeeds, and today a failed read shows "Could not reach the server." with no retry. The Offline strip lives in the app shell (`AppShell`), so that card would hide it. This changes: when the auth read fails (a network error or any non-OK answer), `AuthGate` mounts the app anyway in an "unknown sign-in" state. The app shell shows the Offline strip and the "Can't reach gx. Retrying…" empty state, and the existing connection loop (`connection.ts`) does the retrying with its own backoff. `AuthGate` adds no retry loop of its own.
- When the server comes back, the connection loop's first request answers 401 if the login has expired. The existing signed-out path then runs: `onSignedOut` makes `AuthGate` re-read `GET /v1/web/auth` and show the login or setup screen. If the login is still valid, the first live snapshot arrives and the empty state disappears. A 401 from a `/v1/` data route is the signed-out signal. `GET /v1/web/auth` is public and never answers 401, so a failed read there is always a network or server error.
- Once the app is mounted, the existing Offline strip and connection state drive "offline", as today. The worker does not detect reachability. The implementer adds no second detector.
- When the shell is open and there is no snapshot, the pages show the same short empty state ("Can't reach gx. Retrying…") in place of the page content. This empty state is new; wording is as the map settled it. It disappears when the connection loop gets its first live snapshot.
- No data is cached by the worker. The existing per-tab snapshot copy in session storage stays as it is: a reload in the same tab may show that tab's own copy under the Offline strip, and a closed and reopened app has none, which is the cold open this spec covers.
- The login page is never served from the worker's cache as a `/v1/` response. A 401 from a `/v1/` data route still signs the page out through the existing API wrapper.

### Install and Force refresh UI (approved mock, variant A)

- Both rows sit at the bottom of the gear menu, in this order, under their own separator after Tips.
- "Install app" has three behaviours, chosen by what the browser offers: (1) in Chrome, Edge and Android, when the browser has fired its install-available event, the row shows the browser's own prompt on click (the page keeps the event and replays it); the row's caption "Chrome / Edge" shows only on desktop Chrome and Edge, as in the mock, and there is no caption on Android; (2) on iOS and iPadOS, in every browser, the row opens a bottom sheet with three steps (tap Share, tap Add to Home Screen, tap Add) and a device-neutral title (the mock's "Install gx on your iPhone" is the iPhone wording; the implementer picks the neutral title); (3) on macOS Safari the sheet says File, then Add to Dock. The sheet has a Done button. The step wording per platform is an implementer's choice within those steps.
- Platform detection: iOS and iPadOS are detected with `navigator.maxTouchPoints > 1` together with the Mac platform check, because iPadOS Safari sends a Mac user agent. Only macOS Safari without touch gets the Add to Dock text. Every other browser without an install-available event gets no row.
- "Install app" is hidden when the app runs installed (standalone), detected with the existing standalone helper. After the user accepts the browser prompt, or the browser reports the app installed, the row hides without a reload. After the user dismisses the browser prompt, the row hides until the browser fires a new install-available event, because a saved event can be prompted only once.
- A browser that does not trust gx's root cannot register the worker and so shows no install prompt. When worker registration fails with a security error, the gear menu shows a one-line hint, in place of the Install row. No root-trust page is reachable from the signed-in app today: `TrustHelp` and `CertContext` exist only on the sign-in screens. So `AuthGate` also gives `CertContext` to the signed-in app, and the hint links the certificate download (`cer_path`) or reuses `TrustHelp`. The implementer picks which. The hint shows only after that failure, not in browsers that simply have no install support. The sheet text on Safari does not mention trust. (Chrome and Edge behaviour with a trusted root is checked on a real device, see Verification.)
- "Force refresh" always shows, installed or not. It asks no confirmation. While the connection is down (the Offline strip shows), it shows a "Can't reach gx" toast and does nothing: unregistering the worker and reloading would land on the browser's error page. Otherwise, on click it shows a toast "Clearing the cache and reloading…", unregisters the service worker, deletes the app shell caches, then reloads from the network (a reload that bypasses the worker and HTTP cache). It clears only the worker's caches and registration, not the login cookie and not session storage. The implementer decides whether the reload is a plain `location.reload()` after unregistering, provided the next load is served from the network.
- The menu is part of the existing gear menu on every device type; at the phone width it behaves as the existing phone menu does.

### Modules touched

- The embedded asset serving (the static handler): serves the manifest with the explicit manifest `Content-Type`; no header changes otherwise.
- The web UI build: emits the unhashed worker at the root, ships the manifest and icons from the public assets.
- The web UI shell: registration at start-up (not in dev), the install-availability store, the two gear-menu rows, the root-trust hint, the steps sheet, the toast and the offline empty state.
- The sign-in check gate (`AuthGate`): mounts the app in an unknown sign-in state when the auth read cannot reach the server, and gives `CertContext` to the signed-in app.
- The worker: new, with its cache and fetch rules in a plain module that takes `caches` and `fetch` as inputs.

## Testing Decisions

A good test checks external behaviour (what a request gets back, what the page shows, what ends up cached), not the worker's internal helpers or cache names.

Seams, highest first:

1. The Playwright fixture seam, with no new flow. The PWA steps join an existing flow on scenario `mixed`, and the signed-out step joins the existing `login` flow, as the e2e rules require (a new flow needs a scenario no existing flow starts, and render checks belong in vitest). The steps check: the manifest link and its fields, worker registration, a cold open with the server stopped (the fixture can stop its server mid-flow) in a new page of the same browser context (a new context has no worker), showing the shell, the Offline strip and the empty state; and Force refresh. Install row visibility and the steps sheet are vitest only. The suite fails when a page reloads more than a flow allows, so the flow declares the reloads it expects (Force refresh reloads once). Because requests a worker handles bypass `page.route`, and several flows rely on `page.route` and reload counting, the Playwright config and the gx-ui-check runner set `serviceWorkers: "block"` by default, and only the PWA steps allow workers. The fixture may not be on localhost and uses gx's own root, which Playwright's Chromium does not trust (ticket 07 saw Chrome refuse registration for that reason). The first implementation ticket is therefore a spike that registers a worker in the fixture under Playwright. If it fails, the PWA steps launch Chromium with `--ignore-certificate-errors` or trust the fixture root, and the result is recorded here. Update the catalog and the flow cap in step with the code.
2. Vitest, for the worker's cache and fetch logic with a faked `caches` and `fetch`: network first with timeout then cache, a network error or non-OK answer falling back to the cache at once, only a 200 refreshing the cached index, a slow request refreshing the copy, the cached index replaced only with its assets, `/v1/*` and non-GET never answered or stored, pruning of old hashed files and old caches on activate. Vitest component tests also cover the Install row's visibility rules (prompt available, a dismissed prompt, iOS and iPadOS detection including the iPad Mac user agent, macOS Safari, standalone, no support), the root-trust hint after a security-error registration failure, and `AuthGate` mounting the app after a failed read and re-reading after a 401, Force refresh doing nothing while offline, with the standalone helper and the install event faked, since Playwright cannot produce a real browser install prompt or a real Safari.
3. A Go test at the existing web server seam (a server started in a test, the way the current static-serving and login tests do): the manifest answers with `application/manifest+json`, `/sw.js` and the manifest answer 200 without a login cookie, `sw.js` carries `no-cache`, and non-public `/v1/` routes (the snapshot and the event stream) still answer 401 signed out while the public auth routes answer as today.
4. The real devices (no automation): a human-in-the-loop verify ticket (see Verification).

Prior art: the static-serving and login Go tests under the web server package; the existing `menu`, `phone` and `restart` Playwright flows for the gear menu and `stopServer`; the connection and snapshot vitest tests for the Offline strip; the reload-allowance flow for counting expected reloads.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built. Checks use the `gx-ui-check` flow runner on the fixture, plus a real-device pass.

1. Stories 1, 2, 9, 10, 11, 25, 29: the PWA steps on the existing `mixed` flow check the manifest link and fields (name, short_name, id, start_url, scope, display, colours). Server check: `curl -ks https://<web_url>/<manifest>` returns 200 and `Content-Type: application/manifest+json`. Real-device: iPhone Home Screen install opens standalone with the gx icon and name.
2. Story 3: Mock states line `a-steps-ios`. New vitest component tests for iOS and iPadOS detection (including the iPad Mac user agent) showing the Share steps.
3. Story 4: new vitest component test for the macOS Safari steps text (File, then Add to Dock). Real-device: macOS Safari Add to Dock.
4. Story 5: Mock states line `a-menu-chromium`. Real-device: desktop Chrome and Edge with gx's root CA trusted; Android Chrome best-effort.
5. Story 6: Mock states line `a-menu-installed`; the Install row vitest tests cover it with standalone faked.
6. Story 7: new vitest component test (no prompt, no steps: no row).
7. Story 8: Real-device: icon looks right on the iPhone Home Screen, the Mac Dock and a Chrome install (no clipped ears on the maskable icon). Server check: the icon files named by the manifest return 200 as PNG.
8. Stories 12, 13, 14: the PWA steps on the existing `mixed` flow open the page, wait for the worker to register, stop the fixture server (`stopServer`), open the app in a new page of the same browser context (the cold open, so no per-tab copy exists; the server stop is the flow's last step), and check the shell, the Offline strip, the empty state, and that no agent or epic data shows. Reconnect (13) is checked by a new `AuthGate` vitest test (app mounts after a failed auth read; a later 401 shows the login screen) plus the existing connection vitest tests, and a real-device check: stop and start the server with the installed app open.
9. Stories 15, 16, 17: vitest worker tests (timeout then cache, a slow request refreshing the copy, a changed build replacing the copy). Real-device: upgrade a running server to a new build and reopen the installed app.
10. Story 18: vitest worker tests (no `respondWith` for `/v1/*` and non-GET; nothing stored). The PWA steps sign in with the worker active and check the event stream still updates the page. Go test: non-public `/v1/` routes answer 401 signed out.
11. Story 19: existing `login` flow still passes with the worker allowed in its PWA step (a 401 signs the page out).
12. Story 20: the PWA step in the existing `login` flow: the sign-in screen shows with a worker registered, after a reload.
13. Stories 21, 22, 23, 24: Mock states lines `a-menu-chromium`, `a-menu-installed` and `a-refreshing`; the PWA steps click Force refresh with no confirmation and check the toast, that the old registration and a cache entry planted before the click are gone, that exactly one reload happened (the page then registers a fresh worker as at start-up), and that the row shows in a plain tab and with standalone faked (vitest for the visibility part). While the connection is down, Force refresh shows the "Can't reach gx" toast and leaves the worker and caches alone (vitest).
14. Story 26: Real-device: the iPhone and macOS Safari install work on both a hostname origin and an IP origin (ticket 07 results are the baseline).
15. Story 27: Real-device: Chrome on a Mac without gx's root trusted still shows the normal web page.
16. Story 28: existing `workspaces` flow (edge swipe, standalone) and `phone` flows still pass.
17. Story 30: `go test` for the web server package and `pnpm vitest run` for the worker tests pass.
18. Story 31: the human-in-the-loop verify ticket lists the real-device checks: iPhone Home Screen install, macOS Safari Add to Dock, desktop Chrome and Edge with gx's root CA trusted, Android Chrome best-effort. It also confirms `skipWaiting` and `clients.claim` behaviour on iOS.
19. Story 32: new vitest component test (security-error registration failure shows the hint with the root-trust link; no failure shows no hint). Real-device: Chrome on a Mac without gx's root trusted shows the hint.
20. Story 33: new vitest component test (after a dismissed prompt the row is hidden until a new install-available event).

New flows or scenarios to write: no new Playwright flow. The PWA steps join an existing `mixed` flow and the existing `login` flow; the fixture needs no new scenario. The steps fake the install-available event and standalone mode in the page only where Playwright is the seam; the Install row states are otherwise vitest. A spike ticket first proves worker registration in the fixture. The Mock states below use existing scenarios, so none is new. The root-trust hint and the offline empty state have no stored mock state: both are built from the existing gear-menu row and page empty-state styles, and the verify ticket checks them in the fixture. The vitest and Go tests above are new implementation tickets, and the Playwright `serviceWorkers: "block"` default and the gx-ui-check runner change are one more.

**Mock states** (mock file `$(gx tickets root)/pwa/mocks/06-install-and-refresh-menu.html`, variant A):

- `$(gx tickets root)/pwa/mocks/06-install-and-refresh-menu.html`, state `a-menu-chromium`, width 390, fixture scenario `mixed` (the install event faked in the page), parts `header`, `footer`, `gear-button`, `gear-menu`, `install-row`, `force-refresh-row`.
- `$(gx tickets root)/pwa/mocks/06-install-and-refresh-menu.html`, state `a-steps-ios`, width 390, fixture scenario `mixed` (Safari on iPhone faked by user agent in the page), parts `install-steps`.
- `$(gx tickets root)/pwa/mocks/06-install-and-refresh-menu.html`, state `a-menu-installed`, width 390, fixture scenario `mixed` (standalone faked), parts `header`, `footer`, `gear-button`, `gear-menu`, `force-refresh-row`.
- `$(gx tickets root)/pwa/mocks/06-install-and-refresh-menu.html`, state `a-refreshing`, width 390, fixture scenario `mixed`, parts `header`, `footer`, `gear-button`, `refresh-toast`.

## Out of Scope

- Web Push and the app icon badge: the push Notification Surface, subscriptions per device, VAPID keys, notification taps and the badge count. The person chose to skip them on 2026-11-09. Push would start as a new effort, and the closed decisions in the push tickets stay parked.
- Buttons on a notification to answer an agent without opening the app (moot without push).
- Offline data and actions queued while offline: a stale snapshot of a live dashboard is of little use, and a queue adds sync and conflict work.
- An app store listing or a native wrapper.
- An update prompt or a waiting-worker flow.
- Any change to the `Cache-Control` headers, to the login flow or to the source-IP gate.

## Further Notes

- Platform limits that this spec relies on: install buttons (the `beforeinstallprompt` event) exist only in Chromium, and a saved event can be prompted only once; iOS and macOS Safari have no install prompt and need manual steps (Share, Add to Home Screen; File, Add to Dock); iOS browsers other than Safari can add to the Home Screen through Share since iOS 16.4; iPadOS Safari sends a Mac user agent. (The research behind this lives on the unmerged branch `research/pwa-platform-limits`, which implementation tickets cannot read.)
- Device test (ticket 07, 2026-11-09): service worker registration and install worked on an iPhone (iOS 18.7, Home Screen) and macOS Safari 27.2 (Add to Dock), on both a hostname origin and an IP origin, behind gx's own root CA. The Mac Safari pass probably ran on an accepted certificate exception, so it is not confirmed on a trusted root. Chrome 156 on a Mac without the root trusted failed to register the worker (`SecurityError: An SSL certificate error occurred when fetching the script`); that says nothing about a trusted root. Chrome, Edge and Android are not confirmed, and the real-device verify ticket is where they get checked.
- Each origin (hostname or IP, each port) is its own install, with its own cookie jar on iOS; the installed app must be able to sign in itself (see `docs/webui-security-ios-cookie-loss.md`). The worker never touches cookies.
- Notes on the security setup: `docs/web-ui-security.md`, `docs/webui-security-ios-root-trust.md`.
- The mock's header shows the gx version ("v0.42" in the mock); the real version string is whatever the sidebar header already shows.
