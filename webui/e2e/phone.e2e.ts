import { PNG } from "./agentsKit";
import { expect, nav, test } from "./fixture";

// 375 px is below the wide layout's 62em breakpoint.
test.use({ viewport: { width: 375, height: 812 } });

// Phone 23 and 53a (the "united" scenario) live in jump.e2e.ts flow 22, which already pays the settle wait.
test.describe("53. Phone, epic page", () => {
  test("a card's meta sits below its title, and answer and fix buttons are full-width tap targets", async ({ page, start }) => {
    // Recovery would turn 07 into a waiting parent, so it stays off as in flow 2.
    const fixture = await start("mixed", { config: { "recovery.enabled": "false" } });
    const long = "A very long ticket title that would wrap onto a second line on a phone screen";
    await fixture.cli("tickets", "add", "demo:mixed-epic", "--slug", "long-title", "--body", `# ${long}\n\n## What to build\n\nx`);
    await page.evaluate(() => (location.hash = "#/e/demo:mixed-epic"));
    const link = page.getByRole("link", { name: "02 open ready" });
    await expect(link).toBeVisible();
    const title = (await link.getByText("open", nav("open")).boundingBox())!;
    const meta = (await link.getByText("ready", nav("ready")).boundingBox())!;
    expect(meta.y).toBeGreaterThanOrEqual(title.y + title.height);
    // The meta is indented 80px under the title.
    await expect(link.locator("[data-meta]")).toHaveCSS("padding-left", "80px");

    // A long title wraps, and its meta sits below the whole title.
    const longTitle = page.getByText(long, nav(long));
    await expect(longTitle).toBeVisible();
    const box = (await longTitle.boundingBox())!;
    const lineHeight = await longTitle.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight));
    expect(box.height).toBeGreaterThan(lineHeight * 1.5);
    const longMeta = (await longTitle.locator("xpath=ancestor::a").locator("[data-meta]").boundingBox())!;
    expect(longMeta.y).toBeGreaterThanOrEqual(box.y + box.height);

    // The bar's buttons wrap to a full-width row of touch-sized buttons.
    const queue = page.getByRole("button", nav("Queue"));
    expect((await queue.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await queue.click();
    const answer = page.getByRole("button", nav("Answer"));
    const fix = page.getByRole("button", { name: "Fix it with an agent" });
    // 07 only starts once 02 and 03 have merged, each after the server's 3s finish debounce, as in flow 2.
    await expect(page.getByText("4/8")).toBeVisible({ timeout: 30_000 });
    await expect(answer).toBeVisible();
    await expect(fix).toBeVisible();

    const card = (button: typeof answer) =>
      page.locator("div").filter({ has: button }).filter({ has: page.locator("a") }).last();
    // Each button is as wide as the note row it sits in, less the 80px indent.
    for (const button of [answer, fix]) {
      const row = (await button.locator("xpath=ancestor::*[@data-note-row]").boundingBox())!;
      const box = (await button.boundingBox())!;
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(box.width).toBeCloseTo(row.width - 80, 0);
    }

    // In 06's card the meta, question and Answer stack in that order, their text 80px from the content edge.
    const askCard = card(answer);
    const metaEl = askCard.locator("[data-meta]");
    await expect(metaEl).toHaveCSS("padding-left", "80px");
    const askMeta = (await metaEl.boundingBox())!;
    const question = (await askCard.getByText("Which option should the fake worker take?").boundingBox())!;
    const ab = (await answer.boundingBox())!;
    expect(askMeta.y + askMeta.height).toBeLessThanOrEqual(question.y);
    expect(question.y + question.height).toBeLessThanOrEqual(ab.y);
    expect(askMeta.x + 80).toBeCloseTo(question.x, 0);
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

  // Close sits in the header, above the idea box.
  const startBtn = dialog.getByRole("button", nav("Start design"));
  const discardBtn = dialog.getByRole("button", nav("Discard"));
  const close = await dialog.getByRole("button", nav("Close")).boundingBox();
  const idea = await dialog.getByRole("textbox", nav("Rough idea")).boundingBox();
  expect(close!.y + close!.height).toBeLessThanOrEqual(idea!.y);

  // The footer is Discard at its left edge and Start design at its right edge, each at natural width.
  await expect(startBtn).toBeVisible();
  const footer = discardBtn.locator("xpath=..");
  const [foot, d, s] = [(await footer.boundingBox())!, (await discardBtn.boundingBox())!, (await startBtn.boundingBox())!];
  const pad = 12;
  expect(d.x).toBeCloseTo(foot.x + pad, 0);
  expect(s.x + s.width).toBeCloseTo(foot.x + foot.width - pad, 0);
  // Natural width: the label plus the button's padding, far short of the stretched half the footer once gave it.
  const content = await startBtn.evaluate((el) => {
    const range = document.createRange();
    range.selectNodeContents(el);
    const cs = getComputedStyle(el);
    return range.getBoundingClientRect().width + parseFloat(cs.paddingLeft) + parseFloat(cs.paddingRight) + parseFloat(cs.borderLeftWidth) + parseFloat(cs.borderRightWidth);
  });
  expect(s.width).toBeCloseTo(content, 0);

  // With the idea empty the one-line hint shows above the footer, and Start keeps its width.
  await dialog.getByRole("textbox", nav("Epic slug")).fill("phone-epic");
  await expect(dialog.getByText("Saved")).toBeVisible();
  const hint = dialog.getByText("Write an idea to start.");
  await expect(hint).toBeVisible();
  const h = (await hint.boundingBox())!;
  const foot2 = (await footer.boundingBox())!;
  expect(h.y + h.height).toBeLessThanOrEqual(foot2.y);
  expect((await startBtn.boundingBox())!.width).toBeCloseTo(s.width, 0);

  // A thumbnail click opens the image viewer.
  await dialog.getByRole("textbox", nav("Rough idea")).evaluate((el, b64) => {
    const data = new DataTransfer();
    data.items.add(new File([Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))], "pasted.png", { type: "image/png" }));
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  }, PNG.toString("base64"));
  const open = dialog.getByRole("button", { name: "Open image" });
  await expect(open).toHaveCount(1);
  await open.click();
  const closeViewer = page.getByRole("button", { name: /^Close ✕/ });
  await expect(closeViewer).toBeVisible();
  await closeViewer.click();
  await expect(closeViewer).toBeHidden();

  // Tab keeps focus inside the dialog.
  for (let i = 0; i < 12; i++) {
    await page.keyboard.press("Tab");
    expect(await dialog.evaluate((el) => el.contains(document.activeElement))).toBe(true);
  }
});

