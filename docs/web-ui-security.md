# Web UI security

## Plain HTTP risk

The web UI listener speaks plain HTTP on your Tailscale or LAN address. It has no TLS.

- Anyone who can sniff the traffic can read the auth cookie or the `?gx_token=` link.
- A sniffed cookie or `?gx_token=` link gives full control of the UI.
- That includes answering permission approvals and sending instructions to a full-permission agent.

Run the UI only on networks you trust. TLS and token expiry are tracked in `gx:webui-security`.

## Upgrade note: hostnames need allowed-hosts

The listener now checks the `Host` header. If you open the UI by a MagicDNS name or any other
hostname, add that name to `server.web.allowed-hosts` before you upgrade. Otherwise the UI answers
HTTP 403.

```json
{ "server": { "web": { "allowed-hosts": ["my-laptop.tailnet.ts.net"] } } }
```

IP addresses and `localhost` are unaffected.

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
