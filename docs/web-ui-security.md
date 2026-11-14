# Web UI security

## How the web UI is protected

The web UI listener serves HTTPS only. A plain-HTTP request on the same port is redirected to
`https://`.

- **Certificates.** On the first start with the listener on, gx makes its own root certificate and a
  server certificate signed by it. The root is offered as an iOS profile and a Mac `.cer` file. Its
  SHA-256 fingerprint shows on the setup, login and Settings pages and in `gx server status`.
- **Password plus TOTP.** One person logs in with a password and a code from an authenticator app.
  Set `server.web.totp` to `false` for password only. It defaults to `true`. Turned back on with no
  saved secret, the next login adds the authenticator after the password.
- **Web logins.** Each successful login on a device is its own web login. It ends after 90 days
  without use. The Settings page lists the web logins, renames them and signs them out one at a time.
  It also changes the password and resets the authenticator. Both of those sign out every other web
  login.
- **Source-IP rule.** The listener closes connections from public source IPs before TLS. It accepts
  the private LAN ranges, `100.64.0.0/10` (Tailscale), loopback, link-local and IPv6 ULA.
  `X-Forwarded-For` is ignored.
- **Host and Origin checks.** The listener checks the `Host` header against `server.web.allowed-hosts`
  and checks `Origin` and `Content-Type` on writes.
- **Rate limit.** Wrong passwords, codes and Setup codes count toward a login rate limit.
- **Headers on every response.** Every response carries `Content-Security-Policy: frame-ancestors
  'none'`, `Referrer-Policy: same-origin` and `X-Content-Type-Options: nosniff`. Hosted-file responses
  replace the first two (see "Hosted files").
- **`connect-src` on hosted files.** Every file response carries `connect-src https://<request Host>`.
  A hosted page cannot `fetch`, use XHR, open a WebSocket or EventSource, or send a beacon to another
  site. Images, styles, scripts and navigation can still carry data out, and a CDN `<script src>` still
  loads. A page that calls an outside API stops working.

## First-run setup

1. Run `gx server status` or `gx server web-url --qr`. Both print a one-time Setup code.
2. Open the URL. The page asks for the Setup code, then a password.
3. With TOTP on, add gx to an authenticator app (QR code, "Add to authenticator app" link or key)
   and enter one code. That device is signed in.

The TOTP secret shows only on the setup page, never in the CLI.

On a new device, trust the root certificate first (the iOS profile or the Mac `.cer` from the
login page), then sign in.

## Recovery from the CLI

A local shell is trusted. These commands need the server running and the web UI on.

- `gx server web-logins` lists web logins with id, name, created, last used and last IP.
- `gx server web-revoke <id>` signs out one web login.
- `gx server web-password` sets a new password without the old one, and signs out every web login.
- `gx server web-reset-authenticator` clears the TOTP secret, signs out every web login and prints a
  new Setup code. The next web login asks for the Setup code and the password, then adds a new
  authenticator.
- `gx server web-reset` asks y/N, then clears the password, the secret, every web login and the rate
  limits, and prints a new Setup code. Pass `--yes` to skip the prompt. Without a terminal and
  without `--yes` it refuses.

## Upgrade note: sign in again on every device

This version has a hard cut. The shared token is gone. An old token link or old cookie lands on the
setup page, or on the login page after setup.

The server certificate covers the names in `server.web.allowed-hosts`. Do these steps in order:

1. Add every name you use to `server.web.allowed-hosts` before the first start with this version, so
   the certificate covers your domain from the start.

   ```json
   { "server": { "web": { "allowed-hosts": ["my-laptop.tailnet.ts.net"] } } }
   ```

   IP addresses and `localhost` need no entry. A name that is missing gets HTTP 403.
2. Start the server and finish the first-run setup above.
3. On each device, trust the root certificate (iOS profile or Mac `.cer`).
4. On each device, sign in again.

## Answering a paused ticket from a phone

When an agent pauses a ticket with a question, the question shows in the queue. Open the ticket page,
read the question, and send your answer from the browser. The running agent picks it up.

The same flow has command-line verbs:

- `gx tickets ask <project:epic/NN>` records a question and pauses the ticket with the reason "Waiting for your answer".
- `gx tickets answer <project:epic/NN>` answers the question a paused ticket is waiting on.
- `gx tickets log <addr>` prints the ticket's raw log events, one JSON object per line.

## Prompt images

Three routes carry the images pasted into a session's Prompt box. All three need a signed-in web login.

- **Upload** (`POST /v1/sessions/images?id=<session>`). The body is the image's raw bytes, so this is
  the one web write that skips the JSON `Content-Type` rule. It keeps the `Origin` check, which a
  cross-site page cannot forge. It accepts PNG, JPEG, GIF and WebP up to 20 MB, decided from the
  bytes and never from the upload's name or type. gx picks the saved file name. An unknown session,
  an Exited session and a session with no chat reader are refused.
- **Serve** (`GET /v1/sessions/images?session=<id>&id=<image>`). The image id is matched against the
  file names gx wrote in that session's `images` folder. No part of the request becomes a path, so
  `..`, an absolute path, another session's id and an unknown id all return 404. The response has
  `Content-Type` detected from the file bytes, `X-Content-Type-Options: nosniff` and a long
  `private` cache header.
