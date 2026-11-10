# The hook answers Claude's question dialog; keystrokes are the fallback

The hook is the primary path for answering Claude's question dialog. Keystrokes into the terminal are
the fallback.

- The hook answers through `updatedInput`, so Claude receives the person's answer as structured
  input.
- The hook fails open: if it cannot answer, the terminal dialog still shows and the person can use it.
- The keystroke path reads the screen first and stops on any mismatch. It never sends keys into a
  dialog it does not recognise.

Why: a hook is exact. It names the question and the answer, and nothing depends on what the screen
looks like. Keys are blind input: a wrong guess about which dialog is showing lands in the wrong
place.

What it costs: while the hook holds, it hides the terminal dialog. "Answer in the terminal" releases
the hook so the dialog shows and the person can answer there.

Reasons and probe results: `docs/research/regular-questions-claude-dialog.md`.
