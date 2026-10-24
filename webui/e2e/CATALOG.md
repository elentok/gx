# Web e2e flow catalog

Run with `make web-e2e`. Each flow starts its own fixture server (`testutil/gxfixture`), opens the
`web_url` once for the web cookie, and never reloads the page. The suite fails if a page reloads.
Retries are off. Locators use roles and visible text; no `data-testid` is used yet.

Flows keep the numbers of the e2e plan, so a gap in the numbering is a flow not written yet.
Code: `epics.e2e.ts`.

## 1. Queue an epic

- **Scenario:** `ready` (one epic, `ready-epic`, not queued)
- **Steps:**
  1. Open the Epics page.
  2. Click the epic's Queue button.
  3. Click Queue in the nav.
- **Pass:** the queue shows a card linking to `ready-epic`, and the "All quiet" empty state is gone.

## 2. Epic looks right

- **Scenario:** `mixed` (`mixed-epic`: done, open, blocked, draft, cancelled, asking and failing tickets)
- **Steps:**
  1. Open the Epics page; the row reads `demo · 2/7`.
  2. Open `mixed-epic`.
  3. Click Queue on the epic page.
  4. Click Queue in the nav.
- **Pass:**
  - Before queueing, the epic page reads "Not queued · 2 of 7 done", folds "1 done · 1 cancelled",
    and shows ticket lines `open`, `blocked · after 02`, `drafted · draft`, `asks`, `fails`.
  - After queueing, the fake worker settles: the epic page reads "Running · 4 of 7 done", ticket 06
    shows "needs-answer" with its question, and the queue card shows `4/7`, the drafted and asking
    tickets, and the question.

## 6. Outside changes show up

- **Scenario:** `ready`
- **Steps:**
  1. Open the Queue page; it shows "All quiet".
  2. Run `gx server queue add demo:ready-epic` through the CLI against the fixture's server.
- **Pass:** without any action in the page, the queue shows a card for `ready-epic`.
