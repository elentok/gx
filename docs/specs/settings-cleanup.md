# Settings cleanup

Epic `settings-cleanup` (project gx). Regroups the page that `docs/specs/settings-page.md` specifies.

Where this spec and `settings-page.md` disagree, this spec wins; the listed section names, the sidebar
shape and the phone home are replaced here. Where this spec and a stored mock disagree, follow this
spec. The known disagreements are under "Where the mocks are wrong" in Further Notes.

Terms: Settings category, Settings section and Settings group, as defined in GLOSSARY.md "Settings page".

## Problem Statement

The settings page has 14 flat global sections (Execution queue to Ticket store) and a flat list of keys
inside each. A person looking for a key cannot tell which section holds it: Agents and Models sit far
apart, CPU watchdog and Process watchdog are two sections for one topic, desktop banner and sound keys
sit in Agents, and the Agents section shows 11 unrelated rows in one block. The
Models section lists 12 keys in a row (`agents.claude.light.model` and so on), so a person cannot see
the three tiers of Claude and Codex side by side.

## Solution

The sidebar groups the sections into Settings categories: Agents, Ralph loop, Notifications, Web UI,
System and Account. Each section page splits its keys into titled Settings groups. The Models section
shows one table per agent (Claude, Codex) with the tiers Light, Standard and Strongest as rows and
Model and Effort as columns. A project's view uses the same categories and shows only the sections that
hold a key the project can override. Old section links redirect to the key's new section. Only the page
changes: keys in config.json and project.json keep their paths.

## User Stories

