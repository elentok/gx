## Problem Statement

An agent-built UI mock does not look like the real app. The notification settings mock (`mock-05.html`) used invented colors and `system-ui`. The person then judges a look that the build will never have, and the real page surprises them later.

The variant switcher has the same problem. Each mock gets a different bar, and it is hard to use on a phone, where the person often flips between variants.

The problem is not only gx's. Other projects have their own design systems, and mocks also come as claude.ai artifacts, where the agent cannot run the app.

## Solution

Mock guidance moves into `gx-prototype`, with no new skill.

- A generic `MOCK.md` tells the agent how to build a mock that looks like the app. It works in any project.
- A generic switcher snippet file sits next to `MOCK.md`. Every mock in every project uses it.
- A `gx/` folder holds the gx-only parts: the generated token file with the Chakra base layer, the generated `fonts.css`, the save script for the in-app route and pointers to the real components, including `PrototypeSwitcher`.
- Every mock gets one standard variant switcher, the pill, which works on a phone and on a desktop.
- gx-design enforces the guidance with a check in the store step and a design-review bullet. A tested Go helper checks stored mocks for invented values.
- gx keeps one permanent dev-only `PrototypeSwitcher` in webui, so a prototype route never writes its own switcher.

## User Stories

1. As a person judging a mock, I want it to look like the real build, so that I approve a look I will actually get.
2. As a person judging a mock, I want the same fonts, colors, spacing and icons as the real page, so that a difference I see is a design choice and not an accident.
3. As an agent building a mock in gx, I want one guide that says what to do first, so that I do not invent colors or fonts.
4. As an agent building a mock in another project, I want generic steps that tell me to find that project's own theme file, fonts, components and icons, so that I do not read gx-only instructions.
5. As an agent in a project that can run its app, I want to build the mock as a route in the real app and save it to one HTML file, so that the mock uses the real components.
6. As an agent that cannot run the app, I want a standalone-file fallback, so that I can still make a mock.
7. As an agent building a claude.ai artifact, I want the project's tokens to win over the look the artifact design skill pushes, so that the artifact matches the app.
8. As an agent building a standalone gx mock, I want a generated token file with every theme token and the Chakra base layer, so that I paste real values and never guess them.
9. As a maintainer, I want the token file, the base layer and `fonts.css` generated from the real theme and checked by one drift test, so that none of them can drift.
10. As an agent building a standalone gx mock, I want a committed `fonts.css` that inlines the Saira faces, so that a saved file renders in Saira and Saira Condensed.
11. As an agent building a standalone mock, I want a computed-style comparison with the real page before approval, so that line height, disabled opacity and row heights match.
12. As a person reading a mock built from source only, I want the file to say so at the top, so that I know it was never compared with a running page.
13. As a person flipping between variants on a phone, I want a switcher with 48px touch targets that fits 390px, so that I can use it with a thumb.
14. As a person flipping between variants, I want the switcher to look the same in every mock and in the app, so that I never wonder if it is part of the design.
15. As a person judging a design, I want the switcher in fixed neutral colors with a MOCK tag, so that it is clearly not part of the design.
16. As a person sharing a standalone mock, I want the chosen variant kept in the URL hash, so that a reload and a shared link show the same variant.
17. As a person on a desktop, I want the arrow keys to cycle variants, and I do not want them to fire while I type in a field.
18. As an agent building an in-app route in gx, I want the switcher to exist as one webui component using `?variant=`, so that I do not write a new one for each prototype.
19. As a maintainer, I want the production build to hide the switcher, so that a stray prototype merge cannot show it to users.
20. As a maintainer, I want a sync test between the webui component and the snippet, so that the two looks never diverge.
21. As an agent finishing a prototype in gx, I want step 6 to keep `PrototypeSwitcher` and drop only the variants, so that the permanent component stays. In other projects step 6 still drops the switcher unless the project keeps a permanent one.
22. As a person approving a mock, I want the stored file to hold only approved states, with no scripts and no switcher, so that later checks compare the approved look only.
23. As a gx-design agent, I want a check in the store step, so that a mock with invented values is not stored.
24. As a design reviewer, I want a rubric bullet on mock fidelity, so that a spec whose mock ignores the app's tokens is a finding.
25. As a maintainer, I want a Go helper that checks stored mocks, so that raw colors, `system-ui`, scripts, switcher markup and missing state markup are caught without reading the file.
26. As a maintainer, I want the helper to ignore the inlined stylesheet (tokens, base layer and fonts in one `data-mock-inlined` style) and nothing else, so that real tokens and real rules do not raise false alarms while a hand-written palette does.
27. As a maintainer, I want the helper to be tested, so that I can trust its findings.
28. As an agent, I want the helper's error to name the file and the rule, so that I can fix the mock without guessing.
29. As an agent in gx-design, I want every UI mock to follow this guidance, so that no mock skips it.
30. As a future agent, I want the dropped `gx-mock` skill gone, so that two guides do not give conflicting instructions.

