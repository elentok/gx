# Staging UI brainstorm

## Prompt 1

The current status UI still follows the original two-section staging model:

- file list on the left
- unstaged diff on top
- staged diff on bottom

That works, but it creates a few UX problems:

1. after staging, the hunk disappears from the current view and "jumps" to the other section
2. the second section permanently consumes half of the available space
3. navigation is more complex than it needs to be because the user has to think about status vs
   diff, and then staged vs unstaged inside diff
4. it is easy to lose track of which section is active

---

### What other Git UIs tend to do

There are a few common patterns:

- `GitHub Desktop` uses a single diff surface with per-file or per-line inclusion in the commit
  rather than a separate always-visible staged pane.
- `Sublime Merge` also centers the workflow around one main diff surface, with staging done from the
  current hunk or selection.
- `GitKraken` groups files into staged and unstaged lists, but the detailed diff view is still a
  single working area.
- `git-cola` stages the current selection or hunk from one diff editor.
- `Magit` leans heavily on collapsible sections rather than equal-size split panes.
- `lazygit` is the closest precedent to the current layout, but even there the split is only one
  possible presentation, not the only model.

The broad trend is that most tools avoid showing two equally important diff panes at the same time.
They usually make one area primary and treat the other state as metadata, a collapsible section, or
an alternate mode.

### UX directions

#### Option 1: focused section + collapsed strip

Show only one diff section at full height. The other section is reduced to a compact strip with a
title, item count, and maybe the first hunk header.

Example:

- `Unstaged` is expanded and scrollable
- `Staged (3 hunks)` is shown as a 1-3 line strip below it
- `Tab` swaps which section is expanded

Why it helps:

- fixes the wasted half-screen problem
- makes the current working area obvious
- preserves the existing staged/unstaged mental model
- requires much less code churn than a full redesign

To make staging feel less jumpy:

- leave a temporary ghost row in the source pane saying `Moved to Staged`
- flash the destination strip or title in green
- only switch focus if the source section becomes empty

This is the safest and strongest incremental improvement.

#### Option 2: single scroll surface with collapsible staged/unstaged groups

Render one continuous diff pane with section headers:

- `Unstaged`
- `Staged`

Only one group is expanded by default. The other can be collapsed or partially expanded.

Why it helps:

- the user stays inside one scrollable surface
- staged/unstaged still exist, but they feel like sections rather than separate panes
- a staged hunk still "moves", but it remains inside the same overall page, which improves visual
  continuity

This is conceptually cleaner than the current layout, and closer to the way Magit presents related
information.

#### Option 3: single diff view with staged state as an attribute

Show one file diff only. Each hunk or line has a state marker:

- unstaged
- staged
- mixed

Pressing `space` toggles the state of the active hunk or selection in place.

Why it helps:

- no jumping between panes at all
- the user focuses on the file, not on the index/worktree split
- this matches the direction many GUI clients take

The tradeoff is implementation complexity. Git fundamentally stores staged and unstaged changes in
different places, so presenting them as a single coherent diff is much harder than the other two
options.

### Recommendation

I would implement `focused section + collapsed strip` first.

Why:

- it directly solves all three core complaints
- it fits the current code structure
- it keeps the existing staged/unstaged model intact
- it can reuse the current section coloring and flash behavior

Suggested behavior:

- default to `Unstaged` expanded
- render the inactive section as a compact strip with count and color
- keep `Tab` as the section switch
- keep the cursor in the current section after stage/unstage unless that section becomes empty
- add a short ghost/flash transition so movement is visible

### Wide-screen variation

On wide terminals, consider optionally presenting the two states side-by-side:

- `Unstaged` on the left
- `Staged` on the right

This uses horizontal space better than a top/bottom split and makes comparisons easier, but it
still should not be the default if both panes are given equal weight all the time. The key idea is
still that one section should feel primary.

## Ranking

1. focused section + collapsed strip
2. single scroll surface with collapsible groups
3. single diff view with staged-state overlay

## Conclusion

The current split-pane model is understandable, but it over-exposes Git's internal staged/unstaged
separation in the main interaction surface. Most successful Git UIs make one working area primary
and treat the other state as secondary, collapsible, or implicit.

For `gx status`, the best next step is to keep the staged/unstaged distinction, but stop giving both
states equal permanent space. A focused primary pane with a collapsed secondary strip is the best
balance between better UX and implementation cost.

---

## Prompt 2

Let's go with wit the `focused section + collapsed strip`:

1. Unstaged collapses to the top
2. Staged collapses to the bottom
3. Both sections are visible (one collapsed, one expanded), even when one of them is empty

Open questions:

1. Currently Tab switches between all panes, but it's confusing, I'm not sure what would be better:
   1. h/l moves between file tree and diff and tab switches between staged/unstaged
   2. tab moves between file tree and diff and s/u switches to staged/unstaged
   3. some other variation
2. I want clearer frame color difference between staged/unstaged, what do you suggest?

---

### Recommendation

This layout is the right next step:

- `Unstaged` collapses upward into a strip at the top
- `Staged` collapses downward into a strip at the bottom
- the active section expands in the middle
- both strips remain visible even when one section is empty

That keeps the layout stable and preserves orientation. If one section is empty, its strip should
still show a label such as `Unstaged (empty)` or `Staged (empty)`.

### Keybinding recommendation

I would choose:

- `h` / `l` move between file tree and diff
- `Tab` switches between `Unstaged` and `Staged`

Why:

- moving between file tree and diff is a major pane transition, so it should use directional keys
- switching between staged and unstaged is a local mode change inside the diff area, so `Tab` fits
  naturally there
- `h` / `l` already match the spatial model of left pane vs right pane
- introducing `s` / `u` for section switching would make the command set less discoverable and more
  arbitrary

Suggested navigation model:

- file tree focus:
  - `j` / `k` move between files
  - `l` or `enter` moves into diff
- diff focus:
  - `h` returns to file tree
  - `Tab` swaps the expanded section
  - `j` / `k` move within the active section
  - `space` stages or unstages the active hunk, line, or selection

This keeps one clean rule:

- directional keys move between major panes
- `Tab` switches subviews inside the diff pane

### Frame color recommendation

The clearest approach is to give each diff section a stable identity color:

- `Unstaged` uses `orange`
- `Staged` uses `green`

Then use intensity, not hue, to show whether the section is active or collapsed.

Concretely:

- expanded `Unstaged`: orange border, orange title, stronger left marker
- collapsed `Unstaged`: dim orange title, subtle orange-tinted border
- expanded `Staged`: green border, green title, stronger left marker
- collapsed `Staged`: dim green title, subtle green-tinted border

The important rule is:

- hue communicates section identity
- intensity communicates active vs inactive

That is better than using one hue for active focus and another hue for staged state, because the
user otherwise has to decode two meanings from the same border.

### Additional color notes

The file tree should stop using `orange` for active focus. Once `orange` becomes the stable
`Unstaged` identity color, reusing it for the file tree would blur the distinction between
navigation focus and worktree state.

So the visual hierarchy should be:

- file tree: `blue` when focused, `subtle` when unfocused
- unstaged diff: orange family
- staged diff: green family

This gives each area one stable meaning:

- `blue` = navigation focus / neutral app chrome
- `orange` = worktree / unstaged changes
- `green` = index / staged changes

### Summary

The best interaction split is:

- `h` / `l` for file tree vs diff
- `Tab` for unstaged vs staged

The best frame-color split is:

- blue for focused file tree
- orange for `Unstaged`
- green for `Staged`
- stronger saturation/boldness for the expanded section
- dimmer variants for the collapsed strips
