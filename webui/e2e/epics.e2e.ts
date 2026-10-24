import { expect, test } from "./fixture";

const nav = (name: string) => ({ name, exact: true });

test("1. Queue an epic", async ({ page, start }) => {
  await start("ready");
  await page.getByRole("link", nav("Epics")).click();
  await expect(page.getByRole("link", { name: "ready-epic" })).toBeVisible();

  await page.getByRole("button", nav("Queue")).click();

  await page.getByRole("link", nav("Queue")).click();
  await expect(page.getByRole("link", { name: "ready-epic" })).toBeVisible();
  await expect(page.getByText("All quiet")).toHaveCount(0);
});

test("2. Epic looks right", async ({ page, start }) => {
  await start("mixed");
  await page.getByRole("link", nav("Epics")).click();
  await expect(page.getByText("demo · 2/7")).toBeVisible();
  await page.getByRole("link", { name: "mixed-epic" }).click();

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

  await page.getByRole("link", nav("Queue")).click();
  const card = page.getByRole("link", { name: "mixed-epic" });
  await expect(card).toBeVisible();
  await expect(page.getByText("4/7")).toBeVisible();
  await expect(page.getByText("drafted", { exact: true })).toBeVisible();
  await expect(page.getByText("asks", { exact: true })).toBeVisible();
  await expect(page.getByText("Which option should the fake worker take?")).toBeVisible();
});

test("7. Epic completion", async ({ page, start }) => {
  const fixture = await start("finishing", { hold: true });
  await page.getByRole("link", nav("Queue")).click();
  await expect(page.getByRole("link", { name: "finishing-epic" })).toBeVisible();
  await expect(page.getByText("1/2")).toBeVisible();

  await fixture.release();

  // The last ticket merges; the epic leaves the active queue for Recently finished.
  await expect(page.getByText("All quiet")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Recently finished" })).toBeVisible();
  await expect(page.getByText(/demo · 2\/2/)).toBeVisible();
  await page.getByRole("link", { name: "finishing-epic" }).click();
  await expect(page.getByText(/2 of 2 done/)).toBeVisible();
});

test("8. Edge states", async ({ page, start }) => {
  await start("edge");
  await page.getByRole("link", nav("Queue")).click();
  await expect(page.getByText("All quiet")).toBeVisible();

  // The Epics page lists no empty epic, so it is opened by its route.
  await page.getByRole("link", nav("Epics")).click();
  await expect(page.getByRole("link", { name: "empty-epic" })).toHaveCount(0);
  await page.evaluate(() => (location.hash = "#/e/demo:empty-epic"));
  await expect(page.getByText("No tickets yet")).toBeVisible();

  await page.getByRole("link", nav("Epics")).click();
  await page.getByRole("link", { name: "blocked-epic" }).click();
  await expect(page.getByText("waiting · blocked · after 01")).toBeVisible();
  await expect(page.getByText("unmerged · open")).toBeVisible();
});

test("6. Outside changes show up", async ({ page, start }) => {
  const fixture = await start("ready");
  await page.getByRole("link", nav("Queue")).click();
  await expect(page.getByText("All quiet")).toBeVisible();

  await fixture.cli("server", "queue", "add", "demo:ready-epic");

  await expect(page.getByRole("link", { name: "ready-epic" })).toBeVisible();
  await expect(page.getByText("All quiet")).toHaveCount(0);
});
