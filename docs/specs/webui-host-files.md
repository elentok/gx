# webui-host-files

Map epic: `gx:webui-host-files`. Decisions come from its tickets 01 to 11 (tickets 08, 10 and 11 are
spec rounds 1 and 2). Research: `docs/research/webui-host-files-pdf-csp-sandbox.md` (ticket 09) and
`docs/research/webui-host-files-sandboxed-assets.md` (ticket 02). Approved mock:
`$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`. Read
`docs/web-ui-security.md` before any security change. Terms: **Hosted file** and **Known root** are in
`GLOSSARY.md`.

## Problem Statement

Agents often write files for the person: Markdown notes, HTML pages, Mermaid diagrams, mocks,
screenshots. The person runs gx on a remote machine, so the person cannot open those files from the
phone or laptop. The agent then starts its own `python -m http.server` or similar. That server has no
login, listens on another port, and stays running after the session.

When an agent mentions a file in chat, the path is plain text. The person cannot open or preview it.
The same holds for paths in tool lines, the person's own prompts, question cards, ticket bodies and
epic summaries.

## Solution

gx hosts files itself, behind the existing web login, on the one web port.

- **A file route** serves a file from a known root. Every response carries `Content-Security-Policy:
  sandbox allow-scripts`, so an agent's HTML runs its scripts but cannot read gx or call its API as
  the person. An HTML page loads its own CSS, images, scripts and `fetch` calls through a short-lived,
  folder-scoped token in the URL path.
- **File links in the web UI.** A path that points at an existing hosted file becomes a link in chat
  text, tool lines, the person's prompts, question cards, ticket bodies, ticket sections and epic
  summaries. Hover (or keyboard focus) shows a preview card. A tap on a phone shows a bottom sheet.
  The link opens a viewer page with a Source toggle, Raw, Download and Copy link.
- **`gx files url <path>`** prints the viewer link for the agent to send. **`gx files resolve`** and
  **`gx server files-roots`** back the web UI check and the roots list.
- **Agent guidance.** Every gx-launched session (loop, Agents tab, Design, plain chat; Claude and
  Codex) gets one system prompt sentence: never start your own HTTP server, run `gx files url`. Four
  skills get a one-line note.

Only the web UI gets file links. The TUI, Slack and Telegram keep plain text.

## User Stories

### The file route and its limits

1. As a person on my phone, I want to open a Markdown file an agent wrote from a gx link, so that I read it without the agent starting a server.
2. As a person, I want every hosted file to need my web login, so that nobody on the network reads my files.
3. As a person, I want gx to host only files under known roots (registered repos and their worktrees, the tickets store, owned temp folders, and folders I added), so that a link cannot read an arbitrary file.
4. As a person, I want `..`, a symlink that leaves a root, an unknown root, a denied name and a temp file owned by another user to all answer the same "not found", so that the route cannot be used to probe my disk.
5. As a person, I want secret-like files (`.env*`, `.git/`, `.ssh`, `.aws`, `.npmrc`, `*.pem`, `*.key`, `id_*`) never hosted, even under a known root, so that a link cannot expose a credential.
6. As a person, I want files in a temp folder hosted only when I own them, so that a file another local user planted in `/tmp` is refused.
7. As a person, I want an agent's HTML page to run its scripts but be unable to read any gx page or API response, so that a page an agent wrote cannot act as me.
8. As a person, I want an HTML page's relative CSS, images, scripts and `fetch` calls to load, so that interactive agent pages work.
9. As a person, I want the asset link of an HTML page to expire after one hour and renew when I reopen the stable link, so that a leaked asset link stops working.
10. As a person, I want an asset link to cover only the page's own folder and below, so that a leaked link reads nothing else.
11. As a person, I want the stable file link never to expire, so that a link from a chat last week still works.
12. As a person, I want a restart of gx to end all asset links and have the stable link renew them, so that no old token survives.
13. As a person, I want large media to seek (HTTP Range), so that a screen recording an agent made is usable.
14. As a person, I want each file served with a fixed content type and no sniffing, so that a text file never runs as a page.
15. As a person, I want gx pages to refuse framing and hosted files to be framable only by gx, so that another site cannot frame my UI.
16. As a person, I want file pages to send no Referer, so that an absolute path or a token never leaks to a site an agent's page links to.

### File links in the web UI