1. As a gx user, I want the settings sidebar to list sections under categories (Agents, Ralph loop, Notifications, Web UI, System, Account), so that I find a setting by topic.
2. As a gx user, I want Agents and Models in one category, so that all agent settings sit together.
3. As a gx user, I want the two queue sections, Budget, Pull requests and Evolve and recovery under Ralph loop, so that the settings of the loop sit together.
4. As a gx user, I want the CPU and process watchdogs in one Watchdogs section, so that I do not look in two places for one topic.
5. As a gx user, I want the desktop banner and sound keys under Notifications, so that every way gx tells me something is in one place.
6. As a gx user, I want the web access keys (listen, allowed hosts, sign-in code) in a Web UI section called Access, so that I find the network and sign-in settings together.
7. As a gx user, I want the sign-in code setting to stay in Web UI, Access, so that Account keeps only live actions and Access keeps config keys.
8. As a gx user, I want Keybindings and a Display and development section in Web UI, so that every web-only setting is under one category.
9. As a gx user, I want the Server, Ticket store, Disk space and Watchdogs sections under System, so that the machine-level settings sit together.
10. As a gx user, I want Account to be a category that lists Web logins, Password, Authenticator, Certificate and "Sign out of this device", so that those pages stay one click away and look like the other categories.
11. As a desktop user, I want each category to collapse and expand, so that a long sidebar stays short.
12. As a desktop user, I want only the category of the current section to start open, so that I see where I am and nothing else.
13. As a desktop user, I want a collapsed category to show the summed changed count of its sections, so that I see where a changed key hides without opening it.
14. As a desktop user, I want a section in an open category to show its own changed count, so that I see which sections hold keys set in the file.
15. As a desktop user, I want a section page to show its category as a breadcrumb above its title, so that I know where I am.
16. As a gx user, I want every section to be split into titled Settings groups, so that a page is not one flat list.
17. As a gx user, I want a group title to carry a changed count, so that I see which group holds the changed keys.
18. As a gx user, I want a group with one key to keep its title, so that every section has the same shape.
19. As a gx user, I want a group to show only its title, with the help text staying on each key, so that help sits next to what it explains.
20. As a gx user, I want the Execution queue section split into Agent limits and Waiting and stalled agents, so that caps and waits are apart.
21. As a gx user, I want Budget split into Spend limits and Alerts, and Pull requests split into Skills, CI fix limits and Waiting, so that each topic has a block.
22. As a gx user, I want Evolve and recovery split into Evolve and Recovery, so that the two features are apart.
23. As a gx user, I want Notifications split into Desktop, Telegram and Slack, so that each channel has a block.
24. As a gx user, I want Server, Ticket store, Disk space and Watchdogs split into the groups in the table below, so that related keys sit together.
25. As a gx user, I want the Models section to show one table per agent with Light, Standard and Strongest as rows and Model and Effort as columns, so that I see all tiers of one agent at once.
26. As a gx user, I want Claude and Codex to be separate groups in Models, so that each agent has its own block with its own changed count.
27. As a gx user, I want each tier row to show its key pattern (for example `agents.claude.light.*`), so that I can match a cell to its key.
28. As a gx user, I want the Standard row to stand for the key without a tier prefix (`agents.claude.*`), so that I know it is the default tier.
29. As a gx user, I want a cell to show its effective value, and faint italic text ("Claude default", "Codex default") when that value is empty (a key set to `""`, or a missing key whose gx default is empty, as for Claude Light effort), with the amber edge and a reset only when the key is set in the file, so that I tell gx's default from "the agent CLI decides" and still see what I changed.
30. As a gx user, I want a cell that is set in the file to show an amber edge and its own reset button, so that I see what I changed and can undo one cell.
31. As a gx user, I want a cell to show its value and open the shared text control (input, Save, Cancel; Enter saves, Escape cancels) inside the cell when I click it, so that I edit a model or effort the same way as any other text key. Text rows gain Enter and Escape too, because the control is shared.
32. As a gx user, I want saving an empty field to write `""` in global scope (and remove the key in a project), so that I can still clear a tier to the agent CLI's own setting from the page.
33. As a gx user, I want Models to be the only table and every other group to be a list of keys, so that the page stays predictable.
34. As a gx user in a project view, I want the sidebar to show the same categories but only sections that hold an overridable key, so that I see only what applies.
35. As a gx user in a project view, I want Project first and Checks last, so that identity comes first and the checks editor comes last.
36. As a gx user in a project view, I want a note under the section title that an empty value uses the global setting, so that I understand an empty cell.
37. As a gx user in a project view, I want an empty Models cell to show "global: <value>" as its placeholder (the global effective value: gx's default, or "Claude default" / "Codex default" when that value is empty, so "global: Claude default" for a missing Claude Light effort), so that I see what the project inherits.
38. As a gx user in a project view, I want a project's cell that holds a value to show the amber edge and a reset that removes the override, so that I can drop one override.
39. As a gx user in a project view, I want the other overridable rows to keep their "Uses the global value" and "Overridden" states, so that nothing I know changes.
40. As a gx user in a project view, I want the project Project section to hold a read-only Repository group (name, repo, vcs) and a Delivery group (trunk, landing, delivery, auto fast-forward merge, auto pull request), so that identity and delivery rules are apart.
41. As a gx user with a bookmark to `/settings/s/cpu-watchdog`, `/settings/s/process-watchdog` or `/settings/s/web-ui`, I want the link to redirect to the section that now holds those keys, so that old links keep working.
42. As a gx user with a bookmark to a project's old section link, I want the same redirect inside that project, so that project bookmarks keep working.
43. As a phone user, I want Settings home to list every category as a heading with its sections as tiles, so that I see the same tree as the desktop sidebar.
44. As a phone user, I want a tile to show its changed count and a chevron, so that I see where changed keys are.
45. As a phone user, I want a section to open as a drill-down with a back arrow and its category as breadcrumb, so that I can climb out without the browser back (from a project section the arrow returns to that project's home).
46. As a phone user, I want a scope picker on Settings home, so that I switch between Global and a project there, and a project's home (`/settings/p/<project>`) to be Settings home with the picker set to that project.
47. As a phone user, I want the Models table to stay a table with narrower cells and the key pattern hidden, so that it fits 390px.
48. As a gx user, I want search results to group by section (with its category as breadcrumb) and then by group, so that I see where each hit lives.
49. As a gx user, I want the match highlighted in the key name, so that I see why a row matched.
50. As a gx user, I want the sidebar chips to show hit counts while a search is active and the categories with hits to be open, so that I see where the hits are.
51. As a gx user, I want a search on the phone to show the same grouping and to cover only the scope picked in the scope picker, like the desktop (a deliberate change: the old phone search covered global and every project at once), with each hit a link that opens its section, so that the phone and desktop agree.
52. As a contributor adding a config key, I want the registry entry to name the key's category, section and group, so that the page puts the key in the right place with no web change.
53. As a contributor, I want the web to hold no list of sections or categories, so that a new section needs no web change.
54. As a contributor, I want a test that fails when an entry has no category, section or group, or when a category or section is empty, so that the page never shows a hole.
55. As a contributor, I want the `.ai/index.md` rule "Adding a config key" to say that an entry names its category, section and group, so that I do not forget.

## Implementation Decisions

### Page tree

The registry holds, per key: category, section and group. Names and field shapes are the implementer's
choice. The tree is this table. Keys keep their paths; only the page changes. `{,light.,strongest.}`
means three keys.

Category names: Agents, Ralph loop (provisional: the queue has several names, so the name may change
later; it is one string in the registry), Notifications, Web UI, System, Account. A project view adds
two more: Project (first) and Checks (last, a one-section category).

| Category | Section | Group | Keys |
|---|---|---|---|
| Agents | Agents | Defaults | `agents.default-harness` |
| | | Sessions | `agents.restore-sessions-on-start`, `agents.fix-agent-remote-control` |
| | | Launch | `agents.claude-args`, `agents.writable-directories` |
| | | Runner | `agent-runner`, `runner-wait` |
| | Models | Claude | `agents.claude.{,light.,strongest.}{model,effort}` |
| | | Codex | `agents.codex.{,light.,strongest.}{model,effort}` |
| Ralph loop | Execution queue | Agent limits | `execution-queue.max-agents` (project: `max-agents`, `project.max-agents`), `execution-queue.max-agents-per-epic`, `execution-queue.max-one-off-agents` |
| | | Waiting and stalled agents | `execution-queue.waiting-slot-grace`, `execution-queue.max-released-agents`, `execution-queue.stalled-nudge-delay` |
| | Budget | Spend limits | `budget.soft-limit`, `budget.hard-limit` |
| | | Alerts | `budget.long-ticket-turns`, `subscription.suppress-extra-usage-warning` |
| | Pull requests | Skills | `skills.ci-fix`, `skills.pr-text` |
| | | CI fix limits | `pr.ci-fix-attempts-per-check`, `pr.ci-fix-attempts-per-pr` |
| | | Waiting | `pr.no-checks-grace` |
| | Evolve and recovery | Evolve | `evolve`, `project.evolve` (project view) |
| | | Recovery | `recovery.enabled`, `recovery.disabled`, `recovery.follow-ups`, `recovery.notify-hold` |
| Notifications | Notifications | Desktop | `agents.banners.enabled`, `agents.sounds.{enabled,done,attention}` |
| | | Telegram | `notifications.telegram.{bot-token,chat-id}` |
| | | Slack | `notifications.slack.webhook-url` |
| Web UI | Access | Network | `server.web.listen`, `server.web.allowed-hosts`, `server.web.files.roots` |
| | | Sign-in | `server.web.totp` |
| | Keybindings | Leader key | `server.web.leader-key-mac`, `server.web.leader-key` |
| | | Editors | `server.web.vi-escape` |
| | Display and development | Display | `server.recently-finished` |
| | | Development | `server.web.dev-frontend`, `server.web.dev-dir` |
| System | Server | Runtime | `server.watch-config`, `server.tab-env` |
| | | Logs | `server.log-retention` |
| | Ticket store | Location | `ticket-store.path` |
| | | Git sync | `ticket-store.commit-debounce`, `ticket-store.push-remote` |
| | Disk space | Thresholds | `disk-space.warn-below`, `disk-space.clean-below` |
| | | Cleanup | `disk-space.cleanup-command` |
| | Watchdogs | CPU | `cpu-watchdog.{enabled,threshold-percent,sustain,cooldown}` |
| | | Process | `watchdog.{max-tree-processes,interval,ulimit-headroom}` |
| Account | Web logins, Password, Authenticator, Certificate, "Sign out of this device" (unchanged pages and action) | | |
| Project (project view only) | Project | Repository (read-only header) | `name`, `repo`, `vcs` |
| | | Delivery | `trunk`, `landing`, `delivery`, `auto-ff-merge`, `auto-pr` |
| Checks (project view only) | Checks | Checks | `checks` |

- **One owner of the tree.** The Go registry owns categories, sections, groups and their order. The
  read response carries each entry's category, section and group and the order of categories and
  sections. The web drops `GLOBAL_SECTIONS` and every hard-coded section name; the sidebar, the phone
  home, search and the redirects are all built from the response. The web appends only the Account
  category, which has no registry keys.
- **Single-key groups stay groups**, so every section has the same shape.
- **Group order and key order** inside a group follow the registry order, which follows the table.
- **Project view.** The read for a project returns only the keys that apply to it, as today, so
  its tree is whatever those keys fill. Order: Project first, then the categories in the order above
  that still hold a key, Checks last. Account never shows in a project view. Notifications and System
  show only if a key there is project-overridable (System: Disk space, group Cleanup, through
  `disk-space.cleanup-command`; Notifications: none today, so it does not show).
- **Groups in a project** show only their overridable keys; a group with none is hidden. For example,
  a project's Agents section shows Defaults (`agents.default-harness`) and Launch
  (`agents.writable-directories` only).
- **Retired section links.** The old slugs `web-ui`, `cpu-watchdog` and `process-watchdog` have no
  section of that name now. `web-ui` redirects to Access (the section that holds the old section's
  `listen` and `allowed-hosts` keys); `cpu-watchdog` and `process-watchdog` redirect to Watchdogs. The
  redirect is data owned by the registry and sent in the read response, so the web has no section
  names. The same redirect applies inside a project: `/settings/p/<project>/s/<old>` goes to the new
  slug when the project has that section, else to the project's first section. A slug that is a
  section today (`agents`, `server`, `notifications` and the rest) does not redirect. Section slugs stay
  the section name, lower-cased with dashes (`display-and-development`, `evolve-and-recovery`).
- **`/settings/agents` is the one exception to "the web holds no section names".** It is not a
  `/s/<slug>` link, so it stays a fixed route in the web that redirects to `/settings/s/agents`; it
  does not go through the registry's retired-slug data.

### Sidebar (desktop and iPad mini)

Look B of the approved sidebar mock.

- Scope picker and search box stay on top. Under them, each Settings category is a collapsible heading
  with a caret; its sections are indented links.
- Only the category of the current section starts open. A person's open and close clicks stay while
  the page is open; opening a section from a link opens its category. Nothing is stored in the browser.
- A collapsed category shows the sum of its sections' changed counts as a chip (hidden at zero). A
  section in an open category shows its own chip.
- "Changed" keeps its meaning from `settings-page.md`: present in the file (in a project, present in
  project.json).
- Account lists Web logins, Password, Authenticator (shown only with the sign-in code on; "Add
  authenticator" when none is set up, as today), Certificate and "Sign out of this device". These stay
  live pages and an action, not registry sections.
- A section page shows its category as a breadcrumb above the title. The breadcrumb is not a heading
  (flow 117 matches the section title by heading role).

### Section page and groups

- A section page is a stack of titled blocks. Each block has the group title and a changed-count chip
  (hidden at zero) in a title bar, then its rows. The row controls, Reset link, project states, risky
  confirms, restart marks and secrets are unchanged from `settings-page.md`.
- Help text stays on each key. A group has no description (the registry gets no group help field).
- The Checks section keeps its checks editor and the Project section keeps its read-only Repository
  header; their cards and rows are unchanged.

### Models table

Look A of the approved Models mock. Models is the only table.

- Each agent group (Claude, Codex) renders one table. Rows: Light, Standard, Strongest. Columns:
  Model, Effort. The row header shows the key pattern in small text: `agents.claude.light.*`,
  `agents.claude.*` (Standard) and `agents.claude.strongest.*`; same for Codex.
- The registry says which group is a table and which tier and column each key is. The web does not
  parse key paths. Field shapes are the implementer's choice.
- A cell shows its value. A click opens the shared text control (input, Save, Cancel) inside the cell.
  The shared `TextControl` in `webui/src/settings/SettingsRow.tsx` gains Enter (save) and Escape
  (cancel), so text rows gain them too and rows and cells stay one control. Enter and Escape act on
  that cell only; no other row changes. After a successful save, Cancel or Escape the cell goes back to
  showing its value. After a refused save it stays open with the draft and shows "Not saved: <reason>"
  under the cell, as for any key. A cell save goes through the same writer, validation and reload as a
  row save; no new endpoint.
- A cell shows its effective value. A missing key shows gx's own default (for example `haiku`, `opus`,
  `high`) as a plain, unchanged value. When the effective value is empty, the cell shows faint italic
  text naming the agent CLI's own setting, "Claude default" or "Codex default". That happens for a key
  set to `""` and for a missing key whose gx default is empty (`agents.claude.light.effort`, which has
  no gx default). The amber edge and the reset follow "set in the file" only, so a missing
  `agents.claude.light.effort` shows faint "Claude default" with no amber edge, and a key set to `""`
  shows the same text with the amber edge and a reset. Saving an empty field writes `""` in global
  scope and removes the key in project scope. Reset removes the key, so gx's default comes back.
- A cell that is set in the file (including `""`) shows an amber edge and a reset button (arrow) inside
  the cell. Reset removes that one key. The group title's chip counts the set cells of that agent.
- Project view: a note under the title says "Project overrides for <project>. An empty value uses the
  global setting." An empty cell (key missing) shows "global: <value>" in the faint style, where
  <value> is the global effective value: the global key's value, else gx's default, else "Claude
  default" (or "Codex default") when that is empty. A project cell set to `""` (possible from an
  earlier text save or a hand edit) is "present in project.json": it shows the same faint "global:
  <value>" text (the value inherited through `firstNonEmpty`) with the amber edge and a reset, counts
  in the group chip, and its reset removes the key. A set cell with a value shows the amber edge and
  its reset removes the project's override (its tooltip says "Use global value"). The "Uses the global
  value" rows and the Override button do not appear in the Models section; the table replaces them.
- The note shows only in the Models section. Other project sections keep their per-row "Uses the
  global value" and "Overridden" states.
- Phone: the table stays, with narrower cells and the key pattern hidden. The row header shows Light,
  Standard or Strongest only. The table scrolls inside its block if a long value needs it; the page
  does not scroll sideways.
- In search results Models keys show as ordinary rows with their full key path, not as a table.

### Phone

- Settings home shows the scope picker and search box on top, then every category as a heading with its
  sections as tiles. Categories do not collapse on the phone. A tile shows its changed chip and a
  chevron. Account lists Web logins, Password, Authenticator, Certificate and "Sign out of this
  device" as tiles. The separate "Projects" list of the old home is gone: the scope picker picks the
  project, and the home then shows the project's tree.
- Project scope on the phone: `/settings/p/<project>` is Settings home with the scope picker set to that
  project. It shows the project tree and no Account.
- A section opens as a drill-down page with a back arrow in the top bar and its category as breadcrumb.
  The back arrow returns to Settings home; from a project section it returns to that project's home
  (`/settings/p/<project>`). Groups stack as on the desktop; every control fits 390px
  with no sideways page scroll.

### Search

- Search stays client-side over the entries of the current scope, by name, key path and description.
- Results replace the section page. They group by section (title, with its category as a breadcrumb),
  then by group (a smaller sub-heading), with the match highlighted in the name or key path. The
  heading says "<n> results for "<query>"". All hits show; nothing is cut to "and N more".
- While a search is active the sidebar shows only categories that have hits, open, with hit counts in
  the chips in place of changed counts. Clearing the search restores the section page and the normal
  sidebar.
- On the phone, the search box on Settings home shows the same results grouping. It covers only the
  scope picked in the scope picker, like the desktop. This is deliberate: the old phone search covered
  global and every project at once, and that is gone. Each hit stays a link that opens its section
  (it is not an editable row).

### Registry guard

- The guard test also asserts: every entry has a category, section and group; no section is empty; no
  category (other than Account, which the web adds) is empty; no group is empty. A second test pins the
  placement of every key to the table above, so a move is a deliberate change.
- `.ai/index.md` "Adding a config key" gains one line: the entry names its category, section and group.

## Testing Decisions

A good test checks what the page shows and what the read response contains, never component structure
or registry layout. No new seam: use the existing ones, highest first.

1. **Go test at the registry package.** The tree table (placement of every key), the guard additions
   above, the project tree for the `settings` fixture (Project first, Checks last, no Notifications),
   and the retired-slug redirects. Prior art: the guard and schema tests in `config/registry`.
2. **Server API test** through the existing settings read tests: the read response carries category,
   section, group, the category and section order and the retired-slug redirects; the project read
   carries only applicable keys. The `gx server settings --json` verb returns the same.
3. **vitest** for page logic with no server: sidebar open and closed categories and chip sums, the
   group blocks with chips and single-key groups, the Models table (empty, set, project placeholder,
   cell save, empty save writing `""` in global and removing the key in a project, narrow layout with the key pattern hidden; a missing
   `agents.claude.light.effort` showing faint "Claude default" with no amber edge and, in a project, "global: Claude default"; a project
   cell set to `""` showing faint "global: <value>" with the amber edge, counted in the chip, and reset removing the key; Enter saves and
   Escape cancels in both a text row and a Models cell, a cell closing after save, Cancel or Escape and staying open with the draft
   after a refused save), search grouping and hit counts (the phone search covers only the picked scope), the phone home tree
   (`/settings/p/<project>` on the phone is Settings home with the picker on that project; a project section's back arrow goes to that
   home), and the redirects. Prior art: `SettingsShell.test.tsx`,
   `SettingsPage.test.tsx`, `SettingsRow.test.tsx`.
4. **Playwright**, per `gx-playwright-e2es`. Default to no new flow: extend flows 200 and 201 in
   `settings-page.e2e.ts` (they already walk the global and project journeys; update their section
   steps to the new tree) and add the Models cell edit and an old-link redirect to them. The flow
   budget is respected; the implementer reads the skill before writing a flow.
5. **Fixture scenario.** Extend the `settings` scenario so the stored mock states render: non-default
   values in the Claude and Codex tiers, a global tier key set to `""` (`agents.codex.light.effort: ""`), in the watchdog keys and in
   Execution queue, a project with Models overrides, and a project with a `checks` entry.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built, in the `settings`
scenario unless noted. Mock states are listed at the end; a story that names a state means that
state's look check passes, and the look is not repeated per story.

Server and CLI checks used by many stories:

- `GET` of the settings read endpoint for `global` returns, per key, category, section and group, the
  category and section order, and the retired-slug redirects. Expected: status 200.
- `gx server settings --json` returns the same placement fields as the endpoint; with `--project <name>`
  it returns only applicable keys.
- `go test` of the registry package passes; moving a key to a section that does not exist in a scratch
  copy makes the guard fail naming the key.

Per story:

1. Open Settings; the sidebar lists Agents, Ralph loop, Notifications, Web UI, System and Account. Flow: extended 200. Mock states: `desktop-sidebar`.
2. Agents and Models are under Agents. Mock states: `desktop-sidebar`.
3. Execution queue, Budget, Pull requests and Evolve and recovery are under Ralph loop. Mock states: `section-with-groups`.
4. Watchdogs is one section with CPU and Process groups. Mock states: `phone-section`.
5. `agents.banners.enabled` and `agents.sounds.*` appear under Notifications, Desktop, and not under Agents. Flow: extended 200.
6. Access holds Network (`server.web.listen`, `server.web.allowed-hosts`, `server.web.files.roots`). Flow: extended 200.
7. `server.web.totp` is in Web UI, Access, Sign-in; Account does not list it. Flow: extended 200.
8. Keybindings and Display and development are under Web UI. Flow: extended 200.
9. Server, Ticket store, Disk space and Watchdogs are under System. Flow: extended 200.
10. Open Account; it lists the five entries; "Sign out of this device" signs out. Flow: `settings.e2e.ts` flows 106 to 112 (web-logins scenario). Mock states: `desktop-sidebar-account`.
11. Click a category heading; it opens, click again and it closes. Mock states: `desktop-sidebar`.
12. Load a section of Ralph loop; only Ralph loop starts open. Mock states: `section-with-groups`.
13. With keys set in Agents (Models) and System (Watchdogs), collapsed Agents and System show the summed chips. Mock states: `desktop-sidebar`.
14. An open category's sections show their own chips. Mock states: `section-with-groups`.
15. A section page shows its category as breadcrumb. Mock states: `section-with-groups`.
16. Every section page shows titled groups. Mock states: `section-with-groups`.
17. Change a key; its group chip rises by one. Flow: extended 200. Mock states: `section-with-groups`.
18. A section with a single-key group (Ticket store, Location) shows the group title. Mock states: no mock row; follow spec.
19. Group blocks show no description text; each key still shows its help. Mock states: `section-with-groups`.
20. Execution queue shows Agent limits and Waiting and stalled agents with the keys in the table. Mock states: `section-with-groups`.
21. Budget shows Spend limits and Alerts; Pull requests shows Skills, CI fix limits and Waiting. Mock states: no mock row; follow spec.
22. Evolve and recovery shows Evolve and Recovery. Mock states: no mock row; follow spec.
23. Notifications shows Desktop, Telegram and Slack. Mock states: no mock row; follow spec.
24. Each of Server, Ticket store, Disk space and Watchdogs shows its groups from the table. Mock states: `phone-section`.
25. Open Models; each agent shows a table with Light, Standard, Strongest and Model, Effort. Mock states: `desktop-global`.
26. Claude and Codex are separate blocks with their own chips. Mock states: `desktop-global`.
27. Each tier row shows its key pattern. Mock states: `desktop-global`.
28. The Standard row shows `agents.claude.*` and `agents.codex.*`. Mock states: `desktop-global`.
29. A missing key shows gx's default (for example `haiku`) as a plain, unchanged value; a missing `agents.claude.light.effort` (no gx default) shows faint italic "Claude default" with no amber edge; the fixture's `agents.codex.light.effort: ""` shows faint italic "Codex default" with the amber edge and a reset. Mock states: `desktop-global` (see "Where the mocks are wrong").
30. A set cell shows an amber edge and a reset; reset removes only that key from config.json. Flow: extended 200 (Models step). Mock states: `desktop-global`.
31. Click a cell; the input with Save and Cancel opens inside it. Type `opus`, press Enter or Save; config.json holds the key; Escape or Cancel discards a draft and changes only that cell; a refused value shows "Not saved:" under the cell. Flow: extended 200 (Models step).
32. Save an empty field in a set global cell; config.json holds the key as `""` and the cell shows the "Claude default" text. Reset it; the key is gone and gx's default value shows. In a project, save an empty field in a set cell; the key is gone from project.json. Flow: extended 200 and 201 (Models step).
33. No other section renders a table (Watchdogs, Execution queue and Web UI show list rows). Mock states: `phone-section`.
34. Switch the scope picker to the fixture project; only sections with overridable keys show, in categories. Flow: extended 201. Mock states: `project-scope`.
35. The project sidebar order is Project first and Checks last. Flow: extended 201. Mock states: `project-scope`.
36. In the project's Models the note says an empty value uses the global setting. Mock states: `project-scope`, `desktop-project`.
37. An empty project cell shows "global: <value>" (change the global value and the text follows); for Claude Light effort with no global value it reads "global: Claude default". A project cell set to `""` in project.json shows the same text with the amber edge and a reset, and reset removes the key. Mock states: `desktop-project`.
38. Set a project cell, then click its reset; project.json gains and loses the key. Flow: extended 201 (Models step). Mock states: `desktop-project`.
39. In the project's Execution queue, `execution-queue.max-agents-per-epic` shows "Uses the global value" and Override; the per-project cap row keeps its states. Flow: extended 201.
40. The project's Project section shows Repository (read-only) and Delivery groups; trunk, landing, delivery, auto fast-forward merge and auto pull request edit and save. Flow: extended 201.
41. Visit `/settings/s/web-ui`, `/settings/s/cpu-watchdog` and `/settings/s/process-watchdog`; the browser lands on Access, Watchdogs and Watchdogs. Flow: extended 200 (redirect step).
42. Visit `/settings/p/<project>/s/cpu-watchdog`; the project has no Watchdogs, so the browser lands on the project's first section; visit `/settings/p/<project>/s/web-ui` likewise. Flow: extended 201 (redirect step).
43. At 390px open Settings home; every category shows as a heading with its sections as tiles and none collapses. Mock states: `phone-home`.
44. Tiles show changed chips and chevrons. Mock states: `phone-home`.
45. Tap a tile; the section opens with a back arrow and the category breadcrumb; the back arrow returns to Settings home. In a project (`/settings/p/<project>`), open a section; the back arrow returns to `/settings/p/<project>`. Mock states: `phone-section`.
46. On Settings home pick a project in the scope picker; the home shows the project's tree with no Account, and the URL is `/settings/p/<project>` with the picker set to that project. Flow: extended 200 at 390px (the existing phone step). Mock states: `phone-home`.
47. At 390px open Models; the table shows with no key pattern and no sideways page scroll. Mock states: `phone-global`, `phone-project`.
48. Search `agents`; hits group by section with category breadcrumb, then by group. Mock states: `search-results`.
49. The match is highlighted in the name and key path. Mock states: `search-results`.
50. While searching, the sidebar shows hit counts and only matching categories, open; clearing the search restores the section page. Mock states: `search-results`.
51. On the phone home type `agents`; the results group by section then group, cover only the scope in the picker (switch the picker to a project and the hits change; no "Global ·" or "<project> ·" cross-scope groups), and tapping a hit opens its section. Mock states: no mock row; follow spec.
52. Registry guard test: an entry with no group fails with the key named.
53. `rg GLOBAL_SECTIONS webui/src` finds nothing; add a scratch registry section and it shows in the sidebar with no web change (vitest on the response shape).
54. Registry guard test: an empty section, category or group fails; the placement test fails when a key moves.
55. `.ai/index.md` "Adding a config key" names the category, section and group (inspect with `rg`).

Also: `#/settings/agents` lands on `#/settings/s/agents` (the fixed redirect under Page tree). Flow: `codex.e2e.ts` 117.

New flows and scenarios to write (these become implementation tickets):

- Extend the `settings` fixture scenario as in Testing Decisions (tier values, a global `agents.codex.light.effort: ""`, watchdog and queue keys, project Models overrides, a project `checks` entry).
- Keep `codex.e2e.ts` flow 117 green. It visits `#/settings/agents`, expects `#/settings/s/agents`, then
  matches `getByRole("heading", { name: "Agents", exact: true })` at 600px and clicks the first
  "Override for dotfiles" in `/settings/p/dotfiles/s/agents`. The new "Agents" category breadcrumb must
  not be a heading (render it as plain text or a link), so the locator keeps matching one element and
  the flow and its CATALOG.md text stay as they are.
- Extend flows 200 and 201 in `settings-page.e2e.ts`: the new tree steps, the Models cell edit and reset, the retired-link redirects. No new flow unless `gx-playwright-e2es` and the flow budget allow it.
- Retarget the settings step of the dev-mode flow (`dev-mode.e2e.ts`) from `#/settings/s/server` to
  `#/settings/s/display-and-development`, where `server.web.dev-frontend` now lives. Its CATALOG.md
  text changes from "the Settings row for `server.web.dev-frontend`" to "the Settings row for
  `server.web.dev-frontend` (Web UI, Display and development)".
- Update `webui/e2e/CATALOG.md` for the changed flow text.

**Mock states** (11 approved). The fixture is the `settings` scenario for every state (extended as above).

- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/02-sidebar-layout-mock.html`, `desktop-sidebar`, width 900, scenario `settings`, parts: sidebar, scope-picker, search.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/02-sidebar-layout-mock.html`, `desktop-sidebar-account`, width 900, scenario `settings`, parts: sidebar, scope-picker, search, sign-out.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/02-sidebar-layout-mock.html`, `phone-home`, width 390, scenario `settings`, parts: phone-home, sign-out.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/02-sidebar-layout-mock.html`, `phone-section`, width 390, scenario `settings`, parts: phone-section, group-cpu, group-process.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/02-sidebar-layout-mock.html`, `project-scope`, width 900, scenario `settings`, parts: sidebar, scope-picker, search, override-note.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/02-sidebar-layout-mock.html`, `section-with-groups`, width 900, scenario `settings`, parts: sidebar, scope-picker, search, group-agent-limits, group-waiting-and-stalled-agents.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/02-sidebar-layout-mock.html`, `search-results`, width 900, scenario `settings`, parts: sidebar, scope-picker, search.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/03-models-section-mock.html`, `desktop-global`, width 900, scenario `settings`, parts: sidebar, group-claude, group-codex, tier-light, tier-standard, tier-strongest, cell.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/03-models-section-mock.html`, `desktop-project`, width 900, scenario `settings`, parts: sidebar, override-note, group-claude, group-codex, tier-light, tier-standard, tier-strongest, cell.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/03-models-section-mock.html`, `phone-global`, width 390, scenario `settings`, parts: group-claude, tier-light, tier-standard, tier-strongest, cell.
- `/Users/david/.local/share/gx/tickets/gx/settings-cleanup/mocks/03-models-section-mock.html`, `phone-project`, width 390, scenario `settings`, parts: group-claude, tier-light, tier-standard, tier-strongest, cell.

## Out of Scope

- Renaming keys in config.json or project.json. The effort is page only, and gx has no config key
  migration.
- New settings keys, and any change to a key's type, default, validation or restart flag.
- TUI config.
- A table for any group other than the Models agents (Watchdogs and other repeated key sets stay lists).
- Group help text.
- Remembering which categories are open between visits.
- A fixed list of effort values per agent (an effort cell stays free text).

## Further Notes

**Where the mocks are wrong** (follow this spec):

- The `project-scope` state wrongly shows Notifications in a project's sidebar (no project-overridable
  key lives there today) and wrongly hides System (Disk space), which shows through the
  project-overridable `disk-space.cleanup-command` (see Project view above).
