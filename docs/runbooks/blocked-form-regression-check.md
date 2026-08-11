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