- **Delete** (`POST /v1/sessions/images/delete`, JSON `{id, image}`). It removes an image that has
  not been sent, under the upload's session rules. A second delete answers `image-not-found`.

Removing an Exited session deletes its state folder, images included.

CLI: `gx agents image add|get|rm`.

## Hosted files

gx shows files from the gx machine in the web UI. The viewer is a hash route, `/#/files<absolute path>`.
`gx files url <path>` prints that link (`--raw` prints the raw link).

### Routes

- **Raw** (`GET /v1/files/<absolute path>`). Needs a signed-in web login (401 without it). It supports
  Range. `?download=1` adds `Content-Disposition: attachment` and never redirects. A cookie request for
  an HTML file answers `302` to the token route.
- **Token** (`GET /v1/files/t/<token>/<absolute path>`). It serves any file under the token's folder, so
  an HTML page can load its sibling files. It needs no cookie. Its responses add
  `Access-Control-Allow-Origin: *`; no other route sends it.
- A file whose path starts with a top-level folder named `t` has no raw link, and `gx files url`
  refuses it.
- Both routes are web-only (ADR 0028). The JSON routes live under `/v1/hosted-files/`: `gx files url`,
  `gx files resolve` and `gx server files-roots list|add|remove`.

### Roots and refusals

- **Known roots.** Registered project repos, their worktrees, the tickets store, and the temp folders
  (`/tmp`, `/private/tmp`, the server's `$TMPDIR`). They are read on each request, so a new worktree
  needs no restart. `server.web.files.roots` adds extra folders. `gx server files-roots add|remove`
  edits only that list. `add` refuses `/`, the home folder itself, a missing path and a non-folder.
  A path under home but outside every root is refused.
- **Deny list.** Any path segment (case-insensitive, on the requested and the symlink-resolved path)
  that is `.env*`, `.git`, `.ssh`, `.aws`, `.npmrc`, `.netrc`, `.git-credentials`, `.gnupg`, `.kube`,
  `.docker`, `.pgpass`, `*.pem`, `*.key`, `*.p12`, `*.pfx`, or starts with `id_rsa`, `id_ed25519`,
  `id_ecdsa` or `id_dsa` (a name ending in `.pub` is allowed). It applies to the raw, token and
  resolve routes. It matters most for tokens, because a token for a file in a repo's top folder
  covers the whole repo.
- **gx's own folders.** The state, config, sessions and conversations folders are denied whole,
  even inside a temp root or a repo. In the data folder everything is denied except the tickets store.
- **Temp folders.** Under a temp root gx serves only a regular file that the gx user owns and that has
  link count 1. Every folder between the temp root and the file must also be owned by the gx user.
- **Path rules.** On the raw and token routes, a non-canonical path answers 404: `.` or `..`
  segments (also percent-encoded), `%2F`, `%5C`, `//`, a backslash or a NUL byte. The `resolve` verb
  joins and cleans its candidate, so `../sibling.md` works there. Every check failure gives the same
  404, so a caller cannot tell which rule refused.

### Token and login gate

- The token holds the file's folder, an expiry of one hour and the id of the web login it was made for.
  It is signed with a key that lives in memory, so a restart ends every token.
- Each token request checks the login is still live. Signing out or revoking the login, or changing
  the password or authenticator, ends its tokens.
- A bad, expired or out-of-folder token answers `302` back to the raw link, which mints a fresh one
  if the cookie is valid.
- `/v1/files/t/` is the only `/v1/` prefix that works without a login. The gate hands such a request
  to the token handler directly, so it never reaches another handler. The exemption stays behind the
  source-IP and Host checks and does not renew the cookie.

### Headers

Every file response (200, 206, 302, 401 and 404, including the gate's 401) carries:

- `Content-Security-Policy: sandbox allow-scripts; frame-ancestors 'self'; connect-src https://<request Host>`.
  If the Host fails the allowed-hosts check, `connect-src` is `'none'`.
- `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` and `Cache-Control: private, no-store`.
- No `Cross-Origin-Resource-Policy`, so token responses load from the page's opaque origin.

The login cookie is not sent for sub-requests of a sandboxed page, and its `Origin` is `null`, so a
hosted page cannot read gx's JSON routes or write to them.

### Known gaps

- A PDF keeps the sandbox, and a sandboxed PDF has no preview in the viewer. The viewer offers Raw and
  Download.
- Real iOS Safari and desktop Safari are untested. Playwright WebKit is the only evidence.

## Upgrade note: run migrate

The person who runs the gx server runs `gx tickets migrate <path>` once after you upgrade. Pass the
tracker root as `<path>`. The server also runs the steps it has not applied yet at start-up.

The step moves each live `## Needs Answer` and `## Handoff` section into the ticket log as a question
(and an answer, if you answered by hand and the ticket is open again). A second run changes nothing.

Until it runs, an old ticket without a log shows no open question on the web, and the web cannot answer it.
Run migrate first.
