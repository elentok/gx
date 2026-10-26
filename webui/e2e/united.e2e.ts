import { card, expect, nav, section, test } from "./fixture";

const NEEDS_YOU = [
  "paused-epic",
  "question-epic",
  "perm-epic",
  "deadlock-epic",
  "blocked-merge-epic",
  "awaiting-epic",
  "design-question",
  "design-stopped",
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
    "In progress",
    "Ready to queue",
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
  for (const name of NEEDS_YOU) {
    await expect(needs.getByRole("link", { name, exact: true })).toBeVisible();
    await expect(card(page, name).getByTestId("pill")).toBeVisible();
  }
  await expect(page.getByTestId("pill")).toHaveCount(NEEDS_YOU.length);
  const progress = await section(page, "In progress");
  await expect(progress.getByTestId("pill")).toHaveCount(0);
  const queue = await section(page, "Ready to queue");
  await expect(queue.getByTestId("pill")).toHaveCount(0);

  // A click outside the title does not navigate; the title does.
  await card(page, "paused-epic").getByText("demo", { exact: true }).click();
  await expect(page).toHaveURL(/#\/epics$/);
  await card(page, "paused-epic").getByRole("link", { name: "paused-epic", exact: true }).click();
  await expect(page).toHaveURL(/#\/e\/demo:paused-epic$/);
  await page.getByRole("link", nav("← Epics")).click();

  // Only merge-blocked and awaiting-merge cards offer Merge.
  for (const name of ["blocked-merge-epic", "awaiting-epic"]) {
    await expect(card(page, name).getByRole("button")).toHaveText(["Merge"]);
  }
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

// The fixture's epics have no git branch, so the server refuses the merge. A refusal on the card
// shows the click reached the server from the card, with no navigation.
test("17. Merge from the card", async ({ page, start }) => {
  await start("united");
  await section(page, "Needs you");
  for (const name of ["awaiting-epic", "blocked-merge-epic"]) {
    await card(page, name).getByRole("button", nav("Merge")).click();
    await expect(card(page, name).getByRole("alert")).toBeVisible();
  }
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
  await page.getByRole("link", nav("Open design →")).click();
  await expect(page).toHaveURL(/#\/e\/demo:design-epic$/);
  await page.getByRole("link", nav("Open implementation epic →")).click();
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
