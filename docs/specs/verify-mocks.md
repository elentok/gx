# Verify mocks

## Problem Statement

A person who designs a web UI epic with gx approves a mock during the design map. After the epic is
built, nothing checks the built page against that mock. The implement agents and the verify step
work from the spec text and the flow catalog. They never open the mock.

gx:epic-page-redesign2 shows the cost. A late side-by-side comparison of the built page with the
mocks found 48 differences in layout, spacing, sizes, colours and type. Each one had passed
implementation, code review and verify.

Three gaps cause this:

- No agent can find the approved mock. It lives in a claude.ai artifact or on a throwaway variant
  branch, mixed with rejected variants, and no ticket or spec names it.
- No implement ticket says which parts of the mock it builds, at which widths and states, or with
  which fixture data.
- gx-verify-epic uses text snapshots, and a text snapshot cannot show a look difference.

## Solution

When a UI prototype map ticket is resolved, the prototype session stores the approved mock once, as
one standalone HTML file in the ticket store at `<root>/<map>/mocks/<NN>-<slug>.html`. The file
holds only the approved look. Each approved state is a section marked with its state name and
width, and each part of the page inside it is marked with a part name. The repo and the `-impl`
branch never hold the file.

gx-to-spec ends the spec's `## Verification` section with a "Mock states" list: one line per
approved state, naming the mock file, the state, the width, the fixture scenario and the parts.
gx-to-tickets gives each UI implement ticket that builds part of a mock a `## Mock check` section.
It names the mock file, the states and widths, the parts the ticket builds and the fixture scenario.
It also budgets the screenshots. gx-to-tickets copies the Mock states lines into verify parts like
story entries.

An implement agent with a `## Mock check` section starts the fixture scenario, screenshots the mock
and the built page in playwright-cli at each listed state and width, and compares them side by side
by eye. It checks only the parts it built. It lists each difference, then fixes it or records it
under `## Spec deviations`. The verify step does the same for the whole page, one Mock states line
at a time. A look difference there is a finding and gets a fix ticket like any other finding.

The rule is the one epic-page-redesign2 set: the mock wins on look (layout, spacing, sizes, colours,
type), and later decisions win on behaviour. Pixel-perfect is not a goal.

## User Stories

### Storing the approved mock (gx-prototype, gx-wayfinder)

1. As a person who approves a UI prototype in a design map, I want the session that resolves the
   prototype ticket to store the approved mock in the ticket store, so that later agents check the
   built page against the look I approved.
2. As a person, I want the stored mock to hold only the approved rows and states, with no rejected
   variants, so that no agent copies a look I turned down.
3. As a person, I want the claude.ai artifact to stay as it is, so that the copy people review is
   not changed by the store step.
4. As a prototype agent working from a claude.ai artifact mock, I want to copy the approved rows and
   states from the artifact's HTML into a new standalone HTML file, unchanged in look, so that the
   stored file matches what the person approved.
5. As a prototype agent working from an in-repo `?variant=` route, I want to open the approved
   variant in playwright-cli at each approved state and width and save it as one self-contained HTML
   file, so that the stored mock does not need the app or the throwaway branch to render.
6. As a prototype agent saving a variant route, I want the save to inline every CSS rule from
   `document.styleSheets` and drop all scripts, so that runtime styles (the web UI's Chakra and
   emotion rules) are kept and the file is static.
7. As a prototype agent, I want to wrap each approved state in a section with
   `data-mock-state="<name>"` and `data-mock-width="<px>"`, drawn inside a box of that width, so
   that an agent can screenshot one state by selector.
8. As a prototype agent, I want to mark each part of the page with `data-mock-part="<kebab-name>"`,
   unique within its state section, and to give the same part the same name in every state, so
   that specs and tickets can name the parts each check covers.
9. As a later agent, I want each state name to be unique within the mock file and to be the name
   specs and tickets use, so that a Mock states line and a `## Mock check` section point at exactly
   one section.
10. As a prototype agent, I want to name the file `<NN>-<slug>.html` after the prototype ticket and
    put it in `<root>/<map>/mocks/`, so that every map's mocks live in one known place.
11. As a prototype agent, I want to write the file just before I record the ticket's Resolution, and
    link both the stored file and the artifact from the Resolution, so that the decision and its mock
    stay together.
12. As a person, I want the stored file to count as the approved mock even if someone edits the
    artifact later, until a map ticket approves the change, so that the check has one fixed target.
13. As a prototype agent resolving a ticket that replaces an earlier approved look, I want to say so
    in the Resolution and delete the old file, so that `mocks/` holds only current approved looks.