## Implementation Decisions

**Home of the guidance**

- No new skill. The `gx-mock` first attempt and its color-only test are dropped. The branch `gx-mock-skill-prototype` is input only and does not merge.
- `gx-prototype/MOCK.md` holds the generic steps. `SKILL.md` and `UI.md` link to it.
- `gx-prototype/switcher-snippet.html` sits next to `MOCK.md`. It is the generic standalone switcher (markup, CSS and script). Every project uses it. It has no gx token or gx path.
- `gx-prototype/gx/` holds the gx-only parts: the generated token file, `fonts.css`, the save script and a pointer to the real components, `PrototypeSwitcher` and the gallery. `MOCK.md` links to it under "In gx". Other projects never read it.
- The bundle embeds the whole `gx-prototype` folder, but `skills/bundle_test.go` keeps an exact `requiredFiles` list. The epic adds `MOCK.md`, `switcher-snippet.html` and each `gx/` file to `requiredFiles`, and `TestEmbeddedBundleContainsRequiredFiles` is the proof.

**Generic steps in `MOCK.md`**

- In every project, try the in-app route first (UI.md sub-shape A or B) and save it to one HTML file.
- Use a standalone file only when the app cannot run or the mock is an artifact.
- A standalone mock passes a computed-style comparison with the real page (line height, disabled opacity, row heights) before approval. If the page cannot be run, the mock says at the top that it was built from source only.
- The agent finds the project's own theme file, fonts, components and icons. gx facts: Chakra UI v3, tokens in the webui theme file, Saira and Saira Condensed, Lucide icons through `react-icons/lu`, one Phosphor import (`Composer.tsx`) and the app's own SVGs (`webui/src/page/PageIcon.tsx`, which mock 02 shows, and inline `<svg>` in other components).
- A mock never uses invented colors, `system-ui` or a guessed font size.
- The project's tokens override the look the claude.ai artifact design skill pushes.
- The standard pill switcher is used in every mock, taken from `switcher-snippet.html`. UI.md step 4 and step 5a point to `MOCK.md` for it.
- Icons are never drawn new. A mock copies the app's own SVG or uses the app's icon library.
- A standalone mock pastes the tokens, the base layer and the fonts into one `<style data-mock-inlined>`, so the helper exempts them. `MOCK.md` and the files in `gx/` say so.
- In-app routes in every project use the `?variant=` param for the switcher. The `#v=` hash is only for a standalone file. UI.md step 3 and `MOCK.md` say so.
- A save script outside gx must write the same inlined-stylesheet marker, `data-mock-inlined`, on the `<style>` element, as the gx save script does. `MOCK.md` says so.

**Token file and base layer (gx only)**

- Generated, not hand-copied. A script builds the Chakra system from the theme config and writes CSS variables for every token: colors, font sizes, spacing, radii, font families and weights.
- The same generator writes the base layer from the same Chakra system: the reset and `themeConfig.globalCss` (body line height, body font and background). The 24px line-height gap seen in the two-ways build came from this layer, not from tokens.
- The generator also writes a committed `gx/fonts.css` with the five faces (Saira 400 and 500, Saira Condensed 500, 600 and 700) as base64 `@font-face` rules with MIME `font/woff2`. It adds about 109 KB to the bundle.
- One Vitest drift test runs the generator and fails when any committed output (tokens, base layer, `fonts.css`) differs from a fresh run. There is no token list to maintain. The computed-style comparison stays as the backstop.
- The script runs with `pnpm exec`, never `npx`.
- The files live in the skill folder, so artifacts and the standalone fallback can paste them unchanged. There is no shell font snippet.

**Standard switcher (the pill)**

- Markup: a fixed bottom-centre pill with a previous button, a label with the variant key and name (for example `B of 3` over the name, truncated), a next button and a `MOCK` tag.
- Look: light neutral chrome (`#f4f4f5`), a 2px dark border and a shadow, in fixed colors. It never uses the app's tokens.
- Size: 48px touch targets at every width, a 16px bottom offset plus the iOS safe area, and `max-width: calc(100vw - 16px)` so it fits 390px.
- Standalone file: show and hide in place, with the choice in the URL hash (`#v=B`). Arrow keys cycle and are ignored in inputs, textareas, selects and contenteditable.
- In-app route in gx: the same markup and CSS as a React component, using the router's `?variant=` param, hidden in production builds.
- Stored mocks carry no switcher and no script. Their states are `data-mock-state` sections.
- The prototype `mocks/03-variant-switcher.html` holds working markup, CSS and script and is the source for `switcher-snippet.html`. Its segmented-bar and corner-chip looks were not chosen and are not shipped.

