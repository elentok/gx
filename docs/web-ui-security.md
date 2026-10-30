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
- **TCP listener.** The opt-in loopback listener (`server.tcp-listen`) refuses any request with an
  `Origin` or `Sec-Fetch-Site` header, or with a non-loopback `Host`, so a browser cannot reach it.

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

## Upgrade note: run migrate

The person who runs the gx server runs `gx tickets migrate <path>` once after you upgrade. Pass the
tracker root as `<path>`. gx does not run it at server start.

The step moves each live `## Needs Answer` and `## Handoff` section into the ticket log as a question
(and an answer, if you answered by hand and the ticket is open again). A second run changes nothing.

Until it runs, an old ticket without a log shows no open question on the web, and the web cannot answer it.
Run migrate first.