14. As a person, I want a variant route made for a design map to stay on the throwaway branch and
    never fold into main, so that the stored mock and that branch are the only outputs of the
    prototype.
15. As a wayfinder agent working a Prototype ticket, I want one line in the Prototype type that
    points at gx-prototype's "Store the approved mock" step, so that I do not skip it.
16. As a person, I want the store step to apply to UI prototypes only, so that logic prototypes
    (LOGIC.md) and TUI mocks are unchanged.

### Spec (gx-design, gx-to-spec)

17. As gx-design's spec step, I want to tell the gx-to-spec subagent which stored mocks the map has
    (the files in `<root>/<map>/mocks/`), so that the closed-ticket gists do not lose the mock paths.
18. As gx-to-spec, I want to end `## Verification` with a "Mock states" list, one line per approved
    state in each stored mock, so that the verify step knows every state to compare.
19. As gx-to-spec, I want each Mock states line to name the full mock path
    (`$(gx tickets root)/<map>/mocks/<NN>-<slug>.html`), the state name, the width, the fixture
    scenario and the part names, so that a verify part can run it with no other reading.
20. As gx-to-spec, I want story Verification entries to refer to Mock states lines by state name, so
    that a story's look check is not written twice.
21. As gx-to-spec, I want a Mock states line whose fixture scenario does not exist to list that
    scenario under the new flows or scenarios to write, so that gx-to-tickets turns it into an
    implement ticket.
22. As gx-to-spec, I want to write no Mock states list when the epic has no stored mock, so that
    older epics and non-UI epics are unchanged.
23. As gx-design's design reviewer, I want a stored mock state with no Mock states line to count as
    a finding, as a story with no Verification entry already does, so that a missing look check is
    caught before tickets are cut.

### Tickets (gx-to-tickets)

24. As gx-to-tickets, I want to add a `## Mock check` section to each implement ticket that builds
    part of a stored mock, so that the implementing agent knows what to compare.
25. As gx-to-tickets, I want the `## Mock check` section to list the mock file, the states with
    their widths, the parts this ticket builds and the fixture scenario, so that the agent checks
    only its own parts.
26. As gx-to-tickets, I want to add no `## Mock check` section to a UI ticket that builds no mock
    part, so that tickets without a look target cost nothing extra.
27. As gx-to-tickets, I want to name the gxfixture scenario for each mock state, or add an implement
    ticket that creates the scenario and blocks every checking ticket, so that each check has
    matching data.
28. As gx-to-tickets, I want to add 10K to an implement ticket's `expected_context_window` for each
    mock state (one state section: one name at one width) in its `## Mock check` section, so that
    the first check, one fix pass and its new screenshots fit the budget.
29. As gx-to-tickets, I want an implement ticket to list at most 4 mock states, counted as state
    sections, so that its screenshots cannot fill the window.
30. As gx-to-tickets, I want a ticket over the 4-state cap split in two, the first building the
    parts and checking the first 4 states and a follow-on implement ticket, blocked by it, checking
    the rest and fixing what differs, so that every state still gets checked.
31. As gx-to-tickets, I want a "Mock state check" row of 8K in the verify token table, so that verify
    parts budget each Mock states line.
32. As gx-to-tickets, I want each Mock states line packed into verify parts like a story, using the
    existing 70K packing (same scenario stays together, the 70K budget wins, a single item over 70K
    gets its own part), so that the verify split needs no new rule.
33. As gx-to-tickets, I want to copy each Mock states line word for word into the verify part body,
    so that a part reads only its own ticket.
34. As gx-evolve, I want the 8K row, the 10K per state and the 4-state cap in gx-to-tickets'
    existing token table, so that I can compare them with actual token counts.

### Implementing (gx-implement)

35. As an implement agent, I want to find the mock check by the `## Mock check` heading, so that I
    know whether the ticket has one.
36. As an implement agent in an `-impl` worktree, I want to find the mock at
    `$(gx tickets root)/<map>/mocks/`, where `<map>` is my epic name without `-impl`, so that I need
    no frontmatter link.
37. As an implement agent, I want to start the ticket's fixture scenario with gx-ui-check and open
    the built page at each listed width, so that I compare against real data.
38. As an implement agent, I want to screenshot each listed part of the mock by the selector
    `[data-mock-state=<s>] [data-mock-part=<p>]`, and the same part of the built page, so that I
    compare like with like.
39. As an implement agent, I want to choose how to find each part on the built page myself, so that
    the ticket names only the mock part.
40. As an implement agent, I want to compare the two screenshots side by side by eye and list each
    difference, so that no difference goes unnoticed.