17. As a person, I want a path in an agent's chat text (prose, inline code or a Markdown link target) to become a link when the file exists, so that I open it with one click.
18. As a person, I want paths in tool-call lines such as `Read(server/files/handler.go)` to be links, so that I open a file an agent touched.
19. As a person, I want paths in my own prompts to be links, so that I reopen a file I named.
20. As a person, I want paths in question cards (question text and option text) to be links, so that I read a file before I answer.
21. As a person, I want paths in ticket bodies, ticket sections and epic summaries to be links, so that I open a file a ticket names.
22. As a person, I want a relative path to resolve against the session's folder, or against the project's main worktree when no session exists, so that `docs/plan.md` opens the right file.
23. As a person, I want `~/` paths and absolute paths to link everywhere, so that a path always works.
24. As a person, I want `path:42` and `path:42:7` to open the file at line 42, so that a cited line is in view.
25. As a person, I want a path with spaces linked when it sits in an inline code span or a Markdown link target, so that spaced names work without guessing where prose ends.
26. As a person, I want fenced code blocks and command output left plain, so that logs are not full of links.
27. As a person, I want URLs never read as paths, so that `https://example.com/a/b` stays a normal link.
28. As a person, I want a path to link only after the server confirms the file exists under a known root, so that I never click a dead link at load.
29. As a person, I want a link to a file that later disappears to dim and show "File not found", so that I can tell the file moved.
30. As a person, I want a path that was missing, denied or outside the roots at load to stay plain text, so that the page does not show dead links.
31. As a person, I want a page to check paths in one batch per chat load and per new message, so that a long chat stays fast.
32. As a person, I want a project with no checked-out main worktree to link only absolute and `~/` paths in ticket text, so that gx never guesses a folder.

### Preview and viewer

33. As a person on a desktop, I want a hover card on a file link with the path, size and a preview, so that I peek before opening.
34. As a keyboard user, I want the card to open on focus and close on Escape, so that I need no mouse.
35. As a person, I want a Markdown file previewed rendered, a Mermaid file previewed as a diagram, an image as a thumbnail and code as numbered lines at the cited line, so that each kind looks right.
36. As a person, I want an HTML file's card to show a placeholder and never a live page, so that hovering never runs a script.
37. As a person, I want a file over 2 MB, a PDF or another binary to show a "no preview" card, so that a hover never loads something big.
38. As a person on a phone, I want a tap on a file link to open a bottom sheet with the preview, Open and Copy link, so that touch has the same preview.
39. As a person, I want Copy link to copy the stable viewer link for every file kind, so that I can paste it to a colleague on the same login.
40. As a person, I want a full viewer page with path, size and age, so that I read a file in the app.
41. As a person, I want Markdown and Mermaid files rendered by default with a Source toggle, so that I choose rendered or raw.
42. As a person, I want an HTML file to run in a sandboxed frame on the viewer page, with a note that it has no access to gx, so that I use the agent's page safely.
43. As a person, I want code and other text shown highlighted with line numbers and the cited line marked, so that I read the code.
44. As a person, I want an image shown inline and an SVG shown as an image that runs no script, so that a hostile SVG is harmless.
45. As a person, I want a file over 2 MB and a binary to show only Raw and Download, so that I am never forced to load a huge file.
46. As a person, I want Raw to work for a PDF, so that the browser's PDF viewer opens it as a page.
47. As a person, I want Raw and Download on every file, so that I open or save the bytes.
48. As a person, I want a missing, denied or out-of-root file to show the same "File not found" page, so that the page does not leak which case it is.
49. As a person, I want a relative link in a hosted Markdown file to resolve against the file's own folder and become a file link, so that a doc set links to itself.
50. As a person, I want a relative image in a hosted Markdown file to load, so that an agent's report shows its screenshots.
51. As a person, I want a link outside the known roots in a hosted Markdown file to stay plain text, so that a doc cannot lead me to a refused path.
52. As a person, I want Mermaid's renderer loaded only when I open a Mermaid file, so that the app loads fast.
53. As a person, I want file links to be real links, so that Cmd-click opens a new tab and the back button works.

### CLI and config

54. As an agent, I want `gx files url <path>` to print a viewer link, so that I send the person one line.
55. As an agent, I want `gx files url --raw <path>` to print the raw link, so that I link the bytes.
56. As an agent, I want a relative path resolved against my working folder, so that I pass what I wrote.
57. As a person with several allowed hosts, I want `gx files url` to use the same host as `gx server web-url`, and `--host` to pick another allowed host, an IP or `localhost`, so that the link opens on my device.
58. As an agent, I want a refusal with a stable reason when the path is missing, outside the known roots or denied, so that I move the file and retry.
59. As a person, I want `gx files resolve` to report whether paths exist under a known root, with canonical path, size, kind and mtime, so that scripts and the web UI share one check.
60. As a person, I want `gx server files-roots list|add|remove` to manage extra folders, so that I host a folder outside my repos.
61. As a person, I want `list` to show built-in and configured roots with a source column, so that I see what gx can host.
62. As a person, I want `add` to refuse `/`, my home folder itself, a missing path and a non-folder, so that one command cannot open my whole disk.
63. As a person, I want built-in roots always on and untouched by `add` and `remove`, so that I cannot lock out my repos by mistake.

### Agent guidance

