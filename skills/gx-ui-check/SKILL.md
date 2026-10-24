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

Build once from the repo root, then start the scenario in the background:

```
go build -o $TMPDIR/gx-ui/gx .
go build -o $TMPDIR/gx-ui/claude ./testutil/agentfake/cmd/fakeclaude
go build -o $TMPDIR/gx-ui/gxfixture ./testutil/gxfixture
$TMPDIR/gx-ui/gxfixture --gx $TMPDIR/gx-ui/gx --fakeclaude $TMPDIR/gx-ui/claude [--hold] <scenario>
```

- Run it with `run_in_background`. It prints one JSON line, then keeps running until SIGTERM.
- The scenario names are in the catalog (`ready`, `mixed`, `finishing`, `paused`, `design`, `edge`).
  An unknown name makes the command list the known ones.
- Pass `--hold` only for a flow that says so (flow 7). It freezes the workers until you delete the
  file named by `hold_file`.
- Each run is its own isolated server and state. Start one per flow.

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

## Reporting

Say which flows you ran, which Pass lines held, and which did not, with what the page showed
instead. If you find behavior the catalog does not describe, say so and do not edit the catalog from
here.
