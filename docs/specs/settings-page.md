# Settings page

Epic `settings-page` (project gx). Handed off as the `settings-page-impl` epic.

Where this spec and the stored mock disagree, follow this spec. The known disagreements are listed
under "Where the mock is wrong" in Further Notes.

## Problem Statement

The web Settings page covers only a sliver of gx's configuration: web logins, password, authenticator,
certificate and the default harness. The other 70-odd keys of `~/.config/gx/config.json`, and nearly
every key of each project's `project.json`, can be changed only by editing JSON by hand. Hand edits
have no validation until the server complains, give no sign of which keys apply live and which need a
restart, and are awkward on the iOS devices the web UI is used from. Nothing makes a new config key
show up on the page, so the page would fall behind again.

## Solution

The Settings page shows and edits every key in config.json and every key in each project's
project.json, except keys only the TUI reads and except a project's `notifications` block (it holds
secrets, which the web page does not put into a project file that is committed to git). It is
generated from one Go registry of keys, so a new key appears on the page by adding a registry entry,
and a guard test fails until that entry exists. The same changes can be made from the command line
with `gx server settings` and `gx server settings-set`, which share the page's writer and validation.

The page has one clean page per section. On desktop and iPad mini a sidebar lists the sections (each
with a "changed" count chip), with a scope picker (Global or a project) and a search box on top. On a
phone there is no sidebar: Settings home lists Security, the global sections and the projects, and a
project opens a list of its sections. Each value type has its own control. A changed key shows a dot
and a "Reset to default (X)" link. A project shows, per overridable key, either "Uses the global
value X" with an "Override for <project>" button or an "Overridden" chip with a "Use global value
(X)" link. A save shows "Saved"; a refused save keeps the draft and shows "Not saved: <reason>"; a
startup-only key shows "Needs restart" before any edit and "Restart needed to apply" after a save;
a risky key asks for confirmation first; secrets are write-only.

## User Stories

