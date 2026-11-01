## Problem Statement

A person who runs ralph-loop pays for every implement session, and two kinds of waste go unseen until the bill arrives.

First, a session can run on a more expensive model than the one its tier names. From Oct 8 to Oct 18, 2026, implement sessions ran on Opus although the worker tier is Sonnet. That window cost about $6.5k of $12.3k. Nothing warned anyone, and the cause is still unknown (follow-up `gx:follow-ups/117`).

Second, a ticket can run for far more turns than its budget implies. Every turn re-reads the whole context, so cost grows with turns. In a sample, the costliest tenth of tickets ran about 180 turns against about 40 for a median ticket, and they made up about a third of the spend. The skills now tell the agent to fork at about 100 tool calls. Nothing checks that it does.

## Solution

gx watches each running implement session and reacts to two conditions.

- **Wrong model.** If the session runs on a model other than the one its tier resolves to, gx stops the session, pauses the ticket, notifies the person, and shows "Wrong model" with the two model names in the web UI. A button resumes the ticket on the worker tier. gx also files a follow-up that asks why it happened, or adds a line to the open one.
- **Long ticket.** If a session reaches a set number of assistant turns (100 by default), gx notifies the person once, shows a "Long-running" badge in the web UI, and files or updates a follow-up. The session keeps running.

## User Stories

1. As a person running ralph-loop, I want a ticket paused when its session starts on a model other than its tier's model, so that a wrong-model run stops after its first turn.
2. As a person, I want the paused ticket to show "Wrong model", the model it ran on and the model it should have run on, so that I see the problem without reading logs.
3. As a person, I want one button on that paused ticket, "Resume on worker", so that I fix it from the web UI.
4. As a person, I want the resume to apply to the next launch only, so that a one-time fix does not become a permanent override.
5. As a person, I want an implement launch refused when its tier has no model, so that the CLI's own default model, which gx does not know, is never used by accident.
6. As a person, I want one notification when a ticket is paused for the wrong model, so that I learn about it while I am away.
7. As a person, I want a follow-up filed when a guard fires, so that the cause gets investigated.
8. As a person, I want a second ticket that hits the same guard to add a "Seen again" line to the open follow-up instead of filing another, so that `follow-ups` does not fill up.
9. As a person, I want one notification when an implement session reaches the turn threshold, so that I can look at a ticket that is running long.
10. As a person, I want the long-ticket alert to fire once per session and not stop the session, so that a ticket that is nearly done is not interrupted.
11. As a person, I want a "Long-running" badge on the ticket page and the queue row, so that I see the long ticket without a notification.
12. As a person, I want to set the turn threshold in my config and turn the alert off with 0, so that the number fits my work.
13. As a person, I want the guards to ignore tickets whose model matches and tickets that are not implement tickets, so that normal runs and review runs produce no noise.
14. As a person using Codex, I want the model guard to use the same pause and follow-up behavior, so that both harnesses are covered.
15. As a person using scripts, I want the resume CLI verb to accept a tier, so that I can do what the button does from a shell, or resume on `strongest` when the Opus run was deliberate.
16. As a maintainer, I want existing investigate follow-ups to be filed exactly as before, so that the new filing path changes nothing for them.
17. As a person, I want a resumed ticket to be judged on its new session only, so that "Resume on worker" does not pause again because of the old transcript.

## Implementation Decisions

- **Model guard.** The monitor loop reads the model of the first assistant turn of the current launch. A launch that resumes an earlier transcript (`ResumeSessionID`) counts turns from the moment it began, not from the start of the transcript file. The model is compared with the model this launch resolved, including a tier override, so a deliberate resume on `strongest` is not paused. The check runs once per launch and only for implement tickets.
- **Matching.** A match is a family-substring match per harness, for example `sonnet` inside `claude-sonnet-5-5`. A table test covers the known names. For Codex, the model of the latest turn is used.
- **Empty tier model.** If the tier resolves to no model, the transcript guard has nothing to compare and skips, logging once. The launch check (next item) already stopped that case.
- **Launch check.** Implement launches build `--model` from the tier alone. When the tier model is empty, no flag is passed and the CLI picks its own default. The server refuses that launch and pauses the ticket as `model-mismatch`. The transcript check is the ground truth for everything else, because a setting or a mid-session switch can change the model without touching the arguments.
- **Pause effect.** The monitor returns a typed error, as the compact-recovery path does. The run's result handling stops the session, keeps the branch and commits, and turns the error into the pause. The pause, the notification and the follow-up are written on the server side.
- **New pause reason, `model-mismatch`.** This adds a pause kind, so ADR 0042 applies. gx cannot recover on its own: whether the Opus run was a mistake or a deliberate choice is the person's call. The class is `answer` and the receiver is `gx`, like `PauseManual`. The only verb is a new `resume-on-worker`, because a plain Resume would repeat the fault. The web button "Resume on worker" ends the pause. Other tiers go through the CLI flag.
- **What the web shows.** The pause writes the model the session ran on and the model it should have run on into two settable ticket fields (the implementer picks their names). The paused ticket reads them for story 2.
- **Resume with a tier override.** The existing resume route (`POST /v1/tickets/resume`, handler `repairResume`, CLI `gx server tickets resume`) gains an optional tier. No new route is added. The override lives on the ticket (a settable field), so it survives a restart. The next launch reads it once and clears it. A resume with no tier behaves as it does today.
- **Follow-ups.** The server's existing filing code is generalized so a caller supplies the sections and a dedupe key. One open follow-up exists per guard kind. A repeat from another ticket adds a dated "Seen again" line naming that ticket, as the investigate path already does. A follow-up that a person has opened is not appended to; the repeat files a new one.
- **Turn count.** The count is the number of assistant turns in the current launch. Subagent turns are not counted if the transcript marks them.
- **Threshold.** A new global config key, `budget.long-ticket-turns`, default 100, with 0 meaning off. There is no per-epic override.
- **Alert effects.** One notification per launch, a mark on the ticket that holds the turn count and that the web UI reads, and a follow-up. The mark is a settable frontmatter field.
- **Codex.** The model guard applies to Codex sessions where the model can be read. The turn count is Claude-only.
- **Web.** The paused ticket shows the label, the two models and the button. The ticket page and queue row show the badge, with the count and threshold in its tooltip. Two new flows are added to the e2e catalog.

