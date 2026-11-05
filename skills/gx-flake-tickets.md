# Writing a flake or timeout ticket

Use this when a ticket or follow-up is about a flaky test, a hang or a timeout.

- **Check the load first (advisory).** Run `uptime`. If the load is high, rerun the test on a quiet
  machine before you call it a hang or a timeout; a dump taken under load names a cause nobody checked.
- **Check shared state before adding a wait.** Look for state two parallel tests share (ids, FIFOs,
  ports, package-level consts) before you add a timeout or lengthen a wait.
- **Acceptance names one exact `-run` pattern and one `-count`**, for example
  `-run '^TestX$' -count=20`, and says to stop there. No "fix any other hang", no escalating counts,
  no broad patterns that match many tests.