1. As a gx user, I want the Settings page to list every config.json key I can set, so that I never have to open the file to change a setting.
2. As a gx user, I want every key grouped into seven sections (Execution queue, Budget, Notifications, Server, Agents, Evolve and recovery, Ticket store), so that I find a key by topic. "Global" is only the sidebar's group label.
3. As a desktop user, I want a sidebar listing the sections, so that I can move between them in one click.
4. As a desktop user, I want a "changed" count chip on each sidebar section, so that I can see which sections hold keys set in the file.
5. As an iPad mini user, I want the same sidebar at 744px, so that the page works as on desktop.
6. As a phone user, I want a Settings home that lists Security, the global sections and the projects, so that I can drill down without a sidebar.
7. As a phone user, I want a project to open a list of its sections, so that I reach a project key in two taps.
8. As a phone user, I want a back button on every drill-down level, so that I can climb out without the browser back.
9. As a gx user, I want a scope picker at the top of the sidebar, so that I can switch between Global and any project.
10. As a gx user, I want to search keys by name, key path or description, so that I find a setting without knowing its section.
11. As a gx user, I want search results grouped by section and shown in place of the section page, so that I see where each hit lives.
12. As a phone user, I want the search box on Settings home, so that I can search without a sidebar.
13. As a gx user, I want each key to show its description, so that I understand it without reading the docs.
14. As a gx user, I want a bool key to be a switch that saves at once, so that toggling is one tap.
15. As a gx user, I want number, duration, size and string keys to be a text field with Save and Cancel, so that I do not save half-typed values.
16. As a gx user, I want a unit hint on duration (`5m`) and size (`50G`) fields, so that I type valid values.
17. As a gx user, I want an enum key to be a segmented control, so that I see every choice.
18. As a gx user, I want a string list to be tags with a remove button and an add field, so that I edit lists without JSON syntax.
19. As a gx user, I want a number list such as `budget.notification-thresholds` to use the same tags control with numeric validation, so that I cannot add a non-number.
20. As a gx user, I want a key that is set in the file to show a dot, so that I see at a glance what differs from the default.
21. As a gx user, I want a "Reset to default (X)" link on a changed key that removes the key from the file, so that I return to the default with one click.
22. As a gx user, I want "Saved" under the field for about two seconds after a save, so that I know it worked.
23. As a gx user, I want a refused save to keep my draft, mark the field red and show "Not saved: <reason>", so that I can fix the value without retyping.
24. As a gx user, I want the writer to refuse a wrongly typed or out-of-range value (a duration that does not parse or is not above zero, a size that does not parse, a value that is not an enum member, a number below its minimum) with the key and a reason, so that a typo such as `5mm` never shows "Saved" while the server silently keeps the default.
25. As a gx user, I want the page to refuse a value that `config.Parse` or the agent-runner validation would reject, so that I cannot write a config the server will not load.
26. As a gx user changing several keys at once, I want a refusal to name the key that failed, so that I know which draft to fix.
27. As a gx user, I want a startup-only key to show "Needs restart" before I edit it, so that I know a restart is part of the change.
28. As a gx user, I want "Restart needed to apply" next to a key after I save a startup-only key, so that I know the saved value is not live yet.
29. As a gx user, I want a live key to apply after the save without a restart, and every key to be either live or flagged startup-only, so that the page never says "Saved" for a value the running server ignores.
30. As a gx user, I want a web save to trigger exactly one config reload and no desktop OS banner, and two saves at the same moment not to lose each other's change, so that saving from my phone does not pop notices on my desktop.
31. As a gx user, I want a confirm dialog naming the risk before I change `server.web.listen`, `server.web.allowed-hosts`, `server.web.totp` (turning it off), `server.tcp-listen`, `server.web.dev-frontend`, `server.web.dev-url` or `ticket-store.path`, so that I do not lock myself out by accident.
32. As a gx user, I want the `ticket-store.path` confirm to say that the old data is not moved, so that I know what the change does.
33. As a gx user, I want the Telegram bot token and Slack webhook URL never sent to the browser, so that a web login cannot read my secrets back.
34. As a gx user, I want a secret to show "Set" or "Not set" with Replace and Clear, so that I can rotate or remove it without seeing it.
35. As a gx user, I want the Telegram chat id shown in full on the page, so that I can check it is the right chat (`gx config show` keeps redacting it).
36. As a gx user, I want keys that run commands (`disk-space.cleanup-command`, `agents.claude-args`, `server.tab-env`, project `checks`) editable without an extra gate, so that I configure them from the page like any other key.
37. As a gx user, I want an empty `cleanup-command` list to stay a real value, so that "run nothing" is distinct from "use the default".
38. As a gx user, I want my other config.json keys to survive a web edit, so that the page never loses a key it does not show.
39. As a gx user with a hand-ordered config.json, I want a web save to keep the existing key order and add new keys at the end of their object, so that my dotfiles diff shows only the keys I changed.
40. As a gx user, I want a web save to merge the changed key into the file as it is now, so that a hand edit I made while the page was open to another key survives.
41. As a gx user, I want a multi-key change to apply together or not at all, so that a partial write never leaves the file half edited.
42. As a gx user with a symlinked config.json, I want the page to write through the symlink, so that my dotfiles setup keeps working.
43. As a gx user, I want the existing Security rows (web logins, password, authenticator, certificate) and "Sign out of this device" to stay, at the bottom of the global sidebar and on the phone home, so that nothing I use today moves away.
44. As a gx user, I want the default harness as a row in the Agents section, so that the separate Agents page is not needed.
45. As a gx user, I want changing the default harness, globally or for a project, to keep refusing a harness whose program is not on PATH and to show the switch notice (waiting and running counts), so that the page loses nothing the web, TUI and CLI show today.
46. As a gx user with a bookmark to `/settings/agents`, I want it to redirect to the Agents section, so that the old link still works.
47. As a gx user, I want the Agents section to hold the default harness, agent models and efforts, `skills`, `agent-runner` and `runner-wait`, so that all agent settings sit together.
48. As a gx user, I want `subscription` keys under Budget, and `cpu-watchdog` and `disk-space` keys under Server, so that related keys sit together.
49. As a gx user, I want `recovery` and `evolve` keys under "Evolve and recovery", so that I find them together.
50. As a gx user on the command line, I want `gx server settings --json` to print every registry key with its current value for global, or only the keys that apply to a project for a project, so that I can read settings without the web page.
51. As a gx user on the command line, I want `gx server settings-set --json` to apply key changes (set or remove) to global or a project through the same writer and validation as the page, so that scripts and the page behave the same.
52. As a gx user, I want each project listed in the scope picker, so that I can open a project's settings.
53. As a gx user, I want a project's "Project" section first, with read-only name, repo and vcs under Identity, so that I see what the project is and cannot break its address.
54. As a gx user, I want to edit a project's trunk, landing and auto fast-forward merge, so that I configure how it lands work.
55. As a gx user, I want a project's sidebar to list "Project", then every section that has an overridable key, then "Checks", so that the page shows only what applies.
56. As a gx user, I want an overridable key that follows global to show "Uses the global value X" and an "Override for <project>" button, so that I see what the project inherits.
57. As a gx user, I want an overridden key to show an "Overridden" chip and a "Use global value (X)" link that removes the key from project.json, so that I can drop an override.
58. As a gx user, I want a per-project "Max live agents for this project" row, absent as "No per-project cap (the global cap of X still applies)" where X is the current global cap, with "Set a cap for <project>", so that I understand it limits on top of the global cap.
59. As a gx user, I want a set per-project cap to show the "Overridden" chip and a "Remove cap" link that removes the key, so that I can lift the cap.
60. As a gx user, I want the other two execution-queue caps (`max-agents-per-epic`, `max-one-off-agents`) to keep "Uses the global value" in a project, so that they behave as overrides.
61. As a gx user, I want a project page to have no Notifications section and no secret rows, and to leave an existing `notifications` block in project.json untouched, so that bot tokens and webhooks are never written into a project file that is committed to git.
62. As a gx user, I want project `evolve` shown as a switch "Evolve for this project", labelled "Follows global mode (X)" when absent, so that I enable or disable it per project.
63. As a gx user, I want "Override for <project>" then "Off" on evolve to write `false`, and "Use global value" or turning it back on to remove the key, so that the file holds only a real override.
64. As a gx user, I want `evolve.deep-read-cap` not to appear in a project, so that I am not offered an override that has no effect.
65. As a gx user, I want a project's Agents section to offer only the keys the config merge applies (default harness, the claude and codex model, effort, light and strongest keys), so that every row I see has an effect.
66. As a gx user, I want a project's `skills` overrides to offer only `implement` and `code-review`, the keys the merge applies, so that no row is inert.
67. As a gx user, I want a project's `disk-space.cleanup-command` as an overridable list, so that one project can clean up differently.
68. As a gx user, I want a Checks section per project with one card per kind (format, lint, typecheck, test), so that I edit each check in its own place.
69. As a gx user, I want each check card to offer default, off, one command, or changed + full, so that I can express every form of `checks`.
70. As a gx user, I want the test card to hold the suite, hang and slow limits, so that I tune them with the test command.
71. As a gx user, I want `slow` to be one duration field, so that the common case is simple.
72. As a gx user whose project.json already has a per-tool `slow` map, I want go, vitest and playwright fields and saving to keep the map form, so that the page does not rewrite my file's shape.
73. As a gx user, I want an empty `slow` field to remove that limit, so that I can clear it.
74. As a gx user, I want `always` to be a tag list of commands on the "changed + full" form, so that I add always-run commands.
75. As a gx user, I want a project.json save refused when `ValidateProjectFile` or the check config parser would reject the result (for example `checks` with `changed` and no `full`), so that a project file stays loadable and `gx check` keeps working.
76. As a gx user whose project repo has `.gx/checks.json`, I want the Checks section to say that file wins and that the edits below have no effect, so that I do not tune a section the server ignores.
77. As a gx user, I want project.json edits committed through the ticket store's commit path, so that the git-backed store stays clean.
78. As a gx user, I want a project.json edit to show its saved value at once and the server to use it, with no reload step, so that the page matches what the server reads.
79. As a gx user, I want name, repo and vcs read-only on the page, so that I cannot break ticket addresses, branches and logs.
80. As a gx user, I want the page not to show `$schema`, the legacy `resume-on-start` alias or any TUI-only key, so that the page lists only web-relevant settings.
81. As a gx user, I want a key that both the TUI and the server read (for example `cpu-watchdog.enabled`) to stay on the page, so that I can still edit it.
82. As a contributor adding a config key, I want a rule in `.ai/index.md` that says to add a registry entry, regenerate the schemas and run the guard test, so that I know the steps.
83. As a contributor adding a config key, I want a guard test that fails with the key name and a pointer to that rule when the key has no registry entry, so that I cannot forget the page.
84. As a contributor removing a config key, I want the guard test to fail on a registry entry for a key that no longer exists, so that dead rows do not stay on the page.
85. As a contributor, I want the guard test to fail when an entry's declared type does not match the key's Go type, or when the registry's write-only set differs from the `secret:"true"` tagged set, so that the control matches the value and no secret leaks.
86. As a contributor, I want the three JSON schema docs generated (config and project from the registry, checks from the `check` package), with a test that fails on a stale file and prints the `-update` command, so that the schemas stop drifting.
87. As a contributor, I want the project key whitelist derived from the registry, so that there is no second hand-written list.
88. As a contributor, I want `projectKeys` to keep accepting the keys the page does not show (such as ignored `agents` sub-keys and `notifications`), so that existing project files still load.
89. As a contributor, I want a `settings` fixture scenario, so that flows and mock checks run against a realistic config and several projects.
90. As a phone user, I want every control usable at 390px with no horizontal page scroll, so that I can change a setting from an iPhone.