41. As an implement agent, I want to fix each look difference, so that the page matches the approved
    look in layout, spacing, sizes, colours and type.
42. As an implement agent, I want to record a difference I keep on purpose under `## Spec
    deviations`, with the reason, so that verify does not report it again.
43. As an implement agent, I want a later decision to win over the mock on behaviour, and the mock
    to win on look, so that I know which source to follow when they disagree.
44. As an implement agent, I want to check only the parts my ticket built, so that I do not spend
    budget on parts other tickets own.
45. As an implement agent, I want to compare a phone state, which is a fixed-width box in the mock
    where media queries do not fire, with the built page at that viewport width, by eye, so that the
    phone layout is checked as the person approved it.
46. As an implement agent, I want to take screenshots at device scale factor 1, cropped to the state
    or part selector, never a full page at retina scale, so that each screenshot costs few tokens.
47. As an implement agent whose ticket names a mock file I cannot find, I want to follow gx-implement's
    existing rule for a check that cannot run (`gx tickets ask`, naming the missing file), so that I
    never report a check I did not do.

### Verifying (gx-verify-epic)

48. As a verify part, I want each Mock states line in my ticket to be checked like a story, so that
    the whole page is compared against the mock.
49. As a verify part, I want to screenshot the whole state section of the mock and the same state of
    the built page for each Mock states line, so that I check the whole page and not only one part.
50. As a verify part, I want the "use text snapshots, not screenshots" rule to have one exception
    for Mock states lines, so that I can see look differences.
51. As a verify part, I want to record one result line per Mock states line, as pass, fail or
    unchecked, so that the collect ticket can merge it into the table.
52. As a verify part, I want a difference listed in the epic's `## Spec deviations` to be skipped,
    so that a difference kept on purpose is not a finding.
53. As a collect or lone ticket, I want a look difference to be a finding that gets a fix ticket
    like any other finding, so that the next round rechecks it.
54. As a lone verify ticket, I want to read the Mock states list as part of the spec's Verification
    section, so that a one-part epic also checks its mocks.
55. As a person, I want an epic with no stored mock to verify as today, so that older epics need no
    backfill.

## Implementation Decisions

### Stored mock file

- Location: `<root>/<map>/mocks/<NN>-<slug>.html` in the ticket store, where `<root>` is
  `gx tickets root`, `<map>` is the map epic and `<NN>-<slug>` is the prototype ticket. Never in the
  repo or the `-impl` branch.
- The ticket store already handles this directory (gx:verify-mocks/02). The ticket loader,
  `validate`, `migrate`, ticket move, the server index, the web UI and TUI epic views and the cleanup
  scan all read only `ticket.md` and `issues/`, or skip non-`.md` files. The store's git sync commits
  and pushes the file. Epic archive moves it with the map, to `<root>/.archive/<map>/mocks/`. No Go
  change is needed.
- Format: HTML only, no stored screenshots. Each approved state is a section with
  `data-mock-state="<name>"` and `data-mock-width="<px>"`, drawn inside a box of that width. A mock
  state is one such section: one name at one width. State names are unique within the file. Each
  part has `data-mock-part="<kebab-name>"`, unique within its state section. The same part keeps the
  same name in every state. The approved widths and states live in these attributes, so no side
  file is needed.
- A part selector is `[data-mock-state=<s>] [data-mock-part=<p>]`. A state selector is
  `[data-mock-state=<s>]`.
- One file per prototype ticket. A later ticket that replaces a look deletes the old file and says
  so in its Resolution.

### gx-prototype

- UI.md gets a new "Store the approved mock" step. The session that resolves a prototype map ticket
  runs it, just before it records the Resolution. The Resolution links the stored file and the
  artifact.
- The step covers both mock sources:
  - A claude.ai artifact: copy the approved rows and states from the artifact's HTML into a new
    standalone file, unchanged in look. The artifact stays as it is.
  - An in-repo `?variant=` route: open the approved variant in playwright-cli at each approved state
    and width, and save a self-contained HTML file. The save inlines every CSS rule from
    `document.styleSheets` and drops scripts. How the save script is written is the implementer's
    choice.
- The step adds the state sections and the part markers, and names parts in kebab-case. State names
  are unique within the file. Part names are unique within a state section, and the same part keeps
  its name across states.
- UI.md's "Capture the answer and clean up" step gets an exception. A variant route made for a
  design map never folds into main. It stays on the throwaway branch, and the stored mock and that
  branch are the outputs.
- LOGIC.md is unchanged.
- The step's exact wording, and whether gx-prototype's SKILL.md also points at it, are the
  implementer's choice.

### gx-wayfinder