64. As a person, I want every gx-launched session to know "never start your own HTTP server, run `gx files url <path>`", so that agents stop starting servers.
65. As a person, I want loop agents to keep their `gx tickets ask` text and get the files sentence too, so that nothing they had is lost.
66. As a person, I want Agents tab, Design and plain chat sessions to get the files sentence on Claude launch and resume, so that a resumed session still knows.
67. As a person, I want Codex sessions to get the sentence on launch, so that Codex agents send links too.
68. As a person, I want gx-prototype, gx-research, gx-investigate and gx-design to tell the agent to give a `gx files url` link at the step where it hands over a file, so that skills do not start servers.

### Documentation

69. As a maintainer, I want `docs/web-ui-security.md` to describe the file route, the token, the deny list and the headers, so that the next security change starts from facts.
70. As a maintainer, I want ADR 0028 amended for the raw and token routes, so that the parity test stays honest.

## Implementation Decisions

### Roots and path checks

- **Known roots, built in.** Registered project repos, their worktrees, the tickets store, and the system temp folders (`/tmp`, `/private/tmp`, `$TMPDIR` of the server). The built-in set is computed on each request from the project registry and the live worktree list, so a new worktree works without a restart. A removed worktree has no files, so its paths stay plain text.
- **Extra roots.** The config key `server.web.files.roots` is a list of extra folders. It holds only extras. `gx server files-roots add|remove` change only this list. `list` shows both sets with a source column (`built-in`, `config`). `add` refuses `/`, the home folder itself, a missing path and a non-folder. A subfolder of home is fine. `remove` of a path not in the list, or of a built-in root, is refused. The config schema file gains the key.
- **Temp folders.** Under a temp root, gx serves only files the gx user owns.
- **Check order for every file request** (raw, token, resolve, viewer): clean the path; resolve symlinks; require a regular file; require the resolved path inside a known root; apply the temp owner rule; apply the deny list. Any failure gives the same 404 (raw, token) or the same "missing" verdict (resolve). Nothing tells the caller which rule failed.
- **Deny list.** `.env*`, `.git`, `.ssh`, `.aws`, `.npmrc`, `*.pem`, `*.key`, `id_*`. It matches any path segment, case-insensitively, against the requested path and the symlink-resolved path. It applies on raw, token and resolve. A token for a file at a repo's top folder covers the whole repo, which is why the deny list also guards the token route.
- **Home.** A path under home but outside every root is refused.

### Routes

- **Raw route.** `GET /v1/files/<absolute path>`. Needs the login cookie (401 without it). Never expires. Supports HTTP Range. It lives under `/v1/` because the gate serves any other GET without a login. A query `download=1` adds `Content-Disposition: attachment` with the file's base name and never redirects.
- **HTML redirect.** A cookie request for an HTML file (without `download=1`) answers `302` to the token route. Other kinds are served directly.
- **Token route.** A sibling prefix such as `/v1/filetoken/<token>/<absolute path>` (the implementer picks the prefix, which must not clash with the raw wildcard, since a root can start with `/t`). It needs no cookie. The token carries the file's folder and an expiry of 1 hour, signed with a key made at server start and kept in memory. A path outside the token's folder (`..` included), a bad signature, an expired token and a token from before a restart all answer `302` back to the stable raw link, which mints a fresh token. The token format is the implementer's choice.
- **Token response extras.** Token responses add `Access-Control-Allow-Origin: *`, so a page's `fetch` from its opaque origin can read sibling files. No other route sends it.
- **No cookie on assets.** Research (ticket 02) found that the login cookie is not sent for sub-requests of a `sandbox allow-scripts` page in Chromium or WebKit (WebKit sends it only for iframe CSS, img and classic script, which the design does not rely on). Scripts cannot read any `/v1/` or gx response, and writes fail the Origin check because the Origin is `null`. `/v1/` GETs must stay free of side effects.
- **Headers on every file-route response** (200, 206, 302, 401, 404): `Content-Security-Policy: sandbox allow-scripts; frame-ancestors 'self'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cache-Control: private, no-store`. No `Cross-Origin-Resource-Policy`: token responses must load from an opaque origin.
- **Content-Type.** HTML, CSS, JS, JSON, SVG, images and PDF get their real type from the extension. Markdown and other text are `text/plain; charset=utf-8`. An unknown extension is text when the first bytes are valid UTF-8 with no NUL, else `application/octet-stream`. The same text-or-binary rule drives the viewer's kind.
- **PDF.** The sandbox stays on PDFs. Chromium renders a CSP-sandboxed PDF as a top-level page, so Raw works. A PDF never renders inside a sandboxed iframe, so the viewer shows no PDF preview, only Raw and Download. Raw opens as a top-level page and Download uses `download=1`.
- **JSON verb routes.** `files/url`, `files/resolve` and `server/files-roots` list, add and remove, each with a CLI verb and `--json`, following ADR 0028 (refusals are results with a stable `reason`). The implementer picks a verb-route prefix that does not clash with the raw wildcard.
- **`url` verb.** Prints the viewer link by default, `--raw` the raw link. `--host` must be an allowed host, an IP or `localhost`, else refusal `host-not-allowed`. Without `--host` it uses the host of `gx server web-url`. A relative path resolves against the caller's working folder on the client. A missing path, a path outside the known roots, a denied path and a folder all answer the single refusal `path-not-allowed` with a generic message. If the web UI is off, it answers the same refusal that `gx server web-url` gives.
- **`resolve` verb.** Request: a list of candidate paths (cap 200, more is refused `too-many-candidates`) and at most one base for relative paths: a session id, a project id (its main worktree), or a folder (inside a known root). No base means only absolute and `~/` paths resolve. `~` is the server user's home. Reply per candidate: verdict `found` or `missing` (denied and out-of-root are `missing`), canonical path, size, kind, mtime.

