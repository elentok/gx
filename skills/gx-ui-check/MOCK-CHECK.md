# Mock check

Compare a built page with the stored mock it came from. gx-implement and gx-verify-epic follow these
steps. The browser commands come from the `playwright-cli` skill.

## Steps

1. Start the fixture scenario with [gx-ui-check](SKILL.md) (section 1). Open its `web_url` once.
2. Open the stored mock file in `playwright-cli` through a `file://` URL. Do not start a local
   server: the mock is one self-contained file.
3. Pick the state and the part you compare. The mock marks them:
   - Part selector: `[data-mock-state=<s>] [data-mock-part=<p>]`
   - State selector: `[data-mock-state=<s>]`
4. Locate the same part on the built page. You choose how: roles, visible text or a CSS selector.
5. Screenshot the mock and the built page at the state's width, at device scale factor 1, cropped to
   the selector. Never take a full page at retina scale.
6. Compare the two screenshots side by side by eye. List each difference.

## What wins

- The mock wins on layout, spacing, sizes, colours and type.
- A later decision wins on behaviour.
- Pixel-perfect is not a goal. Report differences a person would notice.

## Phone states

A phone state is a fixed-width box in the mock, where media queries do not fire. Compare it with the
built page at that viewport width, not at the desktop width.

Stop the fixture when you are done (gx-ui-check, section 3), even when the check failed.
