# Runbook: blocked-form regression check

The orchestrator gate (`parkOnBlockedPane` in `ralphloop/waitforfinish.go`) parks an iteration
whenever herdr reports a pane's `agent_status` as `blocked`. That recognition — which detection
rule in herdr's pane monitor fires, and whether it fires at all — lives entirely outside this repo,
in herdr's own agent-detection manifest. An upgrade to Claude Code or to herdr's manifest can change
what a blocked prompt looks like on screen and silently stop herdr from recognizing it, which would
return the epic to the exact invisible stall this gate exists to eliminate: an agent sits on an
unanswered prompt forever, and nothing pages anyone.

This check is how you catch that regression. It cannot be automated: nothing can drive Claude Code
into a blocked form headlessly (there is no scripted way to produce a permission prompt without an
agent actually needing permission), and no pane is blocked at process start, so there is no seam for
either a unit test or a startup assertion to hook into. Run it by hand after any Claude Code or
herdr upgrade, and periodically otherwise.

## When to run it

- After upgrading Claude Code (the `claude` CLI/app).
- After upgrading herdr, or after herdr's agent-detection manifest updates (herdr pulls detection
  rules from a remote manifest; `herdr agent explain` reports `manifest_source` /
  `manifest_version`, so a version bump there is also a signal to re-run this check).
- Whenever the gate's behavior is in question — e.g. a ticket sat "blocked" for a long time without
  parking, or parked when it shouldn't have.

Run the enforcement check (below) at the same times — it's the same triggers, just a different
regression.

## What to run

1. Pick or start a live Claude Code pane managed by herdr (e.g. an in-progress ralph-loop
   iteration, or any pane you start with `herdr agent start`). Note its herdr target (pane id or
   iteration label, e.g. `no-silent-stalls-iter-16`).
2. Drive that pane into a blocked form by hand — the simplest reliable way is to ask Claude to run
   a shell command that requires permission approval (e.g. something outside an allowed pattern),
   and wait for the permission prompt to appear on screen.
3. Run:

   ```
   gx doctor --check-blocked-form <target>
   ```

4. The check prints instructions and waits for Enter — press Enter once the prompt is showing in
   the pane. It then calls `herdr agent explain <target> --format json` and asserts:
   - `state == "blocked"`
   - `matched_rule.id == "live_blocked_form"`

## What a pass looks like

```
PASS: herdr matched live_blocked_form.
```

`gx doctor --check-blocked-form` exits 0. Nothing further to do — the gate's detection is intact.

## What a failure looks like, and what to do

```
FAIL: herdr matched rule "live_prompt_box" (state "idle"); want "live_blocked_form" (state "blocked").
```

`gx doctor --check-blocked-form` exits non-zero. This means herdr no longer recognizes the on-screen
form as blocked — the exact failure mode that would make `parkOnBlockedPane` never fire, leaving a
blocked agent running forever with no park, no `needs-answer`, and no notification.

If this happens:

1. Confirm it's not a one-off: repeat step 2-3 above once more, ideally with a different kind of
   blocking prompt (permission prompt vs. a `/btw` overlay vs. a model picker), to rule out having
   mistimed the Enter press.
2. Run `herdr agent explain <target> --format json -v` (or without `--format json` for the
   human-readable form) to see every rule herdr evaluated and why none of them matched
   `live_blocked_form` — the `evaluated_rules` list shows each rule's region, matched evidence, and
   priority, which usually pinpoints what changed (a reworded prompt string, a moved prompt
   region, a UI redesign).
3. This is a herdr-side (or Claude Code-side) regression, not a gx bug: file it against herdr's
   agent-detection manifest, or against Claude Code if the on-screen form itself changed shape.
4. Until it's fixed upstream, treat the gate as **not trustworthy** for this failure mode — a
   blocked agent may sit unrecognized. Consider running epics with closer manual supervision (e.g.
   watching pane state directly) until a herdr/manifest fix lands and this check passes again.

## The enforcement contract (herdr 0.8.2+)

The check above covers **detection**: whether herdr recognizes a blocked pane at all. herdr 0.8.2
added a second thing that can regress: **enforcement** — what each command does once a pane is
blocked. A future herdr release that changes this breaks gx just as silently as a detection
regression would, and this section is what catches it.

