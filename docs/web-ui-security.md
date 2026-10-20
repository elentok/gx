# Web UI security

## Plain HTTP risk

The web UI listener speaks plain HTTP on your Tailscale or LAN address. It has no TLS.

- Anyone who can sniff the traffic can read the auth cookie or the `?t=` link.
- A sniffed cookie or `?t=` link gives full control of the UI.
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
