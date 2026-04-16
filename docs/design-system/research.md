# Design System Research

## Goal

Analyze the current `gx` UI surface and research how Bubble Tea and Bubbles applications typically structure reusable components and design systems, so we can define a coherent system for both the TUI and CLI layers.

## Scope Reviewed

Repository areas reviewed:

- `ui/`
- `ui/components/`
- `ui/status/`
- `ui/worktrees/`
- `cmd/`

External sources reviewed:

- Bubble Tea docs: <https://pkg.go.dev/github.com/charmbracelet/bubbletea/v2>
- Bubbles docs: <https://pkg.go.dev/github.com/charmbracelet/bubbles>
- Bubbles `help`: <https://pkg.go.dev/github.com/charmbracelet/bubbles/v2/help>
- Bubbles `key`: <https://pkg.go.dev/github.com/charmbracelet/bubbles/v2/key>
- Bubbles `list`: <https://pkg.go.dev/github.com/charmbracelet/bubbles/v2/list>
- Bubbles `textinput`: <https://pkg.go.dev/github.com/charmbracelet/bubbles/v2/textinput>
- Lip Gloss repo/docs: <https://github.com/charmbracelet/lipgloss>

Current dependency versions in this repo:

- `charm.land/bubbles/v2 v2.0.0`
- `charm.land/bubbletea/v2 v2.0.2`
- `charm.land/lipgloss/v2 v2.0.2`

## Existing UI Surface

The repo already has the beginnings of a design system, but it is split across three layers:

1. Shared primitives in `ui/`
2. Reusable TUI components in `ui/components/`
3. Screen-local styling systems inside `ui/status`, `ui/worktrees`, and some `cmd` flows

### Shared Foundation

Files:

- [ui/styles.go](/Users/david/dev/gx/main/ui/styles.go:1)
- [ui/buttons.go](/Users/david/dev/gx/main/ui/buttons.go:1)
- [ui/command_output.go](/Users/david/dev/gx/main/ui/command_output.go:1)

What exists:

- Basic shared colors: green/yellow/cyan/magenta/red/gray/border
- Shared semantic-ish status styles: synced/ahead/behind/diverged/unknown
- Basic typography helpers: bold, dim
- A single `RenderButton` helper with optional nerd-font pill caps
- Shared command-output recording/logging helpers

Assessment:

- This is a real foundation, but it is too small and too low-level to govern the rest of the UI
- The styling vocabulary is not yet semantic enough to support multiple screens consistently

### Reusable TUI Components

Files:

- [ui/components/modal_confirm.go](/Users/david/dev/gx/main/ui/components/modal_confirm.go:1)
- [ui/components/modal_input.go](/Users/david/dev/gx/main/ui/components/modal_input.go:1)
- [ui/components/modal_menu.go](/Users/david/dev/gx/main/ui/components/modal_menu.go:1)
- [ui/components/modal_output.go](/Users/david/dev/gx/main/ui/components/modal_output.go:1)
- [ui/components/checklist.go](/Users/david/dev/gx/main/ui/components/checklist.go:1)

What exists:

- Modal confirm
- Modal input
- Modal menu
- Modal output
- Interactive checklist

Assessment:

- This is the clearest starting point for a design system
- These components already establish repeated structure: rounded border, title, body, hint
- However, each component still takes raw colors directly instead of consuming a shared theme or style contract

### Status UI

Representative files:

- [ui/status/model_state.go](/Users/david/dev/gx/main/ui/status/model_state.go:1)
- [ui/status/view_main.go](/Users/david/dev/gx/main/ui/status/view_main.go:1)
- [ui/status/view_chrome.go](/Users/david/dev/gx/main/ui/status/view_chrome.go:1)
- [ui/status/view_panes.go](/Users/david/dev/gx/main/ui/status/view_panes.go:1)
- [ui/status/view_footer_help.go](/Users/david/dev/gx/main/ui/status/view_footer_help.go:1)
- [ui/status/search.go](/Users/david/dev/gx/main/ui/status/search.go:1)

What exists:

- Full screen TUI with split panes and overlays
- A complete local palette (`catBase0`, `catText`, `catBlue`, etc.)
- Local panel rendering with titled borders
- Local help/footer rendering
- Local icon registry for status entries
- Local search highlighting

Assessment:

- `status` already behaves like a self-contained design system
- It is richer than the shared `ui/` foundation
- It also duplicates functionality that exists elsewhere, especially panel framing, overlays, and hints

### Worktrees UI

Representative files:

- [ui/worktrees/model_state.go](/Users/david/dev/gx/main/ui/worktrees/model_state.go:1)
- [ui/worktrees/model_view.go](/Users/david/dev/gx/main/ui/worktrees/model_view.go:1)
- [ui/worktrees/model_layout.go](/Users/david/dev/gx/main/ui/worktrees/model_layout.go:1)
- [ui/worktrees/table.go](/Users/david/dev/gx/main/ui/worktrees/table.go:1)
- [ui/worktrees/sidebar.go](/Users/david/dev/gx/main/ui/worktrees/sidebar.go:1)
- [ui/worktrees/icons.go](/Users/david/dev/gx/main/ui/worktrees/icons.go:1)
- [ui/worktrees/keys.go](/Users/david/dev/gx/main/ui/worktrees/keys.go:1)

