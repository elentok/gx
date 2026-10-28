# Mock check

Compare a built page with the stored mock it came from. gx-implement and gx-verify-epic follow these
steps. The browser commands come from the `playwright-cli` skill.

## Steps

1. Start the fixture scenario with [gx-ui-check](SKILL.md) (section 1). That includes its
   `make web-build` when `webui/dist/index.html` is missing. Open its `web_url` once.
2. Serve the mock's directory over local http and open the mock from there. `playwright-cli` blocks
   `file:` URLs, and the block is not to be lifted: never set `allowUnrestrictedFileAccess` or pass a
   `--config` for it. Start the server with `run_in_background`, never `nohup` or `&`:

   ```
   python3 -m http.server 0 --bind 127.0.0.1 --directory "$(gx tickets root)/<map>/mocks"
   ```

   Port `0` picks a free port. Read the port from the server's first output line, then open
   `http://127.0.0.1:<port>/<NN>-<slug>.html`. You may run this server without asking.
3. Run every `playwright-cli` call from inside the repo worktree. Outside a git repo it fails with
   "not inside a git repo". Never `cd $TMPDIR &&` before a call. Give `--filename` an absolute path in
   your scratchpad directory, and `Read` the PNG from that same path.
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

Stop the fixture (gx-ui-check, section 3) and the mock's http server when you are done, even when the
check failed.
