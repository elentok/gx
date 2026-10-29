# iOS and macOS trust for a root certificate gx makes itself

Research for ticket `gx:webui-security/02`. The question is what gx must produce so that iOS and macOS Safari fully trust a root certificate gx makes on first run, and the server certificate that root signs.

Each claim is tagged. **Verified** means a primary source says it, or Apple's trust code does it. **Inference** means it follows from verified facts but no source states it. **Anecdotal** means community reports only.

Apple trust code references are to `apple-oss-distributions/Security` at tag `Security-61901.120.67` (latest commit `db15acbe`, 2025-10-16). This is the `trustd` source that iOS and macOS ship. Files cited:

- `trust/trustd/SecPolicyServer.c` (the per-check code)
- `OSX/sec/Security/SecPolicy.c` (which checks the TLS server policy turns on)
- `OSX/sec/Security/SecPolicyLeafCallbacks.c` (host name matching)
- `trust/trustd/nameconstraints.c`
- `trust/trustd/SecCertificateSource.c`

## Summary

- The root gets ECDSA P-256 with an ECDSA-SHA256 signature. Set BasicConstraints to `CA:true`, critical, with path length 0. Set KeyUsage to `keyCertSign | cRLSign`, critical. Validity is 10 years. Add name constraints that limit the root to the names and private IP ranges gx can serve.
- The leaf (server certificate) gets ECDSA P-256 with ECDSA-SHA256 and `CA:false`. KeyUsage is `digitalSignature`, critical. EKU is `serverAuth`. The SAN lists every DNS name as a `dNSName` and every IP as an `iPAddress` entry. Validity is at most 397 days; 365 days with renewal works. Apple's code rejects leaves over 825 days under a user root.
- gx delivers the root as an unsigned `.mobileconfig` with one `com.apple.security.root` payload, served as `Content-Type: application/x-apple-aspen-config`.
- On the device, the person opens the profile link in Safari, then goes to Settings > Profile Downloaded > Install. Then they go to Settings > General > About > Certificate Trust Settings and turn on the root. This happens once per device.
- When the IP or name list changes, gx re-issues only the leaf, signed by the same root key. Devices do nothing. Only a new root key, or a name outside the root's name constraints, needs a new profile and new trust on every device.
- Go's standard library covers all of it: `crypto/ecdsa`, `crypto/x509`, `crypto/tls` (`GetCertificate` to swap the leaf live), `encoding/base64`, `encoding/pem` and `crypto/rand`. The plist is plain XML written with `fmt` or `text/template`. No new dependency is needed.

## 1. Profile format, content type, device steps

### Format: `.mobileconfig` with a root payload

