# Can Biome lint the text box rules

Ticket: `gx:text-box-standard/02`. Tested on Biome 2.6.6, `webui/` at main (6bfbd2d4d). The plugin files, the fixtures and the DOM probe are in `docs/research/text-box-biome-lint/`.

## Answer

- Biome GritQL plugins catch rule 1 and rule 3 well, and can run as `error` in `pnpm run lint`. They are fast.
- Plugins cannot prove rule 2 (a label is linked). They can only check a proxy: `aria-label`, `aria-labelledby` or `id` on the element.
- Rule 4 works as a plugin (attribute check on the same element).
- A rendered-DOM check is the only correct guard for rule 2, and the best one for rule 3 through wrapper components. It covers only what the gallery renders.
- Recommendation: plugins for rules 1, 3 and 4 (cheap, run on every file), plus a gallery DOM test for rule 2 and as a backstop for rule 3. A Vitest source scan is the worst fit: no parser is available (see below), so it would be regexes.

## What a Biome plugin can do (tested)

Sources: Biome plugin docs (https://biomejs.dev/linter/plugins/) and real runs.

- JSX matches work. Snippets such as `` `<chakra.input $props />` `` and `` `<$tag $props>$children</$tag>` `` match elements, including dotted tags like `chakra.input`. The docs page does not mention JSX. It works in 2.6.6 with `language js(jsx)`.
- Attribute matches work for plain names: `` `type="file"` ``, `` `as="form"` ``. `as="input"` on any component matched too.
- Hyphenated names with a value pattern do not work. `` `data-focus-ring=$_` `` and `` `aria-label=$_` `` never match, so a `not ... contains` check on them flags every element. A regex on the attribute works instead: `r"data-focus-ring.*"`. This cost one wrong run before I found it. A real plugin needs a test fixture so this cannot regress.
- `contains` and `not ... contains` work over the props and children, so "a form that contains a text box" (rule 3) works within one file.
- `register_diagnostic(span=..., message=..., severity="error")` gives `plugin` diagnostics with the message and a code frame. Default severity is `error`. They fail `biome lint`.
- Suppression works: `// biome-ignore lint/plugin: reason` on the line above silences it. A stale one is reported ("Suppression comment has no effect").
- Config: `"plugins": ["./x.grit"]`, or `{ "path": ..., "includes": [...] }` per the docs. I also tried `overrides[].plugins` with `includes` and a `!src/auth/AuthFields.tsx` exclusion, and it worked, so the standard component's own file can be exempt.
- Speed: `biome lint .` over `webui/` takes about 0.2 s without plugins and 0.3 s with six plugins. The docs warn that plugins run on every file.
- Limits (docs and runs): no semantic model, no types, no cross-file or cross-component knowledge, no link between a `<label>` and an input. A plugin sees only the syntax of one file.

## Rule by tool

| Rule | Biome plugin | Vitest source scan | Rendered DOM (gallery) |
| --- | --- | --- | --- |
| 1 Raw text box | Catches `<input>`, `<textarea>`, `chakra.input`, `chakra.textarea`, `<Input>`, `<Textarea>`, and `as="input"`/`as="textarea"`. Skips `file`, `hidden`, `checkbox`, `radio`, `range`. Misses `chakra("input")` aliases and a `type={expr}` (flagged, may be a false positive). Cost: 3 small files, about 1 h. | Same coverage by regex. Misses what multi-line attributes hide. Cost: about the same, but a hand-made tag parser or a new parser dependency. | Catches only what the gallery renders (8 gallery entries hold text boxes; 32 source files have one). Misses every page not in the gallery. |
| 2 Label | Proxy only. Flags a box with no `aria-label`, `aria-labelledby` or `id`. Misses an `id` with no label. False positive on a box wrapped in `<label>` and on a `{...spread}` that supplies the label. Cost: 1 file, but the proxy is the limit. | Same proxy, same limits. | Correct. `el.labels`, `aria-label`, `aria-labelledby` use the real DOM link. Run: 0 unlabelled boxes in the 52 rendered. Cost: one test, about 30 lines. |
| 3 Form with a text box | Catches `<form>`, `<chakra.form>`, `as="form"` with a box inside the same file. Found 6 real sites in `src/`. Misses a box hidden in a wrapper (`AuthField` inside `chakra.form` in LoginPage and SetupPage). Skips checkbox-only forms. | Same by regex only if it matches nested tags. Fragile. | Catches wrapper cases: the probe flagged `LoginCard`, `ChangePasswordView`, `SetupCodeCard` and others (`closest("form")`). Only inside the gallery. |
| 4 `data-focus-ring` | Catches a box with no `data-focus-ring` on the element. 60 hits in `src/` (incl. tests). Misses a `{...spread}` that carries it (false positive), and a standard component that sets it internally (then the plugin should skip that component's file). | Same. | Correct for what renders (`closest("[data-focus-ring]")` also counts a wrapper). Found 8 gallery entries with no ring. |

## Run numbers

- `src/` with all plugins: R1 69 (21 are my fixtures, so about 48 real, including tests), R2 14 (mostly my fixtures; the real sources use `aria-label` already), R3 6 real sites, R4 60.
- Gallery DOM probe: 388 cases rendered, 0 errors, 52 text boxes, 0 unlabelled, form hits in the Login and Setup cards, missing ring in Auth cards and SettingsHome. It ran in about 4.5 s as a Vitest file.

## Why not a source-scan test with a parser

- `webui/package.json` has `typescript ^7.0.2`. TypeScript 7 has no JS compiler API: `ts.createSourceFile` and `ts.ScriptTarget` are undefined, so an AST walk fails at once.
- `@babel/parser` is only a transitive dependency. A source-scan test would need a new direct dependency, or regexes like `raw-style.test.ts`. `raw-style.test.ts` works for flat string patterns (hex colors, font names). Rule 3 needs element nesting, which regex cannot do well.
- The correct fix for nesting is a real parser, and Biome already is one. That is why the plugin beats a regex scan for rules 1, 3 and 4.

## Recommendation

1. Add Biome plugins for rules 1, 3 and 4. Use a `{ path, includes }` entry so the standard component file is excluded. Add a test fixture (like `fixture.tsx.txt`) so a Biome upgrade or a pattern change shows up. Severity `error`.
2. Add a gallery DOM test for rule 2 (label linked) and as a second check for rule 3 through wrappers. Make each text-box component appear in the gallery so the test covers it. This gives a clean guard instead of a proxy check.
3. Skip the regex source scan. If the plugin proxy for rule 2 is too noisy, drop it and rely on the DOM test, rather than keep a check that flags wrapped labels.
4. Prefer a standard component that sets `data-focus-ring`, `aria-label` support and the label itself. Then rules 2 and 4 become checks on one component, and rule 1 is the lint that keeps everyone else off raw boxes. This is the clean design and is cheaper to keep than four plugins.

## Files

- `docs/research/text-box-biome-lint/raw-input.grit`, `raw-textarea.grit`, `as-prop.grit`, `form.grit`, `label.grit`, `focus.grit`
- `docs/research/text-box-biome-lint/fixture.tsx.txt`, `spread.tsx.txt`: inputs I ran the plugins on (renamed `.txt` so lint ignores them)
- `docs/research/text-box-biome-lint/dom-probe.test.tsx.txt`: the throwaway DOM probe
