# Do sandboxed hosted pages load their relative assets?

Ticket: `gx:webui-host-files/02`. Probe date 2026-11-14.

## Answer

- No. With `Content-Security-Policy: sandbox allow-scripts` and a `SameSite=Lax` login cookie, the page's own sub-requests (CSS, images, scripts, `fetch`) do not carry the cookie. They get 401.
- Exception: WebKit sends the cookie for asset tags (CSS, img, classic script) when the page is inside a sandboxed iframe. Not as a top-level page. Do not rely on it. It also contradicts the spec.
- Scripts in the sandboxed page cannot read any gx `/v1/` response. The request is cross-origin from origin `null`, and gx sends no CORS headers.
- Writes fail the Origin check. They send `Origin: null`. In WebKit iframes the cookie goes along with `credentials: 'include'` writes, so the Origin check is the only thing that stops them. It did (403).
- Fix: put a signed, short-lived token in the URL path, scoped to one folder, and send `Access-Control-Allow-Origin: *` on hosted files. Probe confirms every asset type and `fetch` of sibling files then works in both engines and both modes, with no cookie.

## Spec basis

- HTML, sandboxing (section 7.1.5): the sandboxed origin browsing context flag "forces content into an opaque origin". A CSP `sandbox` directive gives the document the same flags (CSP-derived sandboxing flags). Without `allow-same-origin` the flag stays set. https://html.spec.whatwg.org/multipage/browsers.html
- RFC 6265bis section 5.2.1: a top-level document's "site for cookies" is its own origin. For an embedded document it is the top origin only if every ancestor and the document are same-site, "Otherwise ... an opaque origin". An opaque origin is same-site with nothing, so every request from a sandboxed document is cross-site. https://httpwg.org/http-extensions/draft-ietf-httpbis-rfc6265bis.html
- RFC 6265bis, SameSite=Lax (sections 4.1.2.7, 5.6.7.1): Lax cookies go only on same-site requests and cross-site top-level navigations. Sub-resource loads and `fetch` are not navigations. So no cookie.
- Fetch: a sandboxed document's request origin is opaque, so its request to gx is cross-origin. The `Origin` header serializes as `null`. `fetch()` is mode `cors`. The response is readable only with matching CORS headers. `<script type=module>` is also CORS mode. `<link>`, `<img>`, classic `<script>` are no-cors and work without CORS headers.
- The page document itself is a navigation (top-level or iframe), so it still carries the cookie (`Sec-Fetch-Site: none` or `same-origin`). Only its sub-requests lose it.

## Probe

Files in `docs/research/webui-host-files-sandboxed-assets/`: `server.mjs` (HTTPS server, logs cookie/Origin/Sec-Fetch per request), `probe.mjs` (Playwright, 2 engines x 4 modes), `results.json` (raw output).

- Cookie: `__Host-probe_login=1; Path=/; Secure; HttpOnly; SameSite=Lax`. Playwright reports it as secure, httpOnly, Lax.
- Server returns 401 for any request without the cookie (or a token in the path).
- Page: `style.css`, `img.png`, `app.js` (classic), `mod.js` (module), and fetches of `data.json` (relative), `/v1/api.json`, POST `/v1/write` (Origin check like gx), each with and without `credentials: 'include'`.
- Modes: `top` and `iframe` (parent from same origin, `<iframe sandbox="allow-scripts">`), plus `top-token` and `iframe-token` where the page is at `/t/abc/v1/files/d/page.html` and the server accepts the path token and adds `Access-Control-Allow-Origin: *` on those responses.
- Engines: Playwright 1.65.0-alpha Chromium 157.0.8103.15, WebKit 27.2 (macOS). Not real iOS Safari. Playwright WebKit is the same engine family, so expect the same cookie rules, but this was not run on a device.
- In both engines `self.origin` is `"null"`, `localStorage` and `document.cookie` throw SecurityError.

### Results (cookie sent? / outcome)