- **Verified.** A configuration profile is an XML property list. The top level needs `PayloadContent` (array), `PayloadIdentifier`, `PayloadType` (`Configuration`), `PayloadUUID` and `PayloadVersion` (1). `PayloadDisplayName`, `PayloadDescription` and `PayloadOrganization` are optional and show on the detail screen. Source: Apple Device Management, [TopLevel](https://developer.apple.com/documentation/devicemanagement/toplevel).
- **Verified.** The root payload type is `com.apple.security.root`. Its `PayloadContent` is the certificate as `<data>` (base64 of the DER bytes). `PayloadCertificateFileName` is optional. It allows manual install on iOS and macOS. Apple's page includes a full example profile. Source: [CertificateRoot](https://developer.apple.com/documentation/devicemanagement/certificateroot).
- **Verified.** Each payload also carries `PayloadType`, `PayloadVersion`, `PayloadIdentifier` and `PayloadUUID`. Source: [Configuring multiple devices using profiles](https://developer.apple.com/documentation/devicemanagement/configuring-multiple-devices-using-profiles).
- **Verified.** "The system uses this string [PayloadIdentifier] to determine whether to replace an existing profile or add it as a new profile." Source: [TopLevel](https://developer.apple.com/documentation/devicemanagement/toplevel). So gx should keep one stable identifier, for example `com.elentok.gx.root.<root-fingerprint-prefix>`, and a fresh `PayloadUUID` each time it builds the file.
- **Inference.** A bare `.crt` served as `application/x-x509-ca-cert` also installs on iOS. Apple's own old profile-server sample serves the root that way (see below). The `.mobileconfig` is the better choice. gx controls the display name and description, and the stable `PayloadIdentifier` lets a later profile replace the old one instead of piling up.

### Signing

- **Verified.** Signing is optional. "Signing a profile guarantees data integrity. To sign a profile, place the XML property list in a DER-encoded, CMS Signed Data structure." Source: [Configuring multiple devices using profiles](https://developer.apple.com/documentation/devicemanagement/configuring-multiple-devices-using-profiles). A replacement profile must be signed by the same identity as the one it replaces, so starting unsigned and signing later causes trouble.
- **Anecdotal.** iOS labels an unsigned profile "Not Verified" (orange or red) on the install screen, but it still installs. A signed profile from a trusted identity shows "Verified". Sources: [Apple Developer Forums 774588](https://developer.apple.com/forums/thread/774588), [8731](https://developer.apple.com/forums/thread/8731). Signing with gx's own root would not show "Verified" before the root is trusted, so it gains nothing. gx should ship unsigned, and the setup page should say to expect "Not Verified".
- Go's standard library has no CMS (PKCS #7) signer. That is one more reason to stay unsigned.

### Content type

- **Verified.** Apple's profile-server guide sets the response type to `application/x-apple-aspen-config` "so Safari on iOS treats the response as a configuration profile". Its root-certificate handler uses `application/x-x509-ca-cert`. Source: [Creating a Profile Server for Over-The-Air Enrollment and Configuration](https://developer.apple.com/library/archive/documentation/NetworkingInternet/Conceptual/iPhoneOTAConfiguration/profile-service/profile-service.html) (archived Apple doc).
- **Anecdotal.** A misspelled type makes Safari show the raw XML. Source: [Apple Community 1651878](https://discussions.apple.com/thread/1651878).
- **Inference.** Also send `Content-Disposition: attachment; filename="gx-root.mobileconfig"`. Then macOS browsers save it with the right extension.

### Steps on iPhone or iPad

1. The person opens the gx setup page in Safari and taps the profile link. iOS says the profile was downloaded.
2. Settings > "Profile Downloaded" > Install (top right), then follow the prompts. **Verified.** An unopened downloaded profile is deleted after 8 minutes. Only one downloaded profile waits at a time; a second download replaces the first. Stolen Device Protection may block the install away from a familiar location. Source: [Apple support 102400](https://support.apple.com/en-us/102400).
3. Settings > General > About > Certificate Trust Settings > under "Enable full trust for root certificates", turn on the gx root. **Verified.** A manually installed root "isn't automatically trusted for SSL", and profiles from email or a website need this step. Only MDM or Apple Configurator installs skip it. Source: [Apple support 102390](https://support.apple.com/en-us/102390).

### Steps on a Mac

- **Verified.** Download and open the profile. Then go to System Settings > General > Device Management, double-click it under Downloaded, and click Install. It may ask for the password. Source: [Mac User Guide, configuration profiles](https://support.apple.com/guide/mac-help/configuration-profiles-standardize-settings-mh35561/mac).
- **Verified.** In Keychain Access, double-click the certificate, open Trust, and pick the trust setting ("Always Trust" for SSL). Source: [Keychain Access User Guide](https://support.apple.com/guide/keychain-access/change-the-trust-settings-of-a-certificate-kyca11871/mac).
- **Inference, not tested.** A root installed by a profile on macOS may already count as trusted with no Keychain step. Test it on a real Mac. The setup page can tell the person to check Keychain Access if Safari still warns.

### The first fetch

- **Inference.** The listener is HTTPS-only, so the first visit, before trust, shows Safari's "This connection is not private" page. The person must tap through once to reach the setup page and download the profile. That download is open to a man in the middle. gx should print the root's SHA-256 fingerprint in its terminal output and on the setup page. iOS shows the fingerprint in the profile's More Details screen, so the person can compare them before trusting. A plain-HTTP bootstrap port would avoid the warning but break "HTTPS only". Keep HTTPS only.

## 2. Apple's rules for certificates under a user-trusted root

Apple's support page says "All TLS server certificates must comply" with the iOS 13 rules, with no exemption for user roots. Source: [Apple support 103769](https://support.apple.com/en-us/103769). The trust code shows which rules really skip user roots.

| Rule | Applies under a user-trusted root? | Source |
|---|---|---|
| RSA ≥ 2048 bits, EC ≥ P-256, for every cert in the chain | **Yes.** It is skipped only when the leaf itself is the user anchor. | 103769; `SecPolicy.c` `SecPolicyAddStrongKeySizeOptions`; `SecPolicyServer.c` `SecPolicyCheckKeySize` |
| No MD2, MD4, MD5 or SHA-1 signatures | **Yes** for the leaf and any intermediates. A user-trusted root's own signature is not checked. | 103769; `SecPolicyRemoveWeakHashOptions`; `SecPolicyCheckSignatureHashAlgorithms` |
| Host name only from SAN. CN is ignored. | **Yes.** | 103769; `SecPolicyCheckCertSSLHostname` reads only SAN `dNSName` and `iPAddress` |
| EKU extension must exist and contain `serverAuth` | **Skipped** for user and admin roots. The looser rule still applies: an EKU, if present, must contain `serverAuth` or `anyExtendedKeyUsage`. | `SecPolicyCheckServerAuthEKU` ("skip user/admin-anchored chains"); `set_ssl_ekus` |
| Leaf validity ≤ 825 days (NotBefore after 2019-07-01) | **Yes.** The code skips it only when the leaf itself is the user anchor. Under a user root it uses the "all other trust" branch. | 103769; `SecPolicyCheckValidityPeriodMaximums`, `check_other_trust_ssl_validity_maximums` |
| Leaf validity ≤ 398 days | **No.** It applies only to chains to roots preinstalled by Apple. | [Apple support 102028](https://support.apple.com/en-us/102028): "This change will not affect certificates issued from user-added or administrator-added Root CAs"; `check_system_trust_ssl_validity_maximums` |
| Certificate Transparency | **No.** It applies to "publicly trusted" certificates. | [Apple support 103214](https://support.apple.com/en-us/103214); `kSecPolicyCheckSystemTrustedCTRequired` |
| CA/B Forum 200, 100 and 47-day leaf limits (2026-03-15, 2027-03-15, 2029-03-15) | **No, as of this source.** These are Baseline Requirements for publicly trusted CAs. `trustd` has no check for them. The 102028 page lists only the 398-day rule. | [CA/B Forum TLS BR](https://github.com/cabforum/servercert/blob/main/docs/BR.md) §1.2.2 and §6.3.2; trustd source |
| Apple Root Program policy (ECDSA P-384 or RSA 4096 for new roots, EKU on sub-CAs from 2026-08-01) | **No.** It binds CA owners in Apple's program, not local roots. | [apple-root-program/policy.md](https://github.com/apple/apple-root-program/blob/main/policy.md) |
| Root validity limit | **None.** Validity maximums check only the leaf (index 0). Every cert must still be within its own NotBefore and NotAfter dates. | `SecPolicyCheckValidityPeriodMaximums`; `SecPolicyAddBasicX509Options` (`kSecPolicyCheckTemporalValidity`) |
| BasicConstraints on the root | Not enforced by `trustd` (`SecPolicyCheckBasicConstraints` is an empty function). | `SecPolicyServer.c` |

**Anecdotal, consistent with the 825-day rule.** On iPadOS 18.3.2, Safari refused a 30-year Pi-hole certificate. A self-hosted Bitwarden with a 20-year CA and a 10-year server certificate failed on iPhone, and re-issuing the server certificate for 365 days fixed it. Source: [Pi-hole discourse 79235](https://discourse.pi-hole.net/t/apple-clients-do-not-trust-tls-certificates-with-long-validity-periods/79235).

Choices for gx:

- **Key type: ECDSA P-256 for both.** It meets Apple's EC minimum, it is the Go default path, and the keys and handshakes are small. **Inference.** Avoid Ed25519. Go can make Ed25519 certificates, but Apple's TLS key-size policy only names RSA and EC, and Safari does not accept Ed25519 server certificates.
- **Signature: ECDSA with SHA-256.** **Verified.** Go picks `ECDSAWithSHA256` for a P-256 signer when `SignatureAlgorithm` is unset (`crypto/x509/x509.go`, `signingParamsForKey`).
- **Leaf validity: ≤ 397 days.** 825 is Apple's hard limit under a user root. 397 also meets the 398-day rule, in case Apple later drops the user-root exemption. gx re-issues automatically, so a shorter life costs nothing. Renew when fewer than 30 days remain, and on any SAN change. Backdate NotBefore by 1 hour to cover clock skew.
- **EKU: `serverAuth` only.** Apple skips the strict EKU check for user roots, but adding it costs nothing, and Go's own `x509.Verify` and other clients require it.
- **Root: BasicConstraints `CA:true`, path length 0, critical. KeyUsage `keyCertSign | cRLSign`, critical.** `trustd` does not require them, but RFC 5280 does and so do Go and other clients. Path length 0 stops the root from signing a sub-CA. **Inference.** The Certificate Trust Settings toggle lists "root certificates", so the root must be self-signed.
- **Root validity: 10 years.** Apple sets no limit. Rotating the root means re-trusting on every device, so a long life is the point.

## 3. One leaf for IPs and names

- **Verified.** iOS matches the host against SAN `dNSName` entries first. If the host is an IPv4 or IPv6 literal, it compares the address bytes against SAN `iPAddress` entries (`SecPolicyLeafCallbacks.c`, `SecPolicyCheckCertSSLHostname`). So one leaf can list `gertsel.elentok.com`, each `server.web.allowed-hosts` name, `localhost`, `127.0.0.1`, `::1`, the LAN IPs and the tailnet IPs together. IPs must go in `iPAddress` entries, not as text in `dNSName`.
- **Verified.** Go writes SAN entries from `Certificate.DNSNames` and `Certificate.IPAddresses` (`go doc crypto/x509.CreateCertificate`).
- **Inference.** When the list changes, gx makes a new leaf with the same root key and serves it. iOS builds the chain from the leaf to the trusted anchor on every connection, so the device needs no action. Only these changes force a new profile and new trust on every device:
  - a new root key, after loss, compromise or expiry;
  - a new name or IP outside the root's name constraints, if gx sets them (see section 5).
- **Inference, gx wiring.** `requireHost` (`server/webhost.go`) reads `server.web.allowed-hosts` on every request, and a config reload changes it without a listener restart. So the leaf must follow the reload. Keep the current `*tls.Certificate` in an `atomic.Pointer` and return it from `tls.Config.GetCertificate`. **Verified.** `GetCertificate` is called on each handshake (`go doc crypto/tls.Config.GetCertificate`), so a swapped leaf applies to new connections with no restart. Re-check the machine's IPs at start and on reload. For IPs that change at runtime, a periodic check (every few minutes) or re-issuing when a ClientHello arrives for an unknown address works. `ClientHelloInfo.Conn.LocalAddr()` gives the local IP the client reached.
- **Inference.** Store the root key and cert, and the leaf key and cert, under gx's state dir with mode 0600. The root private key is the asset to guard. Anyone holding it can sign certificates that every trusting device accepts.

## 4. Go standard library

No new dependency is needed.

- `crypto/ecdsa.GenerateKey(elliptic.P256(), rand.Reader)` makes both keys.
- `crypto/x509.CreateCertificate(rand, template, parent, pub, priv)` makes both certificates. The root uses `template == parent` (self-signed). **Verified** from `go doc crypto/x509.CreateCertificate` (go 1.27.1, module go 1.25.8):
  - It uses `IsCA`, `BasicConstraintsValid`, `MaxPathLen` and `MaxPathLenZero`, `KeyUsage`, `ExtKeyUsage`, `DNSNames`, `IPAddresses`, `PermittedDNSDomains`, `PermittedDNSDomainsCritical`, `PermittedIPRanges`, `ExcludedDNSDomains`, `ExcludedIPRanges`, `NotBefore`, `NotAfter`, `Subject`, `SerialNumber` and `SignatureAlgorithm`.
  - It generates `SubjectKeyId` for a CA from the public key hash, and copies `AuthorityKeyId` from the parent.
  - A nil `SerialNumber` gets a random RFC 5280 serial.
- `crypto/x509.MarshalECPrivateKey` or `MarshalPKCS8PrivateKey` with `encoding/pem` handles storage. `crypto/tls.X509KeyPair` loads them back.
- `crypto/tls.Config{GetCertificate: ..., MinVersion: tls.VersionTLS12}` with `http.Server.ServeTLS` or `tls.NewListener` serves HTTPS.
- `crypto/sha256` makes the fingerprint shown to the person.
- The `.mobileconfig` is plain XML. Write it with `text/template` (or `fmt`), and put `base64.StdEncoding.EncodeToString(rootDER)` inside `<data>`. Escape the display strings with `xml.EscapeText` (or `html/template`). Make the UUIDs from `crypto/rand` (16 bytes, version-4 bits set, formatted 8-4-4-4-12). Apple's [CertificateRoot](https://developer.apple.com/documentation/devicemanagement/certificateroot) example shows the exact shape to copy.

Profile skeleton (from Apple's example, trimmed):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>PayloadCertificateFileName</key><string>gx-root.cer</string>
      <key>PayloadContent</key><data>BASE64-DER-OF-ROOT</data>
      <key>PayloadDisplayName</key><string>gx root (gertsel)</string>
      <key>PayloadIdentifier</key><string>com.elentok.gx.root.ABCD1234.cert</string>
      <key>PayloadType</key><string>com.apple.security.root</string>
      <key>PayloadUUID</key><string>RANDOM-UUID-1</string>
      <key>PayloadVersion</key><integer>1</integer>
    </dict>
  </array>
  <key>PayloadDisplayName</key><string>gx web UI (gertsel)</string>
  <key>PayloadDescription</key><string>Lets this device trust gx's HTTPS certificate. Fingerprint SHA-256 ...</string>
  <key>PayloadIdentifier</key><string>com.elentok.gx.root.ABCD1234</string>
  <key>PayloadType</key><string>Configuration</string>
  <key>PayloadUUID</key><string>RANDOM-UUID-2</string>
  <key>PayloadVersion</key><integer>1</integer>
</dict>
</plist>
```

## 5. Home-screen web app, service workers, name constraints, other catches

### Home-screen web app and service workers

- **Inference, no Apple doc found.** Certificate Trust Settings sets trust in the system trust store. The code reads it from `kSecTrustStoreDomainUser` in `SecCertificateSource.c`, `SecUserAnchorSource`. Every WebKit client, including Safari, home-screen web apps and `WKWebView`, evaluates the server through the same `trustd`. So a trusted root works the same in a home-screen web app and in service worker fetches. The weak spot is the opposite case. A "visit this website anyway" exception in Safari covers only that Safari page. It does not cover the home-screen app or a service worker's script fetch. So gx must get the person to full trust, not tap-through.
- **Verified, minor.** WebKit bug 206403 (fixed 2020) was about Safari not showing certificate details for pages a service worker served. It was a display issue, not a trust failure. Source: [WebKit bug 206403](https://bugs.webkit.org/show_bug.cgi?id=206403).
- **Inference.** A home-screen web app has its own storage and cookies, separate from Safari. The person logs in again inside the app. That is unrelated to certificates.

### Name constraints on the root

- **Verified.** iOS and macOS honor name constraints on a trusted anchor. `SecPolicyServer.c` starts the permitted and excluded subtrees from the anchor's own constraints (`if (is_anchor_trusted && anchor_permitted_subtrees)`). `nameconstraints.c` supports `dNSName`, `iPAddress`, `rfc822Name`, `directoryName` and URI forms. A critical constraint with an unsupported form fails the chain, as RFC 5280 requires.
- **Verified.** RFC 5280 §4.2.1.10 says a constraint limits only the name forms it lists. A root with only DNS constraints still lets its leaves carry any IP. Constrain both forms. CA/B Forum's BR also uses `permittedSubtrees` this way (BR §7.1.2.10.8).
- **Recommendation: yes, add them.** They limit the damage if the root key leaks. Without them, a stolen key can impersonate any site, such as a bank, to every device that trusts the root. With them, it can only impersonate gx's names and private IPs. Mark the extension critical (`PermittedDNSDomainsCritical: true`). Suggested sets, fixed when gx makes the root:
  - `PermittedDNSDomains`: `localhost`, plus each configured name or its parent domain at root creation. For example, `elentok.com` covers `gertsel.elentok.com` and future hosts under it.
  - `PermittedIPRanges`: `127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `100.64.0.0/10` (Tailscale, [Tailscale KB 1015](https://tailscale.com/kb/1015/100.x-addresses)), `169.254.0.0/16`, `::1/128`, `fc00::/7` (ULA, which holds Tailscale's IPv6 range), `fe80::/10`.
- **Catch.** Constraints are fixed in the root. A later allowed-host outside the permitted domains (for example `gx.otherdomain.net`) would make every device reject the leaf. gx must notice this when it builds the leaf, leave that name out, and warn that adding it means rotating the root and re-trusting devices. That is the cost of the safety. Permitting the parent domain keeps it rare.
- **Inference.** Go's `x509.Verify`, Chrome and Firefox also enforce name constraints, so this choice works the same on other clients.

### Other catches

- **Verified.** The downloaded profile expires after 8 minutes, and only one waits at a time ([102400](https://support.apple.com/en-us/102400)). The setup page should say "install now in Settings".
- **Verified.** The trust toggle is a second, separate step ([102390](https://support.apple.com/en-us/102390)). The most common failure is a person who installs the profile and stops. The setup page should show both steps, and a test link that loads over HTTPS once trust works.
- **Anecdotal.** Installing profiles works from Safari. Other iOS browsers and in-app web views may not open `application/x-apple-aspen-config` (2011 iPhone SDK mailing list). Tell the person to use Safari for setup.
- **Inference.** Leave the CN out, or set it to a plain label. Apple ignores it, and a mismatched CN confuses only people reading the certificate.
- **Inference.** Do not reuse the root key as the leaf key. Keep two keys so the leaf can rotate without touching the root.