- The Prototype ticket type gets one line that points at the "Store the approved mock" step in
  gx-prototype's UI.md. Nothing else in gx-wayfinder changes.

### gx-design

- Step 4 (dispatching gx-to-spec) also seeds the subagent with the list of stored mocks under
  `<root>/<map>/mocks/`, so that it can write the Mock states list.
- The design-review rubric gets one check: each state of each stored mock has a Mock states line.
  A missing line is a finding, like a story with no Verification entry.
- The wording of both is the implementer's choice.

### gx-to-spec

- The template's `## Verification` section ends with a "Mock states" list when the map has a stored
  mock. Each line names:
  - the full mock path, `$(gx tickets root)/<map>/mocks/<NN>-<slug>.html`,
  - the state name,
  - the width,
  - the fixture scenario,
  - the part names in that state.
- Story entries refer to Mock states lines by state name.
- A fixture scenario that does not exist yet is also listed under the new flows or scenarios to
  write.
- No stored mock, no Mock states list.
- The line's exact layout is the implementer's choice, as long as it carries those five fields.

### gx-to-tickets

- The per-ticket template gets a `## Mock check` section, written only on implement tickets that
  build part of a stored mock. It lists the mock file (full path), the states with their widths,
  the parts this ticket builds and the fixture scenario.
- For each mock state, the ticket names the gxfixture scenario. If none exists, gx-to-tickets adds
  an implement ticket that creates it and blocks every ticket that checks that state.
- The existing token table gets:
  - a verify row, "Mock state check (one mock state at one width: screenshot the mock and the built
    page, list the differences)": 8K;
  - an implement rule: 10K added to `expected_context_window` per mock state in `## Mock check`;
  - the cap: at most 4 mock states per implement ticket.
- Both the 10K and the cap count mock states, where a mock state is one state section of the file:
  one name at one width.
- Over the cap, gx-to-tickets splits the ticket. The first builds the parts and checks the first 4
  states. A follow-on implement ticket, blocked by the first, checks the remaining states and fixes
  what differs.
- Each Mock states line packs into verify parts like a story, using only the existing 70K packing.
  Verify parts have no state cap; the packing already limits a part to 6 states (15K base plus 8K
  each). There is no per-mock-file rule.
- gx-to-tickets copies each Mock states line word for word into the verify part body (and the lone
  verify body), as it does story Verification entries.

### gx-implement

- An implement ticket with a `## Mock check` section runs the mock check before it reports
  finished. The agent finds it by that heading.
- The agent finds the map's mock directory by name: `$(gx tickets root)/<map>/mocks/`, where
  `<map>` is the ticket's epic name minus `-impl`.
- The check:
  1. Start the ticket's fixture scenario with gx-ui-check.
  2. For each listed state and width, screenshot each listed part of the mock by selector, and the
     same part of the built page at that viewport width. Device scale factor 1, cropped to the
     selector.
  3. Compare side by side by eye. List each difference.
  4. Fix each difference, or record one kept on purpose under `## Spec deviations` with the reason.
- The mock wins on layout, spacing, sizes, colours and type. A later decision wins on behaviour. A
  phone state is a fixed-width box in the mock, so it is compared with the built page at that
  viewport width.
- The agent picks how to locate each part on the built page.
- A named mock file that is missing is a check that cannot run. The agent follows gx-implement's
  existing announce-and-stop rule (`gx tickets ask`).
- How playwright-cli opens the local mock file (a `file://` URL or a small local server) is the
  implementer's choice.
- The comparison steps are written once and linked from both gx-implement and gx-verify-epic. Where
  that one copy lives is the implementer's choice. A new shared file under `skills/` must be added
  to the bundle's required files.

### gx-verify-epic

- "The verify part body" gets the Mock states lines, copied word for word, with their 8K estimate.
- A part checks each Mock states line like a story: it starts the line's fixture scenario,
  screenshots the whole state section of the mock and the built page at the line's width, and
  compares them by eye.
- The rule in step 4, "Use text snapshots, not screenshots", gets one exception: Mock states lines
  use screenshots, at device scale factor 1, cropped to the state selector.
- A part writes one result line per Mock states line, with the same pass, fail and unchecked
  verdicts as a story line. The line's label (for example the state name and width) and its table
  row label are the implementer's choice.
- Differences listed in the epic's `## Spec deviations` are not findings.
- A look difference is a finding. Collect and lone tickets decide it like any other finding (step 6).
- A lone ticket reads the Mock states list as part of the spec's Verification section.

## Testing Decisions

- A good test here checks the skill text an agent reads. It does not check how an agent compares
  screenshots, which no automated test can see. The skill text is the contract.