## Implementation Decisions

### Registry and generation

- **Registry in Go.** A hand-written registry of settings keys is the one source of truth. Each
  entry carries: key path, type (bool, number, duration, size, enum, string, string list, number
  list, secret, checks), a number's minimum, a description, a section, an enum's values, a
  startup-only (restart) flag, a risky-key flag with its confirm text, and project fields: whether a
  project may override it and, where it differs, the project's own key path. A `secret` type entry is
  write-only. The registry holds no default: the read endpoint takes each default from
  `config.Default()` by key path, so "Reset to default (X)" cannot show a stale X. Names and field
  shapes are the implementer's choice.
- **"Changed" means present in the file**, not "differs from the default". The dot, the sidebar
  count chip and Reset all use it; Reset removes the key.
- **Not derived from struct tags.** Struct tags are not the source; a guard test compares against
  them (see Testing Decisions).
- **Key-path representation.** The implementer picks it, but it must handle the flat dotted key
  `evolve.deep-read-cap`, which is one key and not a nested path.
- **`rawConfig`.** Eight real keys are `json:"-"` on `Config` and its sub-structs (`runner-wait`,
  `recovery.notify-hold`, `server.log-retention`, `cpu-watchdog.sustain`, `cpu-watchdog.cooldown`,
  `execution-queue.spin-window`, `execution-queue.waiting-slot-grace`,
  `execution-queue.stalled-nudge-delay`), so a walk of `Config` tags would miss them. The anonymous
  `raw` struct in `parseWith` is lifted into a named type, `rawConfig`, which is exactly what the file
  holds. The guard walks `rawConfig` and `ProjectFile`. A duration is a `string` in `rawConfig`, so
  the guard recognizes it by the `Config` field the key loads into (`time.Duration`) and checks the
  entry's type against that, not against "number".
- **Schema generation.** `docs/config-schema.json` and `docs/project-schema.json` are generated from
  the registry and committed. `docs/checks-schema.json` is owned by the `check` package, generated
  from `check.Kinds` and the package's command and limits shape; the registry's `checks` entry
  `$ref`s it. `check/check_test.go` already tests that file and keeps doing so. A test regenerates the
  config and project schemas in memory and fails when the committed files differ.
- **Derived whitelist.** The project key whitelist (`projectKeys` in the config package) is built
  from the registry's project-overridable flags, nested sub-keys included. It stops being a second
  hand-written list. The derived whitelist still accepts the keys on the exclusion list that
  project.json allows (the `AgentsConfig` sub-keys the merge ignores, and `notifications`), so
  existing files load.
- **Exclusion list.** The registry has an explicit exclusion list for struct keys that are not
  page settings, each with a reason:
  - `$schema`: not a setting.
  - `resume-on-start`: legacy alias.
  - the `AgentsConfig` sub-keys inside project.json that the merge ignores: "accepted in project.json,
    ignored".
  - project `notifications`: "accepted in project.json, deliberately not editable on the web
    (secrets); ProjectFile still loads it and the page leaves an existing block untouched".
  - the TUI-only keys, each with the reason "TUI only, not shown on the web page":
    `use-nerdfont-icons`, `image-diffs`, `stage-diff-context-lines`, `input-modal-bottom`,
    `name-aliases`, `log` (including `log.important-refs` and `log.hide-refs`), `agents.leader-key` and
    `agents.focus-sidebar-key`. Only `ui/` and `cmd/tui` read them. A key the TUI and the server both
    read, such as `cpu-watchdog.enabled`, stays a normal entry.
- **Sections.** There are seven global sections: Execution queue, Budget, Notifications, Server,
  Agents, Evolve and recovery, Ticket store. "Global" is the sidebar's group label (and "Global
  settings" on the phone home), not a section. Placement: `subscription` in Budget; `cpu-watchdog` and
  `disk-space` in Server; `skills`, `agent-runner` and `runner-wait` in Agents; `recovery` and
  `evolve` in Evolve and recovery. A project shows "Project" first, then each section holding an
  overridable key, then Checks last. There is no "This project only" section and a project never
  shows Notifications.

### API

- **Read.** The server serves the registry and the current values for a target (`global` or a
  project name): the value or its absence for each key, so the page can tell "default" from "set",
  plus each default from `config.Default()`. For a project it also serves the global value of each
  overridable key, for the "Uses the global value (X)" text. Secrets are never sent; the response
  carries only set or not set.
