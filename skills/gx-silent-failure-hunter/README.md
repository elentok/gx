# gx-silent-failure-hunter

Ported from [anthropics/claude-plugins-official](https://github.com/anthropics/claude-plugins-official),
path `plugins/pr-review-toolkit/agents/silent-failure-hunter.md`. Author: Anthropic. Licensed under
Apache-2.0, see `LICENSE`.

gx changed this skill: it is a skill, not an agent (no `model`, `color` or `tools` keys, and
`disable-model-invocation: true`). It reads the shared review files from `gx-code-review` instead of
running a diff, starts with a skip step, reports only confirmed findings in a short list, and hands
them back to the caller.
