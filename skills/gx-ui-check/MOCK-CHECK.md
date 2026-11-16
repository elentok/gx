# Mock check

Compare a built page with the stored mock it came from. gx-implement and gx-verify-epic follow these
steps. The browser commands come from [gx-playwright-cli](../gx-playwright-cli/SKILL.md).

Change files with a file edit (Edit) or file write (Write) only, never `sed -i`, `cat >`, `cat >>` or a heredoc. Write the file first, then use it (for section text, write a file and pipe it with `gx tickets section ... -`). Run one command per call, because a blocked call runs nothing.

## Steps

1. Resolve the mock directory first: `mocks=$(gx tickets root)/<map>/mocks`. Do it before any `cd`;
   outside a git repo `gx tickets root` fails with "not inside a git repo".
2. Start the fixture scenario with [gx-ui-check](SKILL.md) (section 1), adding `--mock-dir "$mocks"`.
   A New Design mock with several drafts (several-drafts, naming, switcher-open) uses the `drafts`
   scenario, which seeds three drafts: do not script the UI to create them.
   That includes its `make web-build` when `webui/dist/index.html` is missing. Open its `web_url`
   once. The fixture serves the mock directory over local http and reports the base URL as
   `mock_url`: open `<mock_url><NN>-<slug>.html`. `playwright-cli` blocks `file:` URLs, and the block
   is not to be lifted: never set `allowUnrestrictedFileAccess` or pass a `--config` for it. Never
   start `python3 -m http.server` yourself. Use `waitUntil: "domcontentloaded"` for a mock page
   (its fonts and images may never settle).
3. Run every `playwright-cli` call from inside the repo worktree. Outside a git repo it fails with
   "not inside a git repo". Never `cd $TMPDIR &&` before a call. Give `--filename` an absolute path in
   your scratchpad directory, and file-read (Read) the PNG from that same path. Pass the fixture's
   `browser_session` as `-s=<browser_session>` on every call.
4. Pick the state and the part you compare. The mock marks them:
   - Part selector: `[data-mock-state=<s>] [data-mock-part=<p>]`
   - State selector: `[data-mock-state=<s>]`

   gx-implement compares by the part selector, gx-verify-epic by the state selector.
5. Locate the same part on the built page. You choose how: roles, visible text or a CSS selector.
6. Resize the viewport to the state's width before each state. Screenshot the mock and the built
   page at that width, at device scale factor 1, cropped to the selector. Never take a full page at retina scale.
   Save one crop per part and state, named `<state>-<part>-mock.png` and `<state>-<part>-built.png`.
7. File-read (Read) both PNGs and compare them side by side by eye. List each difference. A read that returns
   an empty result means the state is unchecked: do not infer the look from computed styles.
   After the first empty read of a state, stop: record `unchecked` for that state, with no retry,
   no other crop and no recheck. Never write `pass` for a state whose PNGs you did not see.
8. Give every listed state one result line: `pass`, `fail` (with the differences) or `unchecked`
   (with the reason).
9. Write the result lines to a file in your scratchpad directory, one per state:
   `<state>: pass <mock.png> <built.png>`, `<state>: fail <differences>` or
   `<state>: unchecked <reason>`. A `pass` lists the PNG paths you viewed. Run
   `go run ./testutil/mockcheck/cmd <file>` from the worktree. It refuses a `pass` whose PNGs are
   missing or do not decode. Fix each reported line (a state you did not see becomes `unchecked`)
   before you report.

## Projects without the gx fixture

Check first: `[ -d testutil/gxfixture ]`. If it is missing, this is not the gx repo, and
`go run ./testutil/gxfixture` cannot work. That is not a tool failure and not a question for a person.

- Do not run the steps above. Do not run `testutil/mockcheck`.
- If the project has a dev server you can start from its `package.json` scripts, start it as a
  background shell call. Use `pnpm run <script>`, never `npx`. Drive it with `playwright-cli`, screenshot
  each listed part (viewport width of its state, device scale factor 1, cropped), and read the PNGs.
  Report what you saw. There is no mock to compare against, because only the fixture serves mock
  files. Compare with the ticket's description instead.
- Report each state as `skipped: no gx fixture` (add what you saw when you drove the app). Never write
  `pass`: nothing was compared with a mock.
- Skipped states do not block `finished`.
- The project's own e2e suite is not a mock check. It stays in gx-verify.

## What wins

- The mock wins on layout, spacing, sizes, colours and type.
- A later decision wins on behaviour.
- Pixel-perfect is not a goal. Report differences a person would notice.

## Phone states

A phone state is a fixed-width box in the mock, where media queries do not fire. Compare it with the
built page at that viewport width, not at the desktop width.

Stop the fixture (gx-ui-check, section 3) when you are done, even when the check failed. That also
stops the mock's http server.