**Permanent PrototypeSwitcher in webui**

- gx keeps one dev-only `PrototypeSwitcher` component in webui. A gx prototype route mounts it and does not write a switcher. The permanent dev route that mounts it is the gallery route, and the implementation ticket names the exact path.
- UI.md step 3 says that in gx the route mounts the existing `PrototypeSwitcher`. Other projects create a switcher as before, from `switcher-snippet.html`.
- UI.md step 6 says that in gx the switcher stays and only the variants are dropped, in both sub-shapes. In other projects step 6 still drops the switcher unless the project keeps a permanent one.
- A sync test compares the component's class names and CSS with `skills/gx-prototype/switcher-snippet.html`, and fails when they differ.

**Enforcement in gx-design**

- UI.md step 5a gets a check: before storing, the agent confirms the mock follows `MOCK.md`. When `[ -d testutil/gxfixture ]` it also runs `go run ./testutil/mocklint/cmd <file>...`, as MOCK-CHECK.md does for `testutil/mockcheck`. In other projects the rubric check is the only enforcement.
- 5a writes the comparison pairs (or the source-only note) and the helper output into the ticket's Resolution.
- The design-review rubric in the gx-design skill gets one bullet. It asks whether each stored mock follows `MOCK.md`: real tokens and fonts, no invented values, and a Resolution that holds the comparison pairs (or the source-only note) and a clean helper result. A mock that does not is a finding. The reviewer checks the Resolution and the helper result, not the design session.
- No new skill and no new gx-design step.

**Stored-mock helper (gx only)**

- A Go package `testutil/mocklint` with a `cmd`, run as `go run ./testutil/mocklint/cmd <file>...`. It checks each given stored mock file and reports findings per file. It is not a test inside `skills/`. Its prior art is `testutil/mockcheck`. It checks:
  - raw color values outside the exempt blocks: hex, `rgb()`, `rgba()`, `hsl()`, `hsla()` and CSS named colors (`white`, `red`);
  - `system-ui`, with the same exemption as raw colors;
  - any `<script>` element;
  - switcher markup, found by the pill's root class;
  - missing `data-mock-state` or `data-mock-width` markup, and a state section with no width.
