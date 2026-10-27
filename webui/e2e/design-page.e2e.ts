import type { Locator, Page } from "@playwright/test";
import { card, expect, nav, sessionRows, test } from "./fixture";

const openDesign = async (page: Page) => {
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await expect(page.getByRole("button", nav("Start design session"))).toBeVisible();
};

// The Design page ring: a Design badge (role img) inside the element that carries data-tone.
const ring = (page: Page) => page.locator("[data-kind='design']").first();

test.describe("64. Design ring", () => {
  for (const width of [390, 1440]) {
    test(`${width}px: the ring sits left of the name and tracks the session`, async ({ page, start }) => {
      const fixture = await start("design");
      await page.setViewportSize({ width, height: 900 });

      // The Epic page ring is the reference for the label.
      await expect(card(page, "design-epic")).toBeVisible();
      await page.evaluate(() => (location.hash = "#/e/demo:design-epic"));
      const epicLabel = (await ring(page).innerText()).trim();
      expect(epicLabel).toMatch(/%$/);

      await openDesign(page);
      const badge = ring(page);
      await expect(badge.getByRole("img", nav("Design"))).toBeVisible();
      const ringBox = (await badge.boundingBox())!;
      const nameBox = (await page.getByRole("heading", { level: 1 }).boundingBox())!;
      expect(ringBox.x + ringBox.width).toBeLessThanOrEqual(nameBox.x);
      expect(Math.abs(ringBox.y + ringBox.height / 2 - (nameBox.y + nameBox.height / 2))).toBeLessThan(nameBox.height);
      await expect(badge).toContainText(epicLabel);
      await expect(badge).toHaveAttribute("data-tone", "muted");

      // Round one up: waiting, no comet.
      await page.getByRole("button", nav("Start design session")).click();
      await expect(page.getByRole("group", nav("Questions"))).toBeVisible();
      await expect(badge).toHaveAttribute("data-tone", "waiting");
      await expect(page.getByTestId("comet")).toHaveCount(0);

      await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", "done");
      await expect(badge).toHaveAttribute("data-tone", "done");
    });
  }

  test("the ring runs with the comet while the session works", async ({ page, start }) => {
    await start("design-working");
    await expect(card(page, "design-epic")).toBeVisible();
    await openDesign(page);
    await expect(ring(page)).toHaveAttribute("data-tone", "muted");
    await page.getByRole("button", nav("Start design session")).click();
    await expect(ring(page)).toHaveAttribute("data-tone", "running");
    await expect(page.getByTestId("comet")).toBeVisible();
  });
});

const moreActions = (page: Page) => page.getByRole("button", nav("More actions"));
const actionsMenu = (page: Page) => page.getByRole("menu", nav("More actions"));

test.describe("65. Design actions menu", () => {
  test("one live session: New session lives in the menu only", async ({ page, start }) => {
    await start("design");
    await expect(card(page, "design-epic")).toBeVisible();
    await openDesign(page);
    await expect(moreActions(page)).toHaveCount(0);

    await page.getByRole("button", nav("Start design session")).click();
    await expect(page.getByRole("group", nav("Questions"))).toBeVisible();
    await expect(page.getByRole("button", { name: /^New session/ })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /\d+ live/ })).toHaveCount(0);

    await moreActions(page).click();
    const item = actionsMenu(page).getByRole("menuitem", { name: /^New session/ });
    await expect(item).toHaveAccessibleName("New session Stop this session and start a fresh one");
    await expect(actionsMenu(page).getByText(/^Live sessions/)).toHaveCount(0);
  });

  test("two live sessions: the list shows the shown one, switches and stops", async ({ page, start }) => {
    const fixture = await start("design-two-live");
    await expect(card(page, "design-epic")).toBeVisible();
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
    await expect(page.getByRole("group", nav("Questions"))).toBeVisible();

    await moreActions(page).click();
    const menu = actionsMenu(page);
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

  test("390px: the menu is a bottom sheet with a grab handle", async ({ page, start }) => {
    await start("design-two-live");
    await page.setViewportSize({ width: 375, height: 800 });
    await expect(card(page, "design-epic")).toBeVisible();
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
    await moreActions(page).click();
    const menu = actionsMenu(page);
    await expect(menu.getByTestId("grab-handle")).toBeVisible();
    const box = (await menu.boundingBox())!;
    expect(box.width).toBe(375);
    expect(box.y + box.height).toBeGreaterThan(780);
  });

  test("1440px: the menu is a popover under ⋯", async ({ page, start }) => {
    await start("design-two-live");
    await page.setViewportSize({ width: 1440, height: 900 });
    await expect(card(page, "design-epic")).toBeVisible();
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
    const trigger = (await moreActions(page).boundingBox())!;
    expect(trigger.width).toBe(44);
    expect(trigger.height).toBe(44);
    await expect(moreActions(page)).toHaveCSS("border-top-width", "1px");
    // Level with the title row, not below it: the Progress sidebar starts under the header.
    const title = (await page.getByRole("heading", { level: 1 }).first().boundingBox())!;
    expect(trigger.y).toBeLessThan(title.y + title.height);
    await moreActions(page).click();
    await expect(actionsMenu(page).getByTestId("grab-handle")).toHaveCount(0);
    // The positioner places the popover after it mounts, so wait for it to leave the origin.
    await expect.poll(async () => (await actionsMenu(page).boundingBox())!.y).toBeGreaterThanOrEqual(trigger.y + trigger.height);
    const box = (await actionsMenu(page).boundingBox())!;
    expect(box.x).toBeLessThan(trigger.x + trigger.width);
    expect(box.x + box.width).toBeGreaterThan(trigger.x);
  });
});

