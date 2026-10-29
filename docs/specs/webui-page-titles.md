# Web UI: a browser tab title per page

Source: the map `gx:webui-page-titles` and its chart grilling. The map has no closed tickets. Built
as the Ralph Loop epic `webui-page-titles-impl`.

## Problem Statement

Every page of the web UI has the same browser tab title, `gx`. It comes from the static `<title>`
in the web UI's HTML file, and no page changes it. The person often keeps several gx tabs open: the
Epics page, a Design they are answering, the Epic it feeds and a Ticket they are watching. All the
tabs read `gx`, so they must click through each one to find the right tab. Browser history, bookmarks
and the tab search show the same useless name.

## Solution

Each page sets its own tab title in one format: `<name> · <Kind> · gx`. The specific part comes
first, because browsers cut long tab titles from the right. The parts are joined by ` · ` (a middle
dot with a space on each side).

- The Epics page reads `Epics · gx`, and the Stats page reads `Stats · gx`. These pages have no
  name part.
- A Ticket page reads `<ticket title> · Ticket · gx`. While the ticket loads, if it is not found,
  or if its title is empty, the name is the ticket's address: `demo:mixed-epic/02 · Ticket · gx`.
- An Epic page reads `<epic slug> · Epic · gx`, for example `mixed-epic · Epic · gx`.
- A Design page reads `<epic slug> · Design · gx`, for example `design-epic · Design · gx`.

The Kind word always shows, so the tab of a Design and the tab of its `-impl` epic differ even when
the slugs look alike. The title has no project prefix and no dev-server marker. Stats filters in the
URL do not change the title. The title follows the page: it changes when the person navigates, and
a Ticket's title updates when a refetch brings a new ticket title.

Every page sets its title through one shared mechanism in the page frame that all pages use. A page
added later must give a title too, or the type check fails.

## User Stories

1. As the person running gx, I want the Epics page tab to read `Epics · gx`, so that I can find the overview tab among many gx tabs.
2. As the person running gx, I want the Stats page tab to read `Stats · gx` whatever range or project filter is in the URL, so that the tab name stays stable while I change filters.
3. As the person running gx, I want a Ticket page tab to read `<ticket title> · Ticket · gx`, so that I can tell which ticket a tab shows.
4. As the person running gx, I want a Ticket page tab to show the ticket's address while the ticket loads, so that the tab never reads only `gx` or goes blank.
5. As the person running gx, I want a Ticket page tab to show the address when the ticket is not found or has an empty title, so that the tab still names what I opened.
6. As the person running gx, I want a Ticket page tab to update when the ticket's title changes on disk, so that the tab matches the page header.
7. As the person running gx, I want an Epic page tab to read `<epic slug> · Epic · gx`, so that I can find an epic's tab.
8. As the person running gx, I want a Design page tab to read `<epic slug> · Design · gx`, so that I can find the Design I am answering.
9. As the person running gx, I want a Design page tab to show its title at once, before the Design list loads and before a session starts, so that a new tab is named right away.
10. As the person running gx, I want the Kind word in every detail title, so that the tab of a Design and the tab of its `-impl` epic tell apart.
11. As the person running gx, I want the specific name first in the title, so that a narrow tab still shows the part that differs.
12. As the person running gx, I want the title to change when I navigate inside the app (top bar links, card links, jump menu, back button), so that a reused tab never keeps an old page's name.
13. As the person running gx, I want an unknown or retired hash to land on the Epics page with the title `Epics · gx`, so that the redirect and the title agree.
14. As the person running gx, I want browser history and bookmarks to store the page's title, so that I can find a past page by name.
15. As a developer adding a page, I want the page frame to require a title, so that a new page cannot ship with the bare `gx` title.
16. As a developer, I want one module to own the title format, so that a format change happens in one place.

## Implementation Decisions

- **One title module.** A small module in the web UI owns the title format. It exports a pure
  function that takes a Kind and an optional name and returns the full title string, and the hook
  that writes it to the document. No other code writes the document title.
- **The page frame sets the title.** All five pages already render through the shared page frame
  (the component that draws the offline strip, the disk strips, the page container and the top
  bar). The frame gains a required title prop: a Kind plus an optional name. The frame calls the
  title hook with it. A page passes its title; it never calls the hook or writes the document title
  itself. Making the prop required means a new page fails the type check until it gives a title.
- **Kinds.** The Kind is a closed set: Epics, Stats, Design, Epic, Ticket. Epics and Stats take no
  name and render as `<Kind> · gx`. Design, Epic and Ticket take a name and render as
  `<name> · <Kind> · gx`. An empty name (an empty address, as in `#/t/` or `#/e/`) is dropped:
  the title is `<Kind> · gx`, for example `Ticket · gx`. The implementer picks how the type encodes this (for example a union
  where only the detail kinds carry a name).
- **Separator.** ` · `: a space, U+00B7 MIDDLE DOT, a space. The trailing part is always `gx`.
- **Ticket name.** The ticket detail's title when the detail is loaded (from the fetch or from the
  page cache repaint) and the title is not empty after trimming. Otherwise the address from the
  route. "Not found" and a failed first fetch both use the address.
- **Epic and Design name.** The epic slug, which is the address after the project and colon. Both
  pages already compute this name from the address for their header, so the title uses the same
  value and needs no loaded data. The title is the same in every state of these pages: loading,
  loaded, not found, a Design without a session, and "Could not load Designs".
