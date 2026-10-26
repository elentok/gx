import { card, expect, nav, section, test } from "./fixture";

test("1. Queue an epic", async ({ page, start }) => {
  await start("ready");
  const ready = await section(page, "Ready to queue");
  await expect(ready.getByRole("link", { name: "ready-epic" })).toBeVisible();

  await card(page, "ready-epic").getByRole("button", nav("Queue")).click();

  await expect(ready.getByRole("link", { name: "ready-epic" })).toHaveCount(0);
  const progress = await section(page, "In progress");
  await expect(progress.getByRole("link", { name: "ready-epic" })).toBeVisible();
});

test("2. Epic looks right", async ({ page, start }) => {
  await start("mixed");
  const mixed = card(page, "mixed-epic");
  await expect(mixed.getByText("2/7")).toBeVisible();
  await expect(mixed.getByText("demo", { exact: true })).toBeVisible();
  await mixed.getByRole("link", { name: "mixed-epic" }).click();

  await expect(page.getByText("Not queued · 2 of 7 done")).toBeVisible();
  await expect(page.getByText("✓ 1 done · 1 cancelled")).toBeVisible();
  await expect(page.getByText("open · open")).toBeVisible();
  await expect(page.getByText("blocked · blocked · after 02")).toBeVisible();
  await expect(page.getByText("drafted · draft")).toBeVisible();
  await expect(page.getByText("asks · open")).toBeVisible();
  await expect(page.getByText("fails · open")).toBeVisible();

  // The worker settles with 02 and 03 merged, 06 asking and 07 failing.
  await page.getByRole("button", nav("Queue")).click();
  await expect(page.getByText(/Running · 4 of 7 done/)).toBeVisible();
  await expect(page.getByText("asks · needs-answer · Waiting for your answer")).toBeVisible();
  await expect(page.getByText("Which option should the fake worker take?")).toBeVisible();

  await page.getByRole("link", nav("← Epics")).click();
  await expect(mixed.getByText("4/7")).toBeVisible();
  await expect(mixed.getByTestId("pill")).toBeVisible();
  await expect(mixed.getByTestId("reason")).toBeVisible();
});

test("7. Epic completion", async ({ page, start }) => {
  const fixture = await start("finishing", { hold: true });
  const progress = await section(page, "In progress");
  await expect(progress.getByRole("link", { name: "finishing-epic" })).toBeVisible();
  await expect(card(page, "finishing-epic").getByText("1/2")).toBeVisible();

  await fixture.release();

  // The last ticket merges; the epic leaves In progress for Finished.
  await expect(progress.getByRole("link", { name: "finishing-epic" })).toHaveCount(0);
  const finished = await section(page, "Finished");
  await expect(finished.getByRole("link", { name: "finishing-epic" })).toBeVisible();
  await expect(card(page, "finishing-epic").getByText("2/2")).toBeVisible();
  await card(page, "finishing-epic").getByRole("link", { name: "finishing-epic" }).click();
  await expect(page.getByText(/2 of 2 done/)).toBeVisible();
});

test("8. Edge states", async ({ page, start }) => {
  await start("edge");

  // An empty epic counts as implemented, so the server files it under In progress.
  const progress = await section(page, "In progress");
  await expect(progress.getByRole("link", { name: "empty-epic" })).toBeVisible();
  await expect(card(page, "empty-epic").getByText("Implemented")).toBeVisible();

  // Filtering to the empty epic hides every other card.
  await page.getByRole("searchbox", nav("Filter epics")).fill("empty");
  await expect(card(page, "empty-epic")).toBeVisible();
  await expect(page.getByRole("link", { name: "blocked-epic" })).toHaveCount(0);
  await page.getByRole("searchbox", nav("Filter epics")).fill("");

  await progress.getByRole("link", { name: "empty-epic" }).click();
  await expect(page.getByText("No tickets yet")).toBeVisible();

  await page.getByRole("link", nav("← Epics")).click();
  await page.getByRole("link", { name: "blocked-epic" }).click();
  await expect(page.getByText("waiting · blocked · after 01")).toBeVisible();
  await expect(page.getByText("unmerged · open")).toBeVisible();
});

test("6. Outside changes show up", async ({ page, start }) => {
  const fixture = await start("ready");
  const ready = await section(page, "Ready to queue");
  await expect(ready.getByRole("link", { name: "ready-epic" })).toBeVisible();

  await fixture.cli("server", "queue", "add", "demo:ready-epic");

  const progress = await section(page, "In progress");
  await expect(progress.getByRole("link", { name: "ready-epic" })).toBeVisible();
  await expect(ready.getByRole("link", { name: "ready-epic" })).toHaveCount(0);
});
