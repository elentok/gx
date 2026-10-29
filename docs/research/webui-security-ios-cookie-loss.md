# iOS cookie loss, 401 on refresh, blank page on resume (gx:webui-security/01)

Why the gx home-screen app on iPhone and iPad loses its `gx_web` cookie, why a refresh sometimes
shows the 401 text, and why the page goes blank when you switch back to gx. Also whether a PWA
(manifest plus service worker) fixes any of it.

Sources are the gx code at `c78c67e7`, WebKit source at `main` (`c0a5af9c`), the WebKit blog, WebKit
Bugzilla, Apple Support, and the cookie spec draft. Each section marks what is **verified** (read
in a primary source or reproduced) and what is **inferred**.

## Summary

| Symptom | Most likely cause | Confidence |
|---|---|---|
| Refresh shows the 401 text (Safari) | `SameSite=Strict`. A page reached by a link from another site loads without the cookie, and a reload of that page keeps it cross-site, so every reload is a 401. Reproduced in WebKit. | High |
| Home-screen app "loses" its cookie, only remove and re-add fixes it | The app has its own cookie jar and no way to sign in from inside it. Any 401 (a dropped cookie on launch, a rotated token, a cross-site entry) leaves it stuck on a plain-text page with no reload button and no place to paste a token link. Re-adding copies Safari's cookie again. | Medium (the "stuck" part is high; the trigger is not proven) |
| Blank page on switch back | Not proven. gx has two defects that produce a blank page: a missing `/assets/*.js` returns `index.html` as `text/html` (reproduced), and `index.html` has no cache headers. Other candidates are a killed web process with a failed reload, and a reload while the VPN is still reconnecting. | Low |

A PWA is not part of the fix. On iOS 26 a manifest is not needed to open as a web app, and neither
a manifest nor a service worker changes how cookies are stored or sent. The fixes are in login,
the cookie attributes, and cache headers.

## 1. How iOS stores and deletes the cookie

**Verified.**

