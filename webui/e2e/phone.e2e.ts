import { PNG } from "./agentsKit";
import { expect, nav, swipe, test } from "./fixture";

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
  // The panel has focus, so no text field raises the keyboard.
  await expect(dialog).toBeFocused();

  // A bottom sheet about 88% of the viewport high, with a grab handle.
  // The dialog can be detached for a moment after it takes focus under load, so wait for a box rather than read it once.
  let box: Awaited<ReturnType<typeof dialog.boundingBox>> = null;
  await expect.poll(async () => (box = await dialog.boundingBox())).not.toBeNull();
  expect(box!.width).toBe(375);
  expect(box!.y + box!.height).toBe(812);
  expect(box!.height / 812).toBeGreaterThan(0.85);
  expect(box!.height / 812).toBeLessThan(0.91);
  await expect(dialog.getByTestId("grab-handle")).toBeVisible();

  // Close is a text button at the right end of the top bar, above the idea box, and there is no "×".
  const startBtn = dialog.getByRole("button", nav("Start design"));
  const discardBtn = dialog.getByRole("button", nav("Discard"));
  const closeBtn = dialog.getByRole("button", nav("Close"));
  const close = (await closeBtn.boundingBox())!;
  const idea = await dialog.getByRole("textbox", nav("Rough idea")).boundingBox();
  expect(close.y + close.height).toBeLessThanOrEqual(idea!.y);
  await expect(closeBtn).toHaveText("Close");
  expect(Math.abs(close.x + close.width - (box!.x + box!.width - 12))).toBeLessThanOrEqual(2);
  await expect(dialog).not.toContainText("×");

  // The footer is the paperclip, Discard, and Start design at its right edge, each at natural width.
  await expect(startBtn).toBeVisible();
  const footer = discardBtn.locator("xpath=..");
  const clip = dialog.getByRole("button", nav("Add image"));
  const [foot, d, s, c] = [(await footer.boundingBox())!, (await discardBtn.boundingBox())!, (await startBtn.boundingBox())!, (await clip.boundingBox())!];
  const pad = 12;
  expect(c.x).toBeCloseTo(foot.x + pad, 0);
  expect(c.x + c.width).toBeLessThanOrEqual(d.x);
  expect(s.x + s.width).toBeCloseTo(foot.x + foot.width - pad, 0);
  // No tile (the paperclip is the only Add image), no limits line, and with an empty draft no image row.
  await expect(dialog.getByRole("button", nav("Add image"))).toHaveCount(1);
  await expect(dialog.locator('[data-mock-part="image-line"]')).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "Open image" })).toHaveCount(0);
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

  // A file picked through the paperclip gets a thumbnail; a click opens the viewer and closing it starts no design.
  const starts: string[] = [];
  page.on("request", (r) => r.url().includes("/v1/designs/open") && starts.push(r.url()));
  const chooser = page.waitForEvent("filechooser");
  await clip.click();
  await (await chooser).setFiles({ name: "picked.png", mimeType: "image/png", buffer: PNG });
  const open = dialog.getByRole("button", { name: "Open image" });
  await expect(open).toHaveCount(1);
  await open.click();
  const closeViewer = page.getByRole("button", { name: /^Close ✕/ });
  await expect(closeViewer).toBeVisible();
  await closeViewer.click();
  await expect(closeViewer).toBeHidden();
  await expect(dialog).toBeVisible();
  await expect(page).toHaveURL(/#\/epics$/);
  expect(starts).toEqual([]);

  // At 744px (tablet) Close is still at the right end of the top bar and the paperclip is still in the footer.
  await page.setViewportSize({ width: 744, height: 1000 });
  const wide = (await dialog.boundingBox())!;
  const closeAt = (await closeBtn.boundingBox())!;
  expect(Math.abs(closeAt.x + closeAt.width - (wide.x + wide.width - 12))).toBeLessThanOrEqual(2);
  await expect(dialog).not.toContainText("×");
  await expect(dialog.getByRole("button", nav("Add image"))).toHaveCount(1);
  const [d2, c2] = [(await discardBtn.boundingBox())!, (await clip.boundingBox())!];
  expect(c2.x + c2.width).toBeLessThanOrEqual(d2.x);
  await page.setViewportSize({ width: 375, height: 812 });

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

  // The question cards own horizontal swipes: a swipe right never opens the sidebar, a swipe left moves to the next
  // question. A wide code block is not in the fixture, so the vitest case in useSidebarSwipe covers a scroller.
  const sidebar = page.getByRole("complementary", nav("Sidebar"));
  const prev = page.getByRole("button", nav("← Prev"));
  const next = page.getByRole("button", nav("Next →"));
  // Picking SQLite moved to the second card; go back to the first.
  await expect(prev).toBeEnabled();
  await prev.click();
  await expect(prev).toBeDisabled();
  const cardsBox = (await questions.getByTestId("question-body").boundingBox())!;
  const cardY = cardsBox.y + 40;
  await swipe(page, [150, cardY], [300, cardY + 5]);
  await expect(sidebar).toBeHidden();
  await expect(prev).toBeDisabled();
  await swipe(page, [340, cardY], [40, cardY + 5]);
  await expect(prev).toBeEnabled();
  await expect(next).toBeDisabled();
  await expect(sidebar).toBeHidden();

  // With a single question (the next round) a swipe right still leaves the sidebar closed.
  await questions.getByRole("button", { name: /Yes/ }).click();
  await page.getByRole("button", nav("Send")).click();
  await expect(questions.getByText("0 of 1 answered")).toBeVisible();
  const single = (await questions.getByTestId("question-body").boundingBox())!;
  await swipe(page, [150, single.y + 40], [300, single.y + 45]);
  await expect(sidebar).toBeHidden();
  await expect(questions.getByText("0 of 1 answered")).toBeVisible();

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
  // The paperclip is at the left edge of the footer, which has no separator and 6px above, 8px below.
  const footer = page.getByRole("form", nav("Composer"));
  const footerBox = (await footer.boundingBox())!;
  expect(boxes[0].x - footerBox.x).toBeLessThan(boxes[1].x - footerBox.x);
  await expect(footer).toHaveCSS("border-top-width", "0px");
  await expect(footer).toHaveCSS("padding-top", "6px");
  await expect(footer).toHaveCSS("padding-bottom", "8px");

  // Keyboard: a visual viewport 300px shorter while the question box is focused drops the top bar, title block
  // and images and leaves the tabs with the ⋯; restoring the viewport and leaving focus brings them back.
  const title = page.getByRole("heading", { level: 1 });
  const more = page.getByRole("button", nav("More actions"));
  const topBar = page.getByRole("button", { name: "Show sidebar", exact: true });
  await page.getByRole("button", nav("Note")).click();
  const box = page.getByRole("textbox", nav("Message"));
  await box.focus();
  const setViewport = (shrink: number) =>
    page.evaluate((px) => {
      const vv = window.visualViewport!;
      Object.defineProperty(vv, "height", { configurable: true, get: () => window.innerHeight - px });
      vv.dispatchEvent(new Event("resize"));
    }, shrink);
  await expect(title).toBeVisible();
  await setViewport(300);
  await expect(title).toBeHidden();
  await expect(topBar).toBeHidden();
  await expect(page.getByLabel("Images")).toHaveCount(0);
  await expect(view).toBeVisible();
  await expect(more).toBeVisible();
  await expect(view.locator("xpath=../..").getByRole("button", nav("More actions"))).toBeVisible();
  await setViewport(0);
  await box.blur();
  await expect(title).toBeVisible();
  await expect(topBar).toBeVisible();
  await expect(more).toHaveCount(1);

  // The ⋯ menu holds the model row and the link to the design epic.
  await more.click();
  const actions = page.getByRole("menu", nav("More actions"));
  await expect(actions.getByText("Model", { exact: true })).toBeVisible();
  await expect(actions.getByText("Model", { exact: true }).locator("..")).toHaveText("ModelClaude Sonnet · medium effort");
  await actions.getByRole("menuitem", { name: /^Open design epic/ }).click();
  await expect(page).toHaveURL(/#\/e\/demo:design-epic$/);
  await page.goBack();
  await expect(view).toBeVisible();

  // Auto-queue: the menu shows "Queue" and flips; the countdown strip sits above the footer with a 40px+ Cancel.
  await auto.click();
  const menu = page.getByRole("dialog", nav("Auto"));
  await expect(menu.getByRole("switch", { name: "Auto-queue off" })).toBeEnabled();
  await menu.getByRole("switch", { name: "Auto-queue off" }).click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: /Auto-queue on$/ })).toHaveText("AdvanceQueue");
  await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", "branch");
  const strip = page.locator('[data-part="countdown-strip"]');
  await expect(strip).toContainText(/Queuing design-epic in \ds/);
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