- **Updates.** The hook writes the title on mount and whenever the computed title string changes.
  It does not restore the previous title on unmount; the next page sets its own.
- **Static title.** The `<title>gx</title>` in the HTML file stays as the title shown before the
  JS bundle runs.
- **Gallery.** The component gallery is a dev tool, not a route. Its tab title is not specified. If a
  gallery case renders a page and that page sets the title, that is acceptable.
- **Implementer's choice:** the module's file name and place, the hook's and function's names, the
  Kind type's shape, and whether the hook uses `useEffect` or `useLayoutEffect`.

## Testing Decisions

- A good test checks what the person sees: `document.title` after rendering a route, or the browser
  page title in e2e. Tests do not check how the frame passes the title or which hook runs.
- **Format function.** A table-driven vitest unit test: each Kind with and without a name, the
  separator character, a name that contains ` · ` or a colon passed through unchanged, and an empty
  name rendered as `<Kind> · gx`.
- **Routes seam (main seam).** Extend the existing routes test, which renders `appRoutes` in a memory
  router with fake fetchers and a fake connection. One case per route asserts `document.title`:
  `#/epics`, `#/stats` with and without filter params, `#/e/<address>`, `#/d/<address>` (before the
  list resolves and after), `#/t/<address>` while the fetch is pending, after it resolves, on
  not-found, with an empty title, and after a refetch that returns a new title; an unknown hash after the redirect; and a navigation from one
  route to another. Prior art: the existing cases in the routes test, which already use `okFor` and
  `detailOf` style fetchers.
- **e2e.** One new flow in the web e2e catalog checks the real browser title with Playwright's
  `page.title()` across the routes. Prior art: flow 15 (Redirects) and flow 50 (Design epic page).
- Run web tests with `make web-test` or `gx brief -C webui vitest src/routes`, and the type check
  with `gx brief -C webui tsc`.

## Verification

This epic changes only the web UI. It has no CLI or server verb checks. Each story maps to the new
e2e flow below, "83. Tab titles", which an implementation ticket writes into the catalog and into
a new e2e file (or an existing one, the implementer's choice). The flow uses the existing scenarios
`mixed` and `design`; no new scenario is needed.

**New flow to write: 83. Tab titles**

- Part A, scenario `mixed`:
  1. Open the Epics page. Pass: title is `Epics · gx`.
  2. Click the top bar's Stats link, then pick a different range and a project. Pass: title is
     `Stats · gx` after each step.
  3. Click Epics, open `mixed-epic`. Pass: title is `mixed-epic · Epic · gx`.
  4. Open ticket `02` from the epic page. Pass: title is `<ticket 02's title> · Ticket · gx`.
  5. Press the browser back button. Pass: title is `mixed-epic · Epic · gx` again.
  6. Set the hash to `#/t/demo:mixed-epic/99`. Pass: the page shows "Ticket not found" and the title
     is `demo:mixed-epic/99 · Ticket · gx`.
  7. Set the hash to an unknown path. Pass: it lands on `#/epics` with title `Epics · gx`.
- Part B, scenario `design`:
  1. Open `#/d/demo:design-epic` before any session starts. Pass: title is
     `design-epic · Design · gx`.
  2. Open `#/e/demo:design-epic-impl`. Pass: title is `design-epic-impl · Epic · gx`.
  3. Open `#/e/demo:design-epic`. Pass: title is `design-epic · Epic · gx`.

Per story:

1. Flow 83 part A step 1; flow 15 (Redirects) still passes.
2. Flow 83 part A step 2; flow 57 (Stats, Grilling section) still passes.
3. Flow 83 part A step 4.
4. Routes unit test: Ticket route with a pending fetch has the address title. The e2e flow cannot
   hold the fetch reliably, so the unit test is the check.
5. Flow 83 part A step 6 (not found). Routes unit test for the empty-title case.
6. Routes unit test: a refetch that returns a new ticket title changes `document.title`. gx has no
   CLI verb that renames a ticket, so the e2e flow does not cover this.
7. Flow 83 part A step 3 and part B step 3.
8. Flow 83 part B step 1.
9. Flow 83 part B step 1 (no session); routes unit test for the Design route before the list loads.
10. Flow 83 part B steps 2 and 3 against step 1: the Design and the epic of the same slug differ by
    Kind.
11. Format function unit test: the name is the first part of every detail title.
12. Flow 83 part A steps 2, 3, 4 and 5.
13. Flow 83 part A step 7.
14. Covered by stories 1 to 8: the browser stores `document.title` in history. No extra check.
15. `gx brief -C webui tsc` passes, and a page frame without the title prop fails the type check
    through a type-level test: a page frame call without `title`, marked `@ts-expect-error`, so the
    type check fails if the prop ever becomes optional.
16. Code review: `rg -n "document.title" webui/src` finds only the title module and tests.

## Out of Scope

- A count of Actionable sessions in the title, such as `(2) Epics · gx`. Filed as
  `gx:follow-ups/124`.
- A project prefix in the title.
- A dev-server marker in the title.
- Stats filter values in the title.
- The favicon and the tab title of the component gallery.
- The TUI.

## Further Notes

- The map says the address shows while data loads or when loading fails. Only the Ticket page's
  name depends on loaded data. The Epic and Design pages take the slug from the address, so they show
  the slug in every state, the same name their headers show.
- The middle dot is U+00B7. Writing it as a literal character in the source is fine; the web UI
  already ships UTF-8.
