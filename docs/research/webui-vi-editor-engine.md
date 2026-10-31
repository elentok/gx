# Editor engine for the web UI's large editor (gx:webui-vi-mode/01)

Which engine the large editor should use: CodeMirror 6 with `@replit/codemirror-vim`, a custom vi
layer on a text area, or something else.

Each claim is marked **verified** (read in a primary source, or reproduced in a prototype),
**inferred** (my reasoning from verified facts), or **unverified** (not checked, or not checkable
here). The prototype ran in desktop Chromium through Playwright 1.64.1 with synthetic key events. It
never ran on a phone, in Safari, or in Firefox. Dates are as of 2026-10-31.

## Recommendation

Use **CodeMirror 6 with `@replit/codemirror-vim`**, loaded with a dynamic `import()` when the editor
first opens. Turn vi mode on and off with a CodeMirror `Compartment`, so one editor serves both
modes.

- It meets the whole agreed command list. Every command passed in the prototype (see "Commands").
  Only `ZZ` needs a few lines of host code.
- It handles variable-width text. The block cursor takes the width of the glyph under it. `j` and `k`
  need one decision from the spec (see "Variable-width text").
- The custom text-area layer would rebuild an undo stack, a block cursor, visual-line geometry, and
  the whole modal engine. That is a large amount of hand-written code. It does not fix the mobile
  problem either, because the mobile problem sits in the browser's input events, not in the engine.
- Cost: about 135 KB gzipped (415 KB minified), loaded only when the editor opens.

**Requirement it does not meet.** On Android, the vim layer is unreliable with the on-screen
keyboard. The maintainers' issue tracker has an open report (replit/codemirror-vim#200) of letters
inserted in normal mode and of getting stuck in insert mode, on a phone and a tablet. I did not test
any device. Ticket 02 (mobile) must design around this, and the spec should not promise full vi
behaviour on an Android on-screen keyboard until a device test passes.

## Summary by requirement

| Requirement | CodeMirror 6 + replit vim | Custom text area layer |
|---|---|---|
| Command list | Met, verified in Chromium. `ZZ` is host code. | Built by hand |
| Variable-width text | Met. Cursor draws at glyph width. `j`/`k` keep the character column. | Needs a mirror element to measure wrapped lines |
| Plain typing, vi off | Met, verified. Same editor, vim extension removed. | Native |
| Mobile keyboards | Known open problems on Android. Not tested on a device. | Same browser limits, none of CodeMirror's workarounds |
| Escape, Cmd-G, Ctrl-G, Cmd-Enter | Page-level capture works, verified in Chromium. Browser-level capture unverified. | Plain `keydown`, same unverified browser part |
| Bundle and lazy load | +135 KB gzip, separate chunk, verified | 0 KB, but much hand-written code |
| Maintenance, licence | MIT. Active, with long gaps between releases. | Ours |

## Commands

I drove the prototype with real key presses and read the document back. Passing, in normal mode
unless noted:

- Movement: `h j k l w e 0 f{char} gg G`.
- Edits: `x`, `dw`, `cw`, `yy`, `p`, `u`, Ctrl-R, `.` (repeat of `dw` and of an insert such as `iXY<Esc>`).
- Mode keys: `i`, `a`, `o`, and `v`/`V` with `d`.
- Mode names: `vim-mode-change` events and `getCM(view).state.vim` expose insert, visual, and
  normal, so the editor can show the mode as text (a built-in `--NORMAL--` status panel also exists,
  via `vim({ status: true })`).

`ZZ` is not in the default keymap (verified: no `ZZ`, `Z`, or `:wq` entry in the engine source). It
works through the documented API, and the prototype confirmed it:

```ts
Vim.defineAction("closeEditor", () => closeEditor());
Vim.mapCommand("ZZ", "action", "closeEditor", {}, { context: "normal" });
```

The README documents `Vim.map`, `Vim.defineEx`, and the cm5 API on `view.cm`.

## Variable-width text

- **Cursor** (verified in source). The block cursor draws a box at the character's own coordinates.
  It uses `view.coordsForChar` when present and copies the glyph, font, and size into the box. A
  wide `W` gets a wide block. The extension needs `drawSelection()` for visual-mode selection.
- **Default font** (verified). CodeMirror's base theme sets a monospace font on `.cm-scroller`. The
  app must override it with a theme or a more specific selector. My plain stylesheet lost to it.
- **`j` and `k`** (verified in the prototype). They move by logical line and keep the same character
  column, as in real vim. From a wide `W` at x=115 px, `k` landed on a narrow letter at x=72 px.
  A long paragraph that wraps over several screen rows counts as one line, so `j` jumps past the
  whole paragraph. Prose fields are mostly long paragraphs, so this is the main decision for the
  spec.
