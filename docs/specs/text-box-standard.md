# Text box standard

## Problem Statement

On an iPad or iPhone, some text boxes in the web UI show AutoFill Contact where the word completions should be. I cannot type with completions in those boxes.

Each text box sets its own label, AutoFill, typing help, focus ring and look. Nothing stops a new text box from bringing the keyboard bugs back, and the boxes look different from page to page.

Two causes are known from device checks:

- On iPhone, the word "name" in a box's label, placeholder, `name` or `id` brings AutoFill Contact. No tested attribute stops it. A `<form>` makes no difference.
- A textarea named only by `aria-label` lets Safari guess the field type from the text around it.

No iPad test page variant showed AutoFill Contact, so the iPad trigger is unknown.

## Solution

Every **Text box** in the web UI uses one shared text box component, `TextBox`. The caller picks a **Text box kind** (prose, line, search or credential), and the component sets the markup rules for that kind. The **Prompt box** is built on it.

Boxes that hold a name are labelled "Title". All boxes share one look in two variants and a fixed set of sizes, with 16px text.

A lint rule fails the build on a raw text box. In development and tests, `TextBox` throws on a label that would bring AutoFill Contact. In production it sends a **Broken rule report**, and gx files a follow-up.

A standing doc, `docs/text-boxes.md`, records each rule and the iOS reason for it.

## User Stories

1. As an iPhone user, I want every box that holds a name to be labelled "Title", so that I get word completions instead of AutoFill Contact.
2. As an iPhone user, I want the "Title" wording in the new chat form, the chat draft page, both rename sheets, the schedule page, new workspace, workspace rename rows and the web login title in settings, so that no name box is missed.
3. As a voice control user, I want the visible text of a box to be the same as its accessible name, so that I can say what I see.
4. As a person managing workspaces, I want each rename row to say which workspace it renames, so that the shared "Title" label does not confuse me.
5. As a screen reader user, I want every text box to have a real linked label, so that I hear what the box is for.
6. As an iPad or iPhone user, I want every textarea to have a real linked label, so that Safari does not guess its field type from nearby text.
7. As a person starting a new chat in a new worktree, I want a hint under the title box that says "Left blank, the chat is named <worktree>.", so that I know the default without a changing placeholder.
8. As a person starting a new chat in an existing worktree or the project root, I want the hint to say "Left blank, gx names the chat.", so that the hint matches what happens.
9. As a person answering a question, I want the text box of a chosen option to be labelled "Answer to question N" with the placeholder "Your answer", so that text written by an agent never becomes a box label.
10. As a person answering a question, I want the chosen option's text to stay visible, so that I know what my text belongs to.
11. As a person filtering blockers, I want the box to say "Search epics", so that its label does not change with the project.
12. As a phone user, I want every text box to use 16px text, so that iOS does not zoom the page when I tap it.
13. As a person using the web UI, I want one look for text boxes on every page, including login, setup and the rest screen, so that the UI looks consistent.
14. As a person using a title, brief or history search box, I want it to stay borderless where it is today, so that the pages keep their calm layout.
15. As a person editing check commands, diff refs, diff filters or workspace rows, I want those boxes to be readable at 16px, so that I do not squint at small fields.
16. As a person editing code-like values, I want the diff file filter, the diff refs and the setup dialog command to stay monospace, so that exact text is easy to read.
17. As a keyboard user, I want each boxed text box to show a focus ring that is not clipped, so that I can see where I am.
18. As a keyboard user, I want the History search to show a ring inside the box, titles to show a background, and the prompt box frame to keep its ring, so that bare boxes still show focus where they should.
19. As a person typing chat, notes and titles, I want browser autocorrect and spellcheck to work, so that I make fewer typos.
20. As a person typing slugs, commands, refs, paths or terminal input, I want typing help off, so that the keyboard does not change my exact text.
21. As a person searching, I want typing help off and a search key on the keyboard, so that autocorrect does not break my query.
22. As an iPhone user, I want free-text boxes to keep AutoFill off, so that Safari does not offer saved cards.
23. As a person signing in or setting up, I want password managers and authenticator code AutoFill to work, and the setup code box to be a literal single-line box with `autocomplete="off"` and autocorrect, autocapitalize and spellcheck off, so that I do not type secrets by hand and the keyboard does not change the setup code.
24. As a person entering a setting secret, I want the text hidden, with `autocomplete="off"` and autocorrect, autocapitalize and spellcheck off, so that the secret is not shown on screen and the keyboard does not change it.
25. As a person using a project or worktree picker, search box or command palette, I want its keys and list behaviour to work as before, so that the move to `TextBox` costs me nothing.
26. As a person writing in a prompt box, I want the expand button, vi editor, completion menu and send keys to work as before, so that the shared component changes nothing there.
27. As an agent adding a text box, I want one component that sets the rules for me, so that I cannot forget one.
28. As an agent, I want the type checker to refuse the attributes that `TextBox` owns, so that I cannot override a rule by accident.
29. As a maintainer, I want lint to fail on a raw text box outside `TextBox`, so that nobody bypasses the standard.
30. As a maintainer, I want a test to fail on any comment that silences that lint rule, so that the standard has no exceptions.
31. As a maintainer, I want a fixture test for the lint rule, so that a Biome upgrade cannot switch it off unnoticed.
32. As a developer, I want `TextBox` to throw in development and tests on "name" in a label, placeholder or `name` attribute, so that I see the broken rule before it ships.
33. As a maintainer adding a setting, I want a server-side test to fail on an editable setting labelled with "name", so that a registry entry cannot break the settings page.
34. As a person using gx in production, I want a broken rule to leave the page working, so that a wording mistake does not block me.
35. As a maintainer, I want a broken rule found in production to be filed as one follow-up per rule and broken value, so that I learn about it without reading logs.
36. As a maintainer, I want repeats from one source on one day to share one "Seen again" line with a count in the open draft, so that I see how often a problem happens without a flooded ticket.
37. As a maintainer, I want a repeat after the draft was closed or opened for work to file a new draft, so that a returning bug is not lost.
38. As the owner of the gx server, I want the report endpoint behind the web login, rate limited and size limited, so that nobody outside can write into my ticket store.
39. As an agent, I want a standing doc linked from the project's agent index, so that I read the rules and their reasons before adding a text box.

