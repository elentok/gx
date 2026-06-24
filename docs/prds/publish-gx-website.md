# PRD — Publish gx: README refresh + landing site

## Problem Statement

`gx` is ready to be published, but it isn't presentable to a stranger. The README leads with a list
of ~40 features and two **stale static screenshots** that predate the current tabbed UI, theme, and
split log/stash views — so a first-time visitor can't quickly grasp what `gx` is or see it in motion.
There is also no landing page: nothing at `gx.elentok.com` to point people at. And the tool's most
distinctive capability — fast review of (often AI-generated) code, with a one-keystroke handoff back
to an agent — is buried under a `cm` "comment" binding whose name gives no hint of what it's for.

## Solution

Make `gx` presentable for a public release on two surfaces that share one set of assets:

1. A polished one-pager at **`gx.elentok.com`** that *looks like `gx` itself* (a faux-TUI screen in
   Catppuccin Mocha), leads with a **review-first** hook, tells the AI-code-review origin story, and
   demonstrates the tool with short animated demos.
2. A **refreshed README** that replaces the stale PNGs with the same animated demos and leads with the
   same hook.

Before documenting anything, sharpen the tool's AI-review story in the product itself: reframe the
`cm` "comment" action as **"Ask AI"** and group it with **"Yank for AI"** under a discoverable
`a` = AI prefix, so the names the website and README teach are the final ones.

## User Stories

1. As a developer evaluating git TUIs, I want a landing page that states in one line what `gx` is, so
   that I can decide in seconds whether it fits my workflow.
2. As a first-time visitor, I want to see `gx` in motion (animated demos), so that I understand the
   experience without installing it.
3. As a visitor on the landing page, I want copy-paste install commands for Homebrew and `go install`,
   so that I can try `gx` immediately.
4. As a visitor, I want the page to load fast and feel hand-crafted (not a templated SaaS page), so
   that I trust the tool's craft.
5. As a visitor, I want the landing page to visually resemble the actual TUI, so that the page itself
   previews the product's aesthetic.
6. As a visitor on a phone, I want the page to collapse cleanly to a single column, so that it's
   readable on mobile.
7. As someone reviewing a flood of AI-generated changes, I want to see that `gx` is built for fast
   review and staging, so that I recognize it solves my actual problem.
8. As a reader, I want the page to explain *why* `gx` exists (reviewing AI code across parallel
   worktrees), so that the feature set feels coherent rather than a grab-bag.
9. As a visitor, I want the demos grouped into clear pillars (review & stage, inspect history, manage
   worktrees), so that I can find the capability I care about.
10. As a visitor, I want to see the staging flow (file/hunk/line, side-by-side toggle), so that I
    understand `gx`'s review ergonomics.
11. As a visitor, I want to see the "Ask AI" / "Yank for AI" handoff, so that I see how `gx` fits an
    agent-assisted workflow.
12. As a visitor, I want to see the inline image-diff capability, so that I appreciate a feature most
    git TUIs lack.
13. As a visitor, I want to see worktree management (sync/rebase status, create/switch), so that I
    understand the parallelism layer.
14. As a visitor, I want clear calls to action (Install, GitHub), so that I can act on my interest.
15. As the maintainer, I want the website to live in the same repo as `gx`, so that demos regenerate
    alongside the binary and never drift.
16. As the maintainer, I want one set of demo GIFs reused by both the README and the site, so that I
    update visuals in one place.
17. As the maintainer, I want the demos generated deterministically from a seeded repo and scripted
    tapes, so that I can regenerate them after UI changes without hand-recording.
18. As the maintainer, I want to deploy with a single `npm run deploy`, so that publishing matches my
    existing cryptowl workflow.
19. As the maintainer, I want the site served at `gx.elentok.com` over HTTPS, so that it has a clean
    public address.
20. As a README reader on GitHub or pkg.go.dev, I want current animated demos instead of stale
    screenshots, so that the README reflects the real tool.
21. As a `gx` user reviewing code, I want a single keystroke prefix (`a`) for AI actions, so that the
    handoff to my agent is discoverable and memorable.
22. As a `gx` user, I want `aa` to open an editor with the focused diff anchored as markdown, so that
    I can write an instruction for my agent against exact lines.
23. As a `gx` user, I want `ay` to yank the focused diff in agent-ready markdown, so that I can paste
    context to my agent instantly.
24. As a long-time `gx` user, I want my old `ya` muscle memory to keep working, so that the rename
    doesn't break my flow.
25. As a `gx` user, I want `t` to toggle hunk/line nav-mode, so that the old `a` toggle can be reused
    for the AI prefix without losing the capability.
26. As a `gx` user pressing `?`, I want an "AI" category in the help overlay grouping the AI actions,
    so that the feature is discoverable in-product.
27. As the maintainer, I want the README key tables and CHANGELOG updated for `t`/`aa`/`ay`, so that
    documentation names the final bindings.

## Implementation Decisions

### Positioning & content (website + README)
- **Hook:** review-first ("a git TUI for reviewing changes, fast") — status is the most-used tab.
  Worktrees are framed as the parallelism layer; AI-code-review is a featured narrative section, not
  the literal tagline.
