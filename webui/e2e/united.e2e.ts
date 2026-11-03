import type { Page } from "@playwright/test";
import { card, expect, nav, section, test } from "./fixture";

const NEEDS_YOU = [
  "paused-epic",
  "question-epic",
  "perm-epic",
  "deadlock-epic",
  "blocked-merge-epic",
  "dirty-trunk-epic",
  "awaiting-epic",
  "design-question",
  "paused", // the one-off ticket
];

const menuButton = (page: Page) => page.getByRole("button", nav("Menu"));
const menu = (page: Page) => page.getByRole("menu", nav("Menu"));
const afkItem = (page: Page) => menu(page).getByRole("menuitemcheckbox", nav("AFK"));
const viModeItem = (page: Page) => menu(page).getByRole("menuitemcheckbox", nav("Vi mode"));

// Also covers 63 (card times: queue round trip), 28 (menu AFK / Vi mode over SSE, Paused chip), 29 (grid
// columns at desktop width) and 30 (Open epic on a deadlock card). Flows 15, 17, 19, 31, 32, 33, 51 and 52
// moved to vitest (routes.test, EpicCard.test, TopBar.test, EpicsPage.test, EpicPage.test).
test("16. Needs you (+63, 28, 29, 30)", async ({ page, start }) => {
  await page.setViewportSize({ width: 1400, height: 900 });
  const fixture = await start("united");
  await expect(page.getByRole("heading", { level: 2 })).toHaveCount(6);
  const titles = await page.getByRole("heading", { level: 2 }).allTextContents();
  expect(titles.map((t) => t.replace(/[▾▸\d]/g, "").trim())).toEqual([
    "Needs you",
    "Implementing",
    "Ready to queue",
    "Designing",
    "Ready to design",
    "Finished",
  ]);

  // 29: two cards share a row at desktop width.
  const firstCard = await card(page, "paused-epic").boundingBox();
  const secondCard = await card(page, "question-epic").boundingBox();
  expect(secondCard!.y).toBe(firstCard!.y);
  expect(secondCard!.x).toBeGreaterThan(firstCard!.x);

  // Ready to design and Finished start collapsed.
  const design = page.getByRole("region", { name: /^Ready to design/ });
  const designHeader = design.getByRole("button", { name: /^Ready to design/ });
  const finishedHeader = page.getByRole("region", { name: /^Finished/ }).getByRole("button", { name: /^Finished/ });
  await expect(designHeader).toHaveAttribute("aria-expanded", "false");
  await expect(finishedHeader).toHaveAttribute("aria-expanded", "false");
  await designHeader.click();
  await expect(designHeader).toHaveAttribute("aria-expanded", "true");
  await expect(design.getByRole("link", { name: "draft-epic", exact: true })).toBeVisible();
  await designHeader.click();
  await expect(designHeader).toHaveAttribute("aria-expanded", "false");

  // Each title counts its cards; Finished counts the cards on screen.
  const needs = await section(page, "Needs you");
  // The permission card joins Needs you only once its held worker asks.
  await expect(needs.locator("[data-section]")).toHaveCount(NEEDS_YOU.length, { timeout: 60_000 });
  await expect(needs.getByRole("button", { name: new RegExp(`${NEEDS_YOU.length}$`) })).toBeVisible();
  const fin = await section(page, "Finished");
  const shown = await fin.locator("[data-section]").count();
  await expect(fin.getByRole("button", { name: new RegExp(`${shown}$`) })).toBeVisible();

  // Every Needs-you reason sits in Needs you; pills and reasons show only there.
  // A stopped Design waits on no one, so it sits in Designing with Continue.
  for (const name of NEEDS_YOU) {
    await expect(needs.getByRole("link", { name, exact: true })).toBeVisible();
    await expect(card(page, name).getByTestId("pill")).toBeVisible();
  }
  await expect(page.getByTestId("pill")).toHaveCount(NEEDS_YOU.length);
  const progress = await section(page, "Designing");
  await expect(progress.getByTestId("pill")).toHaveCount(0);
  await expect(progress.getByRole("link", { name: "design-stopped", exact: true })).toBeVisible();
  await expect(card(page, "design-stopped").getByRole("button")).toHaveText(["Continue"]);
  const queue = await section(page, "Ready to queue");
  await expect(queue.getByTestId("pill")).toHaveCount(0);

  // A click outside the title does not navigate; the title does.
  await card(page, "paused-epic").getByText("demo", { exact: true }).click();
  await expect(page).toHaveURL(/#\/epics$/);
  await card(page, "paused-epic").getByRole("link", { name: "paused-epic", exact: true }).click();
  await expect(page).toHaveURL(/#\/e\/demo:paused-epic$/);
  await page.getByRole("link", { name: /^Epics/ }).click();
  await expect(page).toHaveURL(/#\/epics$/);

  // Only merge-blocked and awaiting-merge cards offer Merge.
  for (const name of ["blocked-merge-epic", "awaiting-epic"]) {
    await expect(card(page, name).getByRole("button")).toHaveText(["Merge"]);
  }
  // A dirty trunk offers both ways out and names the cause, but lists no files on the card.
  const dirty = card(page, "dirty-trunk-epic");
  await expect(dirty.getByRole("button")).toHaveText(["Stash, merge and restore", "Merge again"]);
  await expect(dirty.getByTestId("pill")).toHaveText("Merge blocked");
  await expect(dirty.getByTestId("reason")).toHaveText("Trunk has local changes");
  await expect(dirty.getByTestId("merge-block-files")).toHaveCount(0);
  for (const name of ["running-epic", "queued-epic"]) {
    await expect(card(page, name).getByRole("button")).toHaveCount(0);
  }
  await expect(card(page, "finished-a").getByRole("button")).toHaveCount(0);
  const ready = await section(page, "Ready to design");
  await expect(ready.locator("[data-kind]").filter({ has: page.getByRole("link", { name: "draft-epic", exact: true }) }).getByRole("button")).toHaveCount(0);

  // The filter matches names across sections, and says so when nothing matches.
  const filter = page.getByRole("searchbox", { name: "Filter epics" });
  await filter.fill("perm-epic");
  await expect(card(page, "perm-epic")).toBeVisible();
  await expect(card(page, "paused-epic")).toHaveCount(0);
  await filter.fill("nothing-like-this");
  await expect(page.getByText("No match")).toBeVisible();
  await filter.fill("");

  // 63: a queued epic times from its latest queueing. Format logic is unit-tested (cardTime.test).
  const queuedTime = card(page, "queued-epic").getByTestId("time");
  await fixture.cli("server", "queue", "remove", "demo:queued-epic");
  await fixture.cli("server", "queue", "add", "demo:queued-epic");
  await expect(queuedTime).toHaveText("<1m");

  // 28: AFK and Vi mode flip over SSE; the Paused chip follows `queue pause`.
  const bar = page.getByRole("navigation");
  const paused = bar.getByText("Paused", { exact: true });
  await expect(paused).toHaveCount(0);
  await menuButton(page).click();
  await expect(afkItem(page)).toHaveAttribute("aria-checked", "false");
  await afkItem(page).click();
  await expect(afkItem(page)).toHaveAttribute("aria-checked", "true");
  await expect(menu(page)).toBeVisible();
  // `gx afk` with no argument flips the server state, so "afk off" shows the click turned it on.
  expect(await fixture.cli("afk")).toContain("afk off");
  await expect(afkItem(page)).toHaveAttribute("aria-checked", "false");
  await expect(viModeItem(page)).toHaveAttribute("aria-checked", "false");
  await viModeItem(page).click();
  await expect(viModeItem(page)).toHaveAttribute("aria-checked", "true");
  expect(await fixture.cli("vimode")).toContain("vimode off");
  await expect(viModeItem(page)).toHaveAttribute("aria-checked", "false");
  await page.keyboard.press("Escape");
  await expect(menu(page)).toHaveCount(0);
  await fixture.cli("server", "queue", "pause");
  await expect(paused).toBeVisible();

  // 30: Open epic on a deadlock card navigates to its page.
  await card(page, "deadlock-epic").getByRole("button", nav("Open epic")).click();
  await expect(page).toHaveURL(/#\/e\/demo:deadlock-epic$/);
});

test("18. Joined card", async ({ page, start }) => {
  const fixture = await start("design", { hold: true });
  const joined = card(page, "design-epic");
  await expect(page.getByRole("link", { name: "design-epic", exact: true })).toHaveCount(1);
  await expect(page.getByRole("link", { name: "design-epic-impl", exact: true })).toHaveCount(0);
  await expect(joined.getByTestId("status")).toHaveText("Designing");
  await expect(joined.getByText(/%$/)).toBeVisible();
  const bare = /^(<1m|\d+[mhdw]( \d+[mhd])?)$/;
  await expect(joined.getByTestId("time")).toHaveText(bare);
  await expect(joined.getByRole("link", { name: "design-epic", exact: true })).toHaveAttribute("href", "#/e/demo:design-epic");

  await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", "create-tickets");
  await expect(joined.getByTestId("status")).toHaveText(/^Creating tickets/);
  await expect(joined.getByText("…")).toBeVisible();

  await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", "branch");
  await expect(joined.getByTestId("status")).toHaveText("Not started");
  await expect(joined.getByTestId("time")).toHaveText(/^created .+ ago$/);
  await fixture.cli("server", "queue", "add", "demo:design-epic-impl");
  await expect(joined.getByTestId("status")).toHaveText("Implementing...");
  await expect(joined.getByTestId("time")).toHaveText(bare);
  await expect(joined.getByText(/^\d+\/\d+$/)).toBeVisible();
  await expect(joined.getByRole("link", { name: "design-epic", exact: true })).toHaveAttribute("href", "#/e/demo:design-epic-impl");

  // The two epic pages link to each other.
  await joined.getByRole("link", { name: "design-epic", exact: true }).click();
  await expect(page).toHaveURL(/#\/e\/demo:design-epic-impl$/);
  await page.getByRole("button", { name: "Open design", exact: true }).click();
  await expect(page).toHaveURL(/#\/e\/demo:design-epic$/);
  await page.getByRole("button", { name: "Open implementation epic", exact: true }).click();
  await expect(page).toHaveURL(/#\/e\/demo:design-epic-impl$/);
});
