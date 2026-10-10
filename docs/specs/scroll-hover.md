# Scroll hover: route the mouse wheel by cursor position, not by prior click

## Problem Statement

gx is a mouse-aware TUI: most tabs already respond to clicks, and several panes already scroll with
the wheel. But wheel routing is inconsistent across the app. In most split-pane tabs (Tickets tab,
Tickets' Queue sub-tab, Log tab), the wheel scrolls whichever pane last received a click — so
hovering the preview pane and scrolling does nothing (or scrolls the wrong pane) until you first
click into it. Two panes (Status tab, and the commit diff/filetree component) already got this
right: the wheel scrolls whatever the cursor is currently over, no click required. That correct
behavior was implemented twice, independently, with no shared code between the two.

Investigating this surfaced several real bugs beyond the inconsistency, in tabs and modals the
initial framing hadn't accounted for:

- The Stash tab has no wheel routing of its own at all. Every wheel event is unconditionally
  forwarded, untranslated, to the commit-detail pane; the stash list itself cannot be scrolled by
  wheel under any circumstances, and because the detail pane's own diff/filetree hit-test receives
  coordinates outside its own local space, hovering the list can currently land on the detail
  pane's filetree region instead of doing nothing.
- The Worktrees tab has the identical bug: it has a table plus a details preview pane, and every
  wheel event reaches only the preview — the table can never be scrolled by wheel.
- The PRs tab has no wheel handling at all, in either direction — turning the wheel over it does
  nothing, ever.
- The help modal never reliably receives wheel events. This isn't one uniform defect: some
  embedding sites swallow the event outright with an early-return guard; others let it fall through
  to scroll whatever pane sits behind the modal instead; a couple have no guard at all. And even a
  correctly-forwarded wheel event would do nothing, because the modal's own update logic has no
  mouse-wheel case in the first place.

Two further gaps, unrelated to hover routing but found during the same investigation: the Log tab's
commit-info popup and the PRs tab's comments popup both have no scroll capability at all — long
content is silently clipped, with no way to reach the rest by keyboard or mouse. And three more
viewport-backed modals (the shared output-log viewport used by Status and Log, plus Worktrees' logs
and error modals) can only be scrolled by keyboard today, with wheel input silently discarded.

## Solution

Every pane that can scroll responds to the mouse wheel based on where the cursor is, not on which
pane was last clicked. This applies uniformly across every tab with more than one scrollable region:
Tickets (sidebar + preview), Tickets' Queue sub-tab (tree + preview), Log (commit list, and nested
into the embedded commit-detail pane's own diff vs. filetree), Stash (list, and nested into its own
embedded diff vs. filetree — the same shape as Log, since it embeds the identical component), and
Worktrees (table + details preview). Status and the standalone commit diff/filetree component
already behave this way; they converge onto one shared implementation instead of remaining two
parallel ones. The PRs tab, which has no detail/preview split, gets straightforward wheel support
for its one scrollable region rather than hover routing.

Hover changes only wheel routing. It does not change keyboard focus, click-to-focus, selection, or
add any visual "hovered" highlight — a pane you scroll by hovering is not thereby focused, and a
pane you click still becomes focused exactly as it does today. Hovering a pane with nothing left to
scroll (its content already fully visible) simply does nothing; it does not fall back to scrolling
whichever pane currently has keyboard focus.

The help modal gains mouse-wheel handling for its own content, and every tab that can open it is
fixed to actually route wheel events into it — whether that tab was swallowing them, letting them
fall through to the pane behind the modal, or (for Worktrees and PRs, which build their own wheel
support as part of this work) had no wheel handling to guard in the first place. Five more
previously keyboard-only viewports gain wheel scrolling too: the Log tab's commit-info popup, the
PRs tab's comments popup, the shared output-log viewport (used by Status and Log), and Worktrees'
logs and error modals.

The menu overlays (implement-agent menu, status menu, actions menu, drain menu) and the confirm
dialog are unaffected — none of them clip their content or scroll under the current design; they
simply grow to fit, so hover-scroll has nothing to do there.

## User Stories

1. As a gx user, I want to scroll the Tickets sidebar by hovering it and turning the wheel, so that
   I don't have to click it first just to move within it.
2. As a gx user, I want to scroll the Tickets preview pane by hovering it, so that I can read a long
   ticket body without disturbing which sidebar row is selected.
3. As a gx user, I want the same hover-to-scroll behavior on the Queue sub-tab's tree and preview,
   so that Tickets and Queue feel consistent.
4. As a gx user watching commit history, I want to scroll the Log tab's commit list by hovering it,
   so that browsing history doesn't require a click first.
