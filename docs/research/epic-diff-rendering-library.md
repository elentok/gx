# Picking the Diff page rendering library

Ticket: `gx:epic-diff/02`. Tags: **[source]** read from the package source or types, installed into a scratch dir (never into the repo); **[npm]** npm registry metadata; **[gh]** GitHub API or release page; **[run]** measured here with esbuild; **[gx]** gx source in this worktree. Versions are the latest on 2026-11-13.

## Answer in short

- Recommended: **`@pierre/diffs` 1.6.4** (React entry `@pierre/diffs/react`). It takes a unified patch, has side-aware line annotations and line selection, a virtualizer, and Shiki highlighting that loads per language.
- Runner-up: **`@git-diff-view/react` 0.1.8**. Good comment widgets, but no virtualization and a heavy highlight.js bundle.
- The server should send **one unified patch per file** (`git diff A...B`) and serve **both file versions on demand** from a second endpoint. The patch is the base. The file versions fix expand-context and highlighting.
- Bundle: about **294 KB minified, 77 KB gzip** for the Pierre entry, plus small shared chunks and lazy grammar chunks. All of it can sit in the lazy Diff route chunk.

## Comparison

| | `@pierre/diffs` | `@git-diff-view/react` | `react-diff-view` | `diff2html` | `@codemirror/merge` |
|---|---|---|---|---|---|
| Version, date | 1.6.4, 2026-11-04 [npm] | 0.1.8, 2026-10-15 [npm] | 3.3.3, 2026-03-30 [npm] | 3.4.56, 2026-01-31 [npm] | 6.12.3, 2026-10-19 [npm] |
| License | Apache-2.0 [npm] | MIT [npm] | MIT [npm] | MIT [npm] | MIT [npm] |
| Open issues, last push | 78, 2026-11-13 [gh] | 14, 2026-11-11 [gh] | 11, 2026-03-30 [gh] | 37, 2026-05-08 [gh] | not checked (repo moved to code.haverbeke.berlin [npm]) |
| Input | patch string (`PatchDiff`) or two file texts (`MultiFileDiff`) [source] | `hunks` (patch text) with optional old and new file text, or two file texts [source] | patch text via `parseDiff` (gitdiff-parser) [source] | patch text, returns an HTML string [source] | two full documents only [source] |
| Split and unified | `diffStyle: 'unified' \| 'split'` [source] | split and unified [source] | `viewType` split and unified [source] | side-by-side and line-by-line [source] | `MergeView` (split) and `unifiedMergeView` [source] |
| Syntax highlight | Shiki, JS regex engine by default [source] | highlight.js through lowlight [source] | none built in (refractor is an add-on) | highlight.js, you ship it [source] | CodeMirror language packs [source] |
| Virtualization | `Virtualizer` and `CodeView` components, `disableVirtualizationBuffers` option [source] | none found in `src` [source] | none | none | CodeMirror renders only the viewport [source: CM6 design] |
| Comment or line anchor | `lineAnnotations: {side, lineNumber, metadata}`, `renderAnnotation`, `selectedLines: {start, side, end, endSide}`, `renderGutterUtility` [source] | `extendData`, `renderWidgetLine`, `onAddWidgetClick` [source] | `widgets` keyed by change; one widget per change [source] | none; you hook DOM nodes | build with decorations and widgets yourself |

## `@pierre/diffs` (recommended)

