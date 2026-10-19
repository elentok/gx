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
