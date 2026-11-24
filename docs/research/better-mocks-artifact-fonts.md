# Do inlined Saira fonts render in a real claude.ai artifact?

Ticket: `gx:better-mocks/07`. Date: 2026-11-24.

## Answer

- Not proven by observation. The contract says inline `data:` fonts are allowed, and the publish succeeded. The render result itself could not be read.
- The contract text is the best evidence: it names `data:` URIs as the way to embed assets and names `@font-face` data URIs as the way to use non-Google fonts.
- Size added: 109,393 bytes of CSS. The test page is 111,332 bytes, about 0.7 percent of the 16 MB limit.

## What was done

- Built one test page in the scratchpad: `/private/tmp/claude-501/-Users-david-dev-gx-better-mocks-design/3a285361-ac0a-432d-bfea-16114adf29e7/scratchpad/saira-test.html`.
- It holds the five inlined `@font-face` rules (generated from `webui/src/ui/fonts/*.woff2` with the shell loop from `docs/research/better-mocks-font-snippet.md` on branch `research/better-mocks-font-snippet`).
- A script calls `document.fonts.load()` for each face, reads `FontFace.status`, and compares canvas text width against plain `sans-serif`. It prints the table into the page (`<pre id="out">`). It does not use `document.fonts.check()`.
- Published as a private artifact: https://claude.ai/artifact/D9fde1A4ayPrBYojpJE3L1 (version 1). The publish was accepted with no error.

## What could not be observed

- The page is private and needs a claude.ai sign-in. This session has no signed-in browser, and `Artifact read` returns the published HTML, not the output of its script. So the `FontFace.status` and width table was not seen.
- To close this: open the URL above while signed in. The `<pre>` under the five sample lines shows one row per face, ending in `RENDERS SAIRA` or `FALLBACK`.

## Contract and CSP

Source: the Artifact tool description and the `artifact-design` skill text in this session.

- Scripts load only from a short CDN list. Stylesheets load only from `fonts.googleapis.com`, with font files from `fonts.gstatic.com`.
- Quote: "inline all other CSS and JS and embed assets as data: URIs".
- `artifact-design`, Typography: "A face from anywhere else must be inlined as a @font-face data URI, or the browser silently uses a fallback", and to declare a real fallback stack.
- The documented CSP rule lists blocked hosts and blocked fetch types (fetch/XHR/WebSocket, images, media from the CDNs). It does not say `data:` fonts are blocked, and it tells authors to use `data:` URIs. The exact CSP header was not read.
- `data:` URIs count toward the 16 MB size limit.
- Prior local result (branch `research/better-mocks-font-snippet`): in Chromium all five faces reach `status: "loaded"` and widths differ from the fallback. That test had no `font-src` restriction.

## Fallback if blocked

- Use Google Fonts: `<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Saira:wght@400;500&family=Saira+Condensed:wght@500;600;700&display=swap">`. This is the allowed route. It was not tested here, and it needs network at view time.
- Keep the stack `"Saira", system-ui, sans-serif` (and `"Saira Condensed", "Arial Narrow", sans-serif`) so a block degrades quietly. Per the skill, the browser falls back silently with no visible error.

## Sources

- Artifact tool description and `artifact-design` skill (this session).
- `git show research/better-mocks-font-snippet:docs/research/better-mocks-font-snippet.md`.
- Test artifact: https://claude.ai/artifact/D9fde1A4ayPrBYojpJE3L1.

## Result (observed by the person)

The person opened the test artifact signed in: the page showed `RENDERS SAIRA`, all 5 faces load. The inlined `data:` fonts work in a real claude.ai artifact. Size added: 109,393 bytes of CSS (111,332 bytes for the page), about 0.7% of the 16 MB limit. The Google Fonts fallback is not needed.
