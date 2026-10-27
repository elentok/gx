import type { Page } from "@playwright/test";
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
    await expect(menu.getByRole("button", nav("Stop design-epic"))).toHaveCount(0);
    const other = menu.getByRole("menuitem", { name: /^Claude Code/ });
    await expect(other).toHaveAttribute("aria-current", "false");
    await expect(menu.getByRole("button", nav("Stop Claude Code"))).toBeVisible();

    // Picking the other row shows it: it has no round, so the Questions sheet goes.
    await other.click();
    await expect(page.getByRole("group", nav("Questions"))).toHaveCount(0);

    await moreActions(page).click();
    await expect(menu.getByRole("menuitem", { name: /^Claude Code/ })).toHaveAttribute("aria-current", "true");
    const before = await sessionRows(fixture, "design-epic");
    expect(before.filter((r) => /exited/.test(r))).toHaveLength(0);
    await menu.getByRole("button", nav("Stop design-epic")).click();
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
    await moreActions(page).click();
    await expect(actionsMenu(page).getByTestId("grab-handle")).toHaveCount(0);
    // The positioner places the popover after it mounts, so wait for it to leave the origin.
    await expect.poll(async () => (await actionsMenu(page).boundingBox())!.y).toBeGreaterThanOrEqual(trigger.y + trigger.height);
    const box = (await actionsMenu(page).boundingBox())!;
    expect(box.x).toBeLessThan(trigger.x + trigger.width);
    expect(box.x + box.width).toBeGreaterThan(trigger.x);
  });
});
