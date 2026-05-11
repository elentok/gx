# Design System Spec

## Purpose

Turn the current `gx` UI into a coherent system with:

- shared semantic styles
- reusable primitives and components
- clear ownership boundaries
- a staged migration plan that does not require rewriting screens wholesale

This document is intentionally repo-specific. It is not a generic TUI design-system essay.

## Goals

1. Align CLI and TUI presentation where the concepts are the same
2. Reduce duplicated framing, hint, menu, and overlay logic
3. Keep domain-specific rendering custom where it provides real value
4. Follow Bubble Tea/Bubbles conventions for component composition
5. Make future screens easier to build consistently

## Non-Goals

1. Replace all custom rendering with Bubbles
2. Remove screen-specific visual differentiation
3. Create a large framework before improving existing code
4. Enforce pixel-perfect uniformity between CLI and TUI

## Proposed Architecture

### `ui/theme`

Owns:

- semantic colors
- text emphasis styles
- borders and surfaces
- icon registry
- spacing and small shared constants

This should be the single source of truth for:

- status colors
- panel border colors
- selected/highlight colors
- empty-state and hint colors

### `ui/primitives`

Owns stateless view helpers:

- `Button`
- `Badge`
- `Heading`
- `Hint`
- `Keycap`
- `PanelFrame`
- `ModalFrame`
- `Overlay`
- `EmptyState`

These should accept semantic styles or theme roles, not raw ad hoc colors in most cases.

### `ui/components`

Owns small reusable composite UI pieces:

- `Confirm`
- `InputDialog`
- `Menu`
- `Checklist`
- `OutputView`
- `HelpBar`
- `HelpModal`
- `SpinnerLine`

These can be either:

- stateful models
- or render helpers plus an explicit state struct

The key requirement is that they define a stable reusable contract.

### Screen Packages

`ui/status`, `ui/worktrees`, and CLI flows in `cmd/` should own:

- screen layout
- domain logic
- domain-specific row rendering
- state orchestration

They should not own:

- generic menu rendering behavior
- generic modal framing
- generic button/badge patterns
- generic keybinding rendering

## Canonical Component Catalog

## Foundation Tokens

### Colors

Canonical semantic roles:

- `TextDefault`
- `TextMuted`
- `TextInfo`
- `TextSuccess`
- `TextWarning`
- `TextDanger`
- `TextAccent`
- `BorderDefault`
- `BorderActive`
- `BorderSuccess`
- `SurfaceBase`
- `SurfaceRaised`
- `SurfaceAccent`

Guideline:

- status-specific Catppuccin color choices can still back these roles
- callers should refer to semantic names, not direct palette constants

### Typography / Emphasis

Canonical text roles:

- `Title`
- `Subtitle`
- `Body`
- `Muted`
- `Strong`
- `CodeLike`

### Icons

Canonical semantic icon names:

- `Check`
- `Close`
- `Dash`
- `FolderClosed`
- `FolderOpen`
- `FileModified`
- `FileAdded`
- `FileDeleted`
- `FileRenamed`
- `FileSymlink`
- `Branch`
- `Worktree`
- `Ahead`
- `Behind`
- `Search`
- `Warning`
- `Info`

Each icon should define:

- nerd-font variant
- plain fallback

## Primitives

### Button

Current sources:

- [ui/buttons.go](/Users/david/dev/gx/main/ui/buttons.go:1)

Canonical API direction:

- text label
- variant: default / primary / danger / subtle
- state: normal / selected / disabled
- icon optional
- nerd-font caps optional if that remains a desired look

Used by:

- CLI confirmation
- modal confirmations
- future menu actions if needed

### Badge

Current sources:

- [cmd/print.go](/Users/david/dev/gx/main/cmd/print.go:1)

Canonical API direction:

- variant: info / success / warning / danger / neutral
- icon optional
- terminal-safe plain fallback

Used by:

- `stashify`
- future CLI step banners
- possible inline TUI labels

### Heading

Current sources:

- status modal titles
- worktrees sidebar section titles
- bump picker title

Canonical API direction:

- level or semantic role
- optional icon
- optional trailing meta text

### Hint

Current sources:

- modal hints
- footer help strings
- CLI prompt hints

Canonical API direction:

- consistent dim style
- shared separators
- shared key rendering rules

### PanelFrame / ModalFrame

Current sources:

- `ui/components/modal_*`
- `status` titled panels
- `worktrees` bordered panes

Canonical API direction:

- border variant
- title
- right title/meta
- width
- padding
- optional active state

Important:

- `PanelFrame` and `ModalFrame` should be related, not separate unrelated ad hoc renderers

### Overlay

Current sources:

- [ui/status/view_chrome.go](/Users/david/dev/gx/main/ui/status/view_chrome.go:44)
- [ui/worktrees/model_view.go](/Users/david/dev/gx/main/ui/worktrees/model_view.go:45)

Canonical API direction:

- place modal centered on background
- no screen-specific ownership

## Components

### Confirm

Current variants:

- [ui/confirm/confirm.go](/Users/david/dev/gx/main/ui/confirm/confirm.go:1)
- [ui/components/modal_confirm.go](/Users/david/dev/gx/main/ui/components/modal_confirm.go:1)

