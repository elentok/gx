import type { Page } from "@playwright/test";
import { card, expect, nav, test } from "./fixture";

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