- Seam: skill text tests in the `skills` Go package, in the style of `TestGxDesignChainingRules` and
  `TestGxWayfinderLetsCallerAllowMoreTickets` in `skills/gx_design_rules_test.go`. One test per
  skill checks that each rule is present. Rules that live in the shared comparison file count for
  each skill that links it. Story numbers are in brackets.
  - gx-prototype UI.md: the "Store the approved mock" step, run by the session that resolves the
    ticket (1); only approved rows and states, no rejected variants (2); the artifact stays as it is
    (3); the copy from the artifact's HTML (4); the playwright-cli save of a variant route (5); the
    `document.styleSheets` inlining and the "no scripts" rule (6); the `data-mock-state`,
    `data-mock-width` and `data-mock-part` markers (7); part names unique within a state section and
    reused across states (8); state names unique within the file (9); the mock path (10); the write
    before the Resolution, which links the stored file and the artifact (11); the stored file wins
    over a later artifact edit until a map ticket approves the change (12); a replacing ticket says
    so and deletes the old file (13); the design-map exception to folding into main (14); the step
    applies to UI prototypes only, and LOGIC.md has no store step (16).
  - gx-wayfinder: the Prototype type's pointer to the store step (15).
  - gx-design: the mock list seeded into the spec step (17), and the rubric check (23).
  - gx-to-spec: the "Mock states" list at the end of `## Verification` (18) and its five fields
    (19); story entries refer to Mock states lines by state name (20); a missing fixture scenario is
    listed under the new flows or scenarios to write (21); no stored mock, no Mock states list (22).
  - gx-to-tickets: the `## Mock check` section in the template and its fields (24, 25); no
    `## Mock check` on a UI ticket that builds no mock part (26); the scenario-or-ticket rule, with
    the new scenario ticket blocking every ticket that checks that state (27); the 10K per mock state
    (28); the 4-state cap (29) and its split (30); the 8K row (31); Mock states lines packed like
    stories with the existing 70K packing (32); Mock states lines copied word for word into the
    verify part body and the lone verify body (33); the 8K, 10K and cap in the existing token table
    (34).
  - gx-implement: the `## Mock check` heading (35); the `-impl` name rule for the mock directory
    (36); the fixture scenario started with gx-ui-check (37); the part selector (38); the agent picks
    how to find each part on the built page (39); the side-by-side comparison by eye with each
    difference listed (40); each difference fixed (41) or recorded under `## Spec deviations` (42);
    the mock wins on look and later decisions win on behaviour (43); only the ticket's own parts are
    checked (44); a phone state compared with the built page at that viewport width (45); the scale-1
    cropped screenshots (46); a missing mock file uses `gx tickets ask` (47).
  - gx-verify-epic: each Mock states line checked like a story (48), with screenshots of the whole
    state section (49); the screenshot exception (50); one result line per Mock states line, as
    pass, fail or unchecked (51); differences in the epic's `## Spec deviations` skipped (52); a
    look difference is a finding that gets a fix ticket (53); a lone ticket reads the Mock states
    list (54); the mock steps apply only when the part has Mock states lines, so an epic with no
    stored mock verifies as today (55).
  The exact phrases the tests match are the implementer's choice.
- `skills/bundle_test.go` already checks that relative links resolve and that every bundle file is
  required and embedded. A new shared file for the comparison steps must pass those tests.
- Run `go test ./skills/ -timeout 60s`.

## Verification

This epic changes no web, CLI or server behaviour. It changes only bundled skill text, so
gx-to-tickets publishes no verify tickets for it. The skill text tests above, run with
`go test ./skills/ -timeout 60s`, cover every story.

## Out of Scope

- TUI mocks.
- Logic prototypes (gx-prototype's LOGIC.md).
- Fixing the current epic-page-redesign gaps. gx:epic-page-redesign2 does that.
- Backfilling stored mocks for older epics. The check runs only where a stored mock exists.
- Pixel-perfect comparison, image diff tools and stored screenshots.
- Any Go change to the ticket store, server, web UI or TUI. The store already handles a `mocks/`
  directory.
- A frontmatter link from an `-impl` epic to its map. The name rule is enough.

## Further Notes

- Model case: gx:epic-page-redesign2. Its comparison of the built page with the mocks found 48
  differences, and its Notes set the rule this spec uses: mocks win on look, later decisions win on
  behaviour.
- Cleanup archives a map only after its `-impl` merges, so the mock directory normally stays at
  `<root>/<map>/mocks/` while implement and verify tickets run.
- The decisions behind this spec are the closed tickets gx:verify-mocks/01 to 05.