test("24. Phone, Design page", async ({ page, start }) => {
  const fixture = await start("design", { autoAdvanceDelay: "9s" });

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
  // Send is dimmed until one question is answered; the other stays open, so the gaps popover shows.
  await expect(page.getByRole("button", nav("Send"))).toBeDisabled();
  await page.getByRole("button", { name: /^SQLite/ }).click();
  await page.getByRole("button", nav("Send")).click();
  const popover = page.getByRole("alertdialog", nav("Unanswered questions"));
  await expect(popover).toBeVisible();
  // It scales in, so it is measured once settled.
  await popover.evaluate((el) => Promise.all(el.getAnimations().map((a) => a.finished)));
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

  // Note is an icon with no text; the Auto button shows both short names.
  await expect(page.getByRole("button", nav("Note"))).toHaveText("");
  await expect(page.getByRole("button", { name: /^Auto:/ })).toHaveText("AdvanceQueue");

  // The footer is one row at 390px: Add, Note, Auto, Send, with 44px Add and Note.
  await page.setViewportSize({ width: 390, height: 844 });
  const row = ["Add image", "Note"].map((n) => page.getByRole("button", nav(n)));
  const auto = page.getByRole("button", { name: /^Auto:/ });
  const send = page.getByRole("button", nav("Send"));
  const boxes = [];
  for (const b of [...row, auto, send]) {
    await expect(b).toBeVisible();
    boxes.push((await b.boundingBox())!);
  }
  for (const b of boxes.slice(0, 2)) expect([b.width, b.height]).toEqual([44, 44]);
  const centers = boxes.map((b) => b.y + b.height / 2);
  for (const c of centers) expect(c).toBeCloseTo(centers[0], 0);
  for (let i = 1; i < boxes.length; i++) expect(boxes[i].x).toBeGreaterThanOrEqual(boxes[i - 1].x + boxes[i - 1].width);

  // Auto-queue: the menu shows "Queue" and flips; the countdown strip sits above the footer with a 40px+ Cancel.
  await auto.click();
  const menu = page.getByRole("dialog", nav("Auto"));
  await expect(menu.getByRole("switch", { name: "Auto-queue off" })).toBeEnabled();
  await menu.getByRole("switch", { name: "Auto-queue off" }).click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: /Auto-queue on$/ })).toHaveText("AdvanceQueue");
  await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", "branch");
  const strip = page.locator('[data-part="countdown-strip"]');
  await expect(strip).toContainText(/Queuing in \ds/);
  const cancel = strip.getByRole("button", nav("Cancel auto-queue"));
  expect((await cancel.boundingBox())!.height).toBeGreaterThanOrEqual(40);
  expect((await strip.boundingBox())!.y + (await strip.boundingBox())!.height).toBeLessThanOrEqual((await send.boundingBox())!.y + 1);

  // Cmd/Ctrl+K opens the jump menu as a full-screen sheet; its close button closes it.
  await page.keyboard.press("ControlOrMeta+k");
  const sheet = page.getByRole("dialog", nav("Jump to"));
  await expect(sheet).toBeVisible();
  expect(await sheet.boundingBox()).toMatchObject({ x: 0, y: 0, width: 390, height: 844 });
  await sheet.getByRole("button", nav("Close")).click();
  await expect(sheet).toHaveCount(0);
});
