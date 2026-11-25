# History state in installed apps (back-fwd-nav/02)

Date: 2026-11-25. Versions per MDN browser-compat-data (BCD) at that date: Safari 27.2 (macOS 27 line), Chrome 158.

## Answers

1. **Navigation API.** Yes in both hosts, on current versions.
   - Chrome 102+ (`canGoBack`, `canGoForward`, `currententrychange`). Source: [BCD api/Navigation.json](https://raw.githubusercontent.com/mdn/browser-compat-data/main/api/Navigation.json), [Chrome docs](https://developer.chrome.com/docs/web-platform/navigation-api) ("launched in Chrome 102").
   - Safari 26.2 (2025-12-12) and later, `canGoBack`, `canGoForward`, `currententrychange` all `26.2`. Source: BCD above; [WebKit features in Safari 26.2](https://webkit.org/blog/17640/webkit-features-for-safari-26-2/).
   - MDN marks it Baseline 2026, newly available (Jan 2026): [MDN Navigation API](https://developer.mozilla.org/en-US/docs/Web/API/Navigation_API).
   - Semantics: `canGoBack` is false when the current entry is index 0; `canGoForward` is false at the last index ([HTML spec, canGoBack getter](https://html.spec.whatwg.org/multipage/nav-history-apis.html#dom-navigation-cangoback); [MDN canGoBack](https://developer.mozilla.org/en-US/docs/Web/API/Navigation/canGoBack)).
   - Unconfirmed: that the Dock web app and the Chrome app window expose it exactly like a tab. No source says they differ. Both use the same web engine, so it is expected to work. Verify once on a real install.
   - Unconfirmed: whether Safari 26.2 was a full or partial implementation. A [WebKit bugzilla thread](https://bugs.webkit.org/show_bug.cgi?id=258384) notes small gaps; BCD lists the three members above as plain `26.2`. Safari 26.3 added an `AbortSignal` on `NavigateEvent` ([WebKit 26.3](https://webkit.org/blog/17798/webkit-features-for-safari-26-3/)); that does not touch the members gx needs.
   - Safari older than 26.2 has no Navigation API (BCD above). Dock web apps exist since Safari 17 / Sonoma ([WWDC23 10120](https://developer.apple.com/videos/play/wwdc2023/10120/)), so such installs can exist. They need the fallback.

2. **With react-router, and after reload.**
   - gx uses `createHashRouter` (`webui/src/routes.tsx`) on react-router ^8.5.0. In react-router's source the URL-based history does only `pushState`/`replaceState`/`popstate` ([history.ts](https://raw.githubusercontent.com/remix-run/react-router/main/packages/react-router/lib/router/history.ts)). No code uses the Navigation API. Every `navigate()` and `navigate(-1)` is therefore a normal same-document session-history change, and the Navigation API sees it (`currententrychange` fires; that is the API's purpose, per [Chrome docs](https://developer.chrome.com/docs/web-platform/navigation-api)).
   - So `navigation.canGoBack` and `canGoForward` follow router moves with no extra bookkeeping. Inference from spec and source; not tested in an installed window.
   - After a full reload the document's entry list is rebuilt from the session history, so `canGoBack`/`canGoForward` stay true if entries exist. Unconfirmed in a source I could fetch; MDN's `history.state` page does not mention reload. Test it.
   - One known blind spot: the entry list holds entries of this site. If the app window's first gx entry follows an off-site page, `canGoBack` is false even though the window could go back. For a button that only walks gx screens, false is the right answer. Unconfirmed in the sources fetched ([MDN entries()](https://developer.mozilla.org/en-US/docs/Web/API/Navigation/entries) does not say).

3. **Fallback without the Navigation API.**
   - react-router already stores a position counter at `history.state.idx`. It sets `idx` to 0 on first load, `idx + 1` on push, keeps it on replace, and reads it back on `popstate` ([history.ts](https://raw.githubusercontent.com/remix-run/react-router/main/packages/react-router/lib/router/history.ts), `getUrlBasedHistory`). `history.state` belongs to the entry, so `idx` survives reload ([MDN History.state](https://developer.mozilla.org/en-US/docs/Web/API/History/state): "state at the top of the history stack"; reload survival itself not stated there).
   - `canGoBack = idx > 0`. This is exact for the gx stack.
   - `canGoForward` is not derivable from `idx`: the browser gives no length of the forward part (`history.length` counts the whole stack and has no current position). Keep a `maxIdx`: on a push set `maxIdx = idx` (a push drops forward entries); on a pop leave it; `canGoForward = idx < maxIdx`. Store `maxIdx` in `sessionStorage` so a reload keeps it. Listen to the router's location changes (`useLocation` plus the `navigationType`).
   - What goes wrong with this option:
     - It relies on react-router's private `idx` key. It is not documented API; read it through a tested wrapper, and fall back to own counter in `history.state` if it ever disappears.
     - Another script calling `pushState` directly resets `idx` (react-router's own comment warns of this).
     - `maxIdx` in `sessionStorage` is per window and lost on a new window; the new window then shows forward as unavailable (safe side).
     - Entries added outside the router (a page loaded from a link in another origin) break the count.
   - Recommended: use `navigation.canGoBack/canGoForward` when `"navigation" in window`, subscribe to `currententrychange`; use the `idx`/`maxIdx` fallback otherwise. Keep both behind one hook so the buttons do not know which source answers. Do not simply fake it with always-enabled buttons: that violates "grey out when nowhere to go".

4. **Do the windows handle back and forward keys themselves?**
   - Safari web app: yes. Apple lists Command-[ (back) and Command-] (forward) for web apps ([Keyboard shortcuts and gestures for web apps on Mac](https://support.apple.com/guide/safari/ibrw3ceda9e7/mac)). Command-Left, Command-Right and swipe are not listed on that page: unconfirmed.
   - Safari web app toolbar: the default web app has a toolbar with back and forward buttons and a "Show navigation controls" setting ([Use Safari web apps on Mac](https://support.apple.com/en-us/104996)). But with `display: standalone` in the manifest, "on macOS, the web app will not have a toolbar" ([WWDC23 session 10120](https://developer.apple.com/videos/play/wwdc2023/10120/)). gx's manifest says `"display": "standalone"` (`webui/public/manifest.webmanifest`), so the Dock app has no buttons. That matches the ticket. Note: the shortcuts may still work with no toolbar; unconfirmed.
   - Chrome: the Mac Chrome shortcuts are Cmd+[ or Cmd+Left for back and Cmd+] or Cmd+Right for forward, "for the current tab" ([Chrome keyboard shortcuts](https://support.google.com/chrome/answer/157179)). That page does not say whether an installed-app window gets them. Unconfirmed. Chrome only shows back and reload buttons in app windows for `minimal-ui`, not `standalone` ([Chromium commit](https://chromium.googlesource.com/chromium/src/+/fc7a93e2f74b44b04384ee78403a57f72e5f1730%5E), [Edge/Microsoft forum](https://techcommunity.microsoft.com/t5/discussions/what-happened-to-back-button-and-refresh-button-on-youtube-pwa/m-p/1665629): secondary sources).
   - Two-finger swipe: no primary source for either host. Unconfirmed.
   - Consequence: gx must not bind Cmd+[, Cmd+], Cmd+Left, Cmd+Right itself. Our Leader chords (`,` and `.`) do not collide. If the host already handles the keys, they call the same session history, so the buttons stay correct either way, because the state comes from the history and not from the clicks.

5. **`(display-mode: standalone)`.**
   - Chrome: yes for installed apps with a `standalone` manifest; BCD has `display-mode` and `standalone` from Chrome 42 and Safari 13 ([BCD media.json](https://raw.githubusercontent.com/mdn/browser-compat-data/main/css/at-rules/media.json), [MDN display-mode](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@media/display-mode)). MDN says the mode is "identified by the value set in the manifest" but a browser may not honor it.
   - Safari Dock app: it matched `browser` in the first Sonoma seeds (WebKit bug 257806). It was fixed in the host app by Sonoma beta 3 ([WebKit bug 257806](https://bugs.webkit.org/show_bug.cgi?id=257806)). The bug page shows no direct re-test. Unconfirmed in practice on Safari 27: check once. Apple says a manifest with `display: standalone` yields a no-toolbar web app on macOS ([WWDC23 10120](https://developer.apple.com/videos/play/wwdc2023/10120/)).
   - `isStandalone()` in `webui/src/platform.ts` checks `navigator.standalone === true` or the media query. If Safari reports the media query, it works; if not, `navigator.standalone` on macOS is a separate bug (WebKit 257776, mentioned in the 257806 thread) and not verified.

## What to verify by hand (one 10-minute check on a real install)

- In Safari's Dock app and in the Chrome app: log `matchMedia("(display-mode: standalone)").matches` and `"navigation" in window`.
- Push two routes, reload, then check `navigation.canGoBack`, `canGoForward` and `history.state.idx`.
- Press Cmd+[, Cmd+], Cmd+Left, Cmd+Right and swipe in both windows.