| Request | Chromium top | Chromium iframe | WebKit top | WebKit iframe |
|---|---|---|---|---|
| page navigation | cookie | cookie | cookie | cookie |
| CSS, img, classic script | no cookie, 401, not applied | same | no cookie, 401 | **cookie, 200, works** |
| module script | no cookie, 401 | same | no cookie, 401 | no cookie, 401 |
| `fetch` relative / `/v1/api.json` | no cookie, 401, TypeError | same | no cookie, 401 | no cookie, 401 |
| `fetch(credentials: include)` GET `/v1/api.json` | no cookie | no cookie | no cookie | **cookie, server 200, script gets TypeError (CORS)** |
| POST `/v1/write` | no cookie | no cookie | no cookie | no cookie |
| POST include | no cookie, 401 | no cookie, 401 | no cookie, 401 | **cookie, Origin: null, 403 (Origin check)** |

All requests above carried `Origin: null` when they were `cors` mode, and `Sec-Fetch-Site: cross-site`.

Token modes (`top-token`, `iframe-token`, both engines): style, image, classic and module script, and `fetch('data.json')` all succeed (200, body readable) with no cookie. `/v1/api.json` and `/v1/write` (outside the token path) get no cookie in Chromium and WebKit top, and stay unreadable in all four. WebKit iframe-token still sends the cookie with `credentials: 'include'` (GET 200, POST 403) but the script cannot read the reply.

Findings to carry into the spec:
- A cookie-only design breaks every hosted page that has an external file. Inline-only pages work.
- Do not rely on the WebKit iframe behavior. It sends the cookie, but it is inconsistent (module scripts and `fetch` still lack it) and differs from the spec and from Chromium.
- WebKit iframe: a hosted page can fire blind cookie-carrying GETs at `/v1/` (it cannot read replies). So `/v1/` GET handlers must not have side effects, and the Origin check on writes must stay strict, including rejecting `Origin: null`. Note the probe server's check (host equals Host, parse error means reject) rejects `null`; gx's check in `server/web/assets.go:28-48` should be confirmed to do the same.
- The `Sec-Fetch-Site: cross-site` plus `Origin: null` signature is easy to reject server-side on `/v1/` API paths as extra defense. Not tested in gx.

## If the cookie is not sent: ways to authorize asset requests

1. Signed path token scoped to a folder (recommended). URL `/v1/files/t/<token>/<path>`, token = HMAC over (folder root, expiry), minted when the logged-in user opens the file. Relative URLs inherit the token for free. Probe confirms it works in all cases. Cost: a minter endpoint (cookie-gated) and a verifier; the URL carries a credential, so set `Referrer-Policy: no-referrer`, keep expiry short (minutes) with the main page re-minting on reload, and keep it out of logs. Risk: anyone with the URL reads that folder until expiry. Scope it to the folder, not the whole root. Add `Access-Control-Allow-Origin: *` only on token responses (never on cookie-gated `/v1/` APIs) so page `fetch` of siblings works.
2. Keep cookie, add `allow-same-origin`. Cookie then flows, but page scripts can read every `/v1/` response and make writes with a valid Origin. This breaks the decided threat model. Reject.
3. Separate origin (second port or host) with its own cookie. Cookie flows only if that origin is same-site and cookie `SameSite` allows it; still needs its own login. Ruled out of scope by the map, and adds a certificate and login cost.
4. Rewrite the HTML server-side to inline assets as `data:` URLs. No token, but breaks large files, `fetch`, module scripts, and nested relative paths. Fragile. Reject.
5. Rely on WebKit's iframe behavior. Not portable, contradicts the spec. Reject.

Recommendation: option 1. Mint a folder-scoped, short-lived HMAC token only after the cookie check; serve under `/v1/` still; token paths skip cookie auth but are read-only and limited to that folder.

## Open points

- Real iOS Safari was not tested. Run the probe on a device if the spec depends on it.
- Token lifetime and whether the token is reused across reloads is a design choice for ticket 03.
