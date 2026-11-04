import type { Locator, Page } from "@playwright/test";
import { card, expect, nav, sessionRows, test } from "./fixture";

const openDesign = async (page: Page) => {
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await expect(page.getByRole("button", nav("Start design session"))).toBeVisible();
};

const moreActions = (page: Page) => page.getByRole("button", nav("More actions"));
const actionsMenu = (page: Page) => page.getByRole("menu", nav("More actions"));

test.describe("72. Design actions menu", () => {
  test("two live sessions: popover under ⋯ at 1440px, the list shows the shown one, switches and stops", async ({ page, start }) => {
    const fixture = await start("design-two-live");
    await page.setViewportSize({ width: 1440, height: 900 });
    await expect(card(page, "design-epic")).toBeVisible();
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
    await expect(page.getByRole("group", nav("Questions"))).toBeVisible();

    const trigger = (await moreActions(page).boundingBox())!;
    expect(trigger.width).toBe(44);
    expect(trigger.height).toBe(44);
    await expect(moreActions(page)).toHaveCSS("border-top-width", "1px");
    // Level with the title row, not below it: the Progress sidebar starts under the header.
    const title = (await page.getByRole("heading", { level: 1 }).first().boundingBox())!;
    expect(trigger.y).toBeLessThan(title.y + title.height);

    await moreActions(page).click();
    const menu = actionsMenu(page);
    await expect(menu.getByTestId("grab-handle")).toHaveCount(0);
    // The positioner places the popover after it mounts, so wait for it to leave the origin.
    await expect.poll(async () => (await menu.boundingBox())!.y).toBeGreaterThanOrEqual(trigger.y + trigger.height);
    const box = (await menu.boundingBox())!;
    expect(box.x).toBeLessThan(trigger.x + trigger.width);
    expect(box.x + box.width).toBeGreaterThan(trigger.x);

    await expect(menu.getByText("Live sessions · 2")).toBeVisible();
    const shown = menu.getByRole("menuitem", { name: /^design-epic/ });
    await expect(shown).toHaveAttribute("aria-current", "true");
    await expect(shown).toContainText("✓");
    await expect(menu.getByRole("menuitem", nav("Stop design-epic"))).toHaveCount(0);
    const other = menu.getByRole("menuitem", { name: /^Claude Code/ });
    await expect(other).toHaveAttribute("aria-current", "false");
    await expect(menu.getByRole("menuitem", nav("Stop Claude Code"))).toBeVisible();

    // Picking the other row shows it: it has no round, so the Questions sheet goes.
    await other.click();
    await expect(page.getByRole("group", nav("Questions"))).toHaveCount(0);

    await moreActions(page).click();
    await expect(menu.getByRole("menuitem", { name: /^Claude Code/ })).toHaveAttribute("aria-current", "true");
    const before = await sessionRows(fixture, "design-epic");
    expect(before.filter((r) => /exited/.test(r))).toHaveLength(0);
    await menu.getByRole("menuitem", nav("Stop design-epic")).click();
    await expect
      .poll(async () => (await sessionRows(fixture, "design-epic")).filter((r) => /exited/.test(r)).length)
      .toBe(1);
  });
});

const showChat = (page: Page) => page.getByRole("group", nav("View")).getByRole("button", nav("Chat")).click();

const fontSize = async (el: Locator) => {
  await expect(el).toBeVisible();
  return el.evaluate((node) => getComputedStyle(node).fontSize);
};