5. As a gx user reviewing a commit in the Log tab's detail pane, I want to scroll the diff by
   hovering over it, so that I can read a long diff without an extra click.
6. As a gx user reviewing a commit in the Log tab's detail pane, I want to scroll the filetree by
   hovering over it instead, so that switching between reading the diff and browsing changed files
   feels like moving the mouse, not clicking to "enter" each pane.
7. As a gx user browsing stashes, I want to scroll the stash list by hovering it, so that I can move
   through my stashes with the wheel at all — today this is impossible.
8. As a gx user reviewing a stash's contents, I want to scroll its detail pane by hovering it —
   diving into its diff vs. filetree sub-regions the same way Log's does — consistent with every
   other split-pane tab, and I want hovering the stash list to never accidentally scroll the detail
   pane's filetree the way it can today.
9. As a gx user browsing worktrees, I want to scroll the worktree table by hovering it, so that I
   can move through my worktrees with the wheel at all — today this is impossible, the wheel always
   scrolls the details preview instead.
10. As a gx user reviewing a worktree's details preview, I want to scroll it by hovering it,
    consistent with every other split-pane tab.
11. As a gx user browsing pull requests, I want the mouse wheel to scroll the PR list, so that I can
    move through PRs with the wheel at all — today this is impossible.
12. As a gx user, I want hovering and scrolling to never change which pane has keyboard focus or
    which row is selected, so that using the wheel to read something doesn't silently move my
    keyboard cursor somewhere else.
13. As a gx user, I want scrolling over a pane with nothing left to scroll to simply do nothing, so
    that an idle wheel-turn near the edge of a pane never surprises me by scrolling an unrelated,
    currently-focused pane instead.
14. As a gx user with the help overlay open, I want to scroll it with the mouse wheel, so that I can
    read a keybinding reference longer than one screen without needing a keyboard-only escape hatch.
15. As a gx user, I want the help overlay's wheel-scroll to work from every tab I can open it from —
    including Worktrees and PRs, which had no wheel handling of any kind before this — so that the
    behavior is predictable regardless of where I am in the app.
16. As a gx user reading a commit's full message in the Log tab's commit-info popup, I want to
    scroll it (by keyboard or wheel) when the message is longer than the popup, so that I can read
    the whole thing instead of having it silently cut off.
17. As a gx user reading a long PR comment thread, I want to scroll the comments popup (by keyboard
    or wheel) when it's longer than fits, so that I can read the whole thread instead of having it
    silently cut off.
18. As a gx user watching a long-running command's output in the Status or Log tab, I want to scroll
    that output with the mouse wheel, not just the keyboard, consistent with every other scrollable
    surface in the app.
19. As a gx user viewing a worktree's logs or an error detail in Worktrees, I want to scroll that
    modal with the mouse wheel, not just the keyboard.
20. As a gx developer adding a new split-pane tab in the future, I want a single shared hover
    hit-test helper to reach for, so that I don't have to re-derive rect-based cursor routing from
    scratch or accidentally reintroduce the click-gated pattern this work removes.

## Implementation Decisions

- **Shared hover hit-test helper.** A new, pure helper lives alongside the existing shared
  mouse-wheel primitives (wheel direction, scroll-offset clamping) that already exist for exactly
  this kind of cross-pane reuse. Given a cursor position and one or more candidate rects, it
  reports which rect (if any) the cursor falls in. It has no dependency on any specific pane's
  model.
- **Convergence, not duplication.** The two panes that already implement correct rect-based hover
  routing (Status tab's filetree/diff split; the standalone commit diff/filetree component) are
  migrated onto the shared helper first, with no behavior change, so they become the reference
  implementation expressed through shared code rather than two independent inline copies. Every
  other pane's conversion builds on top of this helper rather than re-deriving its own point-in-rect
  logic a third, fourth, or fifth time.
- **Split-view component gets its own hover capability.** The shared list+detail split-view
  component (used by both the Log tab and the Stash tab) gains a hover-routing method built on the
  shared helper, generalized from the component's existing click hit-test rather than written
  fresh: given a cursor position, it reports whether it falls on the list side or the detail side,
  correctly accounting for the component's fullscreen and collapsed display modes (where only one
  side is present to route to) and its seam between panes (which, like any point outside both
  rects, matches neither and no-ops the wheel). This is added to the component itself, decoupled
  from any specific tab wiring it in, since both consuming tabs need identical routing logic.
- **Nested hover for the Log and Stash tabs.** Both tabs' detail panes embed the same commit
  diff/filetree component that exists standalone elsewhere — Stash's detail pane is not, as
  originally assumed, a single flat region; it needs the identical two-level treatment as Log's.
  Hover routing in both is two levels deep: first determine list vs. detail via the split-view
  component's new capability, then — if the cursor is in the detail pane — translate the cursor
  position (by rewriting the message into the embedded component's own local coordinate space, the
  same way each tab's existing click-routing code already does for clicks, rather than adding an
  origin parameter to the embedded component's hover API) so its own (already correct,
  post-convergence) hover routing disambiguates diff vs. filetree.
