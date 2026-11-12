# Baseline trace of gx-to-tickets runs

Ticket: `gx:faster-ticket-authoring/01`. Script: [`faster-ticket-authoring-trace.py`](faster-ticket-authoring-trace.py)
(run `python3 -I docs/research/faster-ticket-authoring-trace.py`; `--lines <session.jsonl>` prints one run event by event;
`--json` gives raw numbers). Source: 20 design sessions in `~/.claude/projects/-Users-david-dev-gx-*-design/` that
ran `gx tickets publish <x>-impl --manifest`. The session that wrote this note is excluded.

## Method

- Main session only. Each event's gap (time since the previous main event) goes to one bucket by what the event is.
  A subagent's time is counted once, as the main session's idle wait for its notice.
- Run window: Read of `gx-to-tickets/SKILL.md` (or `gx agent step create-tickets`) to the result of
  `gx tickets set <x>-impl --status open` (else the last publish).
- Buckets: `facts agent` (fact-finding subagent wait), `draft agent` (wait for a subagent that drafts the breakdown or
  manifest; older flow), `quiz wait` (gap to the person's reply), `quiz draft` (model time of the quiz turn),
  `manifest` (model time of the big Write, >= 1500 output tokens), `fix loops` (all other time after the first manifest
  Write: lint-claims, publish, verify-plan, spec fixes, set open), `other` (the rest, mostly reading the spec).
- `agent` = total minus quiz wait. Output tokens are deduped by `message.id`; tok/s = output tokens / model gap time.
- Check: eebcaa8c reproduces the known example (17.2 min, quiz wait 11.0, facts agent 3.1, manifest 1.1, fix loops 1.0).
  Fix loops are 1.0 here, not 1.2, because the script ends at the `set --status open` result.
- Timestamps are per content block, so a Write's time is the model streaming it. Limit: the model time of the turn
  that starts the run is lost, a few seconds.

## Two flows in the data

- **subagent-drafts (13 runs)**: the main session starts a "Draft ... ticket breakdown" subagent and waits for it
  (median 12.4 min). The subagent writes the manifest (6K to 43K output tokens). The main session then runs the quiz
  and a short publish (1 publish, no fix loops).
- **main-writes (7 runs)**: the main session writes the manifest itself (median 10.7K tokens, 1.0 min), then runs
  lint-claims, publish and verify-plan fix loops (median 1.0 min, up to 4.1). Only context-window and split-server-pkg
  used a fact-finding subagent.
- In both flows the quiz wait is the person. The agent part is 3.9 min (main-writes) or 13.2 min (subagent-drafts).

## Per-run table (minutes)

| epic | session | total | agent | quiz wait | facts agent | draft agent | quiz draft | manifest | fix loops | other | turns | out tok | tok/s | manifest tok (tok/s) | ctx start>end | tickets | era | lint/pub/verify |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| agent-launch-and-recovery | 1552c708 11-08 | 39.0 | 8.8 | 30.2 | 0.0 | 8.1 | 0.2 | 0.0 | 0.0 | 0.5 | 11 | 5048 | 134 | 0 (0) | 50K>78K | 45 | subagent | 0/3/0 |
| better-epic-header | e42fb89a 11-03 | 49.9 | 3.1 | 46.8 | 0.0 | 0.0 | 0.2 | 1.0 | 0.9 | 1.0 | 23 | 18696 | 113 | 10211 (166) | 49K>118K | 10 | main | 3/2/5 |
| better-nav | 7b831c98 11-06 | 26.9 | 22.9 | 3.9 | 0.0 | 21.4 | 0.3 | 0.0 | 0.0 | 1.2 | 12 | 10131 | 128 | 0 (0) | 63K>112K | 15 | subagent | 0/2/0 |
| better-nav-fixes | 5b8327de 11-07 | 14.2 | 13.2 | 0.9 | 0.0 | 12.4 | 0.2 | 0.0 | 0.0 | 0.6 | 10 | 6507 | 141 | 0 (0) | 63K>91K | 24 | subagent | 0/1/0 |
| better-notifications | 5c6e9246 11-06 | 67.9 | 22.7 | 45.2 | 0.0 | 21.4 | 0.3 | 0.0 | 0.0 | 1.0 | 11 | 6623 | 99 | 0 (0) | 48K>96K | 39 | subagent | 2/2/1 |
| chat-interrupt | ff526540 11-10 | 22.7 | 15.2 | 7.5 | 0.0 | 14.0 | 0.3 | 0.0 | 0.0 | 0.9 | 13 | 8756 | 132 | 0 (0) | 64K>110K | 13 | subagent | 1/1/0 |
| chords | bfa2ff98 11-12 | 4.4 | 3.8 | 0.6 | 0.0 | 0.0 | 0.3 | 1.1 | 0.6 | 1.8 | 36 | 28549 | 134 | 10982 (162) | 51K>155K | 9 | main | 4/2/6 |
| context-window | eebcaa8c 11-09 | 17.2 | 6.1 | 11.0 | 3.1 | 0.0 | 0.4 | 1.1 | 1.0 | 0.6 | 24 | 24058 | 145 | 11245 (169) | 63K>134K | 9 | main | 5/4/8 |
| dev-mode | 5ed12d8d 11-10 | 11.4 | 11.1 | 0.3 | 0.0 | 10.4 | 0.2 | 0.0 | 0.0 | 0.5 | 7 | 3800 | 108 | 0 (0) | 48K>70K | 22 | subagent | 0/1/0 |
| fast-queue-startup | e173cb35 11-06 | 17.9 | 13.3 | 4.5 | 0.0 | 12.4 | 0.3 | 0.0 | 0.0 | 0.6 | 9 | 5064 | 111 | 0 (0) | 49K>76K | 23 | subagent | 0/1/0 |
| nav-fixes2 | d1baba78 11-09 | 32.7 | 7.7 | 25.0 | 0.0 | 7.1 | 0.1 | 0.0 | 0.0 | 0.5 | 10 | 4063 | 128 | 0 (0) | 62K>97K | 19 | subagent | 0/2/0 |
| pr-flow | d1a438ca 11-10 | 23.4 | 22.4 | 1.0 | 0.0 | 20.6 | 0.3 | 0.0 | 0.0 | 1.5 | 17 | 8799 | 94 | 0 (0) | 48K>94K | 32 | subagent | 0/1/3 |
| pwa | 0c404d98 11-09 | 5.5 | 5.5 | skipped | 0.0 | 0.0 | 0.0 | 2.1 | 1.7 | 1.7 | 45 | 40996 | 134 | 19877 (158) | 47K>177K | 13 | main | 8/2/6 |
| regular-questions | 69ceb2a6 11-10 | 22.1 | 11.1 | 11.0 | 0.3 | 9.6 | 0.3 | 0.0 | 0.0 | 0.8 | 14 | 8206 | 134 | 0 (0) | 51K>86K | 30 | subagent | 0/1/0 |
| server-lifecycle-safety | ed468bb1 11-09 | 8.3 | 7.8 | 0.5 | 0.0 | 7.2 | 0.2 | 0.0 | 0.0 | 0.3 | 7 | 4003 | 143 | 0 (0) | 47K>72K | 16 | subagent | 0/1/0 |
| settings-page | 6d2f9ce2 11-09 | 57.0 | 14.9 | 42.1 | 0.0 | 14.2 | 0.3 | 0.0 | 0.0 | 0.5 | 8 | 4938 | 134 | 0 (0) | 48K>73K | 40 | subagent | 0/1/0 |
| skills-manager | 33055b9f 11-10 | 3.2 | 2.4 | 0.9 | 0.0 | 0.0 | 0.2 | 0.9 | 0.5 | 0.7 | 20 | 18552 | 140 | 9261 (167) | 48K>134K | 9 | main | 3/2/4 |
| split-server-pkg | f3a03d3e 11-04 | 28.3 | 27.4 | 0.8 | 21.5 | 0.0 | 0.4 | 1.0 | 4.1 | 0.5 | 37 | 30328 | 96 | 7124 (124) | 55K>157K | 2 | main | 3/2/5 |
| ui-fixes | db860b50 11-09 | 19.5 | 3.9 | 15.6 | 0.0 | 0.0 | 0.3 | 1.0 | 1.5 | 1.1 | 30 | 24950 | 120 | 10742 (180) | 47K>129K | 15 | main | 4/2/6 |
| webui-paste-image | 9079bcce 11-02 | 84.8 | 10.8 | 74.1 | 0.0 | 9.7 | 0.2 | 0.0 | 0.0 | 0.8 | 8 | 4711 | 104 | 0 (0) | 48K>74K | 23 | subagent | 0/1/0 |

Notes on the table:
- `era` is `main` (main session writes the manifest) or `subagent`. `tickets` is counted from the manifest Write
  (main) or from the draft subagent's Write. split-server-pkg shows 2 only because 9 parallel writer subagents each
  wrote a part; its real count is not measured.
- `draft agent` time overlaps the person's quiz wait only in the sense that both are idle time for the main session;
  they do not overlap in time. The quiz comes after the draft.
- `lint/pub/verify` counts Bash calls that mention `lint-claims`, `tickets publish` and `verify-plan` (a call that
  runs several is counted for each).

## Outliers

- webui-paste-image 74 min, settings-page 42, better-epic-header 47, better-notifications 45, agent-launch-and-recovery
  30, nav-fixes2 25: quiz wait is the person being away. No tool activity in the gap. This is the most
  variable cost; the agent side never exceeds 27 min.
- split-server-pkg: 21.5 min facts-agent wait (9 subagents, 6121 s of subagent time, 719K output tokens) and 4.1 min of
  fix loops. A 157K-token context at the end. The only run where the agent, not the person, was the bottleneck by far.
- better-nav, pr-flow, better-notifications: draft agent 20 to 21 min each (108K to 143K subagent output tokens;
  the draft agent explores the code as well as writing).
- pwa: the only run without a quiz (0 person replies). 5.5 min total, 45 turns, 19.9K-token manifest (2.1 min),
  8 lint-claims calls and 1.7 min of fix loops, context 178K at the end. Unattended runs are short but pay more fix loops.
- chords, skills-manager, split-server-pkg: person answered the quiz in under 1 min (a one-word "Yes").

## Medians (minutes unless noted)

- median(all, n=20), minutes: total_s=22.4, agent_s=10.9, quiz_wait=6.0, subagent=0.0, subagent_draft=8.8, quiz=0.3, manifest=0.0, fixloop=0.0, other=0.8, model_s=1.1, tool_s=0.2; tok/s=130; turns=12; ctx_start=49K; ctx_end=97K; manifest_tok=0; tickets=18
- median(main writes manifest, n=7), minutes: total_s=17.2, agent_s=3.9, quiz_wait=0.9, subagent=0.0, subagent_draft=0.0, quiz=0.3, manifest=1.0, fixloop=1.0, other=1.0, model_s=3.5, tool_s=0.3; tok/s=134; turns=30; ctx_start=49K; ctx_end=134K; manifest_tok=10742; tickets=9
- median(subagent drafts manifest, n=13), minutes: total_s=23.4, agent_s=13.2, quiz_wait=7.5, subagent=0.0, subagent_draft=12.4, quiz=0.3, manifest=0.0, fixloop=0.0, other=0.6, model_s=0.8, tool_s=0.1; tok/s=128; turns=10; ctx_start=50K; ctx_end=86K; manifest_tok=0; tickets=23
- median(with quiz, n=19), minutes: total_s=22.7, agent_s=11.1, quiz_wait=7.5, subagent=0.0, subagent_draft=9.6, quiz=0.3, manifest=0.0, fixloop=0.0, other=0.7, model_s=1.1, tool_s=0.2; tok/s=128; turns=12; ctx_start=50K; ctx_end=97K; manifest_tok=0; tickets=19
- median(no quiz, n=1), minutes: total_s=5.5, agent_s=5.5, quiz_wait=0.0, subagent=0.0, subagent_draft=0.0, quiz=0.0, manifest=2.1, fixloop=1.7, other=1.7, model_s=5.1, tool_s=0.4; tok/s=134; turns=45; ctx_start=48K; ctx_end=178K; manifest_tok=19877; tickets=13

## Findings

- The person's quiz wait is larger than all agent time in 8 of 20 runs and is 58% of total wall-clock across all runs
  (median 7.5 min with quiz; range 0.3 to 74 min). It is not agent time. Agent time has a median of 10.9 min over all runs.
- In the subagent-drafts flow the draft agent is 90% of agent time (median 12.4 min). It is output-bound: about 110 to
  140 output tokens per second in the subagent transcripts, 58K to 143K output tokens per run. Time is roughly
  output tokens / 120.
- In the main-writes flow the agent part is short (3.9 min): the manifest Write is about 1 min (10.7K tokens at about
  165 tok/s, faster than the run average of 134 because it is one long stream with no tool calls), fix loops about 1 min,
  everything else about 1 min. Model time is 90% of it, tool time under 10%.
- Quiz drafting is cheap (0.1 to 0.4 min). A reply gap of under 1 min happens when the person just says yes.
- Context per turn: starts at about 49K (SKILL.md, tracker doc, spec already loaded) and ends at 86K (subagent-drafts)
  or 134K (main-writes; 178K for pwa). A 10K-token manifest in the main session adds about 10K per later turn, so
  each fix-loop turn re-reads 120K to 160K tokens (cache reads). A smaller `SKILL.md` helps the start; per-ticket
  files would cut the Write size only if the total output does not grow.
- Fix loops: 3 to 8 lint-claims calls and 4 to 8 verify-plan calls per main-writes run. Each loop turn is 2 to 6 s,
  so the cost is 0.5 to 4.1 min, driven by the number of misses, not by call speed.
- Output rate: 94 to 145 tok/s per run (median 130). Fixed token budgets therefore translate to time at about
  8 s per 1K tokens.
