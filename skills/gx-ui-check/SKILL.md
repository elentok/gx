---
name: gx-ui-check
description: >-
  Check how gx's web UI behaves by driving a fixture server with `playwright-cli`. Starts a named
  scenario, opens its `web_url`, runs flows from the shared e2e flow catalog, and stops the fixture.
  Use to research existing UI behavior or to confirm a UI change in a real browser. On demand only,
  not part of CI.
---

# gx UI check

Drive the web UI of a throwaway gx server and compare what you see with the flow catalog. The
catalog is `webui/e2e/CATALOG.md`, the same file the Playwright suite (`make web-e2e`) follows, so
agents and the suite check the same flows. Read the catalog entry for the flow you run, not the whole
file. Use the `playwright-cli` skill for the browser commands.

## 1. Build and start a scenario

Build once from the repo root, then start the scenario in the background. The `gx` binary embeds
`webui/dist`; without it the page shows "This build has no web UI". Build the UI first when
`webui/dist/index.html` is missing:

```
[ -f webui/dist/index.html ] || make web-build
go build -o $TMPDIR/gx-ui/gx .
go build -o $TMPDIR/gx-ui/claude ./testutil/agentfake/cmd/fakeclaude
go build -o $TMPDIR/gx-ui/gxfixture ./testutil/gxfixture
$TMPDIR/gx-ui/gxfixture --gx $TMPDIR/gx-ui/gx --fakeclaude $TMPDIR/gx-ui/claude [--hold] <scenario>
```

- Run it with `run_in_background`. It prints one JSON line, then keeps running until SIGTERM.
- The scenario names are in the catalog (`empty`, `ready`, `mixed`, `finishing`, `paused`, `united`,
  `dirty-trunk`, `design`, `design-*`, `many-done`, `edge`). An unknown name makes the command list
  the known ones.
- Pass `--hold` only for a flow that says so (flow 7, and the Stash flow on `dirty-trunk`). It
  freezes the workers until you delete the file named by `hold_file`.
- Each run is its own isolated server and state. Start one per flow.
- To run `gx` against the fixture from the shell, pass `--env` and source the output. It prints
  `export` lines instead of the JSON: the `env` entries, plus `GXF_WEB_URL`, `GXF_GX`,
  `GXF_HOLD_FILE`, `GXF_STATE_DIR` and `GXF_SOCKET`. Redirect the background run's stdout to a file,
  wait for it to fill, then `source` it in the same shell as your `gx` calls. Do not retype the
  `HOME`/`XDG_*` exports.

The JSON line has these fields:

- `web_url`: the page URL, with the token that signs you in.
- `gx`: the gx binary to use for CLI steps.
- `env`: `KEY=value` entries. Set them for every `gx` call, so the call reaches this fixture's server
  and no other.
- `hold_file`: the file that holds the workers when you passed `--hold`.

## 2. Open the page and run a flow

1. Open `web_url` once. That visit trades the token for a cookie.
2. Do the flow's steps from the catalog with `playwright-cli`. Use roles and visible text; the page
   has no `data-testid`.
3. Never reload the page. The UI updates by itself, and a reload would hide a missing update. If a
   step needs a CLI call (flow 6 runs `gx server queue add`), run it with the `env` entries set.
4. Compare the page with the flow's **Pass** lines. Report each line as seen or not seen.

## 3. Stop the fixture

Send SIGTERM to the fixture process (stop the background task). It shuts down the server and its
agents and removes its state. Stop it even when the flow failed.

## How the fixture behaves

Read this before you guess; each wrong guess costs a rebuild.

- **Fake worker outcome → ticket status.** The worker's behaviour is per ticket
  (`agentfake.Behaviour*`, set by a scenario's `behaviour`):
  - `finish` (default): commits and reports finished. The ticket merges and ends `done`.
  - `ask`: pauses with a question. The ticket shows as asking.
  - `fail`: the fake exits non-zero. The ticket becomes `needs-repair · Run failed`.
  - Anything that goes idle with no commit and no report is read as a stalled agent. The server
    nudges it after about 120s. Never wait for that: pick `fail` for a failed run.
- **CLI writes reach the server late.** `gx tickets section/set/add` against the fixture changes
  files, and the server picks them up some time later. Do not add `sleep` or `waitForTimeout`. Poll
  the page with `expect(...).toPass()` or `expect.poll`. The delay is a known gap, not a bug in your
  flow.
- **Epic summary.** The page summary is the epic body's `## Summary` section (`## Destination` for a
  map). A scenario's `epicMeta` writes only frontmatter, so set the summary with
  `gx tickets section <epic> Summary -` after the fixture is up (see the late-write note above).
- **Find a locator from the failure.** On a locator failure, read
  `webui/test-results/<test>/error-context.md` first. It holds the page snapshot with the exact
  accessible names (for example "Done 03 timed 2m", not "03 timed"). Fix all the locators it shows
  before you re-run.

## Porting e2e flows

- Run one flow at a time with `make web-e2e` and Playwright's `-g <flow>`. A whole-file run makes
  every wrong locator wait the full 60s timeout.
- When you write an explore ticket for UI or e2e work, run the suite and check each locator and
  card placement against the running UI. Reading the code is not enough. Record which flows fail
  now.

## Reporting

Say which flows you ran, which Pass lines held, and which did not, with what the page showed
instead. If you find behavior the catalog does not describe, say so and do not edit the catalog from
here.
