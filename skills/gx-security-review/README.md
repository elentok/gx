# gx-security-review

Ported from [anthropics/claude-code-security-review](https://github.com/anthropics/claude-code-security-review),
path `.claude/commands/security-review.md`. Author: Anthropic. Licensed under MIT, see `LICENSE`.

gx changed this skill: it is a skill, not a command (no `allowed-tools` key, `name` added, and
`disable-model-invocation: true`). It reads the changed-file list and the diff from the shared review
files of `gx-code-review` instead of running shell lines, starts with a skip step, starts no nested
subagents (it does the false-positive filtering itself in one pass), reports findings with a severity
word and a confidence in a short list, and hands them back to the caller.
