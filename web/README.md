# gx website

The landing site for **gx**, served at [gx.elentok.com](https://gx.elentok.com). It deliberately
renders as a faux-`gx` TUI screen (Catppuccin Mocha, Agave + Nerd Font, bordered panels, pill
badges, sync glyphs) so the page itself previews the product. See
[ADR 0012](../docs/adr/0012-website-in-repo.md) and
[the PRD](../docs/prds/publish-gx-website.md).

## Stack

Vite + **Preact** (JSX, TypeScript), **prerendered to static HTML** at build time
(`@preact/preset-vite`) and fully hydrated on the client. Styling is **hand-written CSS** — no
CSS-in-JS, Chakra, or Emotion. See [ADR 0012](../docs/adr/0012-website-in-repo.md) for why Preact
replaced the original vanilla-TS choice.

## Develop

```bash
npm install
npm run dev      # http://localhost:5173 — renders the primitive preview
npm run build    # type-check + production build into dist/
npm run preview  # serve the production build locally
```

## Deploy

Manual, mirroring the cryptowl workflow (not Git-connected — see ADR 0012):

```bash
npm run deploy   # build + wrangler pages deploy dist --project-name=gx
```

The `gx.elentok.com` custom domain is configured once in the Cloudflare Pages dashboard.

## Layout

```
public/
  logo-440.webp                # web-optimized hero logo (~63 KB), from ../docs/logo.png
  favicon-64.png               # favicon, from ../docs/logo.png
  fonts/                       # self-hosted, offline-safe
    Agave-Regular.woff2        # the terminal font gx is used with
    Agave-Bold.woff2
    SymbolsNerd-Subset.woff2   # Nerd Font glyphs Agave lacks, subset to ~3 KB
src/
  index.tsx                    # entry: hydrate() on the client, prerender() at build
  app.tsx                      # composes the page (hero + primitive preview)
  components/
    Icon.tsx                   # Nerd Font glyph by semantic name
    primitives.tsx             # Panel, Badge, Button, Cmd, SyncStatus
    Tabs.tsx                   # the one stateful (hydrated) widget
  styles/
    tokens.css                 # palette (mirrors ui/styles.go) + lime accent + icon codepoints
    fonts.css                  # @font-face declarations
    base.css                   # reset + page base
    primitives.css             # faux-TUI vocabulary: panel, badge, sync, tabbar, btn
    layout.css                 # page composition (hero, preview panels)
```

## Regenerating the fonts

The web fonts are derived from the installed Agave + Symbols Nerd Font. To rebuild them (e.g. after a
font update), convert the TTFs to woff2 and re-subset the symbol glyphs to the codepoints in
`ui/icons.go` (`fonttools` via `uvx --from "fonttools[woff]"`).
