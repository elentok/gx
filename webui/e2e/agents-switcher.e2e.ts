import type { Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { expect, nav, test } from "./fixture";
import { go, isActionable, row, sessions, switcher, switcherFilter, titleButton } from "./agentsKit";

const switcherRows = (page: Page) => switcher(page).getByRole("option");

/** The switcher's order: Actionable, then Unread, then the rest, each in list order. */
const switcherOrder = (list: SessionInfo[]) => {
  const bucket = (s: SessionInfo) => (isActionable(s) ? 0 : s.unread.unread ? 1 : 2);
  return [0, 1, 2].flatMap((b) => list.filter((s) => bucket(s) === b));
};

test("84. Session switcher", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const idle = list[0];
  const design = list.find((s) => s.design)!;
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);

  // The title button and Cmd-K both open it, on the filter.
  await titleButton(page).click();
  await expect(switcherFilter(page)).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(switcher(page)).toHaveCount(0);
  await page.keyboard.press("Control+k");
  await expect(switcherFilter(page)).toBeFocused();

  // Sections, the current tag, and design sessions listed while the sidebar hides them.
  for (const title of ["Actionable", "Unread", "Other"]) await expect(switcher(page).getByRole("group", { name: title })).toBeVisible();
  const order = switcherOrder(list);
  await expect(switcherRows(page)).toHaveCount(list.length + 1);
  const at = order.indexOf(idle);
  await expect(switcherRows(page).nth(at)).toHaveAttribute("aria-current", "true");
  await expect(switcherRows(page).nth(at).getByText("current")).toBeVisible();
  await expect(switcherRows(page).filter({ has: page.getByText("current", { exact: true }) })).toHaveCount(1);
  await expect(row(page, design)).toHaveCount(0);
  await expect(switcherRows(page).nth(order.indexOf(design)).getByRole("img", { name: "Design" })).toBeVisible();

  // The filter matches the ticket and the dir; the New session row stays.
  const withTicket = list.find((s) => s.ticket)!;
  const ticketCount = list.filter((s) => s.ticket === withTicket.ticket).length;
  await switcherFilter(page).fill(withTicket.ticket!);
  await expect(switcherRows(page)).toHaveCount(ticketCount + 1);
  await expect(switcherRows(page).last()).toContainText("+ New session");
  const other = switcherOrder(list.filter((s) => s.project === "other"))[0];
  await switcherFilter(page).fill(other.dir);
  await expect(switcherRows(page)).toHaveCount(list.filter((s) => s.dir === other.dir).length + 1);

  // Picking a row opens the session and closes the switcher.
  await switcherRows(page).first().click();
  await expect(switcher(page)).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`#/agents/${other.id}$`));
  await expect(row(page, other)).toHaveAttribute("aria-current", "page");

  // Escape closes it.
  await page.keyboard.press("Control+k");
  await expect(switcher(page)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(switcher(page)).toHaveCount(0);

  // An epic's Design page keeps its own jump menu on the same key.
  await go(page, "#/d/demo:design-epic");
  // The jump menu's key listener mounts after the route changes, so press again until it opens.
  await expect(async () => {
    await page.keyboard.press("Control+k");
    await expect(page.getByRole("dialog", nav("Jump to epic"))).toBeVisible({ timeout: 500 });
  }).toPass();
  await expect(switcher(page)).toHaveCount(0);
});
