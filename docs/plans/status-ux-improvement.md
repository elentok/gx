# Focused Staging UI With Filetree Terminology Cleanup

## Summary

Implement the new `gx status` staging layout as a fixed three-region diff surface: a top
`Unstaged` strip, a middle expanded active section, and a bottom collapsed `Staged` strip. At the
same time, normalize `ui/status` terminology so the left pane is consistently called `filetree` in
code, comments, and user-facing text where it refers to the file list pane.

The final interaction model is:

- `h` / `l` move between filetree and diff
- `Tab` switches `Unstaged` vs `Staged` inside the diff
- filetree focus is `blue`
- `Unstaged` identity is `orange`
- `Staged` identity is `green`

## Key Changes

### 1. Terminology cleanup first

- Sweep `ui/status` and rename pane-level `status` / `stage` terminology to `filetree` where it
  refers to the left file list pane.
- Include private names, not just user-facing copy: examples include helpers like
  `statusPaneTitle`, focus/state names like `focusStatus`, selectors like `selectedStatusEntry`,
  and related comments.
- Keep existing `status` terminology only where it refers to the overall command/page (`gx status`)
  or domain concepts that are truly Git status data rather than the pane widget.
- Update tests to follow the new terminology so new UI work is built on the cleaned naming model.

### 2. Diff layout and rendering

- Replace the current two-equal-sections diff rendering with a stable three-region vertical stack.
- Always render:
  - top collapsed `Unstaged` strip
  - middle expanded active diff section
  - bottom collapsed `Staged` strip
- Keep both strips visible even when one side is empty; empty strips render a stable label such as
  `Unstaged (empty)` / `Staged (empty)`.
- The active section expands into the middle region; the inactive section stays collapsed in its
  fixed edge position.
- Fullscreen diff mode should preserve the same three-region model rather than falling back to the
  old single/two-pane logic.
- Viewport sizing and mouse hit-testing must target the correct region based on this new layout,
  not the current 50/50 split assumptions.

### 3. Focus, section switching, and styling

- Separate pane focus styling from diff section identity:
  - focused filetree: `blue`
  - unfocused filetree: `subtle`
  - `Unstaged`: orange family
  - `Staged`: green family
- Use hue for section identity and intensity for expanded vs collapsed state.
- Expanded diff section gets the strongest border/title/marker treatment in its own hue.
- Collapsed strips keep the same hue in a dimmer/tinted treatment.
- Do not reuse `orange` as the generic active-pane color for the filetree once `Unstaged` adopts
  it as identity.
- Preserve existing moved-target flash behavior, but scope it to the new layout so the destination
  strip/section still communicates where the hunk moved.

### 4. Navigation and runtime behavior

- Change keyboard behavior so:
  - from filetree focus, `l` and `enter` move into diff
  - from diff focus, `h` returns to filetree
  - `Tab` switches between `Unstaged` and `Staged` only when diff is focused
- Remove frame-cycling behavior from `Tab`; it should no longer move between filetree and diff.
- Keep `j/k`, paging, visual mode, hunk/line toggles, staging, discard, search, and adjacent-file
  navigation scoped to the currently focused pane/active diff section as they are today.
- Preserve section choice across reload/apply operations unless the chosen section becomes
  impossible to show meaningful content in the middle pane.
- Even when a section is empty, it still exists structurally as a strip; section-switch behavior
  should remain stable and not collapse the overall layout back to a single-section mode.

## Public/Internal Interface Changes

- Rename internal focus/state/type/helper names in `ui/status` from `status`-pane terminology to
  `filetree` terminology.
- Keep external command naming as `status`; this is an internal/UI terminology cleanup, not a
  command rename.
- No intentional user-facing keymap expansion beyond the revised meanings of `h`, `l`, and `Tab`.

## Test Plan

- Terminology refactor safety:
  - existing status model tests compile and pass after the rename sweep
  - no lingering pane-level `status` / `stage` terminology remains in `ui/status` where it
    actually means filetree
- Focus/navigation:
  - `l` moves filetree focus to diff
  - `h` moves diff focus back to filetree
  - `Tab` switches active diff section without changing pane focus
  - `Tab` behavior remains stable when one diff section is empty
- Rendering/layout:
  - both strips render even when `Unstaged` or `Staged` has no diff content
  - active section occupies the middle region
  - collapsed strips stay pinned to top/bottom
  - fullscreen uses the same structural layout
- Styling:
  - focused filetree uses `blue`
  - `Unstaged` uses orange styling
  - `Staged` uses green styling
  - collapsed vs expanded states differ by intensity, not hue swap
- Behavior after stage/unstage:
  - staging from `Unstaged` keeps the layout stable and preserves section choice unless the source
    becomes unusable
  - unstaging from `Staged` behaves symmetrically
  - mouse wheel / mouse targeting route scroll events to the correct strip or expanded section

## Assumptions

- The file list pane title can remain user-facing `Status` for now unless a later UX pass
  explicitly wants it renamed; this plan only requires `filetree` terminology where the
  code/comment/UI text is describing the pane role, not the command/page.
- Empty sections remain switchable via `Tab`, because the layout treats them as persistent
  structural regions.
- No new keybindings are introduced for section switching beyond `Tab`; `s` and `u` are not added.