- **Stash tab bug fix rides along, and is worse than originally scoped.** The Stash tab currently
  has no wheel-routing logic of its own; every wheel event reaches the commit-detail pane
  regardless of cursor position, untranslated — so not only can the stash list never be scrolled by
  wheel, but hovering the list can currently land on the detail pane's filetree sub-region instead
  of doing nothing. Wiring the Stash tab onto the split-view component's new hover capability, with
  the same nested coordinate translation as Log, is both the hover-scroll feature and the fix for
  both symptoms of that bug.
- **Worktrees has the identical missing-list-scroll bug as Stash.** The Worktrees tab has its own
  table-plus-details-preview split with no wheel routing of its own; every wheel event reaches only
  the details preview, so the table can never be scrolled by wheel. It doesn't use the split-view
  component (its layout is hand-rolled), so its fix is a direct rect hit-test off the shared helper,
  not a split-view wiring — but it is the same class of fix as Stash, landing in the same epic.
- **PRs gets basic wheel support, not hover.** The PRs tab has no detail/preview split and no wheel
  handling of any kind — turning the wheel over it does nothing. Since there is only one scrollable
  region, the fix is straightforward wheel support against its existing scroll state (the shared
  wheel-direction/clamp primitives), not hover routing.
- **The help modal's own wheel handling was a separate, prerequisite gap.** The modal's content is a
  single scrollable region — no hover disambiguation needed — but its own update logic has no
  mouse-wheel case at all, independent of whether any embedding site correctly forwards a wheel
  event to it. That gap is fixed on its own, ahead of (and as a dependency of) every other
  help-modal-adjacent fix.
- **Help modal embedding-site fix has three distinct shapes, not one.** The original read was that
  "every site swallows the wheel with an early-return guard" — that's true for only some sites.
  Others let the wheel fall through their guard and scroll whatever pane sits behind the modal
  instead; a couple have no guard on the wheel path at all. The fix differs by shape: a swallowing
  site needs its early return changed to a forward; a falls-through or no-guard site needs a new
  branch added ahead of its normal routing. Because this touches on the order of nine otherwise-
  unrelated tabs/packages, and the shapes needed discovering rather than assuming, this piece of
  work is split into a survey step (enumerate every site, its actual shape, and the fix each one
  needs, without changing code) followed by an implementation step that applies the documented fix
  everywhere. The Worktrees and PRs tabs are explicitly excluded from that survey and its
  implementation: since both start this epic with no wheel handling at all, each builds its own
  forward-to-help-modal branch directly as part of adding its own wheel support, rather than
  leaving a gap for the embedding-site fix to (incorrectly) assume doesn't exist yet.
- **Popup and viewport-modal scroll fixes are unrelated to hover.** The Log tab's commit-info popup,
  the PRs tab's comments popup, the shared output-log viewport (used by Status and Log), and the
  Worktrees logs and error modals are each a single scrollable region with no sibling pane to
  disambiguate against — their fix is simply giving each a working mouse-wheel case (a new
  viewport for the two popups, which have none today; a wheel case alongside the existing
  keyboard-scroll handling for the three viewports, which already have one). The PRs comments popup
  fix is sequenced after the PRs tab's own basic wheel-scroll ticket, since both touch the same
  tab's update path. The three viewport-modal fixes are sequenced after both the Worktrees hover fix
  and the help-modal embedding-site fix, for the same reason — the output-log viewport's embedding
  sites (Status, Log) are exactly the packages the help-modal fix also touches.
- **No focus/selection side effects.** Wheel routing decided by cursor position is deliberately kept
  separate from keyboard focus and click-to-focus, which are unchanged by this work. A hover-scroll
  event must never move keyboard focus, change which row is selected, or trigger any click-only
  behavior (e.g. a checkbox toggle in the Tickets sidebar).
- **No visual hover state.** This spec adds no highlight, border change, or other visual indication
  of which pane the cursor is over. It is scoped purely to which pane a wheel event's scroll is
  applied to.
- **No-overflow no-ops, it does not fall back to focus.** When the hovered pane has nothing left to
  scroll, the wheel event is simply absorbed. It does not fall through to scrolling whichever pane
  currently holds keyboard focus.
- **Confirmed out of scope, not silently dropped:** the menu overlays (implement-agent, status,
  actions, drain) and the confirm dialog render unconditionally with no clipping or viewport under
  the current design — they cannot overflow, so there is nothing for hover-scroll or basic wheel
  support to do there. If a future change gives any of these a scrollable/clipped form, wheel
  support for it is new work, not something this spec silently covers.

