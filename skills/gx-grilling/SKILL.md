---
name: gx-grilling
description: Grill the user relentlessly about a plan, decision, or idea. Use when the user wants to stress-test their thinking, or uses any 'grill' trigger phrases.
---

Interview the user relentlessly until you reach a shared understanding. Map this as a **design tree**: every decision branches into the decisions that hang off it.

Work the tree in **rounds**. The **frontier** is every decision whose prerequisites are already settled: the questions you can ask _now_ without guessing at answers you haven't heard yet. Ask the whole frontier in one round: number each question and give your recommended answer. Then wait for the user's answers before the next round.

Format a round like so:

````
❓ **Q1** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>

```gx-question
type: yes-no
recommended: yes
```

---

❓ **Q2** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>

```gx-question
type: options
recommended: b
options:
  - id: a
    label: "<first alternative>"
  - id: b
    label: "<second alternative>"
```
````

Every question carries a `gx-question` block (the `gx tickets ask` YAML), after the `➡️` line and before `---`. The format is defined in [gx-local-tracker.md](../gx-local-tracker.md) "The question block".

- Pick the type from the question. A yes/no question ("Does...?", "Should...?") is `yes-no`. A "Which...?" or "What...?" question with real alternatives is `options`: list every alternative, with a short label each. Never use `yes-no` for a question that has more than two answers.
- `recommended` must name the same answer as your `➡️` line. If the `➡️` line says "No", write `recommended: no`. For `options`, name the option id your `➡️` line picks. Check this before you send the round.
- Wrap every `label`, `consequence`, `yes` and `no` value in double quotes. A bare value with `: ` or a backtick makes the block unreadable and the question loses its buttons.

Before asking each question, read [gx-unslop](../gx-unslop/SKILL.md) and apply it to the question and your recommendation.

Write questions and recommendations at CEFR B2 level:

- Short sentences. No jargon unless necessary. No filler words.
- One line when one line does it.
- Prefer bullets over long paragraphs.
- Keep every fact or caveat that changes the decision, even at the cost of length. Cut words, not information.

Make every question **self-contained**. The user reads only the question card, not the chat around it, and must be able to answer from the card alone.

- Never use a short label from earlier in the conversation (F5, P2, Q3, "option B") on its own. Name the thing itself: "the completion flow", not "F5". If the label helps, keep it next to the name: "Q3 (storage backend)".
- This applies to the title, the body, the `➡️` line and every option label.
- Put the context the answer depends on in the question body, in a line or two.

Each round the user answers reshapes the tree: settled decisions push the frontier outward and unblock questions that depended on them. Recompute the frontier and ask the next round. A question whose answer depends on another question still open in this round belongs to a _later_ round, not this one.

Finding _facts_ is your job, never the user's. When a frontier question needs a fact from the environment (filesystem, tools, etc.), dispatch a sub-agent to find it; don't ask the user for anything you could look up yourself. Don't block on it: a running exploration is an unsettled prerequisite, so only the questions downstream of it wait for the sub-agent to report; ask the rest of the frontier now. The _decisions_ are the user's: put each to them and wait.

The session is done when the frontier is empty: every branch of the design tree visited, nothing left silently assumed. Do not act on it until the user confirms you have reached a shared understanding.

Ask for that confirmation as a one-question round, so the web shows it as a question. Write the summary first, then the question:

````
❓ **Qn** - **Shared understanding**: Is this a shared understanding I can record and close?

➡️ Yes

```gx-question
type: yes-no
recommended: yes
```
````