## Implementation Decisions

### Scope

- A text box is any native input or textarea the person types text into. Checkbox, radio, range, button, submit, hidden and file inputs are not text boxes. The vi editor is not one.
- The hidden copy textarea used for clipboard fallback and the terminal library's own textarea are not covered.
- Every existing text box moves onto `TextBox`. None stays raw.
- There is no form rule. A `<form>` may hold any text box. Login and setup pages keep their `<form>`. The chat draft page stays a plain `<div>`, and its code comment, which blames the `<form>`, is corrected.

### The component

- `TextBox` is one new component in its own folder under the web UI's shared UI code, with a `kind` prop. The implementer picks prop names that this spec does not give.
- Prose renders a textarea. Line, search and credential render an input.
- It always renders a real `<label htmlFor>`. A `hideLabel` prop hides it visually. It also draws the `optional` marker, a `hint` line and the `invalid` state (red border and `aria-invalid`).
- `TextBox` creates the box's `id` itself. A caller cannot pass one.
- The caller must pass `name`, a stable name for the box's purpose. There is no default.
- Every box sets `data-focus-ring="none"`, and the component draws its own focus style.
- A short comment next to each forced attribute says why it is there.
- It passes through the ref, event handlers, `value`, `defaultValue`, `placeholder`, `disabled`, `required`, `minLength`, `autoFocus`, `rows`, `role`, `list`, `data-*` and ARIA attributes, except `aria-label` and `aria-labelledby`.
- The prop types refuse `aria-label`, `aria-labelledby`, `id`, `type`, `autoCapitalize`, `autoCorrect`, `spellCheck`, `enterKeyHint`, `data-focus-ring`, and every style prop that is not layout. `autoComplete` is allowed only on credential. `inputMode` is allowed only on line and credential.
- Layout props are width, margin, flex and grid placement, minimum and maximum width, and for textareas the minimum and maximum height and the resize direction.
- A box with something drawn inside its end edge, such as the picker caret or the prompt box's expand button, passes it to `TextBox`. The component places it and reserves the room, so no caller sets padding.

### Kinds

| Kind | `autocomplete` | `autocapitalize`, `autocorrect`, `spellcheck` | Other | Caller may set |
| --- | --- | --- | --- | --- |
| prose | `off` | browser defaults | | |
| line | `off` | browser defaults | | `verbatim`, `secret`, `inputMode` |
| line with `verbatim` | `off` | off, off, false | | `secret`, `inputMode` |
| search | `off` | off, off, false | `type="search"`, `enterkeyhint="search"` | |
| credential | required, from the caller | off, off, false | | `autoComplete`, `inputMode` |