## Testing Decisions

- A good test checks behavior at a public seam: the pause state of a ticket, the notification, the follow-up files and the web UI. It does not check internal helpers.
- **Monitor loop:** `ralphloop` tests with fake `Deps`, in `waitforfinish_test.go` and `verifycap_test.go`.
- **Server end to end:** `server/servertest` with a fake claude whose transcript reports a chosen model and a long run of turns. Prior art: `server/latefinish_test.go`.
- **Follow-up filing:** `server/followups_test.go`.
- **Pause reason:** `tickets/schema` table tests, including `pause_label_test.go`.
- **Config:** `config/config_test.go`, including the schema parity test.
- **Resume route and CLI verb:** the existing resume tests and the parity tests described in `.ai/index.md`.
- **Web:** Vitest for the button and the badge, and Playwright flows for both.
- Each regression test must fail on the old code. A test that cannot compile on the old code counts.

## Verification

1. Story 1: run `gx brief go-test -run 'ModelMismatch' ./ralphloop ./server`. Expect pass. Web flow 147 (new, ticket 08) starts a fixture scenario with a wrong-model fake session and checks that the ticket ends paused.
2. Story 2: web flow 147 checks the "Wrong model" label and both model names on the paused ticket.
3. Story 3: web flow 147 clicks "Resume on worker" and checks that the ticket leaves the paused state.
4. Story 4: `gx brief go-test -run 'TierOverride' ./server`. Expect pass: the launch after the resume uses the worker model, and the next launch after that does not use the override.
5. Story 5: `gx brief go-test -run 'EmptyTierModel' ./server`. Expect pass: a tier with an empty model pauses the ticket as `model-mismatch` and starts no session.
6. Story 6: `gx brief go-test -run 'ModelMismatch' ./server` asserts one banner (`servertest.Banners`) per pause. Expect pass.
7. Story 7: `gx brief go-test -run 'ModelMismatchFollowUp' ./server`. Expect pass: after a guard fires, the servertest store has one new draft in the follow-ups epic with the guard's dedupe key.
8. Story 8: `gx brief go-test -run 'FollowUp' ./server`. Expect pass: a second ticket adds one "Seen again" line and files no new draft.
9. Story 9: `gx brief go-test -run 'LongTicket' ./ralphloop ./server`. Expect pass: one notification at the threshold.
10. Story 10: the same test run asserts that the session is still running after the alert and that the alert does not fire at the next turn.
11. Story 11: web flow 148 (new, ticket 10) starts a fixture scenario with a marked ticket and checks the badge on the ticket page and on the queue row.
12. Story 12: `gx brief go-test -run 'LongTicketThreshold' ./config ./server`. Expect pass: the default is 100, and a threshold of 0 produces no alert in a servertest run with a long fake transcript.
13. Story 13: `gx brief go-test -run 'ModelMismatch|LongTicket' ./ralphloop ./server`. Expect the matching-model and non-implement cases to pass.
14. Story 14: `gx brief go-test -run 'ModelMismatchCodex' ./ralphloop ./server`. Expect pass. The Codex case is required (ticket 05).
15. Story 15: `gx server tickets resume --help` lists a `--tier` flag. Run `gx server tickets resume <paused ticket> --tier worker` against the fixture server and expect exit code 0 and the ticket reopened.
16. Story 16: `gx brief go-test -run 'FollowUp' ./server` with the existing investigate case unchanged. Expect pass.
17. Story 17: `gx brief go-test -run 'ResumedLaunch' ./transcript ./ralphloop`. Expect pass: a fake transcript whose old turns are on Opus and whose new launch is on Sonnet does not pause again.

New flows to write: web flows 147 and 148, and the gxfixture scenarios they need. They are part of tickets 08 and 10.

## Out of Scope

- Switching a running session to another model with `/model`.
- Finding why implement sessions ran on Opus from Oct 8 to Oct 18 (`gx:follow-ups/117`).
- Reducing the fixed context of a session (the `fixed-context-monitor` design epic).
- Per-epic turn thresholds.
- Forcing a fork or a pause at the turn threshold.
- Cost-based alerts.
- A plain "Resume" button on a `model-mismatch` pause.

## Further Notes

The thresholds and defaults come from a sample of 61 implement sessions and the ticket cost records of 2,784 tickets. The 100-turn default flags roughly the costliest tenth of tickets.
