# Runbook: Codex smoke checklist

Real Codex only. Run by hand after each Codex upgrade. Never in CI.

Run `gx codex doctor` first. It covers the canned sessions; this list covers what it cannot drive,
such as a real usage limit or a real reviewer denial. Use a scratch ticket on a throwaway epic with
`agent: codex`.

## 1. Usage-limit error and auth failure

- Steps: run a Codex ticket on an account at its usage limit. Repeat with a logged-out Codex
  (`codex logout`).
- Expect: the error text reaches the ticket, and the ticket pauses as `needs-answer`.
- Where gx shows it: the ticket's Action card, and the Queue tab.

## 2. Auto-compaction at the context limit

- Steps: give a Codex ticket a task that fills its context until Codex compacts on its own.
- Expect: the run carries on after compaction, with no stray input and no pause.
- Where gx shows it: the Queue agent's live panel and the ticket log.

## 3. Background flag and wake

- Steps: have Codex start a background task, then finish its turn while the task still runs.
- Expect: gx shows the Background flag. A wake means a follow-up turn; gx changes nothing on its own.
- Where gx shows it: the agent's row in the Queue tab.

## 4. `codex resume` after a restart

- Steps: start a Codex ticket, run `gx server restart` mid-turn, wait for the server.
- Expect: gx adopts the live agent, or resumes it with `codex resume`, and starts no duplicate.
- Where gx shows it: the Queue tab (one agent for the ticket) and the server log.

## 5. Reviewer denial under `--approve-for-me`

- Steps: with Codex's `--approve-for-me` on, ask for an action the reviewer should refuse.
- Expect: the denial text appears and the ticket pauses with a clear reason.
- Where gx shows it: the Action card.

## 6. How often the reviewer escalates

- Steps: run a normal ticket under `--approve-for-me` and count the escalations to a person.
- Expect: escalations are rare. Note the count and the Codex version.
- Where gx shows it: the approval cards on the ticket page; compare across Codex versions.