- **`gj` and `gk`** (verified). They move by screen row and keep the pixel x position. Mapping `j`
  to `gj` and `k` to `gk` with `Vim.map` works and suits prose. **I recommend the spec map them.**
- **Stale goal column** (verified). After the cursor moves by mouse or tap, the next `gj`/`gk` used
  the old x position from before the click (landed at x=27 instead of near 115). Setting
  `getCM(view).state.vim.lastMotion = null` on a pointer-driven selection change fixed it. That is
  internal engine state, so the implementation ticket needs a test that fails if an upgrade breaks
  it. Plain `j`/`k` were not affected in my test.

## Plain typing with vi mode off

Verified. The prototype started without the vim extension, typed text normally, and switched vi
mode on with `compartment.reconfigure(vim())`. Switching off is `reconfigure([])`. The README says
to put `vim()` before other keymaps. Switching does not recreate the editor, so the text and scroll position stay. The undo history should
stay too (inferred, not tested).

Autocapitalize, autocorrect, and spellcheck are set per editor with
`EditorView.contentAttributes`. A different value for vi on and for vi off can sit in the same
`Compartment`. Whether iOS or Android honour those attributes inside a contenteditable is
unverified.

## Mobile browsers

What the sources say, none of it tested on a device.

- CodeMirror's home page lists "Mobile Support" as "Use the platform's native selection and editing
  features on phones." The editor is a contenteditable element with a DOM observer, not a text area.
- CodeMirror's view source says Chrome on Android "doesn't fire useful key events", so it builds key
  events from `beforeinput`. A source comment adds: "preventDefault on beforeinput, though supported
  in the spec, seems to do nothing at all on Chrome". On Chrome 126 and later on Android, it uses the
  `EditContext` API instead.
- The vim plugin has code for composition and for key `Unidentified`/`Process`/`Dead`, which is what
  on-screen and IME keyboards send. It removes and re-inserts the editor element to end a
  composition in normal mode. It also carries a Firefox on Linux workaround.
- The input events spec says `beforeinput` for `insertText` is cancelable, but for
  `insertCompositionText` it is "Not cancelable". Autocorrect, dictation, and IME text arrive as
  composition. No engine can block that input in normal mode by cancelling the event.
- Open issues in replit/codemirror-vim, as of this note:
  - #200, Android phone and tablet, native keyboard and Bluetooth keyboard. Navigation keys insert
    characters in normal mode, and Escape sometimes fails. Updated 2026-05-12.
  - #178, Chinese IME, random input and cursor jumps in normal mode.
  - #159, dead keys inserted as text in normal mode.
  - #243, iOS Safari with a French Bluetooth keyboard, `{` and `}` insert text.
  - #115, `Ctrl-D` does not work in Safari on iPad.
- **Inferred.** On a phone, normal mode should not take text input from the on-screen keyboard.
  Options for ticket 02: keep the editor read-only or `inputmode="none"` while in normal mode and
  drive it from the helper bar, or offer insert mode only. CodeMirror supports both
  (`EditorView.editable`, `contentAttributes`). I did not try either.
- Hardware keyboards on phones and tablets are the same code path as above and are covered by
  #200 and #243. Unverified beyond those reports.

## Taking keys before the browser or the page

Verified in Chromium with synthetic events. A host keymap wrapped in `Prec.highest` received
Ctrl-G, Cmd-G, and Cmd-Enter in normal, insert, and visual mode, and with vi mode off.

- `Mod-` means Cmd on Mac and Ctrl elsewhere. A Ctrl-Enter press on Mac did not trigger `Mod-Enter`.
  The spec says both Cmd-Enter and Ctrl-Enter, so bind `Mod-Enter` and `Ctrl-Enter`.
- The engine ignores keys it has no binding for when they carry Ctrl, Alt, or Meta (source:
  `findKey` returns nothing for them). In normal mode it swallows unbound single printable keys. So
  Ctrl-G and Cmd-G never reach the engine as text.
- **Escape.** In insert mode the engine consumes it (verified: a `document` keydown listener did
  not fire). The source also consumes it in visual mode, which I did not measure. In normal mode the engine does not consume it, and it bubbled to `document`. A parent
  modal that closes on Escape would close on a normal-mode Escape. The host must stop that event
  itself. This feeds ticket 03.