- The `desktop-project` state wrongly shows Notifications, Web UI, System and Account in a project's
  sidebar, and lacks the Project category and the Checks entry that the project tree has. A project's
  sidebar shows only categories that hold an overridable key and never Account.
- The `project-scope` state shows Project and Checks as two separate entries at the ends of the
  sidebar. The category table in ticket 01 lists Checks under a Project category. This spec follows the
  later sidebar decision (ticket 02, with its mock): the Project category is first, and Checks is a
  one-section category, last. The registry therefore names Checks as its own category; every other row
  of the table is as written.
- The `search-results` state cuts its lists short and ends with "and 12 more in Models." That is mock
  shorthand; the page shows every hit.
- The Models mocks show only values, not the edit state of a cell; the cell edit follows the Models
  table section above.
- In `desktop-global`, "Codex default" shows without an amber edge on Codex Light model, Light effort
  and Standard model. Those keys have non-empty gx defaults (`gpt-6-luna`, `medium`, `gpt-6.1-sol`),
  so the page never shows that state. Faint "Claude default" / "Codex default" text without an amber
  edge appears only when the gx default is empty (Claude Light effort). Any other faint cell is a set
  `""` and has the amber edge and a reset.

Implementer's choices: the registry's field names and wire shape for category, section, group, order
and the retired-slug redirects; how the registry marks the Models groups as tables and places each key
in a tier row and a column; the component structure of the sidebar, phone home and table; the exact
spacing, colours and caret glyphs (follow the mocks).