The contract, confirmed against live panes (not fakes, not release notes):

- `agent prompt` refuses outright with `agent_blocked` — no text or Enter reaches the pane.
- `agent send-keys` is not guarded — key presses still reach a blocked pane.
- `agent start` fails outright with `agent_not_ready` when the target pane is already blocked at
  startup.

gx depends on all three. `parkOnBlockedPane` in `ralphloop/waitforfinish.go` relies on `agent
prompt` being refused so a blocked pane is never silently re-prompted. `launchAndPrompt` in
`ralphloop/launch.go` classifies `agent_not_ready` (via `herdr.AgentNotReadyError`) instead of
routing every launch-time block to an opaque needs-repair. Both rely on `agent send-keys` still
reaching the pane, since that's the only write path gx ever uses against a blocked pane.

**What gx answers automatically.** This is the whole answerable set — everything else parks:

- Codex's `trust_directory` dialog, at `agent start`, with a single `agent send-keys <label>
  enter` (`recoverAgentNotReady` in `ralphloop/launch.go`). That's it.
- Everywhere else — a quota-reset dialog, a permission prompt, any rule id gx doesn't recognize —
  gx parks for a human and names the dialog by its `matched_rule.id` (read via `AgentExplain`).
  gx never guesses at an answer.

**gx never bypasses the guard.** herdr also exposes `herdr pane run <pane_id> <text>`, which sends
text straight to the pane runtime with no blocked-state check — a complete bypass of `agent
prompt`'s guard, available today. gx deliberately has no wrapper for it. Do not add one: the guard
is telling gx something true (a blocked pane is showing a dialog, not waiting for a prompt), and
routing around it would reintroduce the exact operator-dialog clobbering the park gate exists to
prevent. If you're reading this because `pane run` looks like a fix for a stuck path, it isn't —
the fix is answering the dialog correctly or parking.

**Two live-pane facts to re-check after any herdr or Claude Code/Codex upgrade.** These are the
facts gx's fakes (`testutil/herdrfake`) encode, and the ones that silently invalidated this epic's
original premises the last time they were checked:

- A single `ctrl+c` on a working Codex pane leaves it `idle` (`matched_rule.id ==
  osc_title_idle`), not `blocked` — and it accepts a prompt immediately after. gx's smart-zone
  `/compact` recovery depends on this: it does not dismiss anything, it just prompts.
- `trust_directory` is cleared by `enter` alone — a two-option list with `1. Yes, continue`
  preselected and a `Press enter to continue` footer. If a future Codex build changes the default
  selection or adds a step, `enter` alone stops being enough and `recoverAgentNotReady` needs a
  different key sequence.

### How to confirm the enforcement contract on a live pane

Like the detection check above, this is necessarily interactive — there's no headless way to drive
Codex into a startup trust dialog or a mid-turn interrupt.

1. Start a Codex agent (`herdr agent start`) in a directory Codex has not trusted before (a fresh
   worktree outside any already-trusted parent, e.g. not under an already-trusted `~/dev/gx`).
2. Confirm `agent start` fails with `agent_not_ready`, and `herdr agent explain <target> --format
   json` reports `state: blocked`, `matched_rule.id: trust_directory`.
3. Clear it: `herdr agent send-keys <target> enter`. Confirm `agent explain` moves the pane to
   `working`/`idle` and the dialog is gone.
4. Start a turn that runs long enough to interrupt mid-flight (any tool-using turn). Interrupt with
   `ctrl+c`.
5. Confirm `agent explain` reports `state: idle`, `matched_rule.id: osc_title_idle` — not
   `blocked`.
6. Confirm a prompt is accepted immediately: `agent prompt <target> "..."` should succeed, not
   return `agent_blocked`.

If any step diverges from what's described above — a different rule id, a different result code, a
prompt refused where it should be accepted — treat it the same way as a detection-check failure:
it's a herdr- or Codex-side change to file upstream, and until it's understood, the corresponding gx
recovery path (`recoverAgentNotReady`, smart-zone `/compact`) should be treated as not trustworthy.