- **Unverified.** Whether Chrome, Safari, and Firefox let a page cancel Cmd-G ("find next") and Ctrl-G
  before the browser acts. Synthetic events never reach browser menus. Chromium has a function
  `IsReservedCommandOrKey` that decides which shortcuts a page cannot override. In fullscreen it
  reserves only the exit commands. I could not retrieve the list of commands it reserves in a normal
  window, so I cannot say whether "find next" is on it. Safari and Firefox were not checked.

## Bundle size and lazy loading

Measured with Vite 7.3.7 on a minimal editor (history, default keymap, `drawSelection`, line
wrapping, vim), pinned to the latest versions below.

| Chunk | Minified | Gzip | Brotli |
|---|---|---|---|
| CodeMirror core (state, view, commands, search, language) | 283 KB | 93 KB | not measured |
| `@replit/codemirror-vim` (with its core) | 133 KB | 42 KB | 37 KB |
| Both | 415 KB | 135 KB | not measured |
| gx web UI today (`index` + `vendor`) | 1,112 KB | 335 KB | not measured |

- The vim package is ESM with `sideEffects: false`. The engine ships as one 265 KB unminified file.
  I did not check how much tree shaking removes.
- With `import()`, Vite put the editor into its own chunk (verified in the prototype). The existing
  `manualChunks` rule only matches React, Chakra, and Emotion, so it would not pull CodeMirror into
  `vendor`.
- The server embeds `webui/dist` with `go:embed`, so extra chunks need no server change.
- A language pack and `basicSetup` are not needed for prose and would add size.
- Content security policy. I found no CSP header in the repo. CodeMirror injects `<style>` elements
  and has a `cspNonce` facet if a policy is added later.

## Maintenance and licence

- Licence: MIT for CodeMirror packages and for `@replit/codemirror-vim` and
  `@replit/codemirror-vim-core`.
- CodeMirror 6 packages are current. `@codemirror/view` 6.43.15 was published 2026-10-19, `state`
  6.7.6 on 2026-09-22. Package metadata shows the repositories at code.haverbeke.berlin.
- `@replit/codemirror-vim` 6.4.0 was published 2026-07-29 with npm provenance. The release before it,
  6.3.0, came on 2025-03-27, a gap of 16 months. The repo has 481 stars, 43 open issues, and is not
  archived. Its last feature commits are from 2026-02-21, and a CI change followed on 2026-05-12. The engine now lives in
  `@replit/codemirror-vim-core` 0.1.0 (published 2026-07-28), which also serves a CodeMirror 5
  package.
- The engine was written by Yunchi Luo as part of CodeMirror and moved to this repo (core README).
  The test suite has 700+ tests (core README).
- Risk, inferred. Releases are rare and bug fixes wait in the issue list. Pin exact versions and
  keep the vim layer behind one gx module so a fork or replacement stays cheap. There is also a
  fork, `@saberzero1/codemirror-vim` (1 star, 6.4.7 published 2026-10-30). I did not evaluate it and
  do not recommend it.
- Unit tests. CodeMirror under jsdom runs after two stubs: `Range.prototype.getClientRects` and
  `getBoundingClientRect`. Without them a requestAnimationFrame measure throws. With them, a vitest
  test sent `keydown` events to `view.contentDOM` and the vim engine edited the document (verified).
  Layout-dependent behaviour (`gj`, cursor position) needs the Playwright e2e suite.

## What a custom layer on a text area would build by hand

The text area has native mobile, spelling, and screen reader support for insert mode, and adds no
bundle. These items are what it would cost, and none of them is small.

1. **The modal engine.** Operators with motions, counts, registers, visual mode, and dot repeat. The
   engine in the recommended option is a 265 KB source file with a 700-test suite.
2. **Undo and redo.** MDN says edits made through `execCommand` keep the undo buffer "unlike direct
   DOM manipulation", and that `execCommand` should be avoided in new projects. So scripted edits to a
   text area need either that deprecated call or our own history stack. `u` and Ctrl-R need one of
   them. Inferred: coalescing an insert session into one undo step is also ours to write.
3. **Block cursor.** A text area cannot draw one. CSS `caret-shape: block` exists but MDN marks it
   experimental and "not Baseline". Otherwise we draw an overlay on a mirror of the text.
4. **Wrapped-line geometry.** A text area gives character offsets only, not screen rows. `gj`, `gk`,
   and a cursor overlay need a hidden element that copies the font and width. Inferred from the
   missing API; I found no primary statement.
5. **Mobile.** The same `beforeinput` limits apply, and the CodeMirror source says cancelling it does
   nothing on Chrome Android. We would meet #200-style problems with none of CodeMirror's
   `EditContext` work.
6. **Selection and key handling.** A visual-mode selection can use the native selection, which is
   an advantage over CodeMirror. Everything else in this list does not have one.

## Other options