const expandNote = (scope: Locator | Page) => scope.getByRole("button", nav("Open the note in a large editor"));

test.describe("66. Note box", () => {
  test("rows, viewport, large editor, Done and Send", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "1h" });
    await page.setViewportSize({ width: 375, height: 700 });
    await expect(card(page, "design-epic")).toBeVisible();
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    // The Q8 note: in the viewport, with the expand button and a Q8 modal.
    const questions = page.getByRole("group", nav("Questions"));
    await questions.getByRole("button", nav("Note on question 8")).click();
    const qNote = questions.getByRole("textbox", nav("Note text for question 8"));
    await expect(qNote).toBeInViewport();
    await expandNote(questions).click();
    await expect(page.getByRole("dialog", { name: /Q8/ })).toBeVisible();
    await page.getByRole("button", nav("Done")).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);

    // The composer note: 5 rows, expand button, "Note on this round", Done keeps the text.
    await page.getByRole("button", nav("Note")).click();
    const message = page.getByRole("textbox", nav("Message"));
    await expect(message).toHaveAttribute("rows", "5");
    await expandNote(page.getByRole("form", nav("Composer"))).click();
    await expect(page.getByRole("dialog", nav("Note on this round"))).toBeVisible();
    await page.getByRole("textbox", nav("Note text in the large editor")).fill("typed in the modal");
    await page.getByRole("button", nav("Done")).click();
    await expect(message).toHaveValue("typed in the modal");

    await questions.getByRole("button", { name: /SQLite/ }).click();
    await questions.getByRole("button", { name: /Yes/ }).click();
    await page.getByRole("button", nav("Send answers")).click();
    await expect(page.getByText("typed in the modal")).toBeVisible();
  });

  test("no round: the message box has 5 rows and the expand button", async ({ page, start }) => {
    await start("design-working");
    await expect(card(page, "design-epic")).toBeVisible();
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();
    await page.getByRole("button", nav("Note")).click();
    const message = page.getByRole("textbox", nav("Message"));
    await expect(message).toHaveAttribute("rows", "5");
    await expandNote(page).click();
    await expect(page.getByRole("dialog", nav("Message to the agent"))).toBeVisible();
  });
});

const showChat = (page: Page) => page.getByRole("group", nav("View")).getByRole("button", nav("Chat")).click();

const fontSize = (el: Locator) => el.evaluate((node) => getComputedStyle(node).fontSize);

test.describe("67. Question top", () => {
  test.use({ viewport: { width: 1280, height: 600 } });

  for (const mode of ["Questions view", "docked in the Chat view"]) {
    test(`a move starts at the question's title, ${mode}`, async ({ page, start }) => {
      await start("design", { autoAdvanceDelay: "1h" });
      await openDesign(page);
      await page.getByRole("button", nav("Start design session")).click();

      const questions = page.getByRole("group", nav("Questions"));
      await expect(questions.getByText("Q8 - Which store?")).toBeVisible();
      if (mode !== "Questions view") await showChat(page);
      // An open note makes the Q8 body taller than the sheet at this height.
      await questions.getByRole("button", nav("Note on question 8")).click();
      await expect(questions.getByRole("textbox", nav("Note text for question 8"))).toBeVisible();

      const body = questions.getByTestId("question-body");
      const title = questions.getByText("Q8b - Migrate at startup?");
      const toEnd = async () => {
        await body.evaluate((el) => (el.scrollTop = el.scrollHeight));
        expect(await body.evaluate((el) => el.scrollTop), "the Q8 body must scroll").toBeGreaterThan(0);
      };
      const expectTitleAtTop = async () => {
        await expect(questions.getByRole("button", nav("Question 8b"))).toHaveAttribute("aria-current", "step");
        await expect
          .poll(async () => Math.abs((await title.boundingBox())!.y - (await body.boundingBox())!.y))
          .toBeLessThanOrEqual(4);
      };
      const back = () => questions.getByRole("button", { name: /^Question 8(, answered)?$/ }).click();

      await toEnd();
      await questions.getByRole("button", nav("Question 8b")).click();
      await expectTitleAtTop();

      await back();
      await toEnd();
      await questions.getByRole("button", nav("Next →")).click();
      await expectTitleAtTop();

      await back();
      await toEnd();
      await questions.getByRole("button", { name: /SQLite/ }).click();
      await expectTitleAtTop();

      await back();
      await toEnd();
      await page.getByRole("button", nav("Send answers")).click();
      await page
        .getByRole("alertdialog", nav("Unanswered questions"))
        .getByRole("button", { name: /^Q8b Migrate at startup\? ›$/ })
        .click();
      await expectTitleAtTop();
    });
  }
});

test.describe("68. Text size", () => {
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

    // Chat view: the message, the prompt and the docked title (the next round, Q9).
    // Continue starts the next round; under load the first press can land before the run ends, so press again.
    const cont = page.getByRole("button", { name: /^Continue/ });
    await expect(async () => {
      await cont.click({ timeout: 2000 });
      await expect(questions.getByText("Q9 - Keep going?")).toBeVisible({ timeout: 2000 });
    }).toPass();
    // The new round can reopen the Questions view just after Q9 shows, so press Chat until it sticks.
    await expect(async () => {
      await showChat(page);
      await expect(page.getByText("Two questions before the spec.")).toBeVisible({ timeout: 2000 });
    }).toPass();
    expect(await fontSize(page.getByText("Two questions before the spec."))).toBe("16px");
    expect(await fontSize(page.getByText("prompt size probe"))).toBe("16px");
    expect(await fontSize(questions.getByText("Q9 - Keep going?"))).toBe("18px");
  });
});