- A home-screen web app does not share storage with Safari. WebKit calls this "by design": Home
  Screen apps are isolated and share no state with the browser
  ([WebKit bug 181849](https://bugs.webkit.org/show_bug.cgi?id=181849)).
- On the Mac, Safari copies the site's cookies into a Dock web app when you add it, and copies no
  other storage ([WebKit features in Safari 17.0](https://webkit.org/blog/14445/webkit-features-in-safari-17-0/)).
  Apple does not document the same for iOS.
- For the main domain of a Home Screen web app, only two kinds of cookie are capped to 7 days:
  cookies created in JavaScript and cookies set in third-party CNAME-cloaked responses. Server-set
  cookies are not capped. John Wilander (WebKit) in
  [bug 237350](https://bugs.webkit.org/show_bug.cgi?id=237350), comment 1. `gx_web` is server-set
  and `HttpOnly`, so ITP's 7-day cap does not apply.
- The 7-day cap on script-writable storage (localStorage, IndexedDB, sessionStorage, service worker
  registrations) counts days of use. A home-screen app has its own counter, and WebKit does "not
  expect the first-party in such a web application to have its website data deleted"
  ([Full third-party cookie blocking and more](https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/)).
- WebKit has an open bug where a Home Screen web app sends wrong or missing cookies on its first
  requests after a relaunch, on iOS 17.2 to 17.4.1 and reported again on iOS 18.1. One reporter saw
  about 30% of launches with no cookies on the first requests. Apple reproduced it but found no root
  cause ([bug 272325](https://bugs.webkit.org/show_bug.cgi?id=272325), status NEW). A related bug
  drops `Lax`/`Strict` cookies from some requests in Safari 16.4 and later
  ([bug 255524](https://bugs.webkit.org/show_bug.cgi?id=255524)).
- The cookie has no `Domain`, so it is host-only: the browser sends it only to the host that set it
  ([draft-ietf-httpbis-rfc6265bis](https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-rfc6265bis) §4.1.2.3, §5.7 step 10).
  `gertsel.elentok.com`, the LAN IP and the tailnet IP are three separate cookie jars.
- The missing `Secure` flag and plain HTTP do not cause deletion. Nothing found in WebKit's rules
  deletes a server-set cookie for being non-`Secure`. (`Secure` only limits where the cookie is sent,
  draft §4.1.2.5.)

**Inferred.**

- Re-adding the app fixes it, so iOS must copy Safari's `gx_web` into the new app at add time, as
  the Mac does. Apple does not say so for iOS.
- The cookie is probably not deleted at all. More likely the app gets one 401 and cannot recover.
  The 401 is `text/plain`, a standalone app has no reload button or address bar, and a token link
  tapped elsewhere opens Safari, not the app. The only way to get a cookie into the app is to add it
  again. The trigger for the first 401 could be bug 272325, a rotated token
  (`gx server web-token --rotate` or a new state dir), or a different host.

## 2. Start URL and cold start

**Verified in code.** The sign-in answers `303` to `/` (`server/webauth.go`), and the app uses a hash
router (`createHashRouter` in `webui/src/routes.tsx`). So when you add the app after signing in, the
page URL is `/` or `/#/<route>`. It never holds `?gx_token=`. `index.html` has no manifest.

**Verified in WebKit source.** A navigation with no initiating document (a launch, a typed URL, a
bookmark) is marked same-site (`FrameLoader::addSameSiteInfoToRequestIfNeeded`, `FrameLoader.cpp`
~3893), so a cold start sends the `Strict` cookie.

**Inferred.** Without a manifest, iOS uses the URL of the page you added as the start URL, and a
cold start loads it fresh. With a working cookie the cold start is fine; without one it shows the
plain 401 text and cannot recover (section 1).

## 3. Why a refresh shows the 401 text

**Verified.**

- The spec says a reload is same-site only "if the reloaded document was originally navigated to
  via a same-site request" (rfc6265bis §5.2). `Strict` cookies are not sent on top-level navigations
  "triggered from a cross-site document context" (§5.6.7.1).
- WebKit's `FrameLoader::reload` copies the old request, including its same-site flag, and
  `addSameSiteInfoToRequestIfNeeded` returns early when the flag is already set
  (`FrameLoader.cpp` ~2370 and ~3893). A redirect keeps the request's flag too
  (`DocumentLoader::willSendRequest`, `DocumentLoader.cpp` ~772).
- Reproduced in Playwright's WebKit (revision 2306, desktop WebKit, not iOS) against a small server
  that copies gx's sign-in (Strict, HttpOnly cookie, `303` to `/`). The cross-site page was served
  from `localhost`, gx from `127.0.0.1`:

  | Step | Page shows | `Cookie` header |
  |---|---|---|
  | Click the sign-in link on a cross-site page | 401 | none (the cookie was set, but the `303` follow-up is cross-site) |
  | Reload | 401 | none |
  | Click a link to `/` on a cross-site page | 401 | none |
  | Reload | 401 | none |
  | Type the URL | signed in | `gx_web=...` |
  | Reload | signed in | `gx_web=...` |

**Inferred.** Any link to gx from another site starts this: a link in a web chat, a GitHub page, or
a gx page on another host (the IP and `gertsel.elentok.com` are different sites). Once a tab is in
this state, reload never fixes it; only typing the URL does. The "missing token" text came from the
original spec (`6afb1b5f`); today's text is "401: open the link printed by `gx server status`".

## 4. Why the page goes blank on switch back

What "blank" means here: `index.html` paints the dark background inline before any JS runs. The
top bar renders without a snapshot (`ConnectedTopBar.tsx`), and a render error inside a route shows
React Router's default error page. So a fully empty dark page means the JS bundle never mounted, or
the whole React root crashed (an error outside the router, for example in a toast, unmounts it).

**Verified in gx (curl against a `gxfixture empty` server).**

- `index.html` and assets come back with no `Cache-Control`, no `ETag` and no `Last-Modified`. The
  embed FS has no mod times, so `http.FileServerFS` sends none.
- A request for a missing asset, `/assets/index-OLDHASH.js`, returns `200`, `text/html`, the body of
  `index.html` (`serveAsset` in `server/web.go` falls back to `index.html` for any missing file).
  A module script served as `text/html` does not run, so the page stays blank.
- The fallback is not needed for routing: the router uses the hash, so only `/` needs `index.html`.

**Verified in WebKit source.**

- With no `max-age`, `Expires` or `Last-Modified`, WebKit's freshness lifetime is 0
  (`computeFreshnessLifetimeForHTTPFamily`, `CacheValidation.cpp` ~135-160), so normal loads
  revalidate.
- Back and forward loads on Apple platforms use stale cache (`ReturnCacheDataElseLoad`,
  `FrameLoader.cpp` ~5030), and subresources follow the main document's policy (~3720).

**Inferred.**

- After a gx upgrade, a back/forward or restored load can use an old cached `index.html`. If its old
  `assets/index-<hash>.js` is no longer cached, gx answers with HTML and the page is blank. This
  needs an upgrade between visits, so it is a rare cause, but the fallback bug is real.
- iOS kills background web processes under memory pressure, and Safari reloads the page when you
  return. If the tailnet VPN has not reconnected yet, that reload fails. A WebKit report describes a
  WKWebView left blank after its process was killed in the background
  ([bug 148685](https://bugs.webkit.org/show_bug.cgi?id=148685), old). This fits "blank on
  switch back" well but is unproven for gx.
- The event stream does not cause a blank page. If iOS freezes the page, the fetch stream can die
  while `live` stays true, and `onVisibility` in `webui/src/connection.ts` reconnects only when
  `!live`. The page then shows stale data until the 45 s watchdog fires. That is stale, not blank.
- To pin the cause down, the page needs to report its own failures: a `window.onerror` and
  `unhandledrejection` handler that posts to the server, and a log line when a load of
  `/assets/*` misses.

## 5. What a manifest and a service worker give on iOS

**Verified.**

- Since Safari 26 (iOS 26), "every website added to the Home Screen opens as a web app", manifest
  or not. A manifest still adds what it defines, such as icons and `display`
  ([WebKit, Safari 26 beta](https://webkit.org/blog/16993/news-from-wwdc25-web-technology-coming-this-fall-in-safari-26-beta/)).
- WebKit refuses to register a service worker whose script URL is not potentially trustworthy
  (`SWServerJobQueue.cpp` ~331, "Script URL is not potentially trustworthy"). Plain HTTP on a LAN IP
  fails this. The register path checks the URL, origin, CSP and MIME type; it has no separate
  certificate check (`ServiceWorkerContainer.cpp`, `ServiceWorkerJob.cpp`).
- A service worker can break `SameSite` cookies in WebKit
  ([bug 232440](https://bugs.webkit.org/show_bug.cgi?id=232440), open: a `Lax` cookie not sent on
  navigations the worker fetched).

**Inferred.**

- Under a user-trusted gx root CA, TLS validates normally, the page is a secure context, and a
  service worker should register. I found no source that tests this exact case.
- A manifest would give a fixed `start_url` (`/`), a scope and an icon list. That is tidy but fixes
  none of the three symptoms.
- A service worker could show an offline shell instead of a blank page when the network is down on
  resume. It would also add a cache that can serve old code, the same class of bug as section 4,
  and bug 232440. Leave it out until the blank page is shown to be a network failure.

## 6. What the fix needs

**Login.**

- Sign-in must work inside the home-screen app. Any unauthenticated request for `/` must return an
  HTML login page (password plus TOTP) at the same URL, never plain text. Then a lost cookie costs
  one login, not a remove and re-add.
- Drop the `?gx_token=` link as the way in. A link from another app or site is exactly the
  cross-site entry that breaks `Strict` cookies.
- When `/v1/snapshot` or `/v1/events` returns 401, the page should go to the login page instead of
  retrying forever behind the Offline strip.

**Session cookie.**

- `SameSite=Lax`, not `Strict`. `Lax` is sent on top-level GET navigations from other sites and on
  reloads, and is still not sent on cross-site POSTs. gx's write guard (Origin must match Host, JSON
  content type, `checkWriteRequest`) already blocks cross-site writes.
- `__Host-` name prefix, `Secure`, `HttpOnly`, `Path=/`, no `Domain`. Set it from the server only, so
  ITP's 7-day cap on script-set cookies does not apply.
- Persistent (`Max-Age`), so it survives app restarts. Renew it on use (sliding expiry) so an active
  device never hits the limit. With server-side revocation the cookie lifetime can be long (for
  example 90 days); the server decides validity.

**Sessions on the server.**

- One session per device, stored on disk (in the state dir), so a server restart or upgrade signs
  no one out.
- The cookie carries a random session id, not a value derived from a shared secret, so revoking one
  device leaves the others signed in.
- Do not rely on iOS keeping the cookie. Given bug 272325, a launch can arrive without it once; the
  login page must make that cheap.

**HTTPS and origin.**

- Pick one canonical host (`gertsel.elentok.com`) and put the app on it. The IPs are separate cookie
  jars and separate sites. If the IPs stay reachable, redirect them to the canonical host, and list
  them in the certificate's SANs so the redirect itself does not fail.
- iOS trusts a manually installed root only after you turn it on in Settings > General > About >
  Certificate Trust Settings ([Apple](https://support.apple.com/en-us/102390)).
- The leaf certificate must have the host in the SAN (the CommonName is ignored), the `serverAuth`
  EKU, at least 2048 bits if the key is RSA, a SHA-2 signature, and a validity of at most
  825 days ([Apple](https://support.apple.com/en-us/103769)). The 398-day limit does not apply to
  user-added roots ([Apple](https://support.apple.com/en-us/102028)).
- HTTPS also makes the page a secure context, so `crypto.randomUUID` and service workers become
  available (see the comment on `randomId` in `connection.ts`).

**Serving the app (independent of security, fix now).**

- `index.html`: `Cache-Control: no-cache` plus an `ETag`, so every load revalidates.
- `/assets/*`: `Cache-Control: public, max-age=31536000, immutable`. The names hold content hashes.
- A missing `/assets/*` file returns `404`, not `index.html`. Fall back to `index.html` only for `/`
  (the router uses the hash).
- Add a root error boundary that shows the error and a reload button, and report client errors to
  the server, so the next blank page leaves evidence.
