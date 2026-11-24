# Build a mock that looks like the real app

Use this when a prototype must look like the project's real UI. A mock that only looks plausible gets approved on the wrong facts.

## Pick the shape

- Try the in-app route first (UI.md sub-shape A or B). Run the real app and save the approved variant to one HTML file.
- Use a standalone file only when the app cannot run or the mock is an artifact.

## Take the look from the project

- Find the project's own theme file, fonts, components and icons before you draw anything.
- Never invent colors, use `system-ui` or guess a font size. Read the value from the theme.
- The project's tokens win over the look the claude.ai artifact design skill pushes.
- Never redraw an icon. Copy the app's own SVG or use the app's icon library.

## Check a standalone mock

- Before approval, compare computed styles with the real page: line height, disabled opacity and row heights.
- If the page cannot be run, say at the top of the mock that it was built from source only.

## Variants

- Every mock uses the standard pill switcher from `switcher-snippet.html`.
- In-app routes use the `?variant=` param in every project. The `#v=` hash is only for a standalone file.

## Inlined styles

- A standalone mock pastes tokens, base layer and fonts into one `<style data-mock-inlined>`. That is the only block the helper exempts.
- A save script outside gx must write the same `data-mock-inlined` marker on the `<style>` element.

## In gx

- Token file: [gx/tokens.css](gx/tokens.css)
- Base layer: [gx/base.css](gx/base.css)
- Fonts: [gx/fonts.css](gx/fonts.css)
- Save script: [gx/save-route-to-html.js](gx/save-route-to-html.js)
- Component, icon and font pointers: [gx/README.md](gx/README.md)