- **Write.** One endpoint takes a target and a list of key changes (set a value, or remove a key).
  The changes apply together or not at all. The wire shape is the implementer's choice.
- **Responses.** A successful write returns the saved values, the restart-needed keys and, for
  `agents.default-harness`, the switch notice. A refusal returns the failing key and a reason the page
  shows under the field.
- **Routes and CLI verbs.** The endpoints follow the repo's route rules: route table entries, a CLI
  verb with `--json` for every route (`TestRouteTableHasCLIVerbs`), and the web-mode allowlist. The
  two verbs are `gx server settings` (read the registry with current values for global or
  `--project <name>`; secrets show only set or not set) and `gx server settings-set` (apply key
  changes, set or remove, to global or `--project <name>`). Both take `--json`. The verbs call the
  same read and the same writer and validation as the page; there is no CLI-only path. The risky-key
  confirm is a page step, so the CLI does not ask. The exact flag syntax for key changes is the
  implementer's choice.
- **Access.** Settings routes need the same web login as every other setting route.

### Writer

- **One key-path writer** for config.json and project.json replaces `setAgentsKey`. It sets or
  removes any key path, keeps every other key, writes atomically and follows a symlink. The default
  harness endpoint calls it too.
- **Key order kept.** `setAgentsKey` decodes into a map and re-marshals, which sorts every object's
  keys. The new writer instead edits the file in place: it keeps the existing key order and appends a
  new key at the end of its object. A server test asserts the order.
- **Merge, last write wins.** There is no version check. The writer re-reads the file, applies the
  changed keys and writes, so a hand edit to another key while the page is open survives. A web write
  holds one writer lock for the whole read-modify-write, so two concurrent web saves both land.
- **Removal.** Reset to default and "Use global value" remove the key from the file. Removing the
  last key of a nested object removes the empty object. An empty `cleanup-command` list is written as
  a real value, never removed.
- **Default-harness hook.** The generic write keeps a per-key hook for `agents.default-harness`, in
  both global and project scope. The hook runs `SetDefault`'s PATH refusal (a harness whose program
  is not on PATH is refused) and returns the switch notice (waiting and running counts) in the write
  response. The default harness endpoint and verb keep working through the same writer and hook.

### Validation

- **Type check first.** Before `config.Parse`, the writer checks each changed value against its
  registry type: a duration parses and is above zero, a size parses, an enum value is a member, a
  number is at or above the entry's minimum. It refuses with the key and a reason and never clamps.
  This is needed because `config.Parse` ignores an unparsable or non-positive duration and clamps
  caps (`max(v,1)`, `evolve.deep-read-cap`), so without it a typo such as `spin-window: "5mm"`
  would show "Saved" while the server keeps the default.
- **Then the existing checks.** The edit is applied to the raw JSON in memory, then the result is
  checked: `config.Parse` and `agentrunner.ValidateSetting` for config.json; `ParseProjectFile`,
  `ValidateProjectFile` and, for `checks`, `check.ParseConfig` for project.json (today
  `ValidateProjectFile` checks only vcs, landing and default-harness, so a broken `checks` would be
  saved and break `gx check`). On an error nothing is written and the page shows the message under the
  field.
- **Naming the failing key.** `config.Parse` errors are not keyed. For a multi-key change the writer
  validates each change alone first, to attribute the refusal to its key, then validates the combined
  result. If the combined result fails while each change passes alone, the refusal names every key of
  the change with the reason.
- The page also validates type shape in the browser (numeric tags, duration and size hints) as
  convenience only; the server check decides.

### Applying a change

