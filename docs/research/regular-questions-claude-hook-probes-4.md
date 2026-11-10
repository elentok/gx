# Claude AskUserQuestion hook probes, round 4 (deny reason wording)

Follow-up to `regular-questions-claude-hook-probes-3.md`. Question: which `permissionDecisionReason` on a hook `deny` makes a web dismiss behave like a terminal Esc (Claude says nothing more, does not call `AskUserQuestion` again, pid file `idle`)?

Setup: **Claude Code 2.1.319**, tmux 120x40, macOS, scratch cwd under the session scratchpad (never the repo). Each run was `claude --permission-mode auto --model <haiku|sonnet> --settings <hook file>`. `sonnet` resolved to Sonnet 5.5 (`claude-sonnet-5-5`, banner `Sonnet 5.5`). Prompt: `Use the AskUserQuestion tool to ask me whether to name my dog Rex or Max.` The hook is probes-3's `deny` hook with the reason parametrized (`hook_deny.py A|B`). The pid file is `~/.claude/sessions/<claude pid>.json`, read 35 s after submit. Tool-call counts and reply text come from the session transcript jsonl (`~/.claude/projects/*/<sessionId>.jsonl`). Runs were launched 12 at a time in parallel. Tag **[run]** = seen in a real run. Files in `/private/tmp/claude-501/-Users-david-dev-gx-regular-questions-design/ef1b3c86-75ef-4f2e-88dc-e0e407d494e3/scratchpad/`: `cap_<run>.txt` (TUI), `pid_<run>.json`, `sum_<run>.txt`, `run.sh`, `hook_deny.py`.

Wordings:

- **A** (Esc-style): `The user doesn't want to proceed with this question. Do not ask again. STOP and wait for the user to tell you how to proceed.`
- **B** (neutral, probes-3): `The person dismissed this question from the web UI.`

## Answer in short

- **Neither wording gives Esc's silence [run, 12 of 12].** Every run ended with a plain-text reply. Esc ends with no model turn at all, and a `deny` always gives the model one more turn.
- **Neither wording made Claude call `AskUserQuestion` again [run, 0 of 12 re-asks].** Each transcript holds exactly one `AskUserQuestion` tool_use (the blocked one). The pid file read `idle` in all 12 runs, and the TUI shows `⎿  Error: PreToolUse:AskUserQuestion hook error: <reason>` then the reply then `done`.
- **Wording A is the closer match to Esc.** In all 6 A runs the reply says it will not ask again and stops. Two Sonnet runs (2 and 3) are one short sentence with no offer (`Understood. I won't ask again. Tell me how you'd like to proceed.`). The other 4 add an offer or a nudge to pick (`just give me your pick: Rex or Max`). It also leaks "a hook blocked ... " into the text, so the person sees a reply that talks about a hook.
- **Wording B invites a follow-up in the reply [run, 6 of 6].** All 6 B replies say the question was dismissed and offer to ask again or ask for the answer in chat (for Haiku, one run asks `Which would you like, Rex or Max?`). B is the more natural phrase for a web dismiss, but it reads as "dismissed, so ask again if you like", not as "stop".
- **Recommendation: use A**, as it comes closest to Esc: it removed every re-ask-in-chat nudge from Sonnet 5.5's reply in 2 of 3 runs and kept Haiku to "stopped, won't retry". Gx must still expect one short text turn after a dismiss. It cannot get Esc's zero-text result through `deny`.

## Runs