What exists:

- Full screen TUI with table + sidebar
- Uses `bubbles/help`, `bubbles/key`, `bubbles/table`, `viewport`, `spinner`, `textinput`
- Own icon registry
- Own search highlighting style
- Own panel framing in main layout

Assessment:

- `worktrees` is closer to “standard Bubble Tea composition” than `status`
- It already treats key bindings and help as reusable structured data
- It still duplicates panel/overlay framing and some status styling patterns

### CLI UI

Representative files:

- [cmd/print.go](/Users/david/dev/gx/main/cmd/print.go:1)
- [cmd/bump.go](/Users/david/dev/gx/main/cmd/bump.go:1)
- [ui/confirm/confirm.go](/Users/david/dev/gx/main/ui/confirm/confirm.go:1)
- [ui/menu/menu.go](/Users/david/dev/gx/main/ui/menu/menu.go:1)

What exists:

- Styled badges for `stashify`
- Success/error terminal output helpers
- Inline confirmation UI using shared buttons
- Standalone interactive menu
- Custom bump picker model with its own styles

Assessment:

- The CLI layer is not visually disconnected from the TUIs, but it is implemented separately
- Several concepts overlap strongly with TUI components: menus, badges, status messages, prompts

## Current Components and Patterns

### Components Already Present

- Buttons
- Badges
- Confirm prompts
- Menus
- Checklists
- Output panels
- Input modals
- Titled bordered panels
- Footer/help bars
- Status messages
- Spinners
- Search highlights
- Icon registries

### Variations of the Same Idea

#### Menu

Current variants:

- [ui/menu/menu.go](/Users/david/dev/gx/main/ui/menu/menu.go:1)
- [ui/components/modal_menu.go](/Users/david/dev/gx/main/ui/components/modal_menu.go:1)
- [cmd/bump.go](/Users/david/dev/gx/main/cmd/bump.go:100)

Observation:

- Three different menu implementations already exist
- They vary mostly by surface and styling, not by domain semantics

#### Modal / Framed Panel

Current variants:

- `ui/components/modal_*`
- [ui/status/view_chrome.go](/Users/david/dev/gx/main/ui/status/view_chrome.go:1)
- [ui/worktrees/model_view.go](/Users/david/dev/gx/main/ui/worktrees/model_view.go:1)

Observation:

- Borders, title treatment, hints, centering, and overlaying are repeated
- There is enough commonality for a shared modal/panel/frame system

#### Help / Keybinding Presentation

Current variants:

- Manual footer/help strings in `status`
- `bubbles/help` + `key.Binding` in `worktrees`
- Manual hint strings in modals and CLI flows

Observation:

- The repo has one structured keybinding system and one string-built system
- This should converge on a single model

#### Icons

Current variants:

- [ui/worktrees/icons.go](/Users/david/dev/gx/main/ui/worktrees/icons.go:1)
- [ui/status/view_chrome.go](/Users/david/dev/gx/main/ui/status/view_chrome.go:155)

Observation:

- Icons are grouped by screen, not by semantic meaning
- That makes consistency harder and encourages drift

#### Status / Semantic Coloring

Current variants:

- Shared status colors in [ui/styles.go](/Users/david/dev/gx/main/ui/styles.go:1)
- Status-specific palette in [ui/status/model_state.go](/Users/david/dev/gx/main/ui/status/model_state.go:168)
- One-off colors in [ui/worktrees/table.go](/Users/david/dev/gx/main/ui/worktrees/table.go:14) and [cmd/bump.go](/Users/david/dev/gx/main/cmd/bump.go:159)

Observation:

- The repo is mixing semantic colors and hard-coded screen-local palettes
- This is the clearest sign that a design system is needed

## Main Inconsistencies

### 1. Two Competing Style Systems

The biggest inconsistency is architectural:

- `ui/styles.go` defines a small shared ANSI-style palette
- `ui/status` defines a richer Catppuccin-ish local palette

Result:

- Shared components cannot naturally consume status styles
- New screens have no obvious guidance on which style system to use

### 2. Repeated Framing and Overlay Logic

Overlay centering and modal placement are duplicated between:

- [ui/status/view_chrome.go](/Users/david/dev/gx/main/ui/status/view_chrome.go:44)
- [ui/worktrees/model_view.go](/Users/david/dev/gx/main/ui/worktrees/model_view.go:45)

Bordered frame rendering is also spread across:

- `ui/components/modal_*`
- `status` panel rendering
- `worktrees` main table/sidebar frames