- A line box with `secret` hides its text, sets `autocomplete="off"` and turns typing help off. It is not the credential kind.
- The credential `autoComplete` is `username`, `current-password`, `new-password` or `one-time-code`. The two password values make the input a password input. The other two make it a text input. The login password uses `current-password` and the authenticator code uses `one-time-code`.
- The setup code is not a credential box. It is a line box with `verbatim`, so AutoFill and typing help are off.
- Dropdown pickers keep their combobox role.
- On every search box, `TextBox` calls `preventDefault` on Escape, after the caller's own key handler, so the browser never clears the text. The page decides what Escape does. The Epics filter and the blocker search, which rely on the browser's clear today, clear their query in their own code. Every search box behaves on Escape as it does today.
- No code comment or doc claims that these attributes change the iOS word strip.

### Which box gets which kind

- Search: jump menu, command palette, History search, Epics filter, blocker search, settings search, model filter, diff file filter and the project pickers.
- Line with `verbatim`: worktree and folder pickers, epic slug, rename slug, diff refs, settings value and list boxes, check commands and limits (including "+ add command" and the setup dialog command), schedule times and late limit, the setup code on the setup and login pages, and the screen modal's "Type text".
- Line with `secret`: settings secrets.
- Line: all title boxes and the rest screen unlock box.
- Prose: every prompt box, the chat draft brief, the design draft prompt, the new chat first prompt and the Start Design hint.
- Credential: every login, setup and account box except the setup code.

### Looks and sizes

- Two looks. `boxed` is the default, with the raised surface background, the hairline border and an 8px radius. `bare` has no border and no background.
- Bare boxes are the prompt box's inner textarea, the chat draft brief, the design draft prompt, the History search, and the chat draft and schedule titles.
- Sizes are `sm` 32px, `md` 40px (default) and `lg` 44px, all with 16px text. `sm` is for workspace rename rows and "+ add command". `lg` is for all credential boxes, the setup code boxes and the screen modal's "Type text". The rest use `md`.
- `size="title"` is an extra size of `bare` for the two large titles, 20px bold, also on touch screens.
- Textareas take their height from `rows` or the page layout, and `size` changes only their padding. The prompt box's `sm` and `md` map to these sizes. The component owns padding and line height, and the draft pages and prompt box keep the spacing they have today.
- Focus styles follow from look, kind and size. A boxed box draws a ring. A bare search box draws a ring inside the box. A bare prose box draws none. A title shows a background. The prompt box's outer frame keeps its ring.
- Only the diff file filter, the diff refs and the setup dialog command are monospace, through a `TextBox` option.
- The rest screen unlock box takes the standard look, and the colors that only it used are removed. The small dashed pill for "+ add command" goes away.
- The shared look lives with `TextBox`. Dropdowns and the vi editor, which borrow the old look objects today, use it from there. The old look objects are removed.

### Labels and wording

- Every box that holds a name is labelled "Title", in both label and placeholder. This covers the new chat form, the chat draft page, the rename chat sheet, the chat draft rename sheet, the schedule page, new workspace, workspace rename rows and the web login title in settings.
- The chat draft page keeps the placeholder "Untitled chat". The design draft's slug boxes keep "Epic slug" and "Slug".
- Where a visible label exists, it is the same string as the accessible name. Each box keeps its current label visibility. Accessible names change only for the "Title" boxes, "Search epics" and the option text box.
- Each workspace rename row is a group named after its workspace, around the "Title" box.
- Labels and placeholders hold fixed text. Question numbers, row numbers, values from fixed lists in code and settings labels from the server's registry are allowed.
- The option text box in a question is labelled "Answer to question N" with the placeholder "Your answer". The option's text never goes into its label or placeholder.
- The blocker search placeholder becomes "Search epics".
- The new chat title box's placeholder no longer shows the worktree name. A hint under the box says "Left blank, the chat is named <worktree>." for a new worktree, and "Left blank, gx names the chat." for an existing worktree or the project root. Server naming does not change.
- A hint is always rendered outside the `<label>`. Only credential boxes link it with `aria-describedby`.
- Buttons, menu items, dialog titles, data fields and error texts keep their wording.
- Existing wrappers stop labelling text boxes. The `Field` wrapper stays for dropdowns.

### Prompt box

