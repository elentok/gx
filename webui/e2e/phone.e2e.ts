import { card, expect, nav, test } from "./fixture";

// 375 px is below the wide layout's 62em breakpoint.
test.use({ viewport: { width: 375, height: 812 } });

test("23. Phone, Epics page", async ({ page, start }) => {
  await start("united");

  // The grid is one column: two cards of a section sit at the same left edge, one under the other.
  const first = await card(page, "paused-epic").boundingBox();
  const second = await card(page, "question-epic").boundingBox();
  expect(second!.x).toBe(first!.x);
  expect(second!.y).toBeGreaterThan(first!.y);

  // The back link stays on one line with the short or the full count.
  await page.evaluate(() => (location.hash = "#/d/demo:design-live"));
  const back = page.getByRole("link", { name: /^← Epics · 8 more/ });
  await expect(back).toBeVisible();
  await expect(back).toContainText(/· 8 more( need you)?$/);
  const line = await back.boundingBox();
  const lineHeight = await back.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight));
  expect(line!.height).toBeLessThanOrEqual(lineHeight * 1.5);
});

test("41. Phone, New Design sheet", async ({ page, start }) => {
  await start("ready");
  await page.getByRole("button", nav("New Design")).click();
  const dialog = page.getByRole("dialog", nav("New Design"));
  await expect(dialog).toBeVisible();

  // A bottom sheet about 88% of the viewport high, with a grab handle.
  const box = await dialog.boundingBox();
  expect(box!.width).toBe(375);
  expect(box!.y + box!.height).toBe(812);
  expect(box!.height / 812).toBeGreaterThan(0.85);
  expect(box!.height / 812).toBeLessThan(0.91);
  await expect(dialog.getByTestId("grab-handle")).toBeVisible();

  // Start design is about twice as wide as Discard; Close sits in the header, above the idea box.
  const start_ = await dialog.getByRole("button", nav("Start design")).boundingBox();
  const discard = await dialog.getByRole("button", nav("Discard")).boundingBox();
  expect(start_!.width / discard!.width).toBeGreaterThan(1.7);
  expect(start_!.width / discard!.width).toBeLessThan(2.3);
  const close = await dialog.getByRole("button", nav("Close")).boundingBox();
  const idea = await dialog.getByRole("textbox", nav("Rough idea")).boundingBox();
  expect(close!.y + close!.height).toBeLessThanOrEqual(idea!.y);
});

test("24. Phone, Design page", async ({ page, start }) => {
  await start("design");

  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await page.getByRole("button", nav("Start design session")).click();
  await expect(page.getByText("Two questions before the spec.")).toBeVisible();

  // Progress is a segment bar whose summary opens a modal with the steps.
  const progress = page.getByRole("region", nav("Progress"));
  await progress.getByRole("button", { name: /Resolve tickets/ }).click();
  const steps = page.getByRole("dialog", nav("Design progress steps"));
  await expect(steps.locator('li[data-step="resolve-tickets"][data-state="current"]')).toBeVisible();
  await steps.getByRole("button", nav("Close")).click();
  await expect(steps).toHaveCount(0);

  // Note and Focus lose their text, and the switch loses its label.
  await expect(page.getByRole("button", nav("Note"))).toHaveText("✎");
  await expect(page.getByRole("button", nav("Focus"))).toHaveText("⤢");
  await expect(page.getByRole("switch")).not.toContainText("Auto-advance");

  // A tap on the title opens a full-screen sheet; its close button closes it.
  await page.getByRole("heading", { level: 1 }).getByRole("button").click();
  const sheet = page.getByRole("dialog", nav("Jump to epic"));
  await expect(sheet).toBeVisible();
  const box = await sheet.boundingBox();
  expect(box).toMatchObject({ x: 0, y: 0, width: 375, height: 812 });
  await sheet.getByRole("button", nav("Close")).click();
  await expect(sheet).toHaveCount(0);
});