- **Monaco.** The README answers "Is the editor supported in mobile browsers or mobile web app
  frameworks?" with "No." The package is MIT but 101 MB unpacked. Rejected on mobile alone.
- **Ace.** BSD-3-Clause, active (pushed 2026-10-29), with a vim key binding in `src/keyboard`. The
  minified core is 479 KB (128 KB gzip) plus 119 KB (37 KB gzip) for vim, so about 164 KB gzip,
  slightly larger than the recommendation. It reads input through a hidden text area. It has
  character-width measuring code in `font_metrics.js`, but I did not test variable-width text, wrapped-line
  motion, or mobile. Not evaluated further. Unverified.
- **CodeMirror 5 with `cm5-vim`.** `cm5-vim` 0.0.6 was last published 2023-02-27. CodeMirror 6 is the
  maintained line. Rejected.
- **ProseMirror or Tiptap.** The npm search for "prosemirror vim" found no vim package. That search is
  not proof that none exists. These editors target rich text and the fields here are plain text.
  Not pursued.

## Prototype wiring worth keeping

```ts
const vimC = new Compartment();
const host = Prec.highest(keymap.of([
  { key: "Mod-g", run: toggle, preventDefault: true },
  { key: "Ctrl-g", run: toggle, preventDefault: true },
  { key: "Mod-Enter", run: submit, preventDefault: true },
  { key: "Ctrl-Enter", run: submit, preventDefault: true },
]));
new EditorView({ state: EditorState.create({ doc, extensions: [
  host, vimC.of([]), history(), drawSelection(), EditorView.lineWrapping,
  keymap.of([...defaultKeymap, ...historyKeymap]),
]}) });
// vi on / off:
view.dispatch({ effects: vimC.reconfigure(on ? vim() : []) });
```

The `toggle` and `submit` handlers in the prototype only logged the call. They did not open or close
anything.

## Open items for other tickets

- Ticket 02 (mobile): design for Android normal mode, composition, and the helper bar. Test on a
  real Android phone and an iPhone before the spec promises anything.
- Ticket 03 (layers): stop normal-mode Escape from reaching a parent modal. Confirm Cmd-G and Ctrl-G
  cancellation in real Chrome, Safari, and Firefox.
- Spec: decide whether `j`/`k` map to `gj`/`gk`. Add the pointer-selection reset to the
  implementation tickets.
- Accessibility. The CodeMirror home page says it "Works well with screen readers and keyboard-only
  users", but that claim does not cover vi normal mode. `EditorView.announce` exists for screen
  reader messages and could announce mode changes. Not tested with a screen reader.
- Safari and Firefox on desktop were not run. The Playwright WebKit build on this machine did not
  match the installed version, and Firefox was not installed.

## Sources

- CodeMirror home page, https://codemirror.net/ (browser support, mobile, accessibility, MIT).
- CodeMirror system guide, https://codemirror.net/docs/guide/ (keymap precedence, contenteditable).
- `@codemirror/view` 6.43.15 source and types (`dist/index.js`, `dist/index.d.ts`): Android handling,
  `EditContext`, `coordsForChar`, `cspNonce`, `announce`.
- `@replit/codemirror-vim` 6.4.0 and `@replit/codemirror-vim-core` 0.1.0 tarballs from the npm registry,
  and https://github.com/replit/codemirror-vim (`src/index.ts`, `src/cm_adapter.ts`, `src/block-cursor.ts`,
  `packages/codemirror-vim-core/vim.js`, README, commit log, issues #115, #159, #178, #200, #243).
- npm registry metadata for `codemirror`, `@codemirror/*`, `ace-builds`, `monaco-editor`, `monaco-vim`,
  `cm5-vim`, `@saberzero1/codemirror-vim`.
- W3C Input Events Level 2, https://www.w3.org/TR/input-events-2/ (cancelable input types).
- Monaco README, https://github.com/microsoft/monaco-editor/blob/main/README.md (mobile answer, MIT).
- Ace, https://github.com/ajaxorg/ace and `ace-builds` 1.45.2 (licence, vim key binding, build sizes).
- MDN, `Document.execCommand`, `CSS caret-shape`, `HTMLInputElement.setRangeText`.
- Chromium `chrome/browser/ui/browser_command_controller.cc` (`IsReservedCommandOrKey`).
- gx: `webui/src/design/NoteBox.tsx`, `webui/vite.config.ts`, `webui/package.json`, `webui/webui.go`.
- My prototype: a scratch Vite project with CodeMirror 6, `@replit/codemirror-vim` 6.4.0, Playwright
  1.64.1 on Chromium, and vitest 3 with jsdom 26. It is not in the repo.