Canonical version:

- one shared confirm behavior model
- different surfaces allowed:
  - inline CLI confirm
  - modal confirm

The key handling and semantic states should be shared.

### Menu

Current variants:

- [ui/menu/menu.go](/Users/david/dev/gx/main/ui/menu/menu.go:1)
- [ui/components/modal_menu.go](/Users/david/dev/gx/main/ui/components/modal_menu.go:1)
- [cmd/bump.go](/Users/david/dev/gx/main/cmd/bump.go:100)

Canonical version:

- one menu model/state contract
- multiple render surfaces if necessary

Variants supported:

- inline menu
- modal menu
- picker menu

### InputDialog

Current variants:

- modal input rendering in `ui/components`
- screen-local text input flows in `worktrees`

Canonical version:

- shared framed input dialog around `bubbles/textinput`

### Checklist

Current source:

- [ui/components/checklist.go](/Users/david/dev/gx/main/ui/components/checklist.go:1)

Canonical version:

- keep custom
- adopt theme tokens
- formalize cursor/selection/checked styling

### OutputView

Current variants:

- modal output in `ui/components`
- status/worktrees output screens

Canonical version:

- shared output view shell
- screen-specific content source remains local

### HelpBar / HelpModal

Current variants:

- custom `status` help/footer
- `bubbles/help` in `worktrees`

Canonical version:

- use `key.Binding` as the canonical binding type everywhere
- prefer generated short help for footers/status bars
- prefer structured full help for modals/expanded help

This is the most important interaction-level standardization.

### SpinnerLine

Current variants:

- [cmd/spinner.go](/Users/david/dev/gx/main/cmd/spinner.go:1)
- worktrees sidebar/status spinner usage

Canonical version:

- spinner + label treated as one semantic component

## Screen-Specific Rules

### Status

Status should keep ownership of:

- diff rendering
- status tree rendering
- staged/unstaged distinction
- selection flash/highlight behavior

Status should stop owning:

- generic panel frame logic
- generic help metadata format
- generic modal frame logic
- generic icon semantics

### Worktrees

Worktrees should keep ownership of:

- table data model
- sidebar content composition
- worktree-specific domain messaging

Worktrees should stop owning:

- screen-local copies of generic overlay and border frame behavior

### CLI

CLI should keep ownership of:

- control flow
- sequencing of commands and prompts

CLI should reuse:

- badges
- confirm interaction semantics
- menu component semantics
- success/error tone and styles

## Current Variants to Canonicalize

### Buttons

Current:

- single `RenderButton`

Canonical:

- keep as the seed of a button primitive, but move to semantic variants

### Badges

Current:

- only `stashify` badge

Canonical:

- promote to a general-purpose badge primitive

### Menus

Current:

- three implementations

Canonical:

- one shared menu system

### Help

Current:

- one structured system, one manual system

Canonical:

- one structured keybinding/help model

### Icon sets

Current:

- screen-local registries

Canonical:

- one semantic icon registry with grouped domains if needed

### Theme

Current:

- shared ANSI-style palette plus separate status palette

Canonical:

- one semantic theme layer that can still internally use the current colors

## Migration Plan

## Phase 1: Foundation Alignment

1. Create semantic theme tokens
2. Create shared overlay helper
3. Create shared frame primitives
4. Document icon semantics and centralize icon lookup

Outcome:

- no user-visible redesign required yet
- immediate reduction of duplication

## Phase 2: Interaction Standardization

1. Make `key.Binding` the canonical keybinding type
2. Introduce shared help rendering conventions
3. Unify menu implementations
4. Unify confirm implementations

Outcome:

- help and prompts become structurally consistent

## Phase 3: Screen Migration

1. Migrate `worktrees` to shared frame/overlay primitives
2. Migrate `status` frame/footer/help usage
3. Migrate CLI flows to shared badges/menu/confirm semantics

Outcome:

- screens still look distinct, but feel like one product

## Phase 4: Refinement

1. Standardize empty states and copy tone
2. Standardize heading hierarchy
3. Add visual examples or snapshot-style tests for primitives/components

## Proposed First Implementation Tickets

The first concrete work items should be:

1. Extract `ui/overlay.go` from duplicated `overlayModal` / `placeOverlay` logic
2. Extract a shared framed container primitive used by both `ui/components` modals and TUI panels
3. Introduce `ui/theme` semantic roles and move `ui/styles.go` toward them
4. Convert `status` help metadata to `key.Binding` and define a shared help renderer
5. Replace the bump picker with the shared menu component

This sequence gives the highest consolidation value without rewriting the whole UI.

## Decision Summary

The design system should be:

- semantic rather than palette-driven
- layered rather than a single helper file
- Bubble Tea/Bubbles-aligned in structure
- conservative about replacing domain-specific rendering
- incremental in migration

The system should unify:

- theme
- frame/overlay scaffolding
- help/keybinding metadata
- menus
- confirms
- badges/icons/hints

It should not force generic abstractions onto custom diff and tree rendering that are already screen-specific and valuable.