- The one exempt block is the inlined stylesheet: the `<style>` element carrying `data-mock-inlined`. It holds the app's real tokens (including Chakra's `:where(:root, :host)` and `.dark` token blocks), reset and fonts (Chakra's reset holds `system-ui` in its font fallback, and its colors are real). A `:root` block outside it is not exempt. The helper recognizes it by the `data-mock-inlined` attribute on its `<style>` element. The save script writes that marker, and mock 02 carries it already. `MOCK.md` and step 5a document it.
- The helper is only for gx. Other projects have no such check.
- Nothing runs the helper on already stored mocks as a gate. The stored mocks `02-one-mock-two-ways.html` and the notification-alignment mock are not restyled (see Out of Scope).

**Save script (gx only)**

- `gx/` ships the working save script from `research/better-mocks-two-ways`. It writes the `data-mock-inlined` marker, keeps `data-mock-*` markup and drops scripts.

**Left to the implementer**

- How the generator is written and where its output files and the drift test sit inside the skill folder and webui.
- How the helper finds the exempt blocks inside the file, and the exact finding messages.
- The wording of the 5a check and the rubric bullet.
- The CSS of the webui component, as long as the sync test passes against the snippet.

## Testing Decisions

A good test checks behavior from outside: the file a command writes, the findings a helper returns, the text a skill carries. It does not check how the generator or the helper is written.

Seams:

- **Generated files.** One Vitest drift test runs the generator and compares its output (tokens, base layer, `fonts.css`) with the committed files. Prior art: the existing webui theme tests and the generated-file checks in the repo.
- **Stored-mock helper.** Go tests in `testutil/mocklint` use small HTML fixtures, one per rule: a hex color, an `rgba()` color, an `hsla()` color and a named color, each outside the inlined stylesheet; a raw color in a `:root` block outside it; a raw color in the inlined stylesheet; a class read from `switcher-snippet.html` as the pill's root class, used by the switcher-markup rule; `system-ui` outside the exempt blocks; `system-ui` inside the inlined stylesheet (must pass); a script; switcher markup; a missing state marker; a missing width marker; and a clean file. Prior art: `testutil/mockcheck`.
- **Skill text.** An anchors test, like the existing `mock_rules_test.go` and `gx_design_rules_test.go`, checks that UI.md steps 3, 5a and 6, `MOCK.md`, `SKILL.md` and the gx-design rubric carry the new rules, including the `data-mock-inlined` marker.
- **Bundle.** `TestEmbeddedBundleContainsRequiredFiles` with the new `requiredFiles` entries.
- **Switcher sync.** A Vitest test compares the `PrototypeSwitcher` class names and CSS with `switcher-snippet.html`.
- **PrototypeSwitcher behavior.** A Vitest test renders `PrototypeSwitcher` under a memory router with `?variant=` and checks that it cycles the param and shows the variant (story 18 and the in-app half of story 17). No catalog flow covers it, because e2e flows open the fixture's `web_url`, which serves the production build, and story 19 hides the switcher there.
- **Snippets run.** The font file and the switcher snippet are checked in the Verification section with a real browser, not in unit tests.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built. The checks run in playwright-cli with the `gx-ui-check` skill at 390px and at desktop width. The fixture serves saved files with `--mock-dir` and reports their `mock_url`, so no check uses the dev server or `gx files url`. The epic changes skills, a webui component and a Go helper. The only web page with a look to check is the mock below.

Fonts and snippets (stories 8 to 12)

8. Generated files. Run the generator, then `git diff --exit-code` on the token file, the base layer and `fonts.css`. Pass: exit 0. Change one color in the theme, rerun the Vitest test. Pass: it fails. Revert the change.
9. Drift. Same run as story 8.
10. Fonts. Build the mock in 6 with `gx/fonts.css`, serve it with `--mock-dir`, open its `mock_url` and read each face's `FontFace.status`. Pass: all five are `loaded`. Do not use `document.fonts.check()` alone, because it returns true for unknown families.
11. Computed-style comparison. Open the standalone mock built in 6 (served with `mock_url`) and the real Desktop settings section (fixture scenario `settings`) at 390px with playwright-cli. Compare line height, disabled opacity and row height. Pass: each pair is equal.
12. Source-only note. Read the `MOCK.md` text through the anchors test. Pass: it tells an agent to put a source-only note at the top of a mock built without a running page.

Look of a mock (stories 1, 2, 5, 6, 7)

1. Mock states: desktop-settings. Also compare the mocks built in 5 and 6 with the real Desktop settings section through MOCK-CHECK.md. Pass: both match.
2. Mock states: desktop-settings. Same comparison as story 1.
5. Build a mock for the Desktop settings section at 390px (state `desktop-settings`, the parts of the Mock states line) as an in-app route, save it with the gx save script, and run `go run ./testutil/mocklint/cmd <file>`. Pass: exit 0 with no findings. The saved file contains the inlined stylesheet, so this also covers `system-ui` inside it.
6. Build the same page as a standalone file from the token file, base layer and `fonts.css`, pasted into one `<style data-mock-inlined>`. Pass: mocklint exits 0 on it, and 10 (all five faces `loaded`) and 11 pass for this mock.
7. Read `MOCK.md`. Pass: it says the project's tokens win over the artifact design skill.

Guidance and enforcement (stories 3, 4, 21 to 24, 29, 30)

3. `gx skills` or the bundle listing shows `gx-prototype` with `MOCK.md`, `switcher-snippet.html` and the `gx/` folder, and `SKILL.md` and `UI.md` link to `MOCK.md`. Pass: the anchors test and `TestEmbeddedBundleContainsRequiredFiles` pass.
4. Read `MOCK.md` outside its "In gx" section, and `switcher-snippet.html`. Pass: neither names a gx path, token file, gx token or gx component. The anchors test checks both.
21. Read UI.md steps 3 and 6. Pass: in gx, the route mounts `PrototypeSwitcher` and step 6 keeps it, dropping only the variants, in both sub-shapes. In other projects step 6 drops the switcher unless the project keeps a permanent one.
22. Open a stored mock. Pass: no `<script>` and no switcher markup.
23. Read UI.md step 5a. Pass: it carries the check, the `[ -d testutil/gxfixture ]` guard, the helper call and the Resolution note.
24. Read the gx-design rubric. Pass: it has the mock-fidelity bullet, and the bullet checks the Resolution and the helper result.
29. Same read as story 24, plus UI.md step 5a.
30. List the bundled skills. Pass: there is no `gx-mock`.

Switcher (stories 13 to 20)

For 13, 15, 16 and 17, build a page from the shipped `switcher-snippet.html` with three variants, and serve it with the fixture's `--mock-dir`. Open its `mock_url`.

13. Open the page at 390px. Pass: the pill fits the viewport, and each button is at least 48px by 48px.
14. Compare the pill on the snippet page with the pill from `PrototypeSwitcher` rendered by the Vitest test. Pass: the sync test passes, which proves the same markup and CSS.
15. Read the computed colors of the pill on the snippet page. Pass: they are the fixed neutral values and no app token.
16. Click next on the snippet page, then reload. Pass: the same variant shows and the URL has `#v=<key>`.
17. Press the right arrow outside a field, then inside a text field, on the snippet page. Pass: it cycles outside and does nothing inside. The in-app half is the Vitest test under 18.
18. Run the Vitest test that renders `PrototypeSwitcher` under a memory router with `?variant=`. Pass: it cycles the param and shows the variant.
19. Build webui for production and look for the switcher. Pass: it is absent.
20. Run the sync test. Pass: it passes. Change a class name in the component. Pass: it fails.

Helper (stories 25 to 28)

25. Run the helper on fixtures with a raw color outside the inlined stylesheet (hex, `rgba()`, `hsla()` and a named color, one each), one with `system-ui`, one with a script, one with switcher markup, one with no state markup, one with a state but no width marker and one with no width marker at all. Pass: each gets a finding and a non-zero exit.
26. Run the helper on `02-one-mock-two-ways.html`. Pass: no color or `system-ui` findings (its tokens, `.dark` block and reset all sit in its `data-mock-inlined` style).
27. Run `go test` on `testutil/mocklint`, capped with `scripts/proc-cap`. Pass: green.
28. Read a finding. Pass: it names the file and the rule.

New flows or scenarios to write (these become implementation tickets)

- Helper fixtures for story 25, as Go test data. These are not web flows.
- The `PrototypeSwitcher` Vitest test and the sync test. These are not web flows.
- No new fixture scenario is needed. The `settings` scenario exists in `testutil/gxfixture/catalog/scenarios.go`, and `settings-page.e2e.ts` already opens Notifications, Desktop with it.

**Mock states**

- `/Users/david/.local/share/gx/tickets/gx/better-mocks/mocks/02-one-mock-two-ways.html`, state `desktop-settings`, width 390, fixture scenario `settings`, parts: settings-page, os-banners-row, sounds-row, done-sound-row, attention-sound-row.

`mocks/03-variant-switcher.html` is not a state mock. See Further Notes.

## Out of Scope

- TUI mocks. They are plain text with no colors, fonts or switcher, a different problem.
- Restyling mocks already stored under `<root>/<map>/mocks/`. That would change approved looks without a map ticket.
- A new skill for mocks, and a new gx-design step.
- Non-gx tools for finding a project's theme. The generic steps tell the agent what to look for, and the agent does the search.
- The segmented-bar and corner-chip switcher looks.
- Any change to how a claude.ai artifact is hosted.

## Further Notes

- Map ticket 07 settled it: a real claude.ai artifact renders all five inlined Saira faces (the person saw `RENDERS SAIRA`). The block adds about 109 KB, about 0.7% of the 16 MB limit. No fallback is needed. Evidence: `docs/research/better-mocks-artifact-fonts.md`.
- `mocks/03-variant-switcher.html` is a reference look for the pill, not a state mock. It carries scripts and a stand-in page, so it has no Mock states line and the stored-mock helper does not run on it.
- Research and prototype branches hold the evidence: `research/better-mocks-font-snippet` (font run, `docs/research/better-mocks-font-snippet.md`) and `research/better-mocks-two-ways` (the standalone and in-app builds, the save script and the throwaway entry, under `docs/research/better-mocks-two-ways/`).
- Dev-server gotchas from the two-ways build: `file:` URLs are blocked in playwright-cli, so serve a saved file with the fixture's `--mock-dir` and open its `mock_url`. A gallery entry needs the `label` field, or the label comes from the key. A throwaway entry needs a connection stub and a router.
- In the two-ways build, the in-app route matched the real page row for row (145px). The standalone file missed the disabled-button opacity (0.5) and the 24px body line height, which is why the generator now writes the base layer and why the computed-style comparison stays as a backstop.
- The page header in the saved route differed from the real page (no mascot toggle, category above the title) because the shell context was missing. A mock that judges the header mounts the shell.
