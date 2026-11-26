# Text boxes in the web UI

A text box is any native input or textarea the person types text into. These are not text boxes:

- checkbox, radio, range, button, submit, hidden and file inputs
- dropdowns
- the vi editor
- the off-screen textarea that `webui/src/clipboard.ts` uses to copy text
- a terminal library's own textarea (the web UI has no such library today)

## Use `TextBox`

Draw every text box with `TextBox` from `webui/src/ui/text-box/TextBox.tsx`. One component sets the
attributes, label and focus style, so no page can forget one.

`PromptBox` (`webui/src/ui/expandable-text/PromptBox.tsx`) is the box for writing to an agent. Its
textarea is a `bare` prose `TextBox` with a hidden label. Its frame draws the focus ring, and its
expand button sits in `endElement`. When it shows the vi editor, no `TextBox` is rendered.

| Prop | What it does |
| --- | --- |
| `kind` | Required: `prose`, `line`, `search` or `credential`. |
| `label` | Required. The text of the linked `<label>`. |
| `name` | Required. A fixed name for the box's purpose. |
| `hideLabel` | Hides the label from sight only. |
| `optional` | Adds "optional" after the label. Screen readers skip it. |
| `hint` | A line under the box. |
| `invalid` | Red border and `aria-invalid`. |
| `verbatim`, `secret` | Line boxes only. |
| `autoComplete` | Credential boxes only, and required there. |
| `inputMode` | Line and credential boxes only. |
| `look`, `size`, `page` | See "Looks and sizes". |
| `mono` | Monospace text. |
| `endElement` | Something drawn inside the end edge. |

`TextBox` also passes through `ref`, `value`, `defaultValue`, `placeholder`, `disabled`, `required`,
`autoFocus`, `role`, `minLength`, `list`, event handlers, `data-*` and other ARIA attributes. `rows`
is for prose.

The prop types refuse `id`, `type`, `autoCapitalize`, `autoCorrect`, `spellCheck`, `enterKeyHint`,
`aria-label`, `aria-labelledby`, `data-focus-ring` and style props that are not layout. `TextBox`
owns these.

## Kinds

Prose renders a textarea. The other kinds render an input.

| Kind | `type` | `autocomplete` | Typing help | Use it for |
| --- | --- | --- | --- | --- |
| `prose` | textarea | `off` | browser defaults | Multi-line free text: every prompt box, the chat draft brief, the design draft prompt, the new chat first prompt, the Start Design hint. |
| `line` | `text` | `off` | browser defaults | Title boxes and the rest screen unlock box. |
| `line` + `verbatim` | `text` | `off` | off | Text kept as typed: worktree and folder pickers, epic slug, rename slug, diff refs, settings values and lists, check commands and limits, schedule times and late limit, setup code, the screen modal's "Text to type". |
| `line` + `secret` | `password` | `off` | off | Settings secrets. |
| `search` | `search` | `off` | off | Jump menu, command palette, History search, Epics filter, blocker search, settings search, model filter, diff file filter, project pickers. |
| `credential` | `password` for the two password values, else `text` | `username`, `current-password`, `new-password` or `one-time-code` | off | Login, setup and account boxes, except the setup code. |

"Typing help off" means `autocapitalize="off"`, `autocorrect="off"` and `spellcheck="false"`.

Reasons:

- `autocomplete="off"`: saved-value popups cover the page. Only a credential box wants AutoFill.
- Typing help off: queries, paths, codes and passwords must not get a capital first letter, a
  dictionary rewrite or a spelling underline.
- `type="search"` and `enterkeyhint="search"`: the phone keyboard shows search keys.

The setup code is a `line` box with `verbatim`, not a credential box, so that AutoFill and typing
help are off. **Not proven:** that browsers and password managers leave it alone on the login and
setup pages. They can ignore `autocomplete="off"` there.

**Not proven:** that a `secret` box blocks saved-password offers. A browser can ignore
`autocomplete="off"` on a password input.

No attribute is known to change the iOS word strip.

Dropdown pickers set `role="combobox"` on their box.

## Looks and sizes

| `look` | Draws |
| --- | --- |
| `boxed` (default) | Raised background, hairline border, 8px radius. |
| `bare` | No border, no background. For the prompt box's textarea, the chat draft brief, the design draft prompt, the History search, and the chat draft and schedule titles. |

| `size` | Input height | Use it for |
| --- | --- | --- |
| `sm` | 32px | Workspace rows and "+ add command". |
| `md` (default) | 40px | Everything else. |
| `lg` | 44px | Credential boxes, setup code boxes, "Text to type". |
| `title` | 36px, 20px bold | The two large page titles. `line` with `look="bare"` only. |

- Text is 16px, because iOS Safari zooms into a focused box with smaller text. A `title` box stays at
  20px on touch screens.
- A textarea takes its height from `rows` or the layout. `size` has no effect on it.
- `page` is the roomy spacing of a page that is one big textarea. Prose with `look="bare"` only.
- Use `mono` only for the diff file filter, the diff refs and the setup dialog command. No iOS reason.
- Pass anything drawn inside the end edge, such as a picker caret or an expand button, as
  `endElement`. `TextBox` places it and reserves the room, so set no padding. Clicks pass through it,
  except on a button. No iOS reason.
- Style props place the box only: width, margin, flex and grid placement. A prose box also takes
  minimum and maximum height and `resize`. The lists are `layoutKeys` and `textareaKeys` in
  `TextBox.tsx`. No iOS reason.