- The prompt box renders its textarea through `TextBox` as a bare prose box. It keeps its frame, expand button, vi editor, completion menu, focus selectors and big prompt box rows.

### The name rule

- On prose, line and search boxes, a label, placeholder or `name` attribute that contains "name", in any case, breaks the rule. Longer words such as "filename" and "Rename" count. Credential boxes are skipped.
- In development and tests, `TextBox` throws. Dev mode and the gallery are development.
- In a production build, the box renders unchanged. `TextBox` sends a broken rule report from an effect, not during render.
- Settings controls pass a fixed `name` for each control type, never one built from the setting's key. A server test fails when an editable text setting's registry label, or its key's last segment where it has no label, contains "name". The read-only project name stays a read-only row.

### Lint guard

- A Biome GritQL plugin with severity `error` fails lint on a raw text box: `<input>`, `<textarea>`, `chakra.input`, `chakra.textarea`, `chakra("input")`, `chakra("textarea")`, Chakra `<Input>` and `<Textarea>`, `as="input"`, `as="textarea"`, and `chakra.input` or `chakra.textarea` used as a value.
- The input types `file`, `hidden`, `checkbox`, `radio`, `range`, `button` and `submit` are allowed. A `type` set from a variable fails.
- Only the `TextBox` folder, test files and test helper files are exempt.
- A Vitest scan of the files Biome lints fails on any comment that silences the plugin: `biome-ignore`, `biome-ignore-all` and `biome-ignore-start`, each for `lint/plugin` or bare `lint`. The scan skips only its own planted examples.
- Biome 2.6 notes from the research branch `research/text-box-biome-lint`: JSX snippets need `language js(jsx)`, and a pattern with a hyphenated attribute name and a value never matches.
- There are no lint rules for a missing label or focus ring, and no rendered-DOM gallery test. The component sets both.
- The guard runs in the existing lint script and Vitest. No new command.

### Broken rule report

- One generic web-only write behind the web login, for any rule in the web UI. `TextBox` is its first caller. It has no CLI verb, as ADR 0028 allows for a browser-only write. Server code files follow-ups directly. The implementer picks the route and field names.
- A report holds a rule id, a message, the broken value, the page URL without secrets and the user agent. The page URL is cleaned the same way as for client errors, in the browser and again on the server, from one shared place on each side.
- The browser sends each rule and value once per page load and also writes it to the console. A failed send is ignored and never redirects. On a signed-out page the report reaches only the console.
- The server allows 20 reports a minute per IP, with its own counter, and a body of at most 16 KB. It refuses an over-limit, oversized or incomplete report as a result with a stable reason. A rule id must be a short slug.
- The server logs every accepted report.
- The target is the epic in `recovery.follow-ups`, default `gx:follow-ups`. A bare epic name means that epic in project `gx`. If the target project is not registered, the server only logs.
- Filing reuses the existing follow-up writer: a draft research ticket of category bug, in an epic created as draft if missing. The dedupe key is made of the rule id and the broken value.
- Only an open draft takes a repeat. If the draft was closed or opened for work, the next report files a new draft.
- The shared writer keeps one "Seen again" line per source per day, for every caller. The first repeat adds the line. A later repeat with the same source and date updates that line to end in a count, as in `Seen again in the web UI on 2026-11-26 (3 times).` The count is of repeats, so the report that filed the draft is not in it, and a line with no count means one repeat.
- The "bad question block" follow-up keeps its own detailed entries.
- The writer takes a source, a description of where the repeat was seen, in place of requiring a ticket address, so that web reports and ticket callers share it. Ticket callers pass their ticket address, so their line reads as today, apart from the count. Web reports pass the fixed source "the web UI".
- A browser sends each rule and value once per page load, so the web count is roughly the number of page loads that hit the broken rule.
- Two reports of the same pair that arrive together file one draft.
- Text from the report is bounded in length and written so that it cannot add a heading, a section or a second dedupe line to the ticket.
- The web listener owns the request checks. The follow-up code owns the filing. They meet through the listener's host interface.
- There is no ticket or epic schema change, no migration step, no config key and no pause kind.

### Order of work

- Build `TextBox` first. Move text boxes by area in several tickets. Turn on the lint ban last. The report endpoint can be built at any point after `TextBox`.

### Standing doc