### Security headers (whole web mux)

- Every non-file response gets `Content-Security-Policy: frame-ancestors 'none'` and `Referrer-Policy: same-origin`. `no-referrer` on the UI would make browsers send `Origin: null` on same-origin POSTs and break the Origin check.
- `X-Content-Type-Options: nosniff` on every response, from middleware. The file routes set their own CSP and referrer policy, which the middleware must not overwrite.
- A full UI CSP (`script-src`, `style-src`) is a follow-up, filed as a follow-up when the epic is done. Until then the viewer must render Markdown without raw HTML and Mermaid with `securityLevel: strict`. Both are rules in this spec.

### Link detection (web UI)

- **Candidates.** Absolute paths, `~/` paths, and relative paths that contain `/` or end in a file extension (a letter followed by 0 to 7 letters or digits). Optional `:line` or `:line:col` suffix. A candidate starts at the start of text, after whitespace, or after `(`, `[`, a quote or a backtick, and never inside `scheme://`. Trailing `.,;:)!?` is trimmed from a bare prose path.
- **Where.** Prose, inline code spans and Markdown link targets in assistant text, ticket bodies, sections and epic summaries. Fenced blocks and command output stay plain. Bare prose paths stop at whitespace; a path with spaces links only in an inline code span or a Markdown link target. Tool-call lines and the person's prompts use the same detection through a plain-text linker and stay plain text otherwise. Question cards use the Markdown renderer's link support for question text and option text.
- **Base for relative paths.** A session's folder (`SessionInfo.Dir`) in chat, in a question card with a live session and on its Agents page. Anything without a session (ticket text, epic summaries, a card with no session) uses the project's main worktree.
- **Check.** One batch `resolve` call per chat load and per new message, with the session id. Ticket and epic pages send one batch per page with the project id. A found file is cached for the page's life. A missing candidate is re-checked on the session's next message. A candidate over the cap of 200 is left plain.
- **Rendering.** `:line` is dropped before the check. The link carries the line in the viewer route (the hash router cannot use `#L<line>`). Links are real `<a href>` that open in the same tab. The shared Markdown renderer gains a link context (base folder or project, found set). It keeps raw HTML off.
- **Missing after found.** A link whose card or viewer fetch returns 404 after it showed as found is dimmed (no underline) and its card says "File not found".

### Look (mock `05-link-preview-viewer-look.html`)

- **Link look A.** Dotted underline, mono font, a small file icon before the path. The same in prose, code spans and tool lines. A missing link is dimmed with no underline.
- **Hover card.** 380px wide. Header (path, size, and for an image its pixel size), a fixed 170px body with a fade, a footer with Open. Markdown and Mermaid render, an image shows a thumbnail, code shows plain numbered lines scrolled to a cited line, HTML shows a placeholder with a "Sandboxed" tag, a missing file shows "File not found". A file over 2 MB or a binary shows a "no preview" body. A relative link in a card's Markdown is plain text and a relative image shows its alt text. The card fetches the file whole when it is under 2 MB and caches the preview for the page's life, keyed by canonical path and mtime. Open delay and close delay are the implementer's choice. The card also opens on keyboard focus and closes on Escape.
- **Phone.** When the primary input cannot hover, a tap on a file link opens a bottom sheet with the same preview, Open (goes to the viewer) and Copy link.
- **Viewer page.** A normal route in the app shell, code-split so the highlighter and Mermaid load on demand. Top bar: path, size and age, Source toggle (Markdown, Mermaid, HTML only), Raw, Download, Copy link. Markdown and Mermaid render by default. HTML runs in an `<iframe sandbox="allow-scripts">` loaded from the stable raw link (which redirects to a token link), under a note "Sandboxed page, scripts allowed, no access to gx". Code and other text show highlighted with line numbers; a cited line is marked and scrolled to (other kinds ignore it). Images show inline, SVG through `<img>`. A file over 2 MB shows "Too large to show". A missing, denied or out-of-root file shows "File not found" with Copy link only. The viewer asks `resolve` for its own path (size, kind, mtime), then fetches the raw route; for an HTML source view the fetch follows the redirect.
- **Copy link** copies the stable viewer link for every kind. Never a token link, never the raw link.
- **Fenced Mermaid in Markdown.** The viewer renders a Markdown file's `mermaid` fences with the same on-demand, strict renderer. The hover card does not.
- **Hosted Markdown links.** A relative link resolves against the file's own folder (inside known roots only) and becomes a file link by the same rules. A relative image loads through the raw route in an `<img>`. A fragment-only link (`#section`) and any link that does not resolve stay plain text. A parse failure in Mermaid shows the error text and the source.