- **Input.** `PatchDiff` takes `patch: string`. `MultiFileDiff` takes `oldFile` and `newFile` (`FileContents`: `name`, `contents`, optional `lang`, `cacheKey`) [source: `dist/react/PatchDiff.d.ts`, `dist/types.d.ts`].
- **Patch plus full files.** A diff parsed from a patch is "partial". The option `loadDiffFiles` fetches both full files for a changed or renamed file, so the renderer can expand context and highlight with full-file state [source: `dist/types.d.ts`, `BaseDiffOptions.loadDiffFiles`]. This matches the server shape below.
- **Anchors.** `DiffLineAnnotation = { side: 'deletions' | 'additions'; lineNumber: number; metadata }`. `SelectedLineRange` has `start`, `side`, `end`, `endSide`. A review comment maps to (file, side, line) directly. `renderAnnotation` renders any React node under the line [source: `dist/types.d.ts`, `dist/react/types.d.ts`].
- **Large files.** The `Virtualizer` React component wraps the file views and renders a window [source: `dist/react/Virtualizer.d.ts`]. Tokenizing stops at `DEFAULT_TOKENIZE_MAX_LENGTH = 1e5` characters [source: `dist/constants.js`]. A worker pool is available (`disableWorkerPool` prop, `@pierre/diffs/worker`) [source].
- **Themes.** `theme` takes one Shiki theme or `{light, dark}`; `themeType` forces a mode. `unsafeCSS` injects CSS [source: `BaseCodeOptions`]. The component renders in a shadow DOM web component [source: `dist/components/web-components.js` calls `attachShadow`]. CSS custom properties pass through a shadow root, so Chakra token variables can reach it. Mapping them is still to be tried.
- **Phone.** `overflow: 'wrap' | 'scroll'` and `diffStyle: 'unified'` give a workable phone layout. No built-in breakpoint switch was found, so the page picks `unified` from a media query itself [source].
- **Bundle [run].** `esbuild --bundle --minify --splitting`, React external, entry importing `PatchDiff` only:
  - Entry chunk 294 KB min, 77 KB gzip. Three small shared chunks plus one of 9 KB gzip load with it.
  - Two dynamic imports (`shiki`, `highlights`) and about 480 lazy chunks: one per Shiki grammar or theme. The whole output tree is 11 MB, but a page loads only the grammars of the languages it shows.
  - There is also a Shiki WASM chunk (162 KB gzip), used only for the optional WASM engine. The default is the JS regex engine [source: README].
  - For scale, the same measure for the CodeMirror `EditorView` alone is 202 KB min, 66 KB gzip.
- **Maintenance.** Active: pushed today, 78 open issues, 6.5k stars [gh]. The 1.x line is stable, but 1.7.0 betas shipped on 6, 8 and 9 Nov, and 1.6.3 changed a call signature; 1.6.4 fixed a broken 1.6.3 build [gh: releases]. Pin the exact version and expect upgrade work. The package is 11 MB unpacked [npm].

## Why not the others

- **`@git-diff-view/react` (runner-up).** Fits comments well: `renderWidgetLine`, `extendData`, `onAddWidgetClick` [source]. Its limits:
  - No virtualization in `src` [source]. A 5000-line file renders every row.
  - Highlighting is skipped for files over 2000 lines (`maxLineToIgnoreSyntax = 2000`) [source: `@git-diff-view/lowlight/src/index.ts`, `core/src/file.ts`].
  - Entry bundle 1.24 MB min, 347 KB gzip, because highlight.js comes with it [run].
  - Version 0.1.x, so the API is not promised stable [npm].
- **`@codemirror/merge`.** Smallest delta: about 73 KB min, 24 KB gzip over the CodeMirror we already ship (275 KB vs 202 KB, [run]), and viewport rendering built in. It needs two full documents, not a patch [source]. It has no comment model, so we would build anchors, gutters, the file-level container and the phone layout ourselves. It is an editor with diff, and it fits a line-anchored review UI worse. It stays the fallback if bundle size becomes the main concern.
- **`react-diff-view`.** Good widget model, but one widget per change, no virtualization, no built-in highlighting, last release March 2026 [source, npm].
- **`diff2html`.** Returns an HTML string. No React model for anchors or comments, no virtualization [source].

## What the server must send

- **Patch text is the base.** Per file, from `git diff A...B` (three-dot, merge-base to B). The patch gives hunks, renames and binary markers, and matches what `PatchDiff` and `git-diff-view` parse. Both work from patch text alone; only the full-file features need blobs.
- **Both file versions on demand.** An endpoint per file returns the old blob (merge-base) and new blob (B). The UI feeds it to `loadDiffFiles`. It gives full-file Shiki state (a hunk can start inside a block comment or a template string) and "expand context". Do not send file versions for all files up front.
- **Review milestone.** Line anchors are (file path, side, line number) as in `DiffLineAnnotation`. For `A...B`, "deletions" lines come from the merge-base blob and "additions" lines from B. The comment record should also store the two commit SHAs, so a comment survives later pushes.

## Not confirmed

- No browser run. Behaviour on a real 5000-line file (frame time, memory) is not measured for any library. For Pierre it is inferred from the `Virtualizer` API.
- Whether Chakra v3 tokens map cleanly into the Pierre shadow DOM, and which Shiki theme pair matches the app, is not tried.
- Pierre's phone behaviour (touch selection, narrow split view) is not tried; the docs site `diffs.com` was too large for our fetch tool, so claims come from the installed types and README.
- The runtime set of chunks a typical page fetches (Shiki core, one theme, a few grammars) is not measured; only build output sizes are.
- `@codemirror/merge` issue count and last commit were not checked (the repo moved off GitHub).
