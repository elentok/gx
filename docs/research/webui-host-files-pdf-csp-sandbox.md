# Does a PDF render under `Content-Security-Policy: sandbox allow-scripts`?

Ticket: `gx:webui-host-files/09`. Probe date 2026-11-14.

## Answer

- Chromium: yes, as a top-level page. The built-in PDF viewer renders it with `sandbox allow-scripts`, with `; frame-ancestors 'self'` added, and with `allow-downloads`. Same as the no-CSP baseline.
- Chromium, inside `<iframe sandbox="allow-scripts">`: no. Sad-face error page (`chrome-error://chromewebdata/`). The baseline PDF with no CSP fails the same way, so the cause is the iframe's `sandbox` attribute, not the response header.
- Chromium, `<embed type=application/pdf>` and `<object>` (no sandbox attribute) with the sandbox header on the PDF: renders. A non-sandboxed `<iframe>` renders too.
- WebKit (Playwright build): top-level PDF always downloads, even with no CSP at all. This build has no inline PDF viewer for navigations, so the probe cannot say how real Safari treats the header. `<embed>`/`<object>` render the PDF (also with the sandbox header). A sandboxed iframe stays blank, with or without the header.
- `Content-Disposition: attachment` downloads at top level in both engines (`probe.pdf` download event).
- Real iOS Safari was not tested. Playwright WebKit 27.2 on macOS is not a device.

Outcome for the spec: **Raw stays for PDF.** Chromium blocks nothing at top level. Do not serve PDFs as attachment. Caveat: do not preview a PDF in an `<iframe sandbox>`; in Chromium that never renders. If gx previews a PDF inline, use `<embed>` or `<object>`, or just link to Raw.

## Probe

Files in `docs/research/webui-host-files-pdf-csp-sandbox/`: `probe.mjs` (server + Playwright, both engines), `results.json` (raw output), `shots/` (one screenshot per case).

- Run: `node probe.mjs`. Plain HTTP server on 127.0.0.1, random port. PDF is a generated minimal one-page file with the text `PDFPROBE` (it is clipped by the page width, expected).
- Engines, both **headed** (`launch({headless: false})`): Playwright Chromium (PDF viewer extension `mhjfbmdgcfjbbpaeojofohoefgiehjai` loads) and Playwright WebKit 27.2 macOS. Same Playwright install as ticket 02.
- Header variants: `none` (baseline), `sandbox` (`sandbox allow-scripts`), `sandbox_fa` (`sandbox allow-scripts; frame-ancestors 'self'`), `sandbox_dl` (`sandbox allow-scripts allow-downloads`), `attachment` (`sandbox allow-scripts` plus `Content-Disposition: attachment`).
- Cases: `top` (goto the PDF), `iframe` (same-origin parent page with `<iframe sandbox="allow-scripts">`), `iframe_nosb` (control, no sandbox attribute), `embed`, `object`. Embed, object and iframe_nosb ran only for `none` and `sandbox`.
- How "renders" was judged: I read the screenshot of each case. Supporting signals: in Chromium the frame list contains `chrome-extension://mhjfbmdgcfjbbpaeojofohoefgiehjai/index.html` (the PDF viewer) and `document.contentType` is `application/pdf`; failure shows `chrome-error://chromewebdata/`. A Playwright `download` event marks a download. WebKit has no viewer frame, so only the screenshot shows rendering. WebKit also showed `Download is starting` on `goto`, which is the download signal.

### Results

R = renders (PDF content visible), D = download event, X = does not render (error page), blank = empty frame, no download.

| Case | Chromium | WebKit |
|---|---|---|
| top, none (baseline) | R | D |
| top, `sandbox` | R | D |
| top, `sandbox` + `frame-ancestors 'self'` | R | D |
| top, `sandbox allow-downloads` | R | D |
| top, `attachment` | D | D |
| `<iframe sandbox="allow-scripts">`, none (baseline) | X | blank |
| `<iframe sandbox="allow-scripts">`, `sandbox` | X | blank |
| same iframe, `sandbox` + `frame-ancestors 'self'` | X | blank |
| same iframe, `sandbox allow-downloads` | X | blank |
| same iframe, `attachment` | blank, no download | blank, no download |
| `<iframe>` without sandbox attr, none / `sandbox` | R / R | D / D |
| `<embed>` / `<object>`, none | R / R | R / R |
| `<embed>` / `<object>`, `sandbox` | R / R | R / R |

Reading the table:
- The header changes nothing in Chromium top-level. Every variant equals baseline.
- Chromium's sandboxed iframe fails for the baseline too, so the cause is the sandbox flags on the iframe. Only the `sandbox` attribute separates `iframe` from `iframe_nosb`. Per the HTML spec, a sandboxed iframe disables plugin content via the sandboxed plugins browsing context flag (https://html.spec.whatwg.org/multipage/browsers.html#sandboxing-flag-set). I did not trace Chromium source, so treat that as the likely reason.
- WebKit top and iframe results are baseline behavior of the Playwright build (the header is not the cause, since `none` also downloads). They do not predict Safari. `frame-ancestors 'self'` had no effect on any case; the iframe is same-origin.
- `allow-downloads` made no difference to rendering. The attachment-in-iframe case is blank because a sandboxed frame without `allow-downloads` blocks the download. I did not re-test it with `allow-downloads` on the iframe attribute; the real fallback is a top-level navigation, which downloaded.
- Playwright treats WebKit PDF navigations as downloads. Whether that also holds in Safari is unverified.

## For the spec

- Raw for PDF stays: Chromium renders a CSP-sandboxed PDF as a top-level page, same as unsandboxed.
- The sandbox stays on every hosted file, PDFs included. No special-casing of the header for PDFs.
- The `Content-Disposition: attachment` fallback works (top-level, both engines). Keep it only as the Download action, not for Raw.
- Preview inside gx: never `<iframe sandbox>` for a PDF. Use `<embed>`/`<object>` (renders in both engines here) or open Raw.

## Open points

- Real iOS Safari and desktop Safari were not tested. Safari renders PDFs inline natively, but the CSP `sandbox` header on a PDF there is unverified. Run `probe.mjs`'s server by hand and open `/pdf/sandbox` on a device.
- Headless Chromium was not tried.
