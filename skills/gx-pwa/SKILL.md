---
name: gx-pwa
description: Set up or debug a LAN web app that people install on an iPhone or iPad Home Screen. Use for a missing or letter-tile Add to Home Screen icon, apple-touch-icon and manifest setup, a self-issued root certificate and its iOS trust profile, or "can't reach the server" and TLS errors on a phone.
---

This is the setup that worked for a Go server with a Vite web app (minitube, copied from gx). Check each section in order when the Home Screen icon is a letter tile.

## Diagnose first

A letter tile (the first letter of the title on the theme color) means iOS got no usable icon. Decide between three causes before touching the icon files:

1. Add temporary logging of every non-API request (path, Host, User-Agent). Open the page on the phone, open the share sheet, and read the log. Remove the logging afterwards and never commit it.
2. If the log shows the phone fetching the icon with 200, the file is fine. Look at trust (below).
3. Serve the built web directory over plain HTTP on another port (`python3 -m http.server 8099 --bind 0.0.0.0` from `dist`), then run Add to Home Screen from `http://<mac-ip>:8099/`. The icon appears over HTTP and not over HTTPS: trust is the cause, not the icon. Stop the server afterwards.

The server log signature of a trust failure: `TLS handshake error ... EOF` bursts about 4 seconds after each share-sheet open (one connection per icon candidate), or `remote error: tls: unknown certificate` from the phone's address. A device that fully trusts the root never sends that alert.

## How iOS finds the icon

- Safari does not fetch the icon. A separate `NetworkingExtension` process does, and a Home Screen icon builder process follows. Both use the system trust store. A Safari "visit this website" exception does not cover them.
- Candidates, all fetched on every share-sheet open: every `<link rel="apple-touch-icon">` and `<link rel="icon">` in the page, plus the default probes `apple-touch-icon.png`, `apple-touch-icon-precomposed.png` and `favicon.ico` at the site root.
- Add `<link rel="apple-touch-icon" sizes="180x180" href="<name>-touch-icon-180.png">`. A dedicated, explicitly sized file name is what gx used.
- Ship a copy named `apple-touch-icon.png` at the site root too, so the default probe finds a file (gx does).
- A missing file must return 404. A single-page-app fallback that answers a probe with `index.html` makes iOS drop the icon. Return 404 for any missing path that has a file extension and for the API prefix; fall back to `index.html` only for extensionless routes.
- iOS caches a failed icon fetch per URL. After fixing a failure, give the file a new name, or the phone keeps the failure.
- Icon file: 180x180 PNG, RGB, no alpha, not interlaced. Extra metadata chunks (sRGB, eXIf) were not the problem, but stripping them to IHDR, IDAT and IEND is harmless.
- Cache headers: `no-cache` for root files (icons, manifest, `sw.js`), `immutable` only for content-hashed files under `assets/`.
- The manifest icons were never requested in our logs. The apple-touch-icon is what the Add to Home Screen dialog showed.

## Manifest

Keep the `<link rel="manifest">` in the page. Serve it as `application/manifest+json` (Go's mime table has no `.webmanifest`). Fields gx uses:

```json
{
  "id": "/",
  "name": "App",
  "short_name": "App",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#0b7f8c",
  "theme_color": "#0b7f8c",
  "icons": [
    { "src": "icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any" },
    { "src": "icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any" },
    { "src": "icon-512-maskable.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable" }
  ]
}
```

The icon paths above are root-relative; write them with a leading slash. Make the maskable icon with extra padding, because masking crops the edges. Also set `<meta name="apple-mobile-web-app-title">`, `<meta name="theme-color">` and `viewport-fit=cover`.

## Certificate

A phone needs HTTPS for the service worker and OPFS, and a LAN host has no public certificate, so the server issues its own.

- Root: ECDSA P-256, CA with pathlen 0, 10 years, critical name constraints. Permit one DNS domain and the private ranges (10/8, 172.16/12, 192.168/16, 100.64/10, 127/8, ::1). The name constraints did not cause the icon failure; gx uses them and works.
- Leaf: ECDSA P-256, 1 year, serverAuth, backdated 1 hour. SANs are the host name and the Mac's own IPs, so a phone that cannot resolve the name can still open the server by address. Renew the leaf at start and on a handshake when it is near expiry.
- Give the root and leaf a product name ("Minitube root"). That name is what the profile list and the trust list show the user, so a wrong name confuses the install.
- Public routes `ca.mobileconfig` at the site root (an iOS profile with a `com.apple.security.root` payload) and `ca.crt`, plus a link in the UI. Exclude the `ca.` files from the service worker cache: a cached profile navigation replays as a download whenever the network fails.
- Accept an IP literal as Host. DNS rebinding needs a domain name in Host, so IPs are safe. Refuse every other name with 403.
- Regenerating the root changes its fingerprint. Every device must install the new profile, enable trust again, and remove the old profile. Compare the profile payload hash with the SHA-256 of `root.pem` to confirm the served profile is current.

## Trust checklist per device

Installing the profile does not trust it. The second switch is the one that was missed.

iPhone and iPad:

- [ ] Open `https://<host>:<port>/ca.mobileconfig` in Safari and tap Allow.
- [ ] Settings → General → VPN & Device Management → the downloaded profile → Install.
- [ ] Settings → General → About → Certificate Trust Settings → switch on full trust for the root. This is a separate step from the install.
- [ ] Remove any older root profile (for example a previous name) and its trust switch.
- [ ] Load the app in a Private tab. A certificate warning means trust is still off.
- [ ] Delete earlier Home Screen shortcuts, then retry Add to Home Screen from both the host name and the IP origin.
- [ ] If Safari says the server cannot be found right after the install, restart the phone (it fixed this once).

Mac:

- [ ] Add the root to the keychain and trust it for SSL (Keychain Access, or `security add-trusted-cert`), so Mac browsers accept it.

Server:

- [ ] `openssl s_client -connect localhost:<port> -servername <host>` shows the expected issuer.
- [ ] The server log has no `unknown certificate` or EOF bursts after a share-sheet open.

## Other gotchas

- A service worker that caches by pathname can store a non-page under the shell key. Store a navigation only when the response is `text/html`, and bump the cache name when you fix it.
- Vite empties `dist` and removes `.gitkeep`. The Go embed needs that file to compile, so restore it after each web build.
- For a reference setup, read gx: `server/webtls` (root, leaf, listener, host check) and `webui/public/manifest.webmanifest`.