- **Page sections, top → bottom:** (1) Hero — faux `gx` window with tagline + status demo + CTAs
  (Install · GitHub ★); (2) Install in seconds — `brew install --cask gx` / `go install …` with copy
  buttons; (3) "The why — reviewing AI code" origin story; (4) Feature showcase in **3 pillars** —
  *Review & stage* (lead), *Inspect history*, *Manage worktrees*, each a short demo + 3–4 captions,
  with "Review for AI" (`ay` + `aa`) as the marquee moment of pillar 1; (5) Closing CTA; (6) Footer
  ending "by David Elentok."
- **Cut from the landing page** (README only): `gx term`, `doctor`, completions, config schema,
  `stashify`, full keybinding tables.

### Visual identity (website)
- The site renders as a `gx` TUI screen: Catppuccin Mocha palette as CSS custom properties, bordered
  panels, pill badges, sync glyphs, all-monospace via a Nerd Font `@font-face`. Must not read as a
  lazygit.dev clone (used only as a structural reference; tig is the anti-pattern).
- **Logo:** keep the original lime `docs/logo.png` unchanged as the single deliberate off-palette pop;
  pull a touch of lime into one site accent (primary CTA / active tab) so it reads intentional. A
  Catppuccin recolor was prototyped and rejected (it washes out the shaded mascot).

### Tech & location
- **Stack:** Vite + vanilla TypeScript + hand-written CSS. No React/Chakra/Emotion. Optional tiny
  vanilla-JS for a demo/tab switcher.
- **Location & deploy:** `web/` subdir in this repo; deployed standalone to Cloudflare Pages. Manual
  deploy, cryptowl parity: `cd web && npm run deploy` →
  `wrangler pages deploy dist --project-name=gx`. Custom domain `gx.elentok.com` configured once in
  the Cloudflare dashboard. **See ADR 0012.**

### Demos (shared by site + README)
- **VHS `.tape` scripts** are the primary medium (3 tapes: status hero, log, worktrees); tape headers
  set a Nerd Font + Catppuccin Mocha theme so output matches the real TUI.
- **One real screen-recording** for the kitty image-diff feature (VHS can't render the graphics
  protocol).
- A deterministic **seed repo** (`web/demo/seed.sh`) provides the state the demos render against:
  multiple worktrees with induced ahead/behind/diverged states vs a local "remote", taggable history,
  dirty files, a stash, one changed image.
- `make demos` regenerates all GIFs into `docs/`; both the README and `web/` consume the same files
  (single source of truth). Animated GIFs are used in the README (not static stills).

### AI-action keybindings (pre-req; do first)
The underlying actions already exist in `ui/comments` (write anchored markdown, open `$EDITOR`) and
the yank-for-AI path. This work is rebinding + relabeling, not new behavior.
- `ui/diffview`: nav-mode (hunk↔line) toggle moves single `a` → **`t`** (a key can't be both an
  instant action and a chord prefix).
- `ui/status` and `ui/commit`: comment `cm` → **`aa`** ("Ask AI"); yank-for-AI `ya` → **`ay`**, with
  `ya` retained as a **hidden back-compat alias**.
- New **"AI"** help category groups `aa` + `ay` in the `ui/help` overlay (categories are string tags
  on bindings).
- Update README key tables and CHANGELOG for `t` / `aa` / `ay`.

## Testing Decisions

Per the maintainer's decision, automated tests cover **only the keybinding module**; demos, website,
and README are presentation/docs and are verified manually (rendering, visual + responsive review).

- **What makes a good test here:** assert external behavior — *which binding fires which action* — not
  internal dispatch wiring. Press a key sequence into the model's `Update` and assert the resulting
  action/state, mirroring existing tests.
- **Modules tested:** the AI-action keybindings in `ui/status` and `ui/commit`, plus the nav-mode
  rebind in `ui/diffview`.
- **Cases:** `aa` triggers Ask AI; `ay` triggers yank-for-AI; legacy `ya` still triggers yank-for-AI
  (alias); `t` toggles nav-mode; bare `a` no longer toggles nav-mode (and begins an AI chord).
- **Prior art:** `ui/status/model_keys_test.go`, `ui/status/model_test.go` (key-press → state
  assertions), `ui/commit/model_keys_test.go`, and `ui/help/help_test.go` for category grouping.

## Out of Scope

- Restyling or recoloring the logo (original `logo.png` kept as-is).
- Any change to the comment/Ask-AI *behavior* itself (`ui/comments`) beyond the rebind and relabel.
- Git-connected / push-to-deploy on Cloudflare (manual deploy only; revisit later if needed).
- Docs content beyond README + the landing page (no separate docs site).
- Automated tests for the website, demo tapes, or seed script.
- Configurable/remappable AI keybindings (fixed `aa`/`ay`/`t` for now).
- Creating the Cloudflare Pages project and adding the `gx.elentok.com` custom domain — a one-time
  manual dashboard step the maintainer performs.

## Further Notes

- ADR 0012 records the in-repo-website decision and the manual-deploy trade-off.
- `CONTEXT.md` is intentionally untouched — this is presentation work, not a domain-model change.
- Do the keybinding pre-req first so the README and website teach the final names once.
- Plan/checklist companion: `docs/plans/publish-gx-website.md`.
