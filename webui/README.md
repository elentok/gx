# gx web UI

React and Chakra app embedded in the gx binary. `make web-dev` serves it with a canned snapshot.
`make web-test` runs the tests.

## Adding a component

1. Create a folder `src/ui/<name>/` with `<Name>.tsx` and `<Name>.test.tsx`.
2. Style with theme tokens only. Raw colors and font names belong in `src/ui/theme.ts`;
   `src/ui/raw-style.test.ts` fails the build otherwise.
3. Take a `tone` prop (see `src/ui/tone.ts`) for color. Do not import snapshot types. Pass plain props.
4. Add an entry to `src/gallery/registry.tsx` with one case per tone and per state.
   Open it at `/gallery.html` in `make web-dev`. The gallery is not part of the production build.
5. Write a render test with `renderThemed` from `src/ui/render.tsx`. Cover each tone and each state.

## Adding a page

A page that shows server data must repaint from the last copy after a reload. iOS Safari drops a
tab that sat in the background, and without a cache the reload shows a blank page or spinner
until the fetch lands.

1. Read the first state with `readPageCache(key, id)` from `src/pageCache.ts` and save every fresh
   result with `writePageCache(key, value, id)`. `id` is the address a detail page shows; it keeps
   one entry per key, so storage stays small.
2. For a detail page, call `useDetail` with a `cacheKey`. The list pages use `useEpicList` as the example.
3. Keep fetching on mount. The cached copy is only the first paint, so the page never trusts it.
4. `src/test-setup.ts` clears `sessionStorage` after each test.
5. While a page has no data yet, show `Spinner` from `src/ui/spinner/Spinner.tsx` with a label such
   as "Loading epics". It centers itself both ways. Do not draw skeleton placeholders.

## Testing a Chakra menu

Chakra v3 menus run on zag-js. Don't read zag sources in `node_modules`; the recipe is short.

- Open the menu with `fireEvent.click` on its trigger, then `await findByRole("menuitem...")`.
- A menu item's click handler runs on a later tick. Assert the callback inside `waitFor`, never
  right after `fireEvent.click`. A direct `expect(spy).toHaveBeenCalled()` fails with 0 calls.
- `src/test-setup.ts` already stubs `ResizeObserver` (jsdom has none; the popper needs it). Don't
  add another stub.
- Example: `src/ui/top-bar/TopBar.test.tsx` (the AFK menu item).
