import type { Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";

const goto = (page: Page, hash: string) => page.evaluate((h) => (window.location.hash = h), hash);

const menu = (page: Page) => page.getByRole("dialog", nav("Jump to epic"));
const filter = (page: Page) => menu(page).getByRole("combobox");
const row = (page: Page, name: string) => menu(page).getByRole("option").filter({ has: page.getByText(name, { exact: true }) });

test("20. Jump menu opens and closes", async ({ page, start }) => {
  await start("united");
  await goto(page, "#/e/demo:running-epic");
  const title = page.getByRole("heading", { level: 1 }).getByRole("button");
  await expect(title).toContainText("running-epic");

  await title.click();
  await expect(filter(page)).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(menu(page)).toHaveCount(0);

  await page.keyboard.press("Control+k");
  await expect(filter(page)).toBeFocused();

  // Ready to design and Finished rows show only once the filter has text.
  await expect(row(page, "draft-epic")).toHaveCount(0);
  await expect(row(page, "finished-a")).toHaveCount(0);
  await expect(row(page, "paused-epic")).toBeVisible();
  await filter(page).fill("draft-epic");
  await expect(row(page, "draft-epic")).toBeVisible();
  await filter(page).fill("finished-a");
  await expect(row(page, "finished-a")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(menu(page)).toHaveCount(0);
});

test("21. Jump menu keyboard and picks", async ({ page, start }) => {
  await start("united");
  await goto(page, "#/e/demo:running-epic");
  const title = page.getByRole("heading", { level: 1 }).getByRole("button");

  // Down moves the cursor to the second row and Enter opens it.
  await title.click();
  await filter(page).fill("finished");
  const second = menu(page).getByRole("option").nth(1);
  const name = (await second.innerText()).split("\n")[0];
  await filter(page).press("ArrowDown");
  await expect(second).toHaveAttribute("aria-selected", "true");
  await filter(page).press("Enter");
  await expect(page).toHaveURL(new RegExp(`#/e/[a-z]+:${name}$`));
  await expect(menu(page)).toHaveCount(0);
  await expect(title).toContainText(name);

  // A Design opens its Design page, a loop epic its epic page, a one-off its ticket page.
  const picks: Array<[string, RegExp]> = [
    ["design-live", /#\/d\/demo:design-live$/],
    ["running-epic", /#\/e\/demo:running-epic$/],
    ["queued", /#\/t\/demo:one-offs\/02$/],
  ];
  for (const [pick, url] of picks) {
    await page.keyboard.press("Control+k");
    await filter(page).fill(pick);
    await row(page, pick).click();
    await expect(page).toHaveURL(url);
    // The shortcut works once the new page has mounted its menu.
    if (pick !== "queued") await expect(title).toContainText(pick);
  }
});

test("22. Jump menu count and back link", async ({ page, start }) => {
  await start("united");
  const waiting = page.getByTestId("waiting-count");

  // Seven other cards need you from a Needs-you epic, eight from one that does not.
  await goto(page, "#/e/demo:paused-epic");
  await expect(waiting).toHaveText("7");
  await goto(page, "#/e/demo:running-epic");
  await expect(waiting).toHaveText("8");

  await page.getByRole("link", nav("← Epics")).click();
  await expect(page).toHaveURL(/#\/epics$/);
  await goto(page, "#/d/demo:design-live");
  await expect(waiting).toHaveText("8");
  await page.getByRole("link", nav("← Epics")).click();
  await expect(page).toHaveURL(/#\/epics$/);
});