### 3. Keybinding and Help Inconsistency

`worktrees` uses:

- `bubbles/help`
- `bubbles/key`
- structured `ShortHelp()` / `FullHelp()`

`status` uses:

- manual footer strings
- hand-authored help modal text

This is one of the highest-value alignment opportunities.

### 4. Menu Fragmentation

There are already enough menus to justify a single menu system:

- CLI interactive menu
- Modal menu
- Bump picker

### 5. Screen-Local Icon Systems

Each major TUI has its own icon vocabulary. This is practical short-term but weak as a system because:

- similar ideas get different icon choices
- plain-text fallbacks are not centralized
- icon semantics are not documented

### 6. CLI and TUI Language Drift

The app already uses:

- badges
- success/error labels
- hints
- prompts
- status lines

But those are not defined as one coherent messaging system across CLI and TUI.

## What Bubble Tea and Bubbles Suggest

## Bubble Tea Patterns

Bubble Tea’s core model is explicit:

- `Init`
- `Update`
- `View`

The most important consequence for a design system is that reusable UI pieces are often stateful models, not just render helpers.

Good fit for stateful model components:

- menus
- confirm dialogs
- checklists
- text inputs
- spinners
- help bars

Good fit for stateless render primitives:

- badges
- buttons
- titled frames
- keycap formatting
- hints
- section headings

## Bubbles Patterns

Bubbles reinforces a few design-system lessons:

- styles are first-class customization points
- keymaps are first-class customization points
- components are designed to be composed into app models
- help generation should be driven from structured bindings rather than repeated prose

Especially relevant packages here:

- `help`
- `key`
- `textinput`
- `viewport`
- `spinner`
- `table`
- `list`

### Practical Takeaways from Bubbles

#### Use `key.Binding` as the canonical keybinding type

This enables:

- generated short/full help
- consistent labels
- centralized remapping later if desired

#### Treat styles as a public API

Bubbles components commonly expose styles or style-like configuration. For `gx`, that implies shared components should accept a style contract or theme, not raw ad hoc colors everywhere.

#### Keep domain-specific rendering custom where needed

Bubbles gives useful generic components, but not everything should be forced into them.

Good candidates to stay custom:

- status diff rendering
- status tree rendering
- worktrees sidebar content

Good candidates to standardize using Bubbles conventions:

- help
- inputs
- menus
- checklists
- modal interaction flows

## Recommended Design-System Shape

The right model for this repo is not a giant global helper package. It is a layered system.

### 1. Theme Layer

Suggested responsibility:

- semantic color tokens
- border tokens
- text emphasis tokens
- spacing constants
- icon registry

Examples of useful semantic tokens:

- `fgDefault`
- `fgMuted`
- `fgInfo`
- `fgSuccess`
- `fgWarning`
- `fgDanger`
- `borderDefault`
- `borderActive`
- `borderSuccess`
- `surfaceBase`
- `surfaceRaised`

### 2. Primitive Layer

Suggested responsibility:

- button
- badge
- hint
- heading
- panel frame
- modal frame
- overlay placement
- keycap rendering
- empty-state rendering

These should mostly be stateless render helpers.

### 3. Component Layer

Suggested responsibility:

- confirm
- input dialog
- menu
- checklist
- output viewer
- spinner line
- help bar / help modal

These should be small composable stateful models or structured render components.

### 4. Screen Composition Layer

Suggested responsibility:

- status screen
- worktrees screen
- bump picker
- CLI confirmation/menu flows

These should consume primitives/components and own only domain-specific behavior.

## High-Value Consolidation Targets

### Immediate / High ROI

1. Unify the theme and semantic colors
2. Extract a shared overlay helper
3. Extract a shared panel/modal frame primitive
4. Standardize keybinding metadata around `key.Binding`
5. Merge menu variants into one component family
6. Consolidate icon registries into semantic icon sets

### Medium ROI

1. Standardize empty states like `clean`, `none`, `no output`, `no selection`
2. Standardize title rows and section headings
3. Standardize CLI success/error/badge presentation with TUI semantics

### Lower Priority

1. Further refine checklist as a more configurable component
2. Evaluate whether some flows should adopt `bubbles/list`
3. Build a small visual/test harness for components

## Things Not To Over-Consolidate

Some rendering should remain domain-specific:

- status diff body rendering
- status tree row structure
- worktrees sidebar content layout
- staged vs unstaged emphasis

The design system should unify scaffolding and semantics, not erase useful screen-specific behavior.

## Overall Conclusion

The repo has enough UI to justify a design system now.

The main issue is not absence of components. It is that the same component ideas already exist in several slightly different forms:

- multiple menu systems
- multiple frame systems
- multiple icon systems
- multiple help systems
- multiple color systems

The best next step is to define a repo-specific design-system spec with:

- a component catalog
- canonical ownership boundaries
- proposed canonical versions of current duplicated pieces
- migration order

That is the natural next pass after this research.
