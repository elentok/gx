# Mock check

Compare a built page with the stored mock it came from. gx-implement and gx-verify-epic follow these
steps. The browser commands come from the `playwright-cli` skill.

## Steps

1. Resolve the mock directory first: `mocks=$(gx tickets root)/<map>/mocks`. Do it before any `cd`;
   outside a git repo `gx tickets root` fails with "not inside a git repo".
2. Start the fixture scenario with [gx-ui-check](SKILL.md) (section 1), adding `--mock-dir "$mocks"`.
   That includes its `make web-build` when `webui/dist/index.html` is missing. Open its `web_url`
   once. The fixture serves the mock directory over local http and reports the base URL as
   `mock_url`: open `<mock_url><NN>-<slug>.html`. `playwright-cli` blocks `file:` URLs, and the block
   is not to be lifted: never set `allowUnrestrictedFileAccess` or pass a `--config` for it. Never
   start `python3 -m http.server` yourself. Use `waitUntil: "domcontentloaded"` for a mock page
   (its fonts and images may never settle).
3. Run every `playwright-cli` call from inside the repo worktree. Outside a git repo it fails with
   "not inside a git repo". Never `cd $TMPDIR &&` before a call. Give `--filename` an absolute path in
   your scratchpad directory, and `Read` the PNG from that same path. Pass the fixture's
   `browser_session` as `-s=<browser_session>` on every call.
4. Pick the state and the part you compare. The mock marks them:
   - Part selector: `[data-mock-state=<s>] [data-mock-part=<p>]`
   - State selector: `[data-mock-state=<s>]`

   gx-implement compares by the part selector, gx-verify-epic by the state selector.
5. Locate the same part on the built page. You choose how: roles, visible text or a CSS selector.
6. Resize the viewport to the state's width before each state. Screenshot the mock and the built
   page at that width, at device scale factor 1, cropped to the selector. Never take a full page at retina scale.
   Save one crop per part and state, named `<state>-<part>-mock.png` and `<state>-<part>-built.png`.
7. `Read` both PNGs and compare them side by side by eye. List each difference. A `Read` that returns
   an empty result means the state is unchecked: do not infer the look from computed styles.
8. Give every listed state one result line: `pass`, `fail` (with the differences) or `unchecked`
   (with the reason).

## What wins

- The mock wins on layout, spacing, sizes, colours and type.
- A later decision wins on behaviour.
- Pixel-perfect is not a goal. Report differences a person would notice.

## Phone states

A phone state is a fixed-width box in the mock, where media queries do not fire. Compare it with the
built page at that viewport width, not at the desktop width.

Stop the fixture (gx-ui-check, section 3) when you are done, even when the check failed. That also
stops the mock's http server.