// One Design at a short viewport: a move starts at the question's title, the header and Prev/Next stay put
// while the body scrolls, and the Send popover sits above Send.
test.describe("74. Question sheet layout", () => {
  test.use({ viewport: { width: 1280, height: 600 } });

  test("a move starts at the title, the rows stay put, the popover sits above Send", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    const send = page.getByRole("button", nav("Send answers"));
    const popover = page.getByRole("alertdialog", nav("Unanswered questions"));
    const composer = page.locator('[aria-label="Composer"]');
    await expect(questions.getByText("Q8 - Which store?")).toBeVisible();

    const dot = questions.getByRole("button", { name: /^Question 8(, answered)?$/ });
    const dot8b = questions.getByRole("button", nav("Question 8b"));
    const count = questions.getByText(/\d of 2 answered/);
    const prev = questions.getByRole("button", nav("← Prev"));
    const next = questions.getByRole("button", nav("Next →"));
    const note = questions.getByRole("textbox", nav("Note text for question 8"));
    const body = questions.getByTestId("question-body");
    const title = questions.getByText("Q8b - Migrate at startup?");

    const toEnd = async () => {
      await expect(body).toBeVisible();
      await body.evaluate((el) => (el.scrollTop = el.scrollHeight));
      expect(await body.evaluate((el) => el.scrollTop), "the Q8 body must scroll").toBeGreaterThan(0);
    };
    const expectTitleAtTop = async () => {
      await expect(dot8b).toHaveAttribute("aria-current", "step");
      await expect
        .poll(async () => Math.abs((await title.boundingBox())!.y - (await body.boundingBox())!.y))
        .toBeLessThanOrEqual(4);
    };

    const pass = async () => {
      await dot.click();
      // An open note makes the Q8 body taller than the sheet at this height.
      if ((await note.count()) === 0) await questions.getByRole("button", nav("Note on question 8")).click();
      await expect(note).toBeVisible();
      await expect(note).toHaveAttribute("rows", "5");
      await expect(body).toBeVisible();

      // The header and Prev/Next stay put while the body scrolls.
      // The sheet is bottom-anchored under a live "Working: ..." status line that comes and goes, so its top
      // moves by that line's height. Read every box in one evaluate (so no reflow falls between reads) and
      // measure the header from the sheet's top and Prev/Next from its bottom.
      const where = async () => {
        const handles = await Promise.all([questions, dot, count, prev, next].map((l) => l.elementHandle()));
        return page.evaluate(([sheet, ...els]) => {
          const s = (sheet as Element).getBoundingClientRect();
          const [dot, count, prev, next] = els.map((e) => (e as Element).getBoundingClientRect());
          const fromTop = (r: DOMRect) => ({ x: r.x, y: r.y - s.y, w: r.width, h: r.height });
          const fromBottom = (r: DOMRect) => ({ x: r.x, y: s.bottom - r.y, w: r.width, h: r.height });
          return { dot: fromTop(dot), count: fromTop(count), prev: fromBottom(prev), next: fromBottom(next) };
        }, handles);
      };
      await body.evaluate((el) => (el.scrollTop = 0));
      const before = await where();
      await toEnd();
      expect(await where()).toEqual(before);
      for (const el of [dot, count, prev, next]) await expect(el).toBeInViewport();

      // A move starts at the question's title.
      await body.evaluate((el) => (el.scrollTop = 0));
      await toEnd();
      await dot8b.click();
      await expectTitleAtTop();

      await dot.click();
      await toEnd();
      await next.click();
      await expectTitleAtTop();

      await dot.click();
      await toEnd();
      await questions.getByRole("button", { name: /SQLite/ }).click();
      await expectTitleAtTop();

      await dot.click();
      await toEnd();
      const heightBefore = (await composer.boundingBox())!.height;
      await send.click();
      await expect(popover).toBeVisible();
      // The popover sits above Send and ends at its right edge, and the composer keeps its height.
      // It scales in, so it is measured once settled.
      expect((await composer.boundingBox())!.height).toBe(heightBefore);
      await popover.evaluate((el) => Promise.all(el.getAnimations().map((a) => a.finished)));
      const pop = (await popover.boundingBox())!;
      const btn = (await send.boundingBox())!;
      expect(pop.y + pop.height).toBeLessThan(btn.y);
      expect(Math.abs(pop.x + pop.width - (btn.x + btn.width))).toBeLessThanOrEqual(2);
      await popover.getByRole("button", { name: /^Q8b Migrate at startup\? ›$/ }).click();
      await expectTitleAtTop();
    };

    await pass();
  });
});

// The font-size tokens are CSS variables that jsdom does not resolve, so this stays a real-engine flow.
test.describe("75. Text size", () => {
  test("chat, prompt, choice, titles, body and meta text", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    // Questions view.
    const questions = page.getByRole("group", nav("Questions"));
    await expect(questions.getByText("Q8 - Which store?")).toBeVisible();
    expect(await fontSize(questions.getByText("Q8 - Which store?"))).toBe("24px");
    expect(await fontSize(questions.getByText("SQLite or files?"))).toBe("17px");
    expect(await fontSize(questions.getByText("0 of 2 answered"))).toBe("13px");
    const choice = questions.getByRole("button", { name: /SQLite/ });
    expect(await fontSize(choice.locator(":scope > *").first())).toBe("16px");

    // Send a round with a note, so the chat has a prompt.
    await page.getByRole("button", nav("Note")).click();
    await page.getByRole("textbox", nav("Message")).fill("prompt size probe");
    await choice.click();
    await questions.getByRole("button", { name: /Yes/ }).click();
    await page.getByRole("button", nav("Send answers")).click();

    // Chat view: the message and the prompt. Send already shows the chat.
    await expect(page.getByText("Two questions before the spec.")).toBeVisible();
    expect(await fontSize(page.getByText("Two questions before the spec."))).toBe("16px");
    expect(await fontSize(page.getByText("prompt size probe"))).toBe("16px");
  });
});