- `docs/text-boxes.md` is a deliverable, linked from `.ai/index.md` so that an agent reads it before adding a text box. It is written once, from the final standard.
- It lists each rule, how to pick a kind, look and size, and the iOS reason behind each rule.
- It says that blocking saved-password offers on a setting secret is not proven, because a browser can ignore `autocomplete="off"` on a password input.
- It says that it is not proven that browsers and password managers leave the setup code box alone on the login and setup pages, because they can ignore `autocomplete="off"` there.
- It records what the device checks showed and what they did not: no iPad variant showed AutoFill Contact, "username" and a textarea with only `aria-label` outside a form were not tested, the iPad was not tested around a "Title" box, and no attribute is known to change the word strip.

## Testing Decisions

- A good test checks what a person or a caller can see: roles, accessible names, text, requests sent, files written. Attributes are the exception here, because the attribute is the behaviour.
- `TextBox` Vitest tests cover the attributes of each kind, `verbatim` and `secret`, the linked label, the hint placement, the focus ring attribute, the name rule's throw, Escape on a search box (the caller's handler sees it first, then the default is prevented), and the production path with its single report per page load. Type tests or `@ts-expect-error` lines cover the refused props.
- Only `TextBox` gets new attribute tests. Each move ticket updates the tests of the pages it touches, mostly to the new wording.
- A fixture test runs the lint plugin on bad and allowed examples, with the examples kept in a file that normal lint skips.
- The ignore-comment scan has planted examples for each suppression form. The prior art is the raw style guard test.
- Server route tests cover the report: login required, limits, refusals, URL cleaning, target resolution, an unregistered project, a first filing, repeats on the same day (one line whose count rises) and on a later day, repeats from two ticket sources, a closed draft, concurrent reports, and text that tries to add a section. The prior art is the client error route tests and the recovery follow-up tests. The web-only route list and the web allowlist tests include the new route.
- A registry test covers the setting label rule.
- The gallery shows each kind, look and size. There is no new entry per moved screen.
- Existing e2e flows are updated to the new wording and to the search box role. No new flow is needed, because Vitest and Go tests cover every new rule.
- No test can show iOS keyboard behaviour. The markup rules rest on the device checks already recorded.

## Verification

Run flows with `gx brief -C webui playwright <file>`, Vitest with `gx brief -C webui vitest <path>` and lint with `pnpm --dir webui run lint`. No new flow or scenario is needed.

Title wording (stories 1 to 4)

- Flows: `agents-create`, `agents-session`, `agents-commands-send`, `agents-permission`, `chords`, `chat-draft`, `draft-history`, `schedules`, `sidebar-rows`, `newdesign`, `workspaces`, `workspaces-touch`, `settings`, all querying "Title".
- Browser check on a temporary server: each surface in story 2 shows a box with the accessible name "Title", and no non-credential text box has "name" in its label, placeholder or `name`. Each workspace rename row is inside a group named after the workspace.

Labels (stories 5 to 6)

- Browser check on the pages above and `login`: every input and textarea that takes text has a linked `<label>` and no `aria-label`.
- Vitest: the `TextBox` label tests.

New chat hint (stories 7 to 8)

- Flows: `agents-create`.
- Browser check: pick a new worktree, then an existing one, then the project root. The hint reads as in each story and the placeholder stays "Title".

Question and blocker wording (stories 9 to 11)

- Flows: `design-page`, `triage`, `vi-editor`, `epics`.
- Vitest: the question sheet tests assert "Answer to question N", "Your answer" and the visible option text.

Look, sizes and focus (stories 12 to 18)

- Flows: `settings-page`, `checks`, `diff-page`, `rest`, `login`, `setup`, `jump`, `chat-draft`, `draft-history`, `phone`.
- Browser check at 390px and 1280px: each page above, computed font size of at least 16px on every text box, monospace on the three code boxes only, and the focus style of a boxed box, the History search, a draft brief, both titles and a prompt box.

Typing help and AutoFill (stories 19 to 24)

- Vitest: the `TextBox` kind tests.
- Browser check of attributes on one box of each kind and variant in the lists under "Which box gets which kind", including login, setup, account pages, the setup code box and a setting secret.

Unchanged behaviour (stories 25 to 26)

- Flows: `jump`, `model-switch`, `agents-create`, `chat-draft`, `agents-composer`, `agents-commands`, `composer-images`, `vi-editor`, `draft-history`, `epics`, `diff-page`, `settings-page`.
- Browser check: Escape in each search box (jump menu, command palette, History search, Epics filter, blocker search, settings search, model filter, diff file filter and a project picker) does what it did before.
- Vitest: Escape clears the query of the Epics filter and the blocker search, and the `TextBox` Escape test.

