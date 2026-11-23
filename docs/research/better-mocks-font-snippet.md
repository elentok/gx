# Does the base64 font snippet render Saira?

Ticket: `gx:better-mocks/01`. Date: 2026-11-23.

## Answer

Yes. The snippet works as written. All five faces load from the inlined data URIs, and the browser uses them, not a fallback. It needs two small fixes. The gx viewer link and `file://` could not be opened by the research session (see "Not verified").

## What was tested

Snippet: `### Fonts` in `skills/gx-mock/SKILL.md`, worktree `/Users/david/dev/gx/gx-mock-skill-prototype` (uncommitted, not edited). Fonts: `webui/src/ui/fonts/*.woff2`.

### 1. Snippet output

Run from the repo root with zsh, bash, sh and dash. All four gave byte-identical output.

```
$ for sh in zsh bash sh dash; do $sh snippet.sh > out-$sh.css; wc -c out-$sh.css; done
109393 out-zsh.css   109393 out-bash.css   109393 out-sh.css   109393 out-dash.css
```

- The weight parsing is right. `${f##*latin-}` then `${w%%-*}` gives 400, 500 for Saira and 500, 600, 700 for Saira Condensed. The `case` picks the right family.
- Output is identical with GNU base64 (coreutils 9.12, wraps at 76 chars, removed by `tr -d '\n'`) and macOS `/usr/bin/base64` (`PATH=/usr/bin:/bin`, `cmp` says same). So the shell is portable.
- The five fonts are `Web Open Font Format (Version 2)` (`file`), with five different SHA-256 hashes, so Saira 400 and 500 are different files even though both are 13876 bytes.
- The `@font-face` rules match `webui/src/ui/fonts.css` (same families, weights and `font-display: swap`).

### 2. Size added

- Raw woff2 files: 81,584 bytes in total (13,876 + 13,876 + 18,044 + 17,980 + 17,808).
- Snippet output: 109,393 bytes (about 107 KiB). Base64 adds one third, plus about 150 bytes of CSS per rule.
- The SKILL.md text says "about 100 KB". That is close enough. "About 110 KB" is more exact.

### 3. Browser check

Test page: one `<style>` with the snippet output, plus five paragraphs (Saira 400, 500, Saira Condensed 500, 600, 700). 111,067 bytes. Opened with `playwright-cli` (Chromium). No Saira is installed on the machine (`fc-list | rg -i saira` is empty), so a match cannot come from a system font.

Evidence from `document.fonts` and a canvas width test (text `Hamburger 0123`, 24px):

| Face | FontFace status | `document.fonts.load()` count | Width with face | Width with `sans-serif` |
|---|---|---|---|---|
| Saira 400 | loaded | 1 | 185.81 | 180.11 |
| Saira 500 | loaded | 1 | 186.24 | 180.11 |
| Saira Condensed 500 | loaded | 1 | 139.37 | 180.11 |
| Saira Condensed 600 | loaded | 1 | 139.54 | 188.09 |
| Saira Condensed 700 | loaded | 1 | 140.47 | 188.09 |

- All five `FontFace` entries have `status: "loaded"`.
- Every width differs from the fallback width, and the four widths differ from each other where the files differ. This shows a real face is used.
- Computed style of the paragraphs: `Saira, sans-serif` and `"Saira Condensed", sans-serif` with the expected weights.
- Do not rely on `document.fonts.check()` alone. For a family that is not in the set and not installed, `check()` returns `true` (MDN, FontFaceSet.check, "Nonexistent fonts"). In the test, `check('24px "NoSuchFontXyz"')` returned `true`. Use `status` and `load()` plus a width compare.

### 4. How the page was served

- `file://`: `playwright-cli open file://...` is refused: `Access to "file:" protocol is blocked`. Not tested.
- `gx files url <path>` link: the viewer link (`https://mini.elentok.com:7422/#/files/...`) shows a sign-in form (password plus authenticator code). The raw link (`--raw`) answers `401 Unauthorized` with `curl`. The browser also rejects the self-issued certificate (`ERR_CERT_AUTHORITY_INVALID`) until HTTPS errors are ignored. The session has no credentials, so this path was not tested.
- Stand-in: the same bytes were served at `https://mock.invalid/test.html` through `playwright-cli route`, with the header `Content-Security-Policy: sandbox allow-scripts; frame-ancestors 'self'`. That header is the CSP the gx file server sent on its 401 response (via `curl -I`), with the `connect-src` part removed. The tables above are from this run. The CSP has no `default-src` or `font-src`, so it does not block `data:` fonts. The 200 response headers were not seen.
- Why `file://` and the gx link should behave the same: a `data:` URI font needs no network request and no same-origin access. This is an inference, not a test.

## Fixes the snippet needs

1. Run it from the repo root. The path `webui/src/ui/fonts/$f.woff2` is relative, so from another folder `base64` fails and the rule gets an empty font. Add `cd "$(git rev-parse --show-toplevel)"` as the first line, or say "run from the repo root". In the prototype worktree the repo root is the worktree root.
2. Quote the path: `base64 < "webui/src/ui/fonts/$f.woff2"`. Harmless today, safer if the path ever has a space.
3. Optional: say "about 110 KB" instead of "about 100 KB", and add `font-style:normal` to match the app. Neither changes rendering.

Not a problem: MIME type `font/woff2` works. Shell portability and weight parsing work (see above).

Limit to know: only the Latin subset is inlined, as in the app. Characters outside Latin fall back to the system font.

## claude.ai artifacts

Source: the Artifact tool's documented contract (tool description in this session). It says:

- External scripts load only from a short list of CDNs.
- Stylesheets load only from Google Fonts.
- "Everything else inline".
- The rendered page must be 16MB or smaller, and "embedded `data:` URIs count toward that limit".

Reading:

- A `<style>` block with `@font-face ... url(data:font/woff2;base64,...)` is inline. It does not load a stylesheet from another host. It fits the contract.
- 109 KB is far below 16 MB (0.7 percent).
- A Google Fonts `<link>` is the other route the contract allows. It was not checked, and inline fonts do not need it.
- Untested: nothing was published as an artifact, because this research ticket only reads and tests locally. The contract does not state the artifact page's CSP for `font-src` and `data:`. So "the browser renders it" is not proven. A one-page private artifact with this test page would settle it.

## Sources

- `skills/gx-mock/SKILL.md`, `### Fonts` in `/Users/david/dev/gx/gx-mock-skill-prototype`.
- `webui/src/ui/fonts.css` and `webui/src/ui/fonts/*.woff2` in the same worktree.
- Commands: the shell loop and `cmp` runs in section 1, `shasum -a 256`, `file`, `playwright-cli open/goto/route/eval`, `gx files url [--raw]`, `curl -skI`.
- MDN, FontFaceSet.check(): https://developer.mozilla.org/en-US/docs/Web/API/FontFaceSet/check
- Artifact tool description (documented contract), this session.
