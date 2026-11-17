# Writing a flake or timeout ticket

Use this when a ticket or follow-up is about a flaky test, a hang or a timeout.

Change files with a file edit (Edit) or file write (Write) only, never `sed -i`, `cat >`, `cat >>` or a heredoc. Write the file first, then use it (for section text, write a file and pipe it with `gx tickets section ... -`). Run one command per call, because a blocked call runs nothing.

- **Check for an existing ticket first.** Before you write a flake or timeout ticket, check
  `<project>:test-health` (and `follow-ups`) for a ticket about the same test (see the dedupe step of
  [gx-follow-up](gx-follow-up/SKILL.md)). If one exists, add a "seen again" entry with the run id,
  tree id and failure text, and write no new ticket.
- **Check the load first (advisory).** Run `uptime`. If the load is high, rerun the test on a quiet
  machine before you call it a hang or a timeout; a dump taken under load names a cause nobody checked.
- **Check shared state before adding a wait.** Look for state two parallel tests share (ids, FIFOs,
  ports, package-level consts) before you add a timeout or lengthen a wait.
- **Acceptance names one exact `-run` pattern and one `-count`**, for example
  `-run '^TestX$' -count=20`, and says to stop there. No "fix any other hang", no escalating counts,
  no broad patterns that match many tests.