Guard rails (stories 27 to 31)

- Lint passes on the finished tree. A temporary file with a raw `<textarea>` outside the `TextBox` folder makes lint exit non-zero. Remove the file afterwards.
- Vitest: the lint fixture test, the ignore-comment scan and the `TextBox` prop type tests. `gx brief -C webui tsc` exits 0.

Name rule (stories 32 to 34)

- Vitest: the throw in development, no throw on a credential box, and the production path rendering the box and sending one report.
- Server: `gx brief go-test ./config/registry` exits 0 and holds the setting label test.

Broken rule report (stories 35 to 38)

- Server: `gx brief go-test ./server/web ./server/recovery` exits 0 and holds the route and filing tests.
- Browser check on a temporary server with `recovery.follow-ups` set to `demo:follow-ups`: from a signed-in page, post one report three times. `gx tickets show demo:follow-ups` on the server's ticket store lists one draft, and its Evidence holds one "Seen again" line ending in "(2 times).". A report with no login cookie is refused. The server log holds a line for each accepted report.

Standing doc (stories 39 to 39)

- `docs/text-boxes.md` exists, covers the rules and reasons listed under "Standing doc", and `.ai/index.md` links it.

## Self-decided design questions

| Question | Choice | Reason |
| --- | --- | --- |
| Does the name rule need to check `id`? | `TextBox` creates the `id` and refuses a caller's. | A device check showed `id` matters. Removing the prop fixes the cause without a second check. |
| Who picks a box's input type? | `TextBox`, from kind, `secret` and the credential `autoComplete`. | A caller cannot make a password box that shows its text, or a search box without the search type. |
| How do carets and the expand button get room when callers set layout only? | The caller passes the element, and `TextBox` places it and pads for it. | Padding props would reopen styling to callers. |
| Which style props count as layout? | Width, margin, flex and grid placement, width limits, and textarea height limits and resize. | These place a box without changing its look. |
| Is the hint linked to the box? | It sits outside the label, and only credential boxes use `aria-describedby`. | The new chat hint says "named". Device checks covered unlinked visible text only, and today's login hint inside the label would join the accessible name. |
| How is a workspace rename row identified? | The row is a group named after the workspace. | It keeps the fixed "Title" label and was device checked around a "Title" box. |
| What `name` do settings boxes use? | A fixed name per control type. | A name built from a key could contain "name". |
| What stops a future setting label with "name"? | A registry test on the server. | The label is written there, so the mistake fails where it is made. |
| Does Escape change on search boxes? | No. `TextBox` blocks the browser's clear on all of them, and the two pages that rely on it clear themselves. | One rule in one place, and the search type does not change how pickers and dialogs behave. |
| Where do the old look objects' other users go? | Dropdowns and the vi editor use the shared look kept with `TextBox`. | One look has one home, and the old objects can be removed. |
| How does a web report reuse the follow-up writer, which expects a ticket? | The writer takes a description of where the repeat was seen. | A fake ticket address or a second writer would be a workaround. |
| What about simultaneous or hostile reports? | The server files one draft per pair, checks the rule id, bounds the text and keeps it from changing ticket structure. | The browser supplies the content of a file in the ticket store. |
| Are test helper files exempt from the lint ban? | Yes, like test files. | They hold test markup and never ship. |

## Out of Scope

- The CodeMirror vi editor, including whether the draft pages use it on touch screens and whether iOS shows completions there.
- A rule about `<form>`.
- Finding what triggers AutoFill Contact on the iPad, and further device checks.
- Checkbox, radio, range, button, submit, hidden and file inputs, dropdowns, the clipboard copy textarea and the terminal library's textarea.
- Server naming of chats.
- Other callers of the broken rule report, a CLI verb for it, and any change to client error reporting beyond sharing the URL cleaning.

## Further Notes

- The keyboard fixes already on `main` are in commit `a4060a3a3`. This work keeps them and puts them inside `TextBox`.
- Title boxes keep spellcheck and autocorrect, as the "UI fixes" spec asked. The new chat, chat draft and rename boxes turn spellcheck on, so unusual words get a red underline.
- Login and setup boxes change their look slightly.
- Glossary terms used here are Text box, Text box kind, Prompt box, Broken rule report and Follow-up.
