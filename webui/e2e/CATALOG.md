# Web e2e flow catalog

Run with `make web-e2e`. Each flow starts its own fixture server (`testutil/gxfixture`), opens the
`web_url` once for the web cookie, and never reloads the page. The suite fails if a page reloads.
Retries are off. Locators use roles and visible text; no `data-testid` is used yet.

Flows keep the numbers of the e2e plan, so a gap in the numbering is a flow not written yet.
Code: `epics.e2e.ts` (flows 1, 2, 6), `pause.e2e.ts` (flow 3).

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

## 3. Pause reasons and recovery

- **Scenario:** `paused` (`paused-epic` with one ticket per pause reason, `approve-epic` whose worker
  blocks on a permission prompt). Each row is its own test with its own server; `paused-epic` is
  queued through the CLI so its tickets show on the queue.
- **Steps (per recovery row):**
  1. Open the Queue page.
  2. Open the epic from its card, then the ticket from its row.
  3. Take the recovery action.
- **Pass:**
  - The reason shows on the queue card, on the epic page row and on the ticket page. An answer-class
    card shows the pause text, a repair-class card shows the label.
  - The action clears the reason from the ticket page without a reload.
- **Rows:**

  | Row | Ticket | Reason | Action |
  | --- | --- | --- | --- |
  | Answer | `paused-epic/03` | Waiting for your answer | type an answer, Send answer |
  | Choose base | `paused-epic/04` | Ambiguous base | pick `01`, Choose base |
  | Retry | `paused-epic/06` | Agent stopped without a result | Retry |
  | Approve | `approve-epic/01` | Needs approval | Allow once |
  | Repair | `paused-epic/07` | Run failed | none: queue card and epic row show "Fix it in the TUI" |

- **Not covered yet:** Resolve conflict (`paused-epic/05`, `merge-conflict`). The server offers the
  button but refuses the resume ("ticket 05 is …, not asking").

## 6. Outside changes show up

- **Scenario:** `ready`
- **Steps:**
  1. Open the Queue page; it shows "All quiet".
  2. Run `gx server queue add demo:ready-epic` through the CLI against the fixture's server.
- **Pass:** without any action in the page, the queue shows a card for `ready-epic`.
