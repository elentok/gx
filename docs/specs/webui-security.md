# webui-security

Map epic: `gx:webui-security`. Decisions come from its tickets 01 to 08. Research:
`docs/research/webui-security-ios-cookie-loss.md` and `docs/research/webui-security-ios-root-trust.md`.
Approved mock: `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`. The
prototype is on the local branch `prototype/webui-security-login-ui`.

## Problem Statement

The person runs gx on a home machine and uses the web UI from an iPhone, an iPad and a Mac, through
the tailnet and the home LAN. They reach it as `gertsel.elentok.com` (a record on the home DNS
server) or by IP.

Today the web UI is protected by one shared token:

- The listener speaks plain HTTP. Anyone on the LAN or tailnet who sniffs traffic can read the
  `?gx_token=` link or the cookie. Either one gives full control, and `POST /v1/sessions/prompt`
  sends text to a full-permission agent, so whoever holds the cookie can run code on the machine.
- The cookie lives 365 days. The only way to revoke one device is to rotate the token, which signs
  out every device.
- A phone that is lost while signed in stays signed in for a year.
- The listener accepts connections from any source IP.

The person also hits three bugs:

- Refreshing the page sometimes shows the plain-text 401 "missing token" message.
- The home-screen app on iPhone and iPad sometimes loses its cookie. The only fix is to remove the
  app and add it again.
- The page sometimes goes blank when switching back to gx, in Safari or in the home-screen app.

## Solution

The shared token goes away. The web UI gets a login for one person with a password plus a TOTP code
from an authenticator app.

- **First-run setup.** The CLI prints a one-time Setup code (`gx server status`,
  `gx server web-url --qr`). The person enters it on the setup page, picks a password, scans a TOTP
  QR code, and enters one code to finish. That device is signed in.
- **Web logins.** Each successful login on a device is its own Web login. It ends after 90 days
  without use. The new Settings page lists the web logins, renames them and signs them out one at a
  time. It also changes the password and resets the authenticator. Both sign out every other web
  login.
- **CLI fallback.** A local shell is trusted. The CLI lists and revokes web logins, changes the
  password and resets the authenticator without the old secrets. `gx server web-reset` clears
  everything and goes back to first-run setup.
- **HTTPS always.** gx makes its own root certificate and a server certificate on the first start
  with the listener on. A plain-HTTP request on the same port is redirected to `https://`. The root
  is offered as an iOS profile and a Mac `.cer` at a public path. Its SHA-256 fingerprint shows on
  the setup, login and Settings pages and in `gx server status`.
- **Reach.** The listener closes connections from source IPs outside the private LAN ranges, the
  tailnet range and loopback, before TLS. The Host allowlist and the Origin plus Content-Type check
  stay.
- **iOS fixes.** The cookie is `SameSite=Lax`, so a reload after a cross-site link keeps it. An
  unauthenticated page shows an HTML login page at the same URL, so the home-screen app can always
  recover. `index.html` and the hashed assets get cache headers, a missing asset is a 404, and a
  root error boundary reports client errors to `gx.log`, so the next blank page leaves evidence.

## User Stories

### Reach and HTTPS

1. As the person, I want the web listener to serve HTTPS whenever `server.web.listen` is set, so
   that nobody on the LAN or tailnet can read my password, TOTP codes or cookie.
2. As the person, I want no config switch that turns HTTPS off, so that I cannot run the web UI in
   plain text by mistake.
3. As the person, I want a plain `http://` request on the web port to get a redirect to `https://`
   on the same host and path, so that old bookmarks and typed addresses still work.
4. As the person, I want no second port for the redirect, so that I keep one address and one
   firewall rule.
5. As the person, I want each allowed name and each IP to work on its own, with no redirect between
   hosts, so that the IP still works when DNS fails.
6. As the person, I want the listener to close connections from public source IPs before TLS, so
   that nothing outside my tailnet and LAN can even start a handshake.
7. As the person, I want the private LAN ranges, `100.64.0.0/10`, loopback, link-local and IPv6 ULA
   allowed, so that all my devices on the tailnet and LAN get in.
8. As the person, I want an IPv4-mapped IPv6 source address treated as its IPv4 address, so that a
   dual-stack listener applies the same rule.
9. As the person, I want `X-Forwarded-For` ignored, so that a client cannot fake its source IP.
10. As the person, I want each refused IP logged at most once a minute, so that `gx.log` shows probes
    without filling up.
11. As the person, I want the Host allowlist (`server.web.allowed-hosts`, IP literals, `localhost`)
    to keep answering 403 for other names, so that DNS rebinding still fails.
12. As the person, I want every write to keep needing a same-host `Origin` and a JSON content type,
    so that a hostile page in my phone's browser cannot post to gx.
13. As the person, I want the old warning about binding `0.0.0.0` gone, so that I am not warned
    about a token sent in plain text that no longer exists.
14. As the person, I want the web listener to stay off until I set `server.web.listen`, so that a
    fresh install opens no network port.

### Certificates and trust

15. As the person, I want gx to make a root certificate and a server certificate on the first start
    with the listener on, so that I need no outside certificate authority.
16. As the person, I want the root limited by critical name constraints to the parent domain of each
    allowed host, `localhost`, and the private, tailnet, loopback, link-local and ULA IP ranges, so
    that a stolen root key cannot sign a certificate for any public site.
17. As the person, I want the server certificate to list every allowed name and every local IP, so
    that every address I use passes the check on iOS.
18. As the person, I want a new IP or a new allowed host to get a new server certificate from the
    same root, live, so that I never re-trust a device for a network change.
19. As the person, I want gx to re-issue the server certificate 30 days before it expires, so that
    the web UI never shows an expired certificate.
20. As the person, I want an allowed host outside the root's constraints left out of the server
    certificate, with a warning in `gx.log` and `gx server status`, so that I know why that name
    fails.
21. As the person, I want a CLI command that makes a new root, so that I can cover a new domain or
    replace a root I no longer trust.
22. As the person, I want to download the root as an iOS profile from a page that needs no login, so
    that I can trust it on a new iPhone before I can sign in.
23. As the person, I want the root as a `.cer` file for the Mac on the same page, so that the Mac
    browsers trust it too.
24. As the person, I want the root's SHA-256 fingerprint on the setup, login and Settings pages and
    in `gx server status`, so that I can check that the certificate my phone got is mine.
25. As the person, I want the certificate help on the login and setup pages folded under the form,
    with the profile and `.cer` links, the Certificate Trust Settings step and the fingerprint, so
    that it is there on the first visit but out of the way later.
26. As the person, I want the certificates kept in `<stateDir>/web-tls/` with 0600 files, so that
    only my user can read the root key.

