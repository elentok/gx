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

  // The card's time label stays on one line.
  const time = card(page, "paused-epic").getByTestId("time");
  await expect(time).toBeVisible();
  const timeBox = await time.boundingBox();
  const timeLineHeight = await time.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight));
  expect(timeBox!.height).toBeLessThanOrEqual(timeLineHeight * 1.5);

  // The Epics tab stays on one line with its count.
  await page.evaluate(() => (location.hash = "#/d/demo:design-live"));
  const back = page.getByRole("link", { name: /^Epics · 9 need you$/ });
  await expect(back).toBeVisible();
  const line = await back.boundingBox();
  const lineHeight = await back.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight));
  expect(line!.height).toBeLessThanOrEqual(lineHeight * 1.5);
});

test.describe("53. Phone, epic page", () => {
  test("buttons sit below the stats line", async ({ page, start }) => {
    await start("united");
    await page.evaluate(() => (location.hash = "#/e/demo:awaiting-epic"));
    const merge = page.getByRole("button", nav("Merge"));
    await expect(merge).toBeVisible();
    const stats = (await page.getByTestId("stats-line").boundingBox())!;
    for (const button of await merge.locator("xpath=..").getByRole("button").all()) {
      const box = (await button.boundingBox())!;
      expect(box.y).toBeGreaterThanOrEqual(stats.y + stats.height);
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
  });

  test("a card's meta sits below its title", async ({ page, start }) => {
    const fixture = await start("mixed");
    const long = "A very long ticket title that would wrap onto a second line on a phone screen";
    await fixture.cli("tickets", "add", "demo:mixed-epic", "--slug", "long-title", "--body", `# ${long}\n\n## What to build\n\nx`);
    await page.evaluate(() => (location.hash = "#/e/demo:mixed-epic"));
    const link = page.getByRole("link", { name: "02 open ready" });
    await expect(link).toBeVisible();
    const title = (await link.getByText("open", nav("open")).boundingBox())!;
    const meta = (await link.getByText("ready", nav("ready")).boundingBox())!;
    expect(meta.y).toBeGreaterThanOrEqual(title.y + title.height);
    // The meta is indented 54px under the title.
    await expect(link.locator("[data-meta]")).toHaveCSS("padding-left", "54px");

    // A long title wraps, and its meta sits below the whole title.
    const longTitle = page.getByText(long, nav(long));
    await expect(longTitle).toBeVisible();
    const box = (await longTitle.boundingBox())!;
    const lineHeight = await longTitle.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight));
    expect(box.height).toBeGreaterThan(lineHeight * 1.5);
    const longMeta = (await longTitle.locator("xpath=ancestor::a").locator("[data-meta]").boundingBox())!;
    expect(longMeta.y).toBeGreaterThanOrEqual(box.y + box.height);
  });

  test("answer and fix buttons are full-width tap targets", async ({ page, start }) => {
    // Recovery would turn 07 into a waiting parent, so it stays off as in flow 2.
    await start("mixed", { config: { "recovery.enabled": "false" } });
    await page.evaluate(() => (location.hash = "#/e/demo:mixed-epic"));
    await page.getByRole("button", nav("Queue")).click();
    const answer = page.getByRole("button", nav("Answer"));
    const fix = page.getByRole("button", { name: "Fix it with an agent" });
    await expect(answer).toBeVisible();
    await expect(fix).toBeVisible();

    const card = (button: typeof answer) =>
      page.locator("div").filter({ has: button }).filter({ has: page.locator("a") }).last();
    // Each button is as wide as the note row it sits in, less the 54px indent.
    for (const button of [answer, fix]) {
      const row = (await button.locator("xpath=ancestor::*[@data-note-row]").boundingBox())!;
      const box = (await button.boundingBox())!;
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(box.width).toBeCloseTo(row.width - 54, 0);
    }

    // In 06's card the meta, question and Answer stack in that order, their text 54px from the content edge.
    const askCard = card(answer);
    const metaEl = askCard.locator("[data-meta]");
    await expect(metaEl).toHaveCSS("padding-left", "54px");
    const meta = (await metaEl.boundingBox())!;
    const question = (await askCard.getByText("Which option should the fake worker take?").boundingBox())!;
    const ab = (await answer.boundingBox())!;
    expect(meta.y + meta.height).toBeLessThanOrEqual(question.y);
    expect(question.y + question.height).toBeLessThanOrEqual(ab.y);
    expect(meta.x + 54).toBeCloseTo(question.x, 0);
    expect(ab.x).toBeCloseTo(question.x, 0);
  });
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
  // The Questions view is up: the first message is hidden.
  const view = page.getByRole("group", nav("View"));
  const chatText = page.getByText("Two questions before the spec.");
  await expect(view.getByRole("button", { name: /^Questions/ })).toHaveAttribute("aria-pressed", "true");
  await expect(chatText).toBeHidden();

  // A step line under the name and the ring open the steps sheet; there is no segmented bar.
  await expect(page.getByRole("region", nav("Progress"))).toHaveCount(0);
  const stepLine = page.getByRole("button", { name: /^Resolve tickets · / });
  const steps = page.getByRole("dialog", nav("Design progress steps"));
  const current = steps.locator('li[data-step="resolve-tickets"][data-state="current"]');
  await stepLine.click();
  await expect(current).toBeVisible();
  await steps.getByRole("button", nav("Close")).click();
  await expect(steps).toHaveCount(0);
  await page.getByRole("button", nav("Show progress steps")).click();
  await expect(current).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(steps).toHaveCount(0);
  await stepLine.click();
  await expect(steps).toBeVisible();
  await page.mouse.click(187, 20);
  await expect(steps).toHaveCount(0);

  // The View toggle replaces the focus buttons: Chat shows the first message, Questions hides it.
  await expect(page.getByRole("button", { name: /focus mode/ })).toHaveCount(0);
  await view.getByRole("button", nav("Chat")).click();
  await expect(chatText).toBeVisible();
  await view.getByRole("button", { name: /^Questions/ }).click();
  await expect(chatText).toBeHidden();

  // Send with gaps: the popover fits the screen and overlaps the sheet.
  const questions = page.getByRole("group", nav("Questions"));
  await page.getByRole("button", nav("Send answers")).click();
  const popover = page.getByRole("alertdialog", nav("Unanswered questions"));
  await expect(popover).toBeVisible();
  const pop = (await popover.boundingBox())!;
  const sheetBox = (await questions.boundingBox())!;
  expect(pop.width).toBeLessThanOrEqual(343);
  expect(pop.x).toBeGreaterThanOrEqual(0);
  expect(pop.x + pop.width).toBeLessThanOrEqual(375);
  expect(pop.y).toBeLessThan(sheetBox.y + sheetBox.height);
  expect(pop.y + pop.height).toBeGreaterThan(sheetBox.y);
  await page.keyboard.press("Escape");
  await expect(popover).toHaveCount(0);

  // The step line stays above the sheet in the Questions view.
  const line = (await stepLine.boundingBox())!;
  expect(line.y + line.height).toBeLessThanOrEqual((await questions.boundingBox())!.y);

  // Note loses its text; the Auto button keeps both names.
  await expect(page.getByRole("button", nav("Note"))).toHaveText("✎");
  await expect(page.getByRole("button", { name: /^Auto:/ })).toHaveText("Auto-advanceAuto-queue");

  // A tap on the title opens a full-screen sheet; its close button closes it.
  await page.getByRole("heading", { level: 1 }).getByRole("button").click();
  const sheet = page.getByRole("dialog", nav("Jump to epic"));
  await expect(sheet).toBeVisible();
  const box = await sheet.boundingBox();
  expect(box).toMatchObject({ x: 0, y: 0, width: 375, height: 812 });
  await sheet.getByRole("button", nav("Close")).click();
  await expect(sheet).toHaveCount(0);
});
