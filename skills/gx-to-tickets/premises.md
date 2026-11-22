# Check the premises

Step 4c companion: [gx-to-tickets](SKILL.md).

`gx tickets draft` and the lint check paths and symbols. They do not check facts you only assumed.
Do not state in a ticket body a fact nobody ran:

- **Named flows and tests**: every acceptance criterion that says a flow or test passes names the
  flow number or test name. Run it on the base and see it pass before you write the criterion.
- **`unverified` label**: a premise you could not run (a menu item list, where a scenario may live, a
  fake's behaviour, a helper's path) is labelled `unverified` in the body, not stated as fact.
- **Search strings, not line numbers**: acceptance criteria give a search string (`rg 'Symbol'`), as
  paths already do.
- **Name the helper and its file**: not "the helper from 12".
- **Shared file ordering**: when two tickets that can run in parallel both append to the same file,
  serialise them with `blocked_by`, or name the split (which part each one owns).
