---
name: gx-test-review
description: Read-only review of the tests in a diff against gx's shared test guidelines. Judges guideline rules, duplicate coverage and test level. Runs in the code-review batch next to the quality and spec reviews.
disable-model-invocation: true
---

# Test review

Review the tests a diff adds or changes. You only read. Do not edit files and do not run anything:
judge from the text.

## 1. Skip when no test file changed

List the files in the diff. A test file is one named `*_test.go`, `*.test.ts`, `*.test.tsx`,
`*.spec.ts`, or one under a `tests/` or `e2e/` directory. When the diff holds none, report
"no test files changed" and stop.

## 2. Read

- The diff, limited to the test files and the code they cover.
- [gx-test-guidelines.md](../gx-test-guidelines.md), the shared rule list.
- `.ai/index.md` in the reviewed repo, for its project-specific test rules and its test levels.

## 3. Judge

- **Guideline rules:** a new or changed test that breaks a rule in the list. Name the rule.
- **Duplicate coverage:** the new test asserts what an existing test already asserts. Point at both.
- **Test level:** the behaviour is tested at a higher level (e2e, server) than needed, or at a level
  that cannot observe it. Say which level fits, using the levels in `.ai/index.md`.

Report only what you can point at in the diff. Each finding names the file and line, the rule or
reason, and the fix.

## 4. Hand over

Return the findings to the caller. They join the other reviews' findings and go through the
consultant triage in [gx-code-review](../gx-code-review/SKILL.md) (approved, deferred, rejected). No
separate path.