Focus styles follow from `look`, `kind` and `size`. No iOS reason.

| Box | Focus style |
| --- | --- |
| `boxed`, and `bare` line or search | A ring inside the box. |
| `title` | A raised background, and the full text instead of an ellipsis. |
| `bare` prose | None. `PromptBox` draws the ring on its frame. |

The ring is in `webui/src/ui/theme.ts`. `webui/src/ui/text-box/look.ts` holds the shared look for
controls that must look like a text box, such as dropdowns and the vi editor. The gallery cases are
in `webui/src/gallery/textBoxEntries.tsx`. Add a case there for a new look or size.

## Labels

- Every box has a real linked `<label>`, which `TextBox` draws. Use `hideLabel` to hide it. Do not
  wrap a text box in another labelling component. iOS: a textarea named only by `aria-label` let
  Safari guess the field type from nearby text.
- A box that holds a name is labelled "Title", with the placeholder "Title". Two boxes differ: the
  chat draft page's placeholder is "Untitled chat", and slug boxes are "Epic slug" or "Slug". iOS:
  see "The name rule".
- `label` and `placeholder` are fixed text. Question numbers, row numbers, values from fixed lists in
  code and settings labels from the server's registry are allowed. A question's answer box is
  "Answer to question N" with the placeholder "Your answer", and the option text stays outside. iOS:
  text a person or an agent wrote can contain "name".
- Put help text in `hint`, which renders outside the `<label>`. Only a credential box links it with
  `aria-describedby`. iOS: "name" in nearby visible text is harmless, but in the label it is not.
- When "Title" repeats, as in workspace rows, put each box in a `role="group"` named after its row
  (`webui/src/shell/ManageWorkspaces.tsx`). iOS: a group name may contain "name".
- `name` is stable: do not build it from user text or a setting's key. Settings controls use one
  fixed `name` per control type. iOS reads `name` for AutoFill.
- On a search box, `TextBox` calls `preventDefault` on Escape after your `onKeyDown`, so the browser
  keeps the text. The page decides what Escape does, including clearing the query. No iOS reason.

## The name rule

On prose, line and search boxes, `label`, `placeholder` and `name` must not contain "name", in any
case. "Filename" and "Rename" count. Credential boxes are skipped, because they want AutoFill. iOS:
"name" brings AutoFill Contact instead of word completions. The match on longer words is wider than
what was tested.

- In dev and tests, `TextBox` throws.
- In a production build, the box renders unchanged. `TextBox` calls `reportBrokenRule` from
  `webui/src/brokenRules.ts` in an effect.

The report is logged to the console and sent to `POST /v1/broken-rules`, once per rule and value per
page load. A failed send is ignored, so a signed-out page reaches only the console. The server
(`server/web/brokenrule.go`) logs each accepted report. The filing
(`server/recovery/brokenrule.go`) writes a draft bug follow-up in the `recovery.follow-ups` epic. A
repeat of the same rule and value adds to a count on that ticket while it is still a draft. If the
target project is not registered, the server only logs. Other web UI rules may use the same report.

A settings label comes from the server's registry. `TestNameRuleGuard` in
`config/registry/registry_test.go` fails when an editable text setting's label contains "name", or
its key's last segment where it has no label. A read-only setting is exempt.

## Lint guard

`webui/biome-plugins/raw-text-box.grit` fails `pnpm --dir webui run lint` on a raw text box:

- `<input>`, `<textarea>`, `<chakra.input>`, `<chakra.textarea>`, `<Input>`, `<Textarea>`
- `chakra("input")`, `chakra("textarea")`
- `as="input"`, `as="textarea"`
- `chakra.input` and `chakra.textarea` used as a value

An input passes only with a literal `type` of `file`, `hidden`, `checkbox`, `radio`, `range`,
`button` or `submit`. A `type` from a variable fails.

`webui/biome.jsonc` exempts three path patterns: `src/ui/text-box/**`, `*.test.ts` and `*.test.tsx`.
Test helper files and e2e flows are linted.

No comment may silence the plugin. `webui/src/ui/lint-ignores.test.ts` fails on `biome-ignore`,
`biome-ignore-all` and `biome-ignore-start` for `lint/plugin` or bare `lint`. The plugin's examples
are in `webui/src/ui/raw-text-box.fixture.tsx.txt`, run by `webui/src/ui/raw-text-box.test.ts`.

## No form rule

A `<form>` may hold any text box. iOS: on a device, a `<form>` made no difference to AutoFill
Contact.

## Device checks

Showed, on iPhone:

- "name" in the `<label>`, `aria-label`, placeholder, or `name`/`id` brings AutoFill Contact.
  `name` and `id` were tested together. No tested attribute stops it (`autocomplete`, `type`,
  `inputmode`, `enterkeyhint`).
- A `<form>` makes no difference.
- Around a "Title" box, "name" is fine in nearby visible text, a dialog title, a row group name, a
  button or the box's value.
- A textarea named only by `aria-label` got AutoFill Contact next to a "Name" box in a form. A
  textarea with its own `<label>` kept word completions there.

Did not show:

- What triggers AutoFill Contact on iPad. No iPad test page variant showed it.
- "username".
- A textarea with only `aria-label` outside a form.
- The iPad around a "Title" box.

The records are the Resolutions of tickets `gx:text-box-standard/05`, `/06` and `/09`. Ticket 06
corrects 05, which suggested the `<form>` was a factor.
