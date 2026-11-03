import type { Page } from "@playwright/test";
import { card, expect, nav, test } from "./fixture";

const goto = (page: Page, hash: string) => page.evaluate((h) => (window.location.hash = h), hash);

test("22. Back link count, then the phone layout", async ({ page, start }) => {
  const fx = await start("united");
  const back = (n: number) => page.getByRole("link", nav(`Epics · ${n} need you`));

  // Eight other cards need you from a Needs-you epic, nine from one that does not.
  // Start on an epic outside Needs you: the permission card joins it only once its held worker asks,
  // so the full count of nine is the signal that the scenario has settled.
  await goto(page, "#/e/demo:running-epic");
  await expect(back(9)).toBeVisible({ timeout: 60_000 });
  await goto(page, "#/e/demo:paused-epic");
  await expect(back(8)).toBeVisible();
  await goto(page, "#/d/demo:design-live");
  await expect(back(9)).toBeVisible();
  await goto(page, "#/e/demo:running-epic");
  await expect(back(9)).toBeVisible();

  // The count is not the link's colour and sits on the waiting-tone pill (the mock's tab count).
  const style = await back(9).getByText("9", { exact: true }).evaluate((el) => {
    const cs = getComputedStyle(el);
    const link = getComputedStyle(el.closest("a")!);
    return { recolored: cs.color !== link.color, bg: cs.backgroundColor };
  });
  expect(style).toEqual({ recolored: true, bg: "rgb(242, 184, 69)" });

  // The title button carries no number and nothing is called waiting-count.
  await expect(page.getByRole("heading", { level: 1 }).getByRole("button")).not.toContainText(/\d/);
  await expect(page.getByTestId("waiting-count")).toHaveCount(0);

  // Clicking the count text itself opens the Epics page.
  await back(9).getByText("9", { exact: true }).click();
  await expect(page).toHaveURL(/#\/epics$/);

  // Phone layout on the same settled scenario. 375 px is below the wide layout's 62em breakpoint.
  await page.setViewportSize({ width: 375, height: 812 });

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
  await goto(page, "#/d/demo:design-live");
  const tab = back(9);
  await expect(tab).toBeVisible();
  const line = await tab.boundingBox();
  const lineHeight = await tab.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight));
  expect(line!.height).toBeLessThanOrEqual(lineHeight * 1.5);

  // On the epic page the buttons sit below the stats line.
  await goto(page, "#/e/demo:awaiting-epic");
  const merge = page.getByRole("button", nav("Merge"));
  await expect(merge).toBeVisible();
  const stats = (await page.getByTestId("stats-line").boundingBox())!;
  for (const button of await merge.locator("xpath=..").getByRole("button").all()) {
    const box = (await button.boundingBox())!;
    expect(box.y).toBeGreaterThanOrEqual(stats.y + stats.height);
    expect(box.height).toBeGreaterThanOrEqual(44);
  }

  // The count follows the server with no reload.
  await goto(page, "#/e/demo:running-epic");
  await expect(back(9)).toBeVisible();
  await fx.cli("tickets", "set", "demo:paused-epic/01", "--status", "open");
  await expect(back(8)).toBeVisible();
});