### First-run setup

27. As the person, I want every page to show the setup page and every API route to answer 401 until
    setup is done, so that nobody uses gx without a password.
28. As the person, I want `gx server status` and `gx server web-url` to print a Setup code while
    setup is pending, so that I can start setup from the shell on the server.
29. As the person, I want the Setup code to be 8 characters with no look-alike letters, so that I
    can type it from a terminal without mistakes.
30. As the person, I want the Setup code to work once and expire after an hour, with a server
    restart making a new one, so that a leaked code is soon useless.
31. As the person, I want `gx server web-url --qr` to print a QR code that opens the setup page with
    the code filled in, so that I can start setup on the phone with one scan.
32. As the person, I want the TUI About dialog to say "Web UI: setup needed, run `gx server web-url
    --qr`" and never show the code, so that the code does not sit on a screen.
33. As the person, I want setup to be one card with "step N of 3" and a progress bar, so that I know
    how far I am.
34. As the person, I want step 1 to tell me when the code is wrong or expired, so that I go back to
    the CLI for a new one.
35. As the person, I want step 2 to ask for the password twice, with "At least 12 characters.", so
    that I do not lock myself out with a typo.
36. As the person, I want step 3 to show the TOTP QR code, the key as text and one code field, so
    that I can add gx to my authenticator app by scan or by hand.
37. As the person, I want gx to save nothing until the TOTP code checks out, with "doesn't match,
    check the phone's clock" on a wrong code, so that a broken authenticator never becomes my only
    way in.
38. As the person, I want the device that finishes setup signed in as the first web login, so that I
    do not log in right after setting up.
39. As the person, I want wrong Setup codes to count toward the login rate limit, so that nobody can
    guess the code.

### Login

40. As the person, I want an unauthenticated page to show an HTML login page at the same URL, never
    plain text, so that the home-screen app can always recover.
41. As the person, I want the login page to be one card with the `gx` wordmark and host, a password
    field, one "Code from your authenticator app" field and Sign in, so that logging in again is
    cheap when iOS drops the cookie.
42. As the person, I want the code field to be one numeric input with `autocomplete="one-time-code"`,
    so that iOS can fill it.
43. As the person, I want one message, "Wrong password or code", whichever part was wrong, so that a
    guesser learns nothing.
44. As the person, I want a code that was already used rejected, so that a code seen over my
    shoulder cannot be used again.
45. As the person, I want one 30-second step of clock skew allowed each way, so that a phone clock a
    little off still works.
46. As the person, I want an IP that fails 5 times to wait 1 minute, doubling up to 1 hour, so that
    one device cannot guess my password.
47. As the person, I want every login to wait 15 minutes after 50 failures in an hour from all IPs,
    so that many devices together cannot guess it either.
48. As the person, I want the page to say "Too many tries. Try again in N minutes." with Sign in
    disabled, so that I know why login does nothing.
49. As the person, I want the login page to tell a locked-out person to run `gx server web-reset` on
    the server, so that I know the way back.
50. As the person, I want to land on the page I was on after logging in again, so that a dropped
    cookie does not lose my place.
51. As the person, I want an API call or the event stream that gets a 401 to send the page to login,
    so that the app never sits on stale data after my web login ends.
52. As the person, I want the page cache cleared on a 401 and on sign out, so that a signed-out phone
    does not show my tickets from `sessionStorage`.

### Web logins

53. As the person, I want each login on a device to be its own web login, so that I can sign out one
    device and keep the others.
54. As the person, I want the cookie to be `__Host-` prefixed, `Secure`, `HttpOnly`, `SameSite=Lax`,
    `Path=/` and without `Domain`, so that a reload after a cross-site link keeps it and no script
    can read it.
55. As the person, I want a web login to end after 90 days without use and to renew on use, with no
    absolute limit, so that a phone I use daily never asks again.
56. As the person, I want web logins to survive server restarts and upgrades, so that upgrading gx
    signs nobody out.
57. As the person, I want only the SHA-256 of each cookie on disk, so that a copy of the state file
    cannot be replayed as a cookie.
58. As the person, I want each web login named from its user agent ("Safari on iPhone"), so that I
    can tell my devices apart.
59. As the person, I want the browser and the home-screen app on one phone to be separate web
    logins, so that the list matches what iOS does.

### Settings page

60. As the person, I want a "Settings" item in the hamburger menu, so that I can find the security
    settings.
61. As the person, I want the Settings page to be a grouped list inside the normal page frame, with
    a Security group (Web logins with a device count, Change password, Reset authenticator,
    Certificate) and a separate "Sign out of this device" group, so that each task is one tap away.
62. As the person, I want each row to open a sub-page with a "‹ Settings" back link, so that the
    page works the same on a phone and a desktop.
63. As the person, I want the Web logins sub-page to show the 90-day note and one row per web login
    with its name, last used time and last IP, and a "This device" chip, so that I can spot a device
    I do not know.
64. As the person, I want a web login with under 30 days left to show "Ends in N days unless used",
    so that I know which devices will be signed out soon.
65. As the person, I want to open one web login and see when it signed in, when it was last used and
    its last IP, so that I can judge if it is mine.
66. As the person, I want to rename a web login, so that "Safari on iPhone" can become "David's
    phone".
67. As the person, I want to sign out another device after a confirm step, so that a lost phone
    loses access at once and a stray tap does nothing.
68. As the person, I want a signed-out device's open page to go to login at once, without a reload,
    so that a stolen phone stops seeing live data.
69. As the person, I want to change the password with the current password, a code and the new
    password twice, so that someone holding my unlocked phone cannot change it.
70. As the person, I want to reset the authenticator with the password and a code from the current
    app, then a new QR code, key and code field, so that I can move to a new phone.
71. As the person, I want the old authenticator to keep working until a code from the new one checks
    out, so that a reset I abandon halfway changes nothing.
72. As the person, I want password change and authenticator reset to say "N other devices will be
    signed out" before I confirm, and then sign out every other web login, so that a leaked password
    or TOTP secret stops working everywhere.
73. As the person, I want failed password and code checks on the Settings page to count toward the
    login rate limit, so that a stolen signed-in phone cannot guess my password there.
74. As the person, I want a Certificate sub-page with the iPhone/iPad profile and the Mac `.cer`
    download rows and the fingerprint, so that I can trust a new device later.
75. As the person, I want "Sign out of this device" on the Settings page, and on this device's own
    web login sub-page, so that I can leave a shared computer signed out.

### CLI

76. As the person, I want `gx server status` to print the plain web URL, the Setup code while setup
    is pending, the root fingerprint, and any names left out of the certificate, so that one command
    shows the web UI's state.
77. As a script, I want `gx server status --json` to carry `web_url` as the plain URL and a
    `web_setup_code` field only while setup is pending, so that I can automate setup.
78. As the person, I want `gx server web-url` to print the plain URL after setup, so that I can open
    the web UI on a new device.
79. As the person, I want `gx server web-reset` to ask y/N, then clear the password, the TOTP
    secret, every web login and the rate limits and print a new Setup code, so that I can recover
    when I lose both my password and my authenticator.
80. As a script, I want `gx server web-reset --yes` to skip the prompt, and `web-reset` without
    `--yes` and without a terminal to refuse, so that a script never resets by accident.
81. As the person, I want a CLI command that lists web logins with id, name, created, last used and
    last IP, so that I can check devices from the shell.
82. As the person, I want a CLI command that revokes a web login by the id from that list, so that I
    can sign out a lost phone when no other device is signed in.
83. As the person, I want a CLI command that changes the password with no old password, asking for
    the new one twice with no echo, so that I can recover a forgotten password and keep my
    authenticator.
84. As a script, I want a non-terminal way to pass the new password, so that I can set it without a
    prompt.
85. As the person, I want a CLI command that resets the authenticator with no old code, prints the
    new QR code and key in the terminal and asks for one code from the new app, so that I can recover
    a lost phone and keep my password.
86. As the person, I want both CLI commands to sign out every web login, so that whatever device was
    lost is out.
87. As the person, I want every new web CLI command to fail with `server-not-running` when the server
    is down, and with "the web UI is off" when the listener is off, so that the CLI never edits state
    behind the server's back.
88. As the person, I want each CLI action to write the same `gx.log` lines as its web version and to
    close the event streams of the web logins it signs out, so that the shell and the web leave the
    same trail.

### Logging

89. As the person, I want `gx.log` to record each login, failed login, revoke, rename, password
    change, authenticator reset and web reset with the IP and the web login name, so that the log
    and the list are my audit trail.
90. As the person, I want `gx.log` never to hold the password, a TOTP code, the TOTP secret, a cookie
    value or a Setup code, so that the log is safe to share.

### Upgrade

91. As the person, I want gx to delete `<stateDir>/web-token` on start and drop `gx server
    web-token`, so that the old shared secret is gone.
92. As the person, I want an old `?gx_token=` link or an old `gx_web` cookie to land on the setup
    page, or the login page after setup, so that old bookmarks lead somewhere useful.
93. As the person, I want an upgrade note that says to add every name to `server.web.allowed-hosts`
    before the first start with this version, so that the root covers my domain from day one.

### iOS reliability

94. As the person, I want `index.html` sent with `Cache-Control: no-cache` and an `ETag`, so that a
    resumed page never runs an old `index.html` against new assets.
95. As the person, I want the hashed assets sent with `max-age=31536000, immutable`, so that they
    load from cache on resume.
96. As the person, I want a missing `/assets/*` file to answer 404, and `index.html` served only for
    `/`, so that a stale page fails loudly instead of loading HTML as a script.
97. As the person, I want a root error boundary that shows a short message and a Reload button, so
    that a crash never leaves a blank page.
98. As the person, I want client errors and unhandled rejections reported to `gx.log`, so that the
    next blank page leaves evidence.
99. As the person, I want the error report to accept any client that passes the listener rules, with
    a size cap and a per-IP rate limit, so that errors on the login page are reported too and a
    device cannot fill the log.

### Developers and agents

100. As an e2e flow, I want `gxfixture` to sign in through the real setup over HTTPS and hand me the
     cookie, so that existing flows keep running signed in with no test bypass in gx.
101. As an e2e flow, I want fixture scenarios with setup pending, with setup done and no cookie, and
     with several web logins, so that I can test setup, login and Settings.
102. As a developer, I want the web-only routes listed in one place and checked by a test, so that a
     new web-only route is a choice, not an accident (ADR 0028 amendment).
103. As a developer, I want the Vite dev frontend (`server.web.dev-url`) to keep working behind the
     HTTPS listener, so that hot reload still works.

## Implementation Decisions

### Modules

- **Web auth module (new, in `server`).** Owns the one 0600 state file and its rules: password hash,
  TOTP secret, last accepted TOTP step, web logins, Setup code, rate limits. It has a small
  interface the web mux, the socket routes and tests call. Suggested shape (names are the
  implementer's): set up, log in, look up a cookie, list, rename, revoke, change password (with and
  without the old checks), begin and finish an authenticator reset, reset everything, current Setup
  code. It takes a clock, so tests set the time for TOTP, idle expiry and rate limits.
- **Web TLS module (new, in `server`).** Makes and loads the root and leaf in `<stateDir>/web-tls/`,
  builds the root's name constraints, re-issues the leaf, serves `GetCertificate`, and builds the
  `.mobileconfig` and `.cer`. Stdlib only (`crypto/ecdsa`, `crypto/x509`, `crypto/tls`,
  `encoding/pem`, `text/template` for the plist).
- **Web listener.** A listener wrapper that applies the source-IP rule on accept, then reads the
  first byte: `0x16` (a TLS handshake) goes to TLS, anything else to the plain-HTTP redirect.
- **Web mux (`server/web.go`).** Gains the web-only routes and the public paths. `requireToken` and
  the token file code go away. The serving rules for `index.html` and assets change.
- **CLI (`cmd`).** `web-url` and `status` change, `web-token` goes, `web-reset` and the new web login
  and account commands are added.
- **TUI About dialog (`ui/app`).** Shows "setup needed" from a setup-pending flag on the handshake.
- **Web UI (`webui`).** New login, setup and Settings pages, a shared fetch helper, an auth gate at
  the app root, a root error boundary, client error reporting, a "Settings" menu item.
- **Fixture (`testutil/gxfixture`) and `webui/e2e/fixture.ts`.** Real sign-in, new scenarios.

### Web login store

- One 0600 file in the state dir holds the Argon2id password hash (with its parameters), the TOTP
  secret, the last accepted TOTP step, and the web logins. The implementer names the file. Writes
  go to a temp file and rename.
- A web login has an id (random, not derived from the cookie, shown by the CLI list), the SHA-256
  of its cookie, a name, the user agent, created, last used and last IP.
- The cookie value is 32 random bytes. The server keeps the hashes in memory and compares in
  constant time.
- Last used and last IP are written at most once a minute per web login. The cookie's `Max-Age`
  (90 days) is renewed at the same rate.
- A web login unused for 90 days is gone: a lookup refuses it and the store drops it.
- Argon2id from `golang.org/x/crypto`. The implementer picks parameters at or above OWASP's minimum
  (19 MiB, 2 passes, 1 lane). The stored parameters let a later version change them.
- Password rule: at least 12 characters. The server checks it on every path; the clients check it
  too, for the message.
- TOTP: SHA-1, 30 seconds, 6 digits, one step of skew each way. A step at or below the last
  accepted step is rejected. The `otpauth://` issuer and label are the implementer's choice.
- The name comes from the user agent ("Safari on iPhone", "Chrome on Mac"). The implementer owns the
  table and the max name length.
- Rate limits live in memory. Per IP: after 5 failures, wait 1 minute, doubling up to 1 hour. A
  success clears that IP's count. Global: 50 failures in an hour from all IPs make every check wait
  15 minutes. Failures are failed logins, wrong Setup codes, and failed password or code checks on
  the Settings page. `web-reset` clears the limits; a restart clears them too.

### Setup code

- 8 characters from an alphabet with no look-alikes, shown as `K7QM-3XPD`. The server holds it in
  memory. It expires an hour after it is made; a read after that makes a new one. A restart makes a
  new one. It exists only while setup is pending.
- Setup step 1 checks the code (a failure counts toward the rate limit) and hands back a new TOTP
  secret tied to that code in memory. Step 2 is client-side. Step 3 sends the code, the password, and
  one TOTP code. The server checks all three, saves, consumes the code and signs the device in. If
  two devices race, the first to finish wins and the other gets the wrong-or-expired message.
- The `--qr` link opens `/#/setup?code=<code>`. The code sits in the hash, so it never reaches the
  server log or a `Referer`, and `index.html` is still served only for `/`.

### Routes

Web-only routes are served on the web mux only. They are not in `routeTable` and have no CLI verb.
The spec amends ADR 0028: "Routes that act on the caller's own web login, or serve public files to
the browser, are web-only. They are listed in one place in `server`, and a test checks that list
against the web mux." The implementer picks the paths. The list:

- Public (no login): the auth state read (setup pending, signed out, or signed in with this web
  login's id), setup step 1 and finish, login, the iOS profile, the `.cer`, the client error report,
  `index.html` at `/`, and the assets.
- With the cookie: sign out, list web logins, rename, revoke one, change password, begin and finish
  an authenticator reset.

Every other web route needs a valid cookie and answers 401 without one. Every non-GET, public or
not, keeps `checkWriteRequest`. Refusals on the web-only routes follow ADR 0028's refusal results
(`200`, `refused: true`, a stable `reason`). The implementer names the reasons: wrong credentials,
rate limited (with the wait in seconds), Setup code wrong or expired, TOTP mismatch, password too
short, unknown web login.

Socket routes, each in `routeTable` with a `gx server …` verb and `--json`, all server writes or
reads (ADR 0030), all refusing with `server-not-running` when the server is down:

- The web info read, whose verb is `gx server web-url`: base URL, setup pending, Setup code while
  pending, root fingerprint, names left out of the leaf. `gx server status` calls it too.
- `gx server web-reset`.
- List web logins, revoke one by id, change password (new password only), reset authenticator (new
  secret and one code), make a new root. The implementer names these commands and flags.

These socket routes are refused on the opt-in loopback TCP listener. Only the 0600 socket proves a
local shell; loopback TCP is open to any page in a browser on the machine. The web listener being
off makes every one of them fail with today's "the web UI is off" message.

### CLI commands

- `gx server status`: `web: <https url>`; while pending, the Setup code and its expiry; the
  fingerprint; a warning per name left out of the leaf. `--json`: `web_url` is the plain URL, and
  `web_setup_code` appears only while setup is pending. The implementer names the fingerprint and
  warnings fields.
- `gx server web-url [--qr]`: while pending, prints the setup link (with the code in the hash) and
  the code. After setup, the plain URL. Gains `--json`.
- `gx server web-reset [--yes]`: asks y/N on a terminal. With no terminal and no `--yes`, it refuses.
  Clears the password, the TOTP secret, every web login and the rate limits, closes every event
  stream from a web login, and prints a new Setup code.
- List: id, name, created, last used, last IP. No rename from the CLI.
- Revoke: takes the id. Closes that web login's event streams.
- Change password: asks twice with no echo, plus a non-terminal input (the implementer picks it,
  for example `--password-stdin`). No old password. Signs out every web login.
- Reset authenticator: the CLI makes the new secret with the same code the server uses, prints the
  QR code (the existing `renderQR`) and the key, reads one code, and sends secret and code in one
  request. The server checks the code against that secret, saves, and signs out every web login.
  Nothing is saved on a wrong code. No old code.
- Make a new root: a flag or command the implementer names. The server swaps the root and leaf
  live. Every device must trust the new root. Web logins are kept.
- `gx server web-token` and `--rotate` are removed.

### Web login streams

The server tracks open event streams by web login id. Revoke (web or CLI), password change, an
authenticator reset and `web-reset` close the streams of every web login they sign out. The page then
gets a 401 on reconnect and shows login.

### TLS

- Root: ECDSA P-256, self-signed, CA true and critical, path length 0, key usage keyCertSign and
  cRLSign, 10 years. Critical name constraints:
  - DNS: `localhost` plus the parent domain of each allowed host (the host minus its first label:
    `gertsel.elentok.com` gives `elentok.com`). A single-label host is permitted as the exact name.
  - IP: `10/8`, `172.16/12`, `192.168/16`, `100.64/10`, `127/8`, `169.254/16`, `::1/128`,
    `fc00::/7`, `fe80::/10`.
  The constraints are fixed when the root is made.
- Leaf: ECDSA P-256, CA false, digitalSignature, EKU serverAuth, NotBefore one hour back, 365 days.
  SAN: `localhost`, each allowed host inside the constraints (dNSName), every local interface IP and
  the listen IP (iPAddress). The CN does not matter.
- Re-issue at start, on a config reload that changes allowed hosts, 30 days before expiry, and in
  `GetCertificate` when a connection arrives on a local IP the leaf lacks. No interface polling.
- Files in `<stateDir>/web-tls/`, 0600, made on the first start with the listener on.
- Profile: an unsigned `.mobileconfig` with one `com.apple.security.root` payload, a stable
  `PayloadIdentifier`, a fresh `PayloadUUID` each build, `Content-Type:
  application/x-apple-aspen-config`. The `.cer` is DER with `Content-Type: application/pkix-cert`.
- Fingerprint: SHA-256 of the root's DER, hex in 8 groups of 8, the same text in the web UI and the
  CLI.
- No HSTS header. HSTS would block the browser's "visit anyway" step before the profile is
  installed.

### Listener

- Source-IP rule on accept, before TLS, against the IP list above. IPv4-mapped IPv6 is unwrapped
  first. A refused connection is closed and logged at most once a minute per IP.
- First byte `0x16` goes to TLS. Anything else is read as plain HTTP and answered with a redirect to
  `https://` on the same host and path, but only when the Host passes `hostAllowed`; any other Host
  gets the 403. The implementer picks the redirect status.
- `WebURL()` returns `https://<addr>`.
- The dev frontend proxy keeps working behind TLS. Vite modules are public, like the built assets.

### Serving

- `index.html` only for `/`, with `Cache-Control: no-cache` and an `ETag` from its content.
- Hashed files under `/assets/` get `Cache-Control: max-age=31536000, immutable`. A missing
  `/assets/*` is a 404.
- Any other path with no file is a 404. Unhashed files at the root (touch icon) get `no-cache`.
- The old `gx_web` cookie and `?gx_token=` parameter are ignored. The page shows setup or login.

### Cookie

`__Host-` prefix (the rest of the name is the implementer's), `Secure`, `HttpOnly`,
`SameSite=Lax`, `Path=/`, no `Domain`, `Max-Age` 90 days renewed on use. Only the server sets it.
Sign out clears it.

### Web UI

- An auth gate at the app root reads the auth state. It shows the setup page (pending), the login
  page (signed out) or the app. It keeps the hash, so after login the person lands where they were.
- One shared fetch helper wraps every API call. On a 401 it clears the page cache
  (`pageCache.ts` and `snapshotCache.ts`) and sends the gate to login. The event stream does the
  same on a 401. Sign out clears the caches too.
- Login, setup and Settings follow the approved mock (variant A card, variant A card stepper,
  variant C grouped list). The prototype's components are a starting point. The implementer picks
  the sub-page routes (for example `#/settings/logins`) and where the pages live.
- The TOTP QR code on the setup and reset pages: the implementer picks server or client rendering.
- A "Settings" item in the hamburger menu opens `#/settings`. "Sign out of this device" is not in
  the menu.
- The root error boundary shows the login card's frame with one line ("Something went wrong") and a
  Reload button. It and `window` error and `unhandledrejection` handlers post to the error report
  route.
- Client error report: message, stack, page URL and user agent. Size-capped and rate-limited per IP
  (the implementer picks the numbers). Written to `gx.log` at warn level. It holds no cookies.

### Logging

`gx.log` records login, failed login (with the reason kind), wrong Setup code, sign out, revoke,
rename, password change, authenticator reset, web reset and new root. Each line has the IP (or
"cli") and the web login name. It never records a password, a TOTP code, the TOTP secret, a cookie
value or a Setup code. Request bodies of the web auth and account routes are never logged.

### Upgrade

- On start, gx deletes `<stateDir>/web-token` if it exists.
- `docs/web-ui-security.md` is rewritten for the new model, with the upgrade note: add every name to
  `server.web.allowed-hosts` before the first start, then trust the profile on each device, then
  sign in again on each device.
- `skills/gx-ui-check/SKILL.md` and `webui/e2e/CATALOG.md`'s intro follow the fixture's new sign-in.
- CHANGELOG entry: hard cut, every device signs in again.

### Pauses

No new pause. A locked-out person recovers with `gx server web-reset`, which is not a pause of work,
so ADR 0042 is unaffected.

## Testing Decisions

A good test checks what a client sees: HTTP status, headers, cookies, response bodies, certificate
fields a TLS client reads, CLI output and exit codes, the rendered page. It does not read the web
auth module's internal fields.

Seams:

1. **Server web listener, through servertest.** The existing `startWeb` setup in `server/web_test.go`
   with an HTTPS client that trusts the root from `<stateDir>/web-tls/`. A `Config` field sets the web
   auth clock; the implementer names it, and production uses the real clock. Tests drive setup,
   login, rate limits, cookies, idle expiry, revoke closing the event stream, serving headers, the
   plain-HTTP redirect, the public paths, the 401s and the Host and Origin checks. The source-IP rule
   is tested as a table over addresses, plus one listener test with a fake connection from a public
   address. Prior art: `server/web_test.go`, `server/webhost_test.go`.
2. **Pure rules.** Table tests for TOTP (RFC 6238 vectors, skew, replay), the Setup code alphabet and
   expiry, the user-agent names, the rate limiter with a fake clock, and name-constraint building
   (parent domain, single label, `localhost`). Certificate tests parse the root and leaf with
   `crypto/x509` and check every field listed under TLS, verify the leaf against the root, and check
   that a name outside the constraints fails `Verify`. A plist test parses the `.mobileconfig`.
3. **CLI.** `cmd` tests against the servertest harness, as `cmd/server_web_test.go` does today: status
   text and `--json`, `web-url` with and without `--qr`, `web-reset` with `--yes` and without a
   terminal, list, revoke, change password from stdin, reset authenticator with a code computed in
   the test, `server-not-running`, "the web UI is off". Plus `TestRouteTableHasCLIVerbs`, a check that
   the account routes refuse on the TCP listener, and the new web-only route list test next to
   `TestWeb_EverythingElseIs404`.
4. **Web.** Vitest for the auth gate, the fetch helper's 401 path and cache clearing, the login,
   setup and Settings pages in every mock state, the error boundary, and the error reporter. A
   gallery entry per mock state. Prior art: `webui/src/connection.test.ts`,
   `webui/src/pageCache.test.ts`, `webui/src/routes.test.tsx`, the `TopBar` tests and gallery entry.
5. **Fixture and e2e.** `gxfixture` signs in through the real routes: it reads `web_setup_code` from
   `gx server status --json`, finishes setup with a fixed password and a TOTP code computed from the
   returned secret, and puts the cookie in its JSON. `fixture.ts` adds the cookie to the browser
   context, and Playwright ignores the self-made certificate. New scenarios:
   - `web-setup`: setup pending, page not signed in.
   - `web-login`: setup done, page not signed in. The fixture JSON carries the password and the
     TOTP secret so a flow can compute a code.
   - `web-logins`: signed in, plus three more web logins (one with last used 70 days ago). Before
     the server starts, the fixture writes them through the web auth module's own store code. The
     running server has no test clock.
   `gxfixture`'s own tests (`testutil/gxfixture/main_test.go`) change from the token URL to the
   cookie.

## Verification

A verify ticket runs these against a temporary gx server from `gxfixture`. "Go test (seam N)" names
the seam from Testing Decisions; the verify part reruns it. New flows take the next free numbers in
`webui/e2e/CATALOG.md` from 83 on; the implementer assigns them. "List", "revoke", "change password",
"reset authenticator" and "new root" below mean the CLI commands the implementer names.

New scenarios to write: `web-setup`, `web-login`, `web-logins` (Testing seam 5). Every existing flow
must pass on the new sign-in.

New flows to write:

- **Flow S, "First-run setup":** scenario `web-setup`. Open `web_url`; the setup card shows step 1.
  Enter a wrong code; see the wrong-or-expired error. Enter the code from `gx server status`.
  Step 2: a short password shows "At least 12 characters."; a mismatch is refused; a good one
  goes on. Step 3: QR and key show; a wrong code shows the clock error, and `gx server status` still
  says setup is pending; the right code opens the Epics page signed in.
- **Flow Q, "Setup from the QR link":** scenario `web-setup`. Open the link `gx server web-url`
  prints; step 1 has the code filled in.
- **Flow L, "Login":** scenario `web-login`. Open `#/stats`; the login card shows. A wrong code
  shows "Wrong password or code". The right password and code open the Stats page.
- **Flow R, "Login rate limit":** scenario `web-login`. Fail 5 times; see "Too many tries. Try
  again in 1 minutes." (or the implementer's plural) and Sign in disabled.
- **Flow C, "Certificate help":** scenario `web-login`. Unfold "Browser warns about this page?";
  the profile and `.cer` links download with their content types; the fingerprint matches
  `gx server status`.
- **Flow G, "Settings and web logins":** scenario `web-logins`. Open the menu, click Settings. See
  the Security group with "4 devices". Open Web logins: four rows, one "This device", one "Ends in
  20 days unless used". Open another row; rename it; the list shows the new name.
- **Flow V, "Sign out another device":** scenario `web-logins` with a second browser context signed
  in through login. From the first, open the second's web login, sign out, confirm. The second
  page shows login with no reload. The CLI list no longer has it.
- **Flow P, "Change password":** scenario `web-logins`. The note reads "3 other devices will be
  signed out". Change it. The list has one row. A new context logs in with the new password.
- **Flow A, "Reset authenticator":** scenario `web-logins`. Enter password and code; see the new QR,
  key and keep note. Leave the page; the old secret still logs in a new context. Do it again and
  finish with a code from the new secret; the old secret now fails and the list has one row.
- **Flow O, "Sign out of this device":** scenario `web-logins`. Click it on the Settings home; the
  login page shows; the CLI list has three rows.
- **Flow X, "Web reset from the CLI":** scenario `web-logins`. Run `gx server web-reset --yes`; the
  open page shows the setup page with no reload; the printed code finishes setup.
- **Flow Y, "CLI revoke":** scenario `web-logins`. Revoke this device's id from the CLI; the page
  shows login with no reload.

CLI and server checks use `curl --cacert <stateDir>/web-tls/<root file>` against the fixture.

1. Flow S, Flow L. Go test (seam 1): a request without the cookie gets 401 or the login page.
2. Go test (seam 1): no config key turns TLS off. Shell check: `rg` finds no such key in
   `docs/config-schema.json`.
3. Server check: `curl -s -o /dev/null -w '%{http_code} %{redirect_url}' http://<addr>/stats`
   prints a 3xx and `https://<addr>/stats`. Go test (seam 1).
4. Server check: the same port answers both the redirect and HTTPS. Go test (seam 1).
5. Go test (seam 1): a request to the IP and a request to an allowed name each work with no
   redirect, and each has its own cookie.
6. Go test (seam 1): a connection from a public address is closed before TLS.
7. Go test (seam 1): each listed range is allowed (table).
8. Go test (seam 1): `::ffff:10.0.0.5` is allowed and `::ffff:8.8.8.8` is refused.
9. Go test (seam 1): `X-Forwarded-For: 10.0.0.1` from a public peer is still refused.
10. Go test (seam 1): two refused connections in a minute write one log line.
11. Server check: `curl -H 'Host: evil.example'` answers 403. Go test (seam 1).
12. Go test (seam 1): a write with a wrong Origin or a form content type answers 403, also on the
    login and setup routes.
13. Go test (seam 1): binding `0.0.0.0` logs no warning.
14. CLI check: with no `server.web.listen`, `gx server web-url` exits non-zero with "the web UI is
    off". Go test (seam 3).
15. Shell check: `<stateDir>/web-tls/` exists after the fixture starts. Go test (seam 2).
16. Go test (seam 2): the root's constraints; a leaf for `example.com` signed by it fails `Verify`.
17. Server check: `openssl s_client -connect <addr> -servername localhost` shows a SAN with
    `localhost` and `127.0.0.1`. Go test (seam 2).
18. Go test (seam 1): a config reload that adds a host serves a leaf with it, from the same root.
    Go test (seam 1): a connection on a local IP the leaf lacks gets a leaf with it.
19. Go test (seam 1): a leaf 29 days from expiry is re-issued at start.
20. Go test (seam 1): an allowed host outside the constraints is not in the leaf, and `gx.log` and
    the web info route carry the warning. CLI check: `gx server status` prints it. Go test (seam 3).
21. CLI check: new root with `--json` succeeds; `gx server status` prints a new fingerprint; the old
    `curl --cacert` fails. Go test (seam 3).
22. Flow C. Server check: `curl -I` on the profile path without a cookie shows `200` and
    `application/x-apple-aspen-config`. Go test (seam 2): the plist parses with one root payload.
23. Flow C. Server check: `curl -I` on the `.cer` path shows `application/pkix-cert`.
24. Flow C. Mock states login-trust-open-phone, settings-cert-phone. CLI check: `gx server status`
    prints the same 8-group fingerprint.
25. Flow C. Mock states login-default-phone, login-trust-open-phone, setup-code-phone.
26. Shell check: `stat` on each file in `<stateDir>/web-tls/` shows mode 0600.
27. Flow S. Server check: in `web-setup`, `curl` of `/v1/snapshot` answers 401. Go test (seam 1).
28. CLI check: in `web-setup`, `gx server status` and `gx server web-url` print the code. Go test
    (seam 3).
29. Go test (seam 2): the alphabet has no `0 O 1 I L`; the code is 8 characters.
30. Go test (seam 1): a used code fails, an hour-old code fails, a restart changes the code.
31. Flow Q. CLI check: `gx server web-url --qr` prints a QR block and a link with `#/setup?code=`.
32. Go test (`ui/app` About test): with setup pending, the About dialog reads "Web UI: setup needed,
    run `gx server web-url --qr`" and holds no code.
33. Flow S. Mock states setup-code-phone, setup-code-desktop, setup-password-phone,
    setup-totp-phone.
34. Flow S. Mock state setup-code-expired-phone.
35. Flow S. Mock state setup-password-phone. Vitest (seam 4): mismatch and short password.
36. Flow S. Mock state setup-totp-phone.
37. Flow S. Mock state setup-totp-error-phone. Go test (seam 1): a wrong finish code saves nothing.
38. Flow S (ends signed in). Go test (seam 1): the finish response sets the cookie and the list has
    one web login.
39. Go test (seam 1): 5 wrong codes lock the IP for logins too.
40. Flow L. Server check: `curl` of `/` with no cookie answers `200 text/html`. Go test (seam 1).
41. Flow L. Mock states login-default-phone, login-default-desktop.
42. Vitest (seam 4): the code input is numeric with `autocomplete="one-time-code"`.
43. Flow L. Mock state login-error-phone. Go test (seam 1): a wrong password and a wrong code give
    the same refusal reason.
44. Go test (seam 1, seam 2): a reused code is refused.
45. Go test (seam 2): codes one step back and ahead pass; two steps fail.
46. Flow R. Go test (seam 1): 5 failures, then 1 minute, then 2 after the next round, capped at 1
    hour; a success clears the count.
47. Go test (seam 1): 50 failures from many IPs lock every IP for 15 minutes.
48. Flow R. Mock state login-limited-phone.
49. Flow L. Mock state login-default-phone (reset-hint part).
50. Flow L (lands on Stats).
51. Flow V, Flow Y. Vitest (seam 4): a 401 from the fetch helper and from the event stream shows
    login.
52. Vitest (seam 4): a 401 and sign out clear `gx.page.*` and the snapshot cache.
53. Flow V.
54. Go test (seam 1): the `Set-Cookie` attributes.
55. Go test (seam 1): a login used on day 89 stays; one unused for 90 days gets 401; `Max-Age`
    renews.
56. Go test (seam 1): a restarted server accepts the old cookie.
57. Go test (seam 1): the state file has no cookie value, only its SHA-256.
58. Go test (seam 2): the user-agent table. Flow G (default names).
59. Go test (seam 1): two logins from one IP make two web logins.
60. Flow G. Vitest (seam 4): the menu has "Settings".
61. Flow G. Mock states settings-home-phone, settings-home-desktop.
62. Flow G. Mock states settings-logins-phone, settings-login-phone.
63. Flow G. Mock states settings-logins-phone, settings-logins-desktop.
64. Flow G. Vitest (seam 4): 29 days left shows the warning, 31 does not.
65. Flow G. Mock state settings-login-phone.
66. Flow G. Go test (seam 1): rename is stored and logged.
67. Flow V. Mock state settings-login-revoke-phone.
68. Flow V. Go test (seam 1): revoke ends that web login's open event stream.
69. Flow P. Mock state settings-password-phone. Go test (seam 1): a wrong current password or code
    is refused.
70. Flow A. Mock states settings-totp-phone, settings-totp-new-phone.
71. Flow A. Go test (seam 1): an abandoned reset leaves the old secret working.
72. Flow P, Flow A. Go test (seam 1): only the current web login is left.
73. Go test (seam 1): 5 wrong current passwords on change password lock the IP for login.
74. Mock state settings-cert-phone. Flow C (downloads).
75. Flow O. Vitest (seam 4): the device's own sub-page has "Sign out this device".
76. CLI check: `gx server status` in `web-setup` prints `web: https://…`, the code and the
    fingerprint; in `web-logins` it prints no code. Go test (seam 3).
77. CLI check: `gx server status --json` has `web_setup_code` in `web-setup` and not in
    `web-logins`; `web_url` has no query. Go test (seam 3).
78. CLI check: `gx server web-url` in `web-logins` prints the plain URL.
79. Flow X. CLI check: `gx server web-reset` with `y` on a terminal prints a new code; with `n`
    changes nothing. Go test (seam 3).
80. CLI check: `gx server web-reset < /dev/null` exits non-zero and changes nothing; `--yes --json`
    succeeds. Go test (seam 3).
81. CLI check: list `--json` in `web-logins` returns 4 entries with id, name, created, last used,
    last IP.
82. Flow Y. CLI check: revoke with an unknown id prints a refusal with its reason.
83. Go test (seam 3): change password asks twice with no echo (pty), and refuses a mismatch.
84. CLI check: `printf 'a-new-password-1\n' | gx server <change password> --password-stdin --json`
    succeeds and a new context logs in with it (or the implementer's flag).
85. Go test (seam 3): reset authenticator prints a QR and key, a wrong code saves nothing, the right
    code computed from the printed key saves.
86. CLI check: after change password, list shows 0 web logins. Go test (seam 3) for both commands.
87. CLI check: each new command against a stopped fixture exits with `server-not-running`. Go test
    (seam 3): "the web UI is off" with the listener off. Go test (seam 3): each refuses on the TCP
    listener.
88. Go test (seam 3): a CLI revoke writes the same log event as a web revoke, and ends the stream.
89. Go test (seam 1): each event writes one `gx.log` line with IP and name.
90. Go test (seam 1): after a full setup, login, password change and reset, `gx.log` holds none of
    the password, codes, secret, cookie or Setup code.
91. Go test (seam 1): a start with `web-token` present deletes it. CLI check: `gx server web-token`
    exits with "unknown command".
92. Server check: `curl` of `/?gx_token=abc` with cookie `gx_web=x` answers the HTML page and sets
    no cookie. Flow L covers the page.
93. Shell check: `docs/web-ui-security.md` has the allowed-hosts upgrade note.
94. Server check: `curl -I /` shows `Cache-Control: no-cache` and an `ETag`; `If-None-Match` gets
    304. Go test (seam 1).
95. Server check: `curl -I` of an asset named in `index.html` shows `max-age=31536000, immutable`.
96. Server check: `curl -I /assets/missing.js` and `/stats` answer 404. Go test (seam 1).
97. Vitest (seam 4): a throwing child shows the error card with Reload.
98. Vitest (seam 4): a thrown error and an unhandled rejection post one report. Go test (seam 1):
    the report lands in `gx.log`.
99. Go test (seam 1): the report route accepts a request with no cookie, refuses one over the size
    cap, and drops reports past the per-IP limit.
100. Every existing flow passes with `make web-e2e`. Go test (`testutil/gxfixture`): the JSON has a
     cookie that opens `/v1/snapshot`.
101. Flows S, L and G run on the three new scenarios.
102. Go test (seam 3): the web-only route test fails when a web-only route is added to the mux
     without the list. Shell check: ADR 0028 has the amendment.
103. Go test (seam 1): with `server.web.dev-url` set to a stub on loopback, a module request is
     proxied over HTTPS without a cookie and an API request still needs one.

**Mock states** (each line: mock path, state, width, fixture scenario, parts):

- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  login-default-phone, 390px, `web-login`, parts: header, card, heading, password, code, submit,
  reset-hint, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  login-default-desktop, 1280px, `web-login`, parts: header, card, heading, password, code, submit,
  reset-hint, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  login-error-phone, 390px, `web-login` after one wrong code, parts: header, card, heading, error,
  password, code, submit, reset-hint, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  login-limited-phone, 390px, `web-login` after 5 wrong codes, parts: header, card, heading, error,
  password, code, submit, reset-hint, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  login-trust-open-phone, 390px, `web-login` with the help unfolded, parts: header, card, heading,
  password, code, submit, reset-hint, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  setup-code-phone, 390px, `web-setup`, parts: header, card, step-header, heading, intro,
  setup-code, submit, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  setup-code-desktop, 1280px, `web-setup`, parts: header, card, step-header, heading, intro,
  setup-code, submit, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  setup-code-expired-phone, 390px, `web-setup` after a wrong code, parts: header, card, step-header,
  heading, intro, error, setup-code, submit, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  setup-password-phone, 390px, `web-setup` at step 2, parts: header, card, step-header, heading,
  password, password-again, submit, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  setup-totp-phone, 390px, `web-setup` at step 3, parts: header, card, step-header, heading,
  totp-qr, totp-key, code, submit, save-note, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  setup-totp-error-phone, 390px, `web-setup` at step 3 after a wrong code, parts: header, card,
  step-header, heading, totp-qr, totp-key, error, code, submit, save-note, trust-help.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-home-phone, 390px, `web-logins`, parts: title, security, sign-out.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-home-desktop, 1280px, `web-logins`, parts: title, security, sign-out.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-logins-phone, 390px, `web-logins`, parts: back, title, lead, login-list.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-logins-desktop, 1280px, `web-logins`, parts: back, title, lead, login-list.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-login-phone, 390px, `web-logins` on another device's web login, parts: back, title,
  login-facts, name, sign-out.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-login-revoke-phone, 390px, `web-logins` after "Sign out this device", parts: back,
  title, login-facts, name, sign-out, confirm, note.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-password-phone, 390px, `web-logins`, parts: back, title, current-password, code,
  new-password, new-password-again, note, submit.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-totp-phone, 390px, `web-logins`, parts: back, title, current-password, code, note,
  submit.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-totp-new-phone, 390px, `web-logins` after the password and code check, parts: back,
  title, totp-qr, totp-key, code, submit, keep-note.
- `$(gx tickets root)/gx/webui-security/mocks/06-login-setup-and-settings-ui.html`,
  settings-cert-phone, 390px, `web-logins`, parts: back, title, downloads, fingerprint.

The three fixture scenarios do not exist yet; they are listed under the new scenarios above.

## Self-decided design questions

These came up after the two spec rounds were used up.

- **Which listeners serve the account socket routes?** Choice: the unix socket only; the loopback
  TCP listener refuses them. Reason: only the 0600 socket proves a local shell. Loopback TCP answers
  any page in a browser on the same machine, so it would let a hostile page reset the password.
- **How does the CLI reset authenticator fit one route?** Choice: the CLI makes the secret with the
  shared TOTP code and sends secret plus code in one request. Reason: one route and one verb, no
  pending state on the server, and the secret never leaves the machine.
- **Where does the QR setup link point?** Choice: `/#/setup?code=…`. Reason: `index.html` stays only
  at `/` (ticket 01), and the code never reaches server logs or a `Referer`.
- **Where does a plain-HTTP request on a wrong Host go?** Choice: 403, no redirect. Reason: a
  redirect to any Host would be an open redirect.
- **What does the root permit for a single-label host?** Choice: the exact name. Reason: it has no
  parent domain, and permitting nothing would leave the host out.
- **How does the web UI handle 401?** Choice: one shared fetch helper for every API call, plus the
  event stream. Reason: per-call checks miss calls. One place puts the rule where it belongs.
- **When is the Setup code consumed?** Choice: checked at step 1, consumed at finish; the first
  device to finish wins. Reason: "saves nothing until the TOTP check passes" means a person who
  quits at step 3 can start again with the same code within the hour.
- **Does a success clear an IP's failure count?** Choice: yes. Reason: one typo a week must not add
  up to an hour-long lockout.
- **Do the web login CLI commands work with the listener off?** Choice: no, they fail with "the web
  UI is off", like `web-url`. Reason: one rule for every web command.
- **How does the CLI get the Setup code?** Choice: a socket read route whose verb is `gx server
  web-url` (gaining `--json`). The handshake carries only a setup-pending flag for the TUI. Reason: a
  secret has no place in the handshake every client reads, and `status` can call the same route.
- **What does the error boundary look like?** Choice: the login card's frame with one line and a
  Reload button. Reason: the mock has no state for it, and this reuses an approved look.
- **How does the fixture make an idle web login?** Choice: it writes the web logins through the
  store code before the server starts. Reason: the running gx binary has no test clock, and a clock
  flag in product code would be a test bypass.
- **How do the web-only routes refuse?** Choice: ADR 0028's refusal results. Reason: clients already
  switch on `reason`; one contract for the whole API.

## Out of Scope

- Multi-user accounts and roles. gx has one person with many devices.
- Any access from the public internet, including a reverse proxy or a tunnel.
- Let's Encrypt or other public certificates. A separate effort if the self-made root causes
  trouble.
- The rest of the Settings page (config editor, server restart). A new map epic.
- Passkeys. Possible later on top of password plus TOTP.
- A readable audit log in the UI. The web login list plus `gx.log` is enough.
- Rename from the CLI.
- A PWA manifest or service worker (ticket 01: neither changes how iOS stores or sends cookies).
- An Origin check on the loopback TCP listener for the other socket routes. This spec only keeps the
  account routes off it. File a follow-up for the rest.

## Further Notes

- The two research files explain the iOS causes and the Apple trust rules behind the cookie, cache
  and certificate decisions. Confidence differs per symptom: high for the 401 on refresh, medium for
  the lost cookie, low for the blank page. The error report is how the next blank page gets a
  cause.
- iOS needs a manual step after installing the profile: Settings > General > About > Certificate
  Trust Settings, turn on the root. Safari's "visit anyway" does not carry over to the home-screen
  app, so full trust is needed.
- The root's DNS constraint is fixed when the root is made. A domain added later needs the new-root
  command and a re-trust on every device. The upgrade note asks for allowed hosts first for this
  reason.
- Terms: Web login and Setup code are in `GLOSSARY.md`. Never call a web login a session.