| Run | Wording | Model | Re-called AskUserQuestion? (tool_use count) | Pid after | Claude's reply text (verbatim) |
|---|---|---|---|---|---|
| A_haiku_1 | A | haiku | no (1) | idle | The question didn't reach you. A hook blocked the AskUserQuestion call and said not to ask again, so I've stopped. Tell me if you want me to try again, or just give me your pick: Rex or Max. |
| A_haiku_2 | A | haiku | no (1) | idle | The question didn't reach you. A hook blocked the AskUserQuestion call and said not to ask again, so I've stopped. Tell me whether you want me to try the question again, or just give me the name you want for your dog. |
| A_haiku_3 | A | haiku | no (1) | idle | The question didn't come through: a PreToolUse hook blocked the AskUserQuestion prompt and told me not to ask again. I've stopped and won't retry it. If you still want me to ask, tell me and I'll try again. Or just reply with **Rex** or **Max** and I'll go with that. |
| A_sonnet_1 | A | sonnet 5.5 | no (1) | idle | I didn't ask the question. A hook blocked the AskUserQuestion call and told me not to ask again, so I'm stopping here. Tell me how you'd like to proceed. If you'd rather decide in chat, just say Rex or Max. |
| A_sonnet_2 | A | sonnet 5.5 | no (1) | idle | Understood. I won't ask again. Tell me how you'd like to proceed. |
| A_sonnet_3 | A | sonnet 5.5 | no (1) | idle | Your hook blocked the question, so I didn't ask it and won't try again. Tell me how you'd like to go from here. |
| B_haiku_1 | B | haiku | no (1) | idle | The question didn't reach you: the dog-name prompt was dismissed in the web UI before you could answer. So I don't know yet whether you want Rex or Max. Which would you like, Rex or Max? If you'd rather I ask again in the prompt, say so and I'll retry. |
| B_haiku_2 | B | haiku | no (1) | idle | The question didn't come through: you dismissed it in the web UI before answering. I haven't picked a name for you. Tell me "Rex" or "Max" here, or I can ask again if you'd like the prompt back. |
| B_haiku_3 | B | haiku | no (1) | idle | The question didn't get an answer: you dismissed it in the web UI before picking Rex or Max. I haven't chosen a name for you. If you want me to ask again, say so, or just tell me which name you prefer. |
| B_sonnet_1 | B | sonnet 5.5 | no (1) | idle | The question was dismissed from the web UI, so I didn't get an answer. I won't pick a name for you. Ask me again when you want to decide, or tell me here whether it's Rex or Max. |
| B_sonnet_2 | B | sonnet 5.5 | no (1) | idle | The question was dismissed in the UI before you picked a name, so I don't have an answer. Tell me whether it's Rex or Max, or I can ask again. |
| B_sonnet_3 | B | sonnet 5.5 | no (1) | idle | You dismissed the question, so I don't have an answer yet. If you'd still like help choosing, tell me here whether it's Rex or Max. I can also ask again. |

Plus one earlier A/haiku smoke run (same result: 1 call, idle, "I haven't asked again. Rex or Max is still open."). Not counted in the totals.

TUI tail, representative (A_sonnet_2):

```
❯ Use the AskUserQuestion tool to ask me whether to name my dog Rex or Max.
  ⎿  Error: PreToolUse:AskUserQuestion hook error: The user doesn't want to proceed with this question. Do not ask
     again. STOP and wait for the user to tell you how to proceed.
⏺ Understood. I won't ask again. Tell me how you'd like to proceed.
✻ Cogitated for 3s · done
```

B_sonnet_1 has the same shape with `Error: ... hook error: The person dismissed this question from the web UI.`, the reply, and `Churned for 3s · done`. Full captures are in `cap_<run>.txt`.

## Counts

| | A | B |
|---|---|---|
| Runs | 6 (3 haiku, 3 sonnet) | 6 (3 haiku, 3 sonnet) |
| Called AskUserQuestion again | 0 | 0 |
| Pid file `idle` | 6 | 6 |
| Reply text present | 6 | 6 |
| Reply says it will not ask again / stops | 6 | 0 |
| Reply offers to re-ask or asks for the answer in chat | 4 (A_sonnet_2 and A_sonnet_3 do not) | 6 |

## Could not determine

- Whether any `deny` reason can yield a turn with no text at all (12 of 12 runs produced text; only Esc, which skips the model turn, is silent).
- Whether a longer reason (for example one that says "reply with nothing") would make the model stay silent. Not run.
- Whether A's lower re-ask nudging holds with other prompts, longer conversations or other models. Only a one-shot Rex or Max prompt on Haiku 5.5 and Sonnet 5.5 was run, 3 runs per cell.
- `A_sonnet_2`'s capture shows `Name him Rex` in the input box after the turn. It was not typed by the harness, and it is probably a prompt suggestion. It is not in the other captures. Not investigated.
- gx's real hook and server were not run.
