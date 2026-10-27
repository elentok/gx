import type { Page } from "@playwright/test";
import { card, expect, holdRoute, nav, section, test, toasts } from "./fixture";

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

test("15. Redirects", async ({ page, start }) => {
  await start("ready");
  for (const hash of ["#/", "#/q", "#/designs", "#/nowhere/at/all"]) {
    await page.evaluate((h) => (window.location.hash = h), hash);
    await expect(page).toHaveURL(/#\/epics$/);
    await expect(page.getByRole("region", { name: /^Ready to queue/ })).toBeVisible();
  }
  const tabs = page.getByRole("navigation");
  await expect(tabs.getByRole("link", nav("Epics"))).toBeVisible();
  await expect(tabs.getByRole("link", nav("Stats"))).toBeVisible();
  await expect(tabs.getByRole("link", nav("Queue"))).toHaveCount(0);
  await expect(tabs.getByRole("link", nav("Designs"))).toHaveCount(0);
});

test("16. Needs you", async ({ page, start }) => {
  await start("united");
  const titles = await page.getByRole("heading", { level: 2 }).allTextContents();
  expect(titles.map((t) => t.replace(/[▾▸\d]/g, "").trim())).toEqual([
    "Needs you",
    "Ready to queue",
    "In progress",
    "Ready to design",
    "Finished",
  ]);

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
  await expect(needs.locator("[data-section]")).toHaveCount(NEEDS_YOU.length);
  await expect(needs.getByRole("button", { name: new RegExp(`${NEEDS_YOU.length}$`) })).toBeVisible();
  const fin = await section(page, "Finished");
  const shown = await fin.locator("[data-section]").count();
  await expect(fin.getByRole("button", { name: new RegExp(`${shown}$`) })).toBeVisible();

  // Every Needs-you reason sits in Needs you; pills and reasons show only there.
  // A stopped Design waits on no one, so it sits in In progress with Continue.
  for (const name of NEEDS_YOU) {
    await expect(needs.getByRole("link", { name, exact: true })).toBeVisible();
    await expect(card(page, name).getByTestId("pill")).toBeVisible();
  }
  await expect(page.getByTestId("pill")).toHaveCount(NEEDS_YOU.length);
  const progress = await section(page, "In progress");
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
  await page.getByRole("link", { name: /^← Epics/ }).click();

  // Only merge-blocked and awaiting-merge cards offer Merge.
  for (const name of ["blocked-merge-epic", "awaiting-epic"]) {
    await expect(card(page, name).getByRole("button")).toHaveText(["Merge"]);
  }
  // A dirty trunk offers both ways out and names the cause, but lists no files on the card.
  const dirty = card(page, "dirty-trunk-epic");
  await expect(dirty.getByRole("button")).toHaveText(["Stash, merge and restore", "Merge again"]);
  await expect(dirty.getByTestId("pill")).toHaveText("Merge blocked");
  await expect(dirty.getByTestId("reason")).toHaveText("Trunk has local changes");
  await expect(dirty.getByTestId("land-block-files")).toHaveCount(0);
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
});

// The fixture's epics have no git branch, so the server refuses the merge on awaiting-epic. Every
// outcome is a toast on the card, with no navigation.
test("17. Merge from the card", async ({ page, start }) => {
  await start("united");
  await section(page, "Needs you");

  await card(page, "awaiting-epic").getByRole("button", nav("Merge")).click();
  await expect(toasts(page).filter({ hasText: /^no branch awaiting-epic in / })).toBeVisible();

  const release = await holdRoute(page, "**/v1/epics/merge");
  await card(page, "blocked-merge-epic").getByRole("button", nav("Merge")).click();
  await expect(card(page, "blocked-merge-epic").getByRole("button")).toBeDisabled();
  release();
  await expect(toasts(page).last()).toBeVisible();
  await expect(toasts(page)).toHaveCount(2);
  await expect(page).toHaveURL(/#\/epics$/);
});

test("18. Joined card", async ({ page, start }) => {
  const fixture = await start("design", { hold: true });
  const joined = card(page, "design-epic");
  await expect(page.getByRole("link", { name: "design-epic", exact: true })).toHaveCount(1);
  await expect(page.getByRole("link", { name: "design-epic-impl", exact: true })).toHaveCount(0);
  await expect(joined.getByTestId("status")).toHaveText("Designing");
  await expect(joined.getByText(/%$/)).toBeVisible();
  await expect(joined.getByRole("link", { name: "design-epic", exact: true })).toHaveAttribute("href", "#/e/demo:design-epic");

  await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", "create-tickets");
  await expect(joined.getByTestId("status")).toHaveText(/^Creating tickets/);
  await expect(joined.getByText("…")).toBeVisible();

  await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", "branch");
  await expect(joined.getByTestId("status")).toHaveText("Not started");
  await fixture.cli("server", "queue", "add", "demo:design-epic-impl");
  await expect(joined.getByTestId("status")).toHaveText("Implementing...");
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

test("19. Follow-ups triage", async ({ page, start }) => {
  await start("united");
  const ready = await section(page, "Ready to design");
  const first = ready.locator("[data-kind]").first();
  await expect(first.getByRole("link", { name: "follow-ups", exact: true })).toBeVisible();
  await expect(first.getByText("demo", { exact: true })).toBeVisible();
  await expect(first.getByTestId("status")).toHaveText("1 bug, 1 idea");

  await first.getByRole("button", nav("Triage")).click();
  const dialog = page.getByRole("dialog", { name: "Triage" });
  await expect(dialog.getByText("/gx-triage-follow-ups --project demo")).toBeVisible();
  await expect(dialog.getByRole("button", nav("Copy"))).toBeVisible();
  await dialog.getByRole("button", nav("Close")).click();
  await expect(dialog).toHaveCount(0);

  await first.getByRole("link", { name: "follow-ups", exact: true }).click();
  await expect(page).toHaveURL(/#\/e\/demo:follow-ups$/);
});

const menuButton = (page: Page) => page.getByRole("button", nav("Menu"));
const menu = (page: Page) => page.getByRole("menu", nav("Menu"));
const afkItem = (page: Page) => menu(page).getByRole("menuitemcheckbox", nav("AFK"));

test("28. Menu: AFK, build text, Paused chip", async ({ page, start }) => {
  const fixture = await start("united");
  const bar = page.getByRole("navigation");
  const paused = bar.getByText("Paused", { exact: true });
  await expect(paused).toHaveCount(0);

  // The hamburger sits right of the Live dot.
  const live = await page.getByRole("status", nav("Live")).boundingBox();
  const button = await menuButton(page).boundingBox();
  expect(button!.x).toBeGreaterThan(live!.x);

  await menuButton(page).click();
  await expect(afkItem(page)).toHaveAttribute("aria-checked", "false");
  await afkItem(page).click();
  await expect(afkItem(page)).toHaveAttribute("aria-checked", "true");
  await expect(menu(page)).toBeVisible();
  // `gx afk` with no argument flips the server state, so "afk off" shows the click turned it on.
  expect(await fixture.cli("afk")).toContain("afk off");
  await expect(afkItem(page)).toHaveAttribute("aria-checked", "false");

  // The build text shows and does nothing.
  const build = menu(page).getByText(/^v\d/);
  await expect(build).toBeVisible();
  await build.click();
  await expect(menu(page)).toBeVisible();
  await expect(page).toHaveURL(/#\/epics$/);

  // With AFK on and the menu closed, the bar has no AFK.
  await afkItem(page).click();
  await expect(afkItem(page)).toHaveAttribute("aria-checked", "true");
  await page.keyboard.press("Escape");
  await expect(menu(page)).toHaveCount(0);
  await expect(bar.getByText("AFK")).toHaveCount(0);

  await fixture.cli("server", "queue", "pause");
  await expect(paused).toBeVisible();
});

test("31. Menu keyboard", async ({ page, start }) => {
  const fixture = await start("united");
  await menuButton(page).focus();
  await page.keyboard.press("Enter");
  await expect(menu(page)).toBeVisible();
  await page.keyboard.press("ArrowDown");
  await expect(afkItem(page)).toHaveAttribute("data-highlighted", "");
  await page.keyboard.press("Enter");
  await expect(afkItem(page)).toHaveAttribute("aria-checked", "true");
  await expect(menu(page)).toBeVisible();
  expect(await fixture.cli("afk")).toContain("afk off");
  await page.keyboard.press("Escape");
  await expect(menu(page)).toHaveCount(0);
});

test("32. Epics project filter", async ({ page, start }) => {
  await start("united");
  const select = page.getByRole("combobox", { name: "Project" });
  await expect(select.locator("option")).toHaveText(["All projects", "demo", "auto"]);
  const search = page.getByRole("searchbox", { name: "Filter epics" });
  const first = await search.boundingBox();
  const picker = await select.boundingBox();
  expect(picker!.x).toBeGreaterThan(first!.x);

  await select.selectOption({ label: "demo" });
  // Open every collapsed section, so the follow-ups card and the Finished cards count too.
  await section(page, "Ready to design");
  await section(page, "Finished");
  const cards = page.locator("[data-section]");
  const shown = await cards.count();
  expect(shown).toBeGreaterThan(0);
  await expect(cards.getByText("demo", { exact: true })).toHaveCount(shown);
  await expect(cards.getByText("auto", { exact: true })).toHaveCount(0);
  await expect(card(page, "follow-ups")).toBeVisible();

  // A name from the other project matches nothing.
  await search.fill("awaiting-auto-epic");
  await expect(page.getByText("No match")).toBeVisible();
  await search.fill("");

  // A second page in the same context reads the stored choice.
  const other = await page.context().newPage();
  await other.goto(new URL(page.url()).origin + "/#/epics");
  await expect(other.getByRole("combobox", { name: "Project" })).toHaveValue("demo");
  await other.close();
});

test("33. No project filter with one project", async ({ page, start }) => {
  await start("ready");
  await expect(page.getByRole("searchbox", { name: "Filter epics" })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Project" })).toHaveCount(0);
});

test("29. Grid columns at desktop width", async ({ page, start }) => {
  await page.setViewportSize({ width: 1400, height: 900 });
  await start("united");
  const first = await card(page, "paused-epic").boundingBox();
  const second = await card(page, "question-epic").boundingBox();
  expect(second!.y).toBe(first!.y);
  expect(second!.x).toBeGreaterThan(first!.x);
});

test("30. Open epic on a deadlock card", async ({ page, start }) => {
  await start("united");
  await card(page, "deadlock-epic").getByRole("button", nav("Open epic")).click();
  await expect(page).toHaveURL(/#\/e\/demo:deadlock-epic$/);
});

test("51. Merge from the epic page", async ({ page, start }) => {
  await start("united");

  // An awaiting-merge epic: Merge is the last button of the group, in the done tone.
  await page.evaluate(() => (location.hash = "#/e/demo:awaiting-epic"));
  const merge = page.getByRole("button", nav("Merge"));
  await expect(merge).toBeVisible();
  await expect(merge).toHaveAttribute("data-tone", "done");
  await expect(merge.locator("xpath=..").getByRole("button").last()).toHaveText("Merge");

  // A land-blocked epic shows the reason under the bar. Merge stays, as the retry: isMergeable allows a
  // paused epic with a land-block reason.
  await page.evaluate(() => (location.hash = "#/e/demo:blocked-merge-epic"));
  // The notice is the icon and the text block that holds the reason and the buttons.
  const notice = page.locator("[data-reason]").locator("xpath=../..");
  await expect(notice).toContainText("Can't merge: needs a rebase");
  await expect(notice.locator("svg").first()).toHaveAttribute("aria-hidden", "true");
  await expect(notice.getByRole("button", nav("Merge again"))).toBeVisible();
  await expect(page.getByRole("button", nav("Merge"))).toHaveCount(0);
});

test("52. Draft impl epic", async ({ page, start }) => {
  await start("united");
  await page.evaluate(() => (location.hash = "#/e/demo:publishing-epic-impl"));
  await expect(page.getByTestId("stats-line")).toHaveText("Publishing tickets · 1 ticket so far");
  const queue = page.getByRole("button", nav("Queue"));
  await expect(queue).toBeDisabled();
  await expect(page.getByTitle("Queue unlocks once all tickets are published")).toBeVisible();
});