- **config.json.** After a write the server calls the existing config reload exactly once, with its
  own trigger, and suppresses the OS banner (the page shows the feedback, and a save from the phone
  should not pop a notice on the desktop). With `server.watch-config` on (the default) the file
  watcher would reload a second time, so the writer arranges that its own write does not trigger a
  second reload (how is the implementer's choice). The page shows the reload's "restart needed for
  <key>" warnings next to the field.
- **Startup-only keys have one list.** `startupOnlyChanges` in `server/settings/reload.go` (5 keys
  today) is derived from the registry's startup-only flag; there is no second list. The two disagree
  today: `runner-wait` is read once at server start (`cmd/serverctl/server.go`) but is never reported.
  The implementer audits every key that `FromConfig` does not carry into live `Settings` (known
  cases: `spin-window`, `spin-cycles`, `retry-storm-launches`, `evolve.deep-read-cap`, `skills.*`,
  `budget.notification-thresholds`, `runner-wait`) and either makes it live or flags it startup-only.
- **project.json.** There is no reload: project.json is read where it is used, and the page shows the
  saved value. The ticket store's commit loop already commits project.json edits (it polls `git
  status` and runs `add -A`, debounced), so the writer adds no commit step of its own; a test confirms
  it.

### Secrets

- `notifications.telegram.bot-token` and `notifications.slack.webhook-url` are write-only. The
  server never sends the value. The page shows "Set" or "Not set" with Replace and Clear. Clear
  removes the key.
- `notifications.telegram.chat-id` carries the `secret:"true"` tag, so `gx config show` keeps
  redacting it, but it is shown in full on the page (ticket 03's decision).
- The registry's write-only set must equal the `secret:"true"` tagged set minus `chat-id`; the guard
  asserts it. The registry has no separate secret flag beyond the `secret` type.
- Secrets exist on the page only in global scope. A project's `notifications` override is not
  editable on the web, because the token or webhook would be written into
  `<store>/<project>/project.json`, which the commit loop commits and pushes to
  `ticket-store.push-remote` when one is set. If project notifications are needed later, that is a
  separate design.

### Risky keys and restart marks

- **Risky-key confirm.** The registry flags `server.web.listen`, `server.web.allowed-hosts`,
  `server.web.totp` (confirm only when turning it off), `server.tcp-listen`,
  `server.web.dev-frontend`, `server.web.dev-url` and `ticket-store.path`. Changing one opens a
  confirm dialog naming the risk, with Cancel and "Change it". Normal validation still runs before
  the write. The confirm is a page step; the server API does not require a confirmation token.
- **Command keys.** `disk-space.cleanup-command`, `agents.claude-args`, `server.tab-env` and project
  `checks` have no extra gate.
- **Restart marks.** The registry flags each startup-only key, so the page shows "Needs restart"
  before any edit. After a save, the server's pending-restart status shows "Restart needed to apply"
  next to the field. Both use the existing pending-restart status, driven by the one derived list
  above.

### Project override rules

- **Override and reset.** An overridable key that is absent from project.json shows "Uses the global
  value X" and an "Override for <project>" button. Once set it shows the "Overridden" chip and a "Use
  global value (X)" link that removes the key.
- **`max-agents`.** The project `max-agents` is its own per-project cap, not an override. The global
  `execution-queue.max-agents` still applies at the same time; there is no inheritance. The row is
  "Max live agents for this project". Absent: "No per-project cap (the global cap of X still
  applies)", where X is the current global `execution-queue.max-agents` (its default is 4, but the
  person may have changed it), with "Set a cap for <project>". Set: the "Overridden" chip and a
  "Remove cap" link that removes the key. The registry carries a separate project key path for this
  row, since the global key lives under `execution-queue` and the project key is top-level.
- **Other caps.** `execution-queue.max-agents-per-epic` and `execution-queue.max-one-off-agents` keep
  "Uses the global value" and Override.
- **`notifications`.** Not overridable on the web. The project page has no Notifications section, no
  "Override for <project>" for notifications and no project secret rows. The registry's notification
  entries are not project-overridable. `ProjectFile` still loads a `notifications` block from
  project.json and the config merge still applies it; the page never reads, shows or changes it, so a
  block a person wrote by hand stays as it is. The guard's exclusion list covers it (see above).
- **`evolve`.** A project shows a switch "Evolve for this project", labelled "Follows global mode
  (X)" when absent. "Override for <project>" then "Off" writes `false`. "Use global value" and
  turning it back on both remove the key. `evolve.deep-read-cap` is not overridable per project and a
  write of it to a project is refused.
- **Project write whitelist.** In project scope the writer refuses any key the registry does not mark
  project-only or project-overridable, including `notifications` keys and the `agents` sub-keys the
  merge ignores. The refusal names the key. The project read (page and `gx server settings`) lists
  only the keys that apply to a project.
- **`agents`.** Only the keys the config merge applies are flagged overridable: `default-harness` and
  the claude and codex model, effort, light and strongest keys. The other `AgentsConfig` sub-keys
  stay accepted in project.json but are on the exclusion list and not shown.
- **`skills`.** Only `implement` and `code-review` are overridable (what `WithProject` applies).
- **`disk-space.cleanup-command`.** Overridable list.
- **Project-only keys.** `trunk`, `landing`, `auto-ff-merge` and `checks` are plain project values.
  `name`, `repo` and `vcs` are read-only.
- **Checks editor.** One card per kind (format, lint, typecheck, test). A kind is default, off, one
  command, or changed + full. The test kind adds suite, hang and slow limits. `slow` is one duration
  field; when the file already holds a per-tool map the editor shows go, vitest and playwright
  fields and saving keeps the map form; an empty field removes that limit. `always` is a tag list of
  commands shown on the "changed + full" form only. `check.LoadConfig` prefers `.gx/checks.json` in
  the project repo over project.json `checks`, so when that file exists the section shows a notice
  that the repo file wins and the edits below have no effect.

### Page structure and routing

- The page structure, sidebar, scope picker, search and phone drill-down follow the stored mock.
  The existing Security pages (web logins, password, authenticator, certificate) and sign out stay.
- `/settings/agents` redirects to the Agents section. The Agents page code is folded into the Agents
  section and removed. The default harness row uses the generic write with its default-harness hook,
  so the PATH refusal and the switch notice stay.
- Search is client-side over the registry entries of the current scope (name, key path,
  description). Results replace the section page and group by section.

### Guard and rule

- **Guard test.** See Testing Decisions.
- **`.ai/index.md`.** A new section "Adding a config key", next to "Adding a Migration step", says:
  add the registry entry (or an exclusion entry with a reason), regenerate the schemas with the
  `-update` command, and run the guard test that names the missing key. CLAUDE.md already says
  `See .ai/index.md`, so no pointer change is needed.

## Testing Decisions

A good test checks external behavior only: what the page shows and what the files and API contain
afterwards, never the registry's internal layout or React component structure.

Seams, highest first, preferring existing ones:

1. **Go test at the registry package** (the guard and the schema snapshot). It is the one new seam.
   - The guard walks the JSON tags of `rawConfig` and `ProjectFile`, nested keys included, and
     compares them with the registry plus the exclusion list. It fails on a struct key with no entry
     and on an entry for a key that no longer exists. The message names the key and points at the
     "Adding a config key" rule in `.ai/index.md`. It also checks each entry's declared type against
     the key's Go type (bool, number, duration, string, list); it does not check descriptions.
     It asserts that the registry's write-only set equals the `secret:"true"` tagged set minus
     `notifications.telegram.chat-id`.
   - A test that the derived project whitelist equals the registry's project-overridable flags plus
     the accepted-but-excluded project keys, and that every project key a project.json fixture uses
     today still validates.
   - A test that the registry's startup-only flags are what `startupOnlyChanges` reports (one list).
   - A second test regenerates the config and project schema files in memory and fails when the
     committed files differ, printing the exact `go test ... -update` command. Prior art:
     `TestSchemaSnapshot` in the `migrations` package. The `check` package's own test covers
     `checks-schema.json`.
   - A test that every global section the page lists holds at least one entry, and that each
     overridable entry has a project placement.
2. **Server API tests**, through the existing server test helpers and `servertest` fixtures (build
   tickets and projects with `servertest.WriteTicketWith`). They cover the writer and the endpoints:
   - set and remove a key path, other keys kept, key order kept and new keys appended, symlink
     followed, atomic write;
   - merge with a concurrent hand edit; two concurrent web writes both land;
   - an all-or-nothing multi-key change, and a refusal that names the failing key;
   - type-check refusals (unparsable duration, non-positive duration, unparsable size, non-member
     enum, number below minimum) and refusals from `config.Parse`, `agentrunner.ValidateSetting`,
     `ValidateProjectFile` and `check.ParseConfig` (a `checks` with `changed` and no `full`), with
     nothing written;
   - a secret is never in a read response;
   - each project override rule (max-agents path, evolve false, ignored agents keys, project
     `notifications` untouched by a write, `evolve.deep-read-cap` rejected for a project);
   - the config reload happens once with no OS banner, and the restart-needed list after a
     startup-only key changes;
   - a project.json write lands in the ticket store commit (the test flushes the store loop or waits
     out its debounce);
   - the default harness hook: PATH refusal and switch notice in both scopes, and the default harness
     endpoint, ported from `setAgentsKey` to the new writer, keeps its tests;
   - the `gx server settings` and `gx server settings-set` verbs return what the endpoints return
     (`--json`), and `TestRouteTableHasCLIVerbs` passes.
3. **vitest** for page logic that needs no server: controls per type, draft and refused-save
   rendering, search, the "Uses the global value" and "Overridden" states, the checks editor forms
   (including the `slow` map form and the repo-file-wins notice), the risky confirm and the phone
   drill-down.
4. **Playwright**, per `gx-playwright-e2es`. Default to no new flow: extend the existing settings
   e2e flows (and the codex flow for the Agents redirect) rather than adding one. A flow is only for a
   real server round trip, browser layout or a multi-page journey: a global edit that survives a
   reload, a project override then "Use global value", a refused save, a secret that is never echoed
   back, and the `/settings/agents` redirect. The flow budget (`playwrightguard`'s `maxFlows`) is
   respected; the implementer reads the skill before writing any flow.
5. **Fixture scenario.** A new `settings` scenario in the gxfixture package: a config.json with
   non-default values across every section, a pending-restart key, secrets set and not set, and three
   projects (one with overrides, a per-project cap, a per-tool `slow` map, and an `evolve: false`; one
   hand-written `notifications` block that the page must leave untouched). `web-logins` and
   `default-harness` are reused. Prior art: the existing `web-logins` and `default-harness` scenarios
   and their tests.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built. Every story that
reads or writes a key through the page is checked in the `settings` scenario unless noted. Mock
states are listed at the end; a story that names a state means that state's look check passes, and
the look is not repeated per story. Where a row has no mock state it says "no mock row; follow spec".

Server and CLI checks used by many stories:

- `GET` of the settings read endpoint for target `global` returns every registry key, none of the
  three secret values, and "set" markers for the secrets. Expected: status 200.
- A write of a bad value (for example `execution-queue.max-agents` set to `"x"`) returns a refusal
  naming the key; the file is unchanged.
- A write of `execution-queue.waiting-slot-grace` set to `"5mm"` (global) is refused with the key and a
  reason, and config.json is unchanged; the same key set to `"5m"` is saved.
- `gx server settings --json` returns the same keys and values as the read endpoint, and
  `gx server settings-set --json` with a key change gives the same result as the write endpoint.
- `go test` of the registry package passes; deleting a registry entry in a scratch copy makes the
  guard fail with the key and the `.ai/index.md` pointer.

Per story:

1. Open Settings in the `settings` scenario; every config.json key appears. Flow: extended settings flow (global section). Mock states: `global-section`.
2. Sidebar lists the seven sections under the Global label (Execution queue to Ticket store); there is no section called Global, and `runner-wait` is in Agents. Mock states: `global-section`.
3. Click through sidebar sections. Mock states: `global-section`.
4. Change a key in a section; its chip count rises. Mock states: `global-section`.
5. At 744px click a sidebar section and change a key; the sidebar stays visible and the key saves. Mock states: `ipad-section`.
6. On the phone home, Security, the global sections and the projects are listed; tapping each opens its page. Mock states: `phone-home`.
7. Tap a project on the phone; its sections list; tapping one opens it. Mock states: `phone-project`.
8. At 390px, open a section and press the back button at each level. Mock states: `phone-section`.
9. Switch the scope picker to a project and back. Mock states: `project-section`.
10. Type a key name, a key path and a description word; each finds the key. Mock states: `search-results`.
11. Search for a word that matches keys in two sections; hits are grouped under section headings in place of the section page, and clearing the search returns to the section. Mock states: `search-results`.
12. On the phone home type a key name in the search box; the hit appears and tapping it opens its row. Mock states: `phone-home`.
13. Descriptions are present on every row. Mock states: `global-section`.
14. Toggle a bool and read config.json: the key is saved. Mock states: `global-section`.
15. Edit a number, a duration, a size and a string and save; cancel discards. Mock states: `global-section`.
16. The duration field shows the `5m` hint and the size field the `50G` hint; saving `10m` writes `"10m"` to config.json. Mock states: `global-section`.
17. Pick an enum value; config.json holds it. Mock states: `global-section`.
18. Add and remove a tag; config.json holds the list. Mock states: `global-section`.
19. No number list is live on the page (`budget.notification-thresholds` is a guard exclusion), so this runs at the registry and server API level, not in the browser: a `number-list` entry refuses `["abc"]` and accepts `[90]`. In the browser, the live string list `server.tab-env` takes `abc` as a tag (a string list has no item validator). Mock states: `global-section`.
20. Change a key and its dot appears; reset it and the dot disappears. Mock states: `global-section`.
21. Reset a changed key; the key is gone from config.json. Mock states: `global-section`.
22. Save a key; "Saved" shows under the field and is gone after about 2s (allow 3s). Mock states: `restart-needed`.
23. Save an invalid value; the draft stays and the error shows. Mock states: `refused-save`.
24. Server API test and page: save `execution-queue.waiting-slot-grace` as `"5mm"` and as `"0s"`, a size such as `"5x"`, an enum value that is not a member and a number below its minimum; each is refused with the key and a reason, nothing is written and nothing is clamped. Mock states: `refused-save`.
25. Server API test of refusals from `config.Parse` and `agentrunner.ValidateSetting`; nothing written.
26. Server API test: a two-key change where one key is invalid is refused naming that key; a change that fails only combined names every key of the change.
27. Open the page with a startup-only key unedited; "Needs restart" shows on exactly the registry's startup-only keys and on no live key. Mock states: `restart-needed`.
28. Save a startup-only key; "Restart needed to apply" shows. Mock states: `restart-needed`.
29. Save a live key; the server's reload applies it with no restart warning. For each key `FromConfig` does not carry (including `skills.*` and `runner-wait`; `spin-window`, `spin-cycles`, `retry-storm-launches`, `evolve.deep-read-cap` and `budget.notification-thresholds` are guard exclusions and not on the page), the audit result holds: it is live (the server uses the new value) or flagged startup-only (the page shows the restart marks).
30. Server API test: one web save causes exactly one reload and no OS banner, even with `server.watch-config` on; two concurrent saves of different keys both land.
31. Change each risky key and confirm the dialog names the risk; Cancel writes nothing, "Change it" writes. Mock states: `confirm-dialog`.
32. Change `ticket-store.path`; the dialog says the old data is not moved. Mock states: `confirm-dialog`.
33. The read response and the page DOM never contain the bot token or webhook URL. Mock states: `secrets`.
34. Replace and Clear a secret; config.json holds or lacks the key. Mock states: `secrets`.
35. The chat id shows in full on the page; `gx config show` still redacts it. Mock states: `secrets`.
36. Edit each command key and save with no extra confirm. Mock states: no mock row; follow spec.
37. Save an empty `cleanup-command` list; config.json holds `[]`, and reset removes the key.
38. Server API test: a config.json with unknown keys and a comment-free layout keeps every other key after a save.
39. Server API test: a config.json whose keys are not alphabetical keeps its key order after a save, and a new key is appended at the end of its object.
40. Server API test: edit another key by hand between page load and save; both edits survive.
41. Server API test: a two-key change with one invalid key writes neither.
42. Server API test: config.json as a symlink; the link and target both remain, target updated.
43. Open each Security row and sign out. Flow: `web-logins` scenario flows. Mock states: `global-section` (account rows), `phone-home`.
44. The default harness row in Agents changes the default. Flow: the codex flow on `default-harness`. Mock states: no mock row; follow spec.
45. Set the default harness to one whose program is not on PATH, globally and for a project; both are refused and nothing is written. Set one that is on PATH while agents wait or run; the response and the page show the switch notice with the counts. The `settings` scenario seeds one waiting and one running agent so the counts show. Mock states: no mock row; follow spec.
46. Visit `/settings/agents`; the browser lands on the Agents section. Flow: extended codex flow on `default-harness`.
47. The Agents section lists the keys named in the story, including `runner-wait`. Mock states: no mock row; follow spec.
48. Each key appears under its section. Mock states: no mock row; follow spec.
49. Each key appears under "Evolve and recovery". Mock states: no mock row; follow spec.
50. Run `gx server settings --json` and `gx server settings --project <name> --json`; the global read lists every registry key and the project read lists only the keys that apply to a project (no global-only key, no `notifications` key); neither shows a secret value.
51. Run `gx server settings-set --json` to set a global key, remove it, and set a project key; the files change as the page would change them, and a bad value is refused with the key and the file unchanged.
52. Open each project from the scope picker. Mock states: `project-section`.
53. Open a project's Project section. Mock states: `project-identity`.
54. Edit trunk, landing and auto-ff-merge; project.json holds them. Mock states: `project-identity`.
55. The project sidebar order is Project, overridable sections, Checks, and it has no Notifications. Mock states: `project-section`.
56. A project key that follows global shows the global value and Override. Mock states: `project-section`.
57. Override then "Use global value"; project.json gains then loses the key. Mock states: `project-section`.
58. The `max-agents` row shows the absent text with the current global cap (change the global cap to 6 and the text says 6) and "Set a cap for <project>". Mock states: `project-section` (this row differs from the mock; follow the spec).
59. Set a cap, see "Overridden" and "Remove cap", remove it; the key is gone. Mock states: `project-section`.
60. The two other caps show "Uses the global value". Mock states: `project-section`.
61. In the project that has a hand-written `notifications` block, the page shows no Notifications section and no secret rows; save another project key and project.json still holds the same `notifications` block; a write of any `notifications` key to a project, by the API or by `gx server settings-set --project`, is refused with the key named and nothing written; so is a write of an `agents` sub-key the merge ignores. Mock states: no mock row; follow spec.
62. Open the project's Evolve row in each of absent, off and on. Mock states: no mock row; follow spec.
63. Server API test and flow: Off writes `false`; "Use global value" and On both remove it. Mock states: no mock row; follow spec.
64. Server API test: writing `evolve.deep-read-cap` to a project is refused with an error naming `evolve.deep-read-cap`; the project page shows no such row.
65. A project's Agents section lists only the merge-applied keys. Mock states: no mock row; follow spec.
66. A project's skills rows are only `implement` and `code-review`. Mock states: no mock row; follow spec.
67. Override `cleanup-command` in a project; project.json holds the list. Mock states: no mock row; follow spec.
68. Open the project's Checks section. Mock states: `checks-editor`.
69. Walk each check card through default, off, one command and changed + full. Mock states: `checks-editor`.
70. Set suite, hang and slow on the test card. Mock states: `checks-editor`.
71. Set `slow` to `10m`; project.json holds it as one duration. Mock states: `checks-editor`.
72. A project.json with a per-tool `slow` map shows three fields and keeps the map form after a save. Mock states: `checks-editor`.
73. Clear a `slow` field; the limit is removed. Mock states: `checks-editor`.
74. Add a command to `always`; project.json holds it. Mock states: `checks-editor`.
75. Server API test: a project.json edit that `ValidateProjectFile` rejects writes nothing; a `checks` of `{"changed":"x"}` with no `full` is refused with `check.ParseConfig`'s message and nothing is written.
76. Create `.gx/checks.json` in the project repo; the Checks section shows the notice that the repo file wins; remove the file and the notice goes. Mock states: `checks-editor` (notice has no mock row; follow spec).
77. Server API test: after a project.json write (and `Flush`, or waiting out the debounce) the store has a commit with it.
78. Save a project key such as `agents.default-harness`; the page shows the new value without a reload, then `gx server default-harness --json` for that project reports the same value.
79. Name, repo and vcs show with no edit control. Mock states: `project-identity`.
80. The page and the read response omit `$schema`, `resume-on-start` and every TUI-only key (`use-nerdfont-icons`, `image-diffs`, `stage-diff-context-lines`, `input-modal-bottom`, `name-aliases`, `log`, `agents.leader-key`, `agents.focus-sidebar-key`); the guard's exclusion list holds each with its reason.
81. `cpu-watchdog.enabled` (read by `cmd/tui` and the server settings) is on the page and editable.
82. `.ai/index.md` has the "Adding a config key" section naming the three steps (inspect with `rg`).
83. Registry guard test, as above.
84. Registry guard test: a removed struct key fails the guard.
85. Registry guard test: a type mismatch fails (including a duration entry typed as number), and a secret-type entry that is not `secret:"true"` tagged, or the reverse (other than `chat-id`), fails.
86. Schema snapshot test: edit a registry description in a scratch copy and the test fails with the `-update` command; the `check` package test fails on a stale `checks-schema.json`.
87. Registry test: the derived whitelist matches the flags.
88. Registry test: a project.json carrying an ignored `agents` sub-key or a `notifications` block still loads.
89. The `settings` scenario starts and serves the page; its fixture test passes.
90. Load every state at 390px; no horizontal page scroll. Mock states: `phone-home`, `phone-project`, `phone-section`.

New flows and scenarios to write (these become implementation tickets):

- The `settings` fixture scenario (new).
- Extensions to the existing settings e2e flow (global edit, project override and reset, refused
  save, secrets) and to the codex flow (the `/settings/agents` redirect). No new flow unless the
  flow budget and `gx-playwright-e2es` allow it.

**Mock states** (13 approved; the `tui-types` state is dropped). The fixture is the new `settings`
scenario for every state; `web-logins` and `default-harness` fit none of the states (they serve the
account-page and Agents-redirect flows above).

- `global-section`, width 1000, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page, reset-link.
- `project-section`, width 1000, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page, overridden-chip, reset-link, inherits-global.
- `project-identity`, width 1000, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page.
- `checks-editor`, width 1000, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page, checks-editor.
- `refused-save`, width 1000, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page, reset-link, error-message.
- `restart-needed`, width 1000, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page, restart-badge, reset-link, saved-message.
- `confirm-dialog`, width 1000, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page.
- `search-results`, width 1000, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page, reset-link.
- `secrets`, width 1000, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page.
- `ipad-section`, width 744, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: sidebar, scope-picker, search, page, overridden-chip, reset-link, inherits-global.
- `phone-home`, width 390, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: page, search.
- `phone-project`, width 390, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: page.
- `phone-section`, width 390, scenario `settings`, mock `/Users/david/.local/share/gx/tickets/gx/settings-page/mocks/04-settings-page-layout.html`, parts: page, overridden-chip, reset-link, inherits-global.


## Out of Scope

- Renaming a project from the web. `name` is part of every ticket address, branch and log, so a safe
  rename is a feature of its own. The page shows `name`, `repo` and `vcs` read-only.
- Adding or removing projects (creating or deleting a `project.json`). `gx tickets migrate` creates
  projects, and deleting one touches its tickets and history.
- `$schema`: not a setting.
- Any key only the TUI reads (the TUI section, "TUI only" chips, `log.important-refs`,
  `log.hide-refs`, `name-aliases`, `agents.leader-key` and the like). They are on the guard's
  exclusion list.
- Overriding `notifications` (the Telegram bot token, the Slack webhook URL, and the whole block) for
  a project from the web. A project's secrets would be committed to the ticket store's git history
  and pushed. The block stays accepted in project.json and untouched by the page; if it is needed
  later, it gets its own design.
- Moving the old data when `ticket-store.path` changes.
- A version or hash check against hand edits of the files (last write wins on the changed key).
- Editing `agents` sub-keys in a project that the config merge ignores.
- CLI verbs beyond `gx server settings` and `gx server settings-set` (the existing
  `gx server default-harness` stays as it is).

## Further Notes

There are no open design questions.

Implementer's choices, not decided here: registry type and field names; the key-path
representation (it must handle the flat dotted key `evolve.deep-read-cap`); the wire shape of the
key-change request and the CLI flag syntax; how the writer keeps the file watcher from reloading a
second time; and which keys outside `FromConfig` become live and which are flagged startup-only (the
audit in "Applying a change").

Facts the code already settles, written here so they are not rediscovered:

- The overridable `skills` keys are `implement` and `code-review` (`WithProject`).
- The store loop already commits project.json (it polls `git status` and runs `add -A`, debounced).
- The TUI-only list is in "Exclusion list" above.
- `check.LoadConfig` prefers `.gx/checks.json` in the repo over project.json `checks`.

### Where the mock is wrong

Follow the spec where the stored mock disagrees:

1. **Project `max-agents` row.** The mock shows "Use global value (6)". The row is a per-project cap
   with no inheritance: "No per-project cap (the global cap of X still applies)" with X the current
   global cap, and "Set a cap for <project>", then "Overridden" and "Remove cap".
2. **TUI section.** The mock's sidebar has a "TUI" section with "TUI only" chips. The page shows no
   TUI-only keys, so the section and chips do not exist.
3. **`tui-types` state.** The mock holds a `tui-types` state. It is dropped, which leaves 13 approved
   states.
4. **"Global" section.** The mock's "Global" is the sidebar group label (and "Global settings" on the
   phone home). There are seven sections, not an eighth called Global.
5. **Project Notifications and secret rows.** Any project-scope Notifications section, "Override for
   <project>" on notifications or project secret row in the mock is not approved: the project page
   has none (see the project override rules).

Ticket 04's resolution says "14 states", a "TUI" section and "TUI only" chips; ticket 06 overrides
it. The mock's evolve project row also predates the switch rule above; follow the spec for it too.