## Testing Decisions

- Tests exercise each pane's public `Update` method with a constructed mouse-wheel message carrying
  specific cursor coordinates, and assert which pane's scroll offset moved — never internal state
  beyond the observable scroll position and the (unchanged) focus/selection. This mirrors the
  existing convention already used for the reference implementations (Status tab's filetree/diff
  split, the commit component's diff/filetree split) and for today's click-gated wheel tests in the
  Tickets tab, its Queue sub-tab, and the Log tab.
- Every hover-scroll test pairs at least two assertions, and each one pins keyboard focus to the
  pane *not* being hovered before scrolling: a wheel event at coordinates inside pane A, with focus
  pinned to pane B, scrolls A and leaves B's offset and focus untouched — and the mirror case. Pinning
  focus to the opposite pane is deliberate: without it, a test can pass by coincidence against
  unchanged click-gated code whenever the pane under test happens to already hold default focus,
  which several of this app's existing panes do. This is what actually proves routing depends on
  cursor position rather than leftover focus state.
- The split-view component's new hover-routing method gets coverage in all three of its display
  modes (normal split, list-only, detail-only), not just the normal split — a pane in fullscreen
  mode must still route the wheel to the one pane that's visible, not no-op it.
- Each converted pane also gets (or keeps) a no-overflow case: a wheel event over a pane with fully
  visible content leaves scroll state unchanged everywhere, rather than falling back to the focused
  pane.
- The shared hover hit-test helper and the split-view component's new hover-routing method each get
  direct unit coverage against synthetic rects and cursor positions, independent of any specific
  tab's model — the same shape as the existing wheel-direction/scroll-clamp helpers' own tests.
- The help modal's own new wheel case gets a direct unit test: scrolling clamps at content
  boundaries the same way its existing keyboard-scroll case does.
- The help-modal embedding-site fix is tested per tab, following each tab's own existing test file
  and conventions (documented explicitly by the survey step before the fix is implemented),
  asserting a wheel event while the modal is open scrolls its content and does not also move the
  scroll state of whatever's behind it. The Worktrees and PRs tabs get the same assertion as part of
  their own wheel-support tickets, since each builds its own forward-to-help-modal handling.
- The commit-info popup and the PR-comments popup each get a test with content taller than the
  popup, asserting both keyboard and wheel scrolling move the visible window, and that a wheel event
  while the popup is open doesn't also move whatever's behind it.
- The three remaining viewport-modal fixes (output-log viewport, Worktrees logs modal, Worktrees
  error modal) each get a wheel-scroll test mirroring their existing keyboard-scroll test.

## Out of Scope

- Any visual indication of hover (highlighted border, dimmed inactive panes, cursor-following
  chrome).
- Changes to keyboard focus, click-to-focus, or selection behavior — these remain exactly as they
  are today.
- The menu overlays (implement-agent, status, actions, drain) and the confirm dialog — confirmed to
  have no scrollable-overflow content under the current design, so there is no wheel-support
  decision to make for them.
- A fallback-to-focused-pane behavior when hovering a non-scrollable pane — deliberately rejected in
  favor of a no-op.
- Any change to how mouse mode itself is enabled at the application level (cell-motion vs. all-motion
  reporting) — wheel events already report cursor position under the app's current mouse mode, so
  no change there is required for this work.

## Further Notes

This spec was produced from a grilling session, targeted codebase research, and a second-opinion
consultation review that caught real drift between the initial research and the actual code — most
notably that Worktrees and Stash both had the same missing-list-scroll bug, that the help modal's
own update logic had no wheel handling at all, and that "every help-embedding site swallows the
wheel" was true for only some sites. The corresponding implementation breakdown is published as the
`scroll-hover` epic in the local ticket tracker (`gx tickets epics`), 14 tickets in dependency
order: a foundational shared-helper ticket; independent Tickets-tab and Queue-tab conversions; a
split-view-component infra ticket (amended with fullscreen/collapsed/seam semantics); Log-tab and
Stash-tab conversions built on it (Stash rewritten to the same nested-hover shape as Log); a
help-modal survey; the help modal's own wheel-handling fix; the help-modal embedding-site fix built
on both; independent Worktrees-hover and PRs-basic-wheel tickets (each building its own
forward-to-help-modal handling); a combined commit-info/PR-comments popup-scroll ticket; a
remaining-viewport-modals ticket (output-log viewport, Worktrees logs/error modals); and a trailing
code-review ticket. This document exists as the durable "why," committed alongside the code it
describes, separate from the tickets that track "what's left."