### Guidance

- **Files sentence** (one line, same for Claude and Codex): "To show the person a file you made (Markdown, HTML, a Mermaid diagram, a mock), never start your own HTTP server. Run `gx files url <path>` and send that link. gx hosts the file behind its web login. If the command refuses the path, save the file in the repo or a temp folder and run it again."
- **Split the prompt.** The harness package keeps the `gx tickets ask` text and gains a second constant for the files sentence. Loop agents get both. Every other session (Agents tab, Design, plain chats; Claude and Codex) gets the files sentence alone. Plain Codex chats keep the question tool.
- **Delivery.** Claude gets it through the append-system-prompt file on every launch and resume. Codex gets it through `developer_instructions` on launch only, since an instruction survives a Codex resume.
- **Side effect.** Codex Design and ticket sessions stop getting the `gx tickets ask` text, which gx-design already forbids there.
- **Skill notes.** One line each in gx-prototype (UI.md's "Surface the URL" and the "Link the mock from the question" step), gx-research (where it reports the findings file), gx-investigate (where it publishes a report) and gx-design (where it hands over a spec or mock): give the person a `gx files url <path>` link, never start an HTTP server. gx-ui-check stays unchanged, because its `http.server` rule is about its own fixture mock.

### Docs and ADR

- `docs/web-ui-security.md` gains a "Hosted files" section: routes, roots, deny list, token, headers.
- ADR 0028 gets an amendment: a route is web-only when it exists only on the web mux. That now also covers routes that stream a body to a browser (hosted-file raw and token). Both go in the web-only list and in the allowlist that `TestWeb_EverythingElseIs404` checks. The JSON routes get verbs: `gx files url`, `gx files resolve`, `gx server files-roots list|add|remove`.
- `GLOSSARY.md` terms Hosted file and Known root gain the config list and the deny list.

## Testing Decisions

- A good test checks external behavior: the HTTP response of a route, a verb's output and exit code, what the person sees. It does not check component internals or exact skill wording.
- **Seams, highest first.**
  1. Web e2e flows through the fixture server: the real cookie, the real sandbox, a real iframe. Needed because the sandbox, the token redirect and the hover card need a browser.
  2. Go server tests on the web mux for the route matrix: login required; root, symlink, `..`, deny list and temp owner checks (the owner check takes an injected "gx user" id, so a test simulates a foreign owner); headers on 200, 206, 302, 401 and 404; Content-Type by kind and by bytes; Range; the HTML redirect, token scope, expiry (injected clock) and a changed key; `download=1`; `resolve` verdicts and base handling; refusal reasons. The route table versus CLI tree parity test and the web-only allowlist test cover the new routes.
  3. CLI verb tests for `gx files url`, `gx files resolve` and `gx server files-roots`, including config persistence.
  4. Vitest on the pure link detector (candidate rules, trimming, URLs, line suffix, fences), the Markdown link context and the hover and sheet components, since a component's render for given props must not be a flow.
  5. Harness tests for the system prompt: loop gets both texts, other sessions the files sentence only, Claude gets the file on launch and resume, Codex gets `developer_instructions` on launch only. Skills package checks that the four notes exist.
- **Flow budget.** Per the gx-playwright-e2es rules, add as few flows as the browser needs: one wide flow and one phone flow, both on the new `host-files` scenario. Pure render checks go to vitest.
- **iOS evidence.** Playwright WebKit is the evidence. The verify ticket reruns the sandbox and asset steps once under WebKit with a one-off project override, not added to the suite. A real device is a known gap.
- Prior art: `server/prompt` image route tests, `webauth` route tests, `chat-images.e2e.ts`, the harness system prompt tests, the config and `gx server web-url` tests, the research probe scripts under `docs/research/webui-host-files-sandboxed-assets/` and `docs/research/webui-host-files-pdf-csp-sandbox/`.

## Verification

A verify ticket runs these against a temporary gx server from `gxfixture`. New flows take the next free numbers in `webui/e2e/CATALOG.md`.

New scenario to write: `host-files`. It holds a project `demo` with a checked-out main worktree and a second worktree, and a Claude session in the main worktree. The repo has `docs/plans/host-files.md`, `.scratch/flow.mmd`, `mocks/04-viewer.html` (with a sibling `style.css`, `app.js`, `data.json` and `shot.png` it loads, a `fetch('data.json')`, and a script that tries to read `/v1/snapshot`), `server/files/handler.go` (50+ lines), `notes/gone.md` (the flow deletes it after the chat loads), `.env`, a 3 MB `big.log`, `doc.pdf`, `blob.bin`, `logo.svg` with a script, a Markdown file with a relative link, a relative image, a `mermaid` fence and a fragment link, and a symlink that points outside every root. A temp folder holds `shot-1.png` owned by the test user. The session's chat holds the mock's chat message (all links), a tool line `Read(server/files/handler.go)`, a prompt with a path, and a question card with a path. A ticket in an epic of `demo` has a body with a relative path and an absolute path.

**Mock states**

- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, chat-links, 900px, `host-files`, parts: chat, message, link-md, link-in-code, link-missing, tool-line.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, hover-md, 380px, `host-files`, parts: popover, header, body, footer.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, hover-mm, 380px, `host-files`, parts: popover, header, body, footer.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, hover-html, 380px, `host-files`, parts: popover, header, body, footer.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, hover-img, 380px, `host-files`, parts: popover, header, body, footer.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, hover-code, 380px, `host-files`, parts: popover, header, body, footer.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, hover-missing, 380px, `host-files`, parts: popover, header, body, footer.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, phone-tap, 340px, `host-files`, parts: phone, chat, sheet, header, body, actions.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, viewer-md, 900px, `host-files`, parts: bar, content, source-toggle, raw, download, copy-link.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, viewer-mm, 900px, `host-files`, parts: bar, content, source-toggle, raw, download, copy-link.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, viewer-html, 900px, `host-files`, parts: bar, content, source-toggle, raw, download, copy-link.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, viewer-code, 900px, `host-files`, parts: bar, content, raw, download, copy-link.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, viewer-img, 900px, `host-files`, parts: bar, content, raw, download, copy-link.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, viewer-big, 900px, `host-files`, parts: bar, content, raw, download, copy-link.
- `$(gx tickets root)/webui-host-files/mocks/05-link-preview-viewer-look.html`, viewer-denied, 900px, `host-files`, parts: bar, content, copy-link.

The mock widths are component widths. Flows run the desktop states with a 900px viewport and phone-tap with a 390px viewport. Each Mock states line is one look check against the named state; story entries below name the state and do not repeat it.

Story entries:

**The file route and its limits (stories 1 to 16)**

- **Stories 1, 2, 11:** CLI: `gx files url docs/plans/host-files.md` from the repo prints an `https://` viewer link and exits 0. New flow "Hosted files, wide" (`host-files`, signed in): open the link, the viewer shows the Markdown (Mock state viewer-md). Open the raw link `/v1/files/<path>` in the same context: 200, `text/plain`. Go test (seam 2): the same requests without the cookie give 401, and a login-less GET of a non-`/v1/` path is not used by the route.
- **Stories 3, 4, 5, 6:** Go test (seam 2) matrix: raw, token and `resolve` give the same 404 or `missing` for `..`, the outside symlink, a path in no root, `.env`, `.git/config`, `id_rsa`, a mixed-case `.ENV`, and a temp file with a foreign owner (injected). In the flow: the symlink and `.env` open the viewer page "File not found" (Mock state viewer-denied). CLI: `gx files url .env` exits non-zero with reason `path-not-allowed`; `gx files url` on the temp `shot-1.png` exits 0.
- **Stories 7, 8, 9, 10, 12:** flow "Hosted files, wide": open `mocks/04-viewer.html` in the viewer (Mock state viewer-html). The frame loads its CSS, script, image and `fetch('data.json')`; the page's attempt to read `/v1/snapshot` fails and a write to `/v1/` answers 403. The raw link as a top-level page redirects to a token URL that serves the same page. In Go tests: a token for one folder returns 302 back for a sibling folder and for `..`; an expired token (injected clock) and a token from a restarted server return 302 to the stable link, which redirects again with a fresh token; a token response has `Access-Control-Allow-Origin: *` and no other route does. Rerun the HTML steps under Playwright WebKit once (iOS evidence).
- **Story 13:** Go test: `Range: bytes=0-9` on a text file gives 206 with `Content-Range`, on both routes.
- **Stories 14, 15, 16:** Go test: header table on 200, 206, 302, 401 and 404 for both routes (CSP with sandbox and `frame-ancestors 'self'`, `nosniff`, `no-referrer`, `no-store`); a UI page and a JSON response carry `frame-ancestors 'none'`, `same-origin` and `nosniff`; Content-Type table for HTML, CSS, JS, JSON, SVG, PNG, PDF, Markdown, an unknown text file and `blob.bin`. In the flow: the viewer's own response has no framing from another origin (a page outside gx cannot frame it).

**File links in the web UI (stories 17 to 32)**

- **Stories 17, 18, 19, 20, 23, 28, 30, 31:** flow "Hosted files, wide": the session's chat shows links in the assistant message, the code span, the tool line, the prompt and the question card (Mock state chat-links). A path to a missing file and one to `.env` stay plain text. The flow counts `/v1/` requests: one `resolve` call per load and one per new message (the fake agent sends a new message). Vitest (seam 4): detector rules, including trimming, URLs, fences and the cap of 200.
- **Story 21, 22, 32:** flow "Hosted files, wide": the ticket page's relative path links against the main worktree and the absolute path links. A second project with no main worktree checked out shows only absolute and `~/` links (add the shape `no-main-worktree` to the scenario). The session link resolves relative paths against the session's folder, shown by a file that exists only in the second worktree.
- **Stories 24, 25, 26, 27:** vitest (seam 4): `handler.go:42` becomes a link with line 42; `~/dev/gx/my notes.md` in a code span links and bare it does not; a fenced block and command output are plain; `https://example.com/a/b` is unchanged. The flow opens `handler.go:42` and the viewer marks line 42 (Mock state viewer-code).
- **Story 29:** flow "Hosted files, wide": delete `notes/gone.md` after load, hover its link: the card says "File not found" and the link is dimmed with no underline (Mock states hover-missing and chat-links, part link-missing).

**Preview and viewer (stories 33 to 53)**

- **Stories 33, 35, 36, 37, 34:** flow "Hosted files, wide": hover each kind in turn: Markdown (Mock state hover-md), Mermaid (hover-mm), HTML placeholder with no frame in the card (hover-html), image (hover-img) and code at line 42 (hover-code). Hover `big.log` and `doc.pdf`: "no preview" bodies. Focus a link with the keyboard: the card opens; Escape closes it.
- **Stories 38, 39:** new flow "Hosted files, phone" (`host-files`, 390px): tap a file link; the sheet shows with Open and Copy link (Mock state phone-tap). Copy link puts the viewer link on the clipboard, not a token or raw link. Open goes to the viewer. The same Copy link check in the wide viewer bar.
- **Stories 40, 41, 42, 43, 44, 45:** flow "Hosted files, wide": Markdown (Mock state viewer-md) with the Source toggle; the Mermaid file (viewer-mm) renders and the Mermaid bundle is requested only then; the HTML file (viewer-html) with the sandbox note; `handler.go` (viewer-code); `logo.svg` shows as an image and its script does not run, and `<img>` is the element in the DOM (viewer-img); `big.log` shows Raw and Download only (viewer-big).
- **Stories 46, 47:** flow "Hosted files, wide": on `doc.pdf` the bar has Raw and Download and no Source toggle; Raw returns 200 `application/pdf` with the sandbox header (open it as a top-level page in Chromium). Download answers with `Content-Disposition: attachment` (Go test).
- **Story 48:** flow "Hosted files, wide": `/files/<missing>`, `.env` and an out-of-root path all show the same "File not found" page (Mock state viewer-denied).
- **Stories 49, 50, 51, 52, 53:** flow "Hosted files, wide": in the Markdown file, the relative link opens the sibling file's viewer, the relative image loads through the raw route, the outside link and the fragment link are plain text, and its Mermaid fence renders. A link is an `<a href>` (a Cmd-click opens a new tab; Back returns).

**CLI and config (stories 54 to 63)**

- **Stories 54, 55, 56, 57, 58:** CLI checks: `gx files url docs/plans/host-files.md` prints the viewer link on the `gx server web-url` host; `--raw` prints `/v1/files/...`; run from a subfolder, a relative path resolves against that folder; `--host localhost` and an allowed host succeed, `--host evil.example` exits non-zero with reason `host-not-allowed` (`--json`: `refused: true`); a missing path, `.env`, a folder and `/etc/passwd` exit non-zero with `path-not-allowed`.
- **Story 59:** CLI check: `gx files resolve --json docs/plans/host-files.md notes/none.md .env` prints `found` with canonical path, size, kind and mtime for the first and `missing` for the others; with `--session <id>` relative paths resolve against the session folder; more than 200 paths answer `too-many-candidates`.
- **Stories 60, 61, 62, 63:** CLI checks: `gx server files-roots add <tmp folder outside roots>` then `list` shows it with source `config`, built-ins show `built-in`, and a file in it is now hostable; restart the server and it stays. `add /`, `add ~`, `add <missing>` and `add <file>` are refused. `remove` of a built-in root and of an unlisted path are refused; `remove` of the added folder works and its files go back to 404.

**Agent guidance (stories 64 to 68)**

- **Stories 64, 65, 66, 67:** Go tests (seam 5) check the launch arguments. Manual check in a fixture Agents tab session, a Design session and a plain Codex chat: the launch command holds the files sentence (and the `gx tickets ask` text only for loop agents). A resumed Claude session holds it again. A resumed Codex session keeps it from launch.
- **Story 68:** skills package checks (seam 5) find the one-line note in each of the four skills; `gx-ui-check` is unchanged.

**Documentation (stories 69, 70)**

- **Story 69:** read `docs/web-ui-security.md` for the "Hosted files" section (routes, roots, deny list, token, headers, the PDF and iOS gaps).
- **Story 70:** the route-table versus CLI-tree parity test passes and `TestWeb_EverythingElseIs404`'s allowlist lists the raw and token routes; ADR 0028 carries the amendment.

## Out of Scope

- Folder listings. A folder path is not linked.
- Live reload of an open file.
- A hook that blocks agents from starting HTTP servers. Guidance is text only.
- A second port or origin for hosted files.
- Editing files from the web UI.
- File links in the TUI, Slack and Telegram.
- A repo AGENTS rule. The system prompt sentence reaches every gx session.
- A full web UI CSP (`script-src`, `style-src`). It is a follow-up.
- A PDF preview inside the viewer. A PDF never renders in a sandboxed iframe.
- A new preview kind for audio or video. The raw route supports Range, nothing more.
- Real iOS Safari and desktop Safari testing, including how Safari treats a CSP-sandboxed PDF. Playwright WebKit is the only evidence. This is a known gap.
- In-page anchors (`#section`) inside hosted Markdown.
- Blocking an agent's HTML from requesting external sites.

## Further Notes

- The research branches and probe scripts are the evidence for the cookie and PDF decisions. If a browser changes how it sends the cookie for sandboxed sub-requests, rerun them.
- Codex sessions lose the `gx tickets ask` text in Design and ticket sessions. This is intended (see Guidance).

## Self-decided design questions

1. **Seams.** Question: which seams does the spec test at? Choice: browser flows, Go route matrix, CLI verbs, vitest detector, harness tests (five seams). Reason: the sandbox and cookie behavior needs a browser, and the security rules are cheaper and exact in Go tests. No user check was possible in this round.
2. **Are denied and out-of-root told apart?** Choice: no. Raw, token and resolve answer the same 404 or `missing`; `gx files url` has one reason `path-not-allowed`. Reason: any difference would be a path oracle.
3. **Download for a file.** Choice: a `download=1` query on the raw route sets `Content-Disposition: attachment` and never redirects. Reason: the HTML redirect would otherwise swallow Download for HTML.
4. **Resolve base.** Choice: one optional base: session, project or folder. Reason: chat needs the session, ticket text needs the project's main worktree, hosted Markdown needs the file's folder (ticket 11).
5. **Resolve cap.** Choice: 200 candidates per call, refusal `too-many-candidates`. Reason: a pasted log must not trigger thousands of disk checks.
6. **Candidate boundaries.** Choice: a candidate starts at text start, whitespace, `(`, `[`, a quote or a backtick, never inside `scheme://`; trailing `.,;:)!?` is trimmed; an extension is a letter plus 0 to 7 letters or digits. Reason: keeps `Read(path)` working, keeps URLs out, and the server check removes false hits like `e.g.`.
7. **Question cards.** Choice: links in question text and option text. Reason: the person reads both before answering.
8. **Large and binary hover cards.** Choice: a "no preview" body with the header. Reason: the 2 MB rule from ticket 05 needs a card state.
9. **Hover card data.** Choice: fetch the whole file when under 2 MB, cache by canonical path and mtime. Reason: Mermaid and Markdown cannot render from a partial file; mtime from `resolve` makes the cache safe.
10. **Touch detection.** Choice: use the bottom sheet when the primary input cannot hover. Reason: a phone and a desktop share one rule, no user-agent sniffing.
11. **Code splitting.** Choice: the viewer route, the highlighter and Mermaid load on demand. Reason: the main bundle must not grow; ticket 05 required it only for Mermaid, the same logic holds for the highlighter.
12. **Fenced Mermaid in Markdown.** Choice: the viewer renders `mermaid` fences, the card does not. Reason: agents write diagrams inside Markdown, and the renderer already loads on demand.
13. **Hash-fragment links in hosted Markdown.** Choice: plain text. Reason: the hash router cannot carry an in-page anchor.
14. **Hosted Markdown in the hover card.** Choice: links as plain text, images as alt text. Reason: a card is a preview, and relative resolution there adds requests for no use.
15. **Token and path rules.** Choice: both the cleaned path stays under the token's folder and the resolved path stays inside a known root and passes the deny list. Reason: a symlink inside the folder must not leave the roots.
16. **Add of a duplicate or nested root.** Choice: accepted and harmless; remove of an unlisted or built-in root is refused. Reason: a nested extra root changes nothing and a refusal adds a reason string with no safety gain.
17. **iOS evidence run.** Choice: rerun the sandbox steps once under Playwright WebKit with a one-off project override, not in the suite. Reason: the suite is Chromium only and the flow budget is tight; ticket 08 named WebKit as the evidence.
18. **Flow count.** Choice: two flows (wide and phone) on one new scenario. Reason: gx-playwright-e2es allows flows only for browser and server round trips, and extends over adding.
19. **Missing at load versus 404 after found.** Choice: plain text at load; dimmed after a 404. Reason: from ticket 08; the spec adds that a cached found file is re-checked only on the viewer or card fetch.
20. **No web UI when off.** Choice: `gx files url` returns the same refusal as `gx server web-url`. Reason: one rule for web-off.
