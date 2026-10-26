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

  await expect(page.getByText("2/7")).toBeVisible();
  await expect(page.getByRole("img", { name: "Implementation" })).toBeVisible();
  await expect(page.getByTestId("comet")).toHaveCount(0);
  await expect(page.getByTestId("stats-line")).toHaveText("Not queued · $0.00 · 0s agent time");
  await expect(page.getByText("1 done, 1 cancelled")).toBeVisible();
  await expect(page.getByRole("link", { name: "02 open ready" })).toBeVisible();
  await expect(page.getByRole("link", { name: "03 blocked after 02" })).toBeVisible();
  await expect(page.getByRole("link", { name: "04 drafted draft" })).toBeVisible();
  await expect(page.getByRole("link", { name: "06 asks ready" })).toBeVisible();
  await expect(page.getByRole("link", { name: "07 fails ready" })).toBeVisible();
  await expect(page.getByRole("link", { name: "05 dropped cancelled" })).toBeVisible();
  // The epic has no stats until work merges, so there is no More stats card yet.
  await expect(page.getByRole("button", { name: /^More stats/ })).toHaveCount(0);

  // The worker settles with 02 and 03 merged, 06 asking and 07 failing.
  await page.getByRole("button", nav("Queue")).click();
  await expect(page.getByText("2/7")).toBeVisible();
  await expect(page.getByTestId("stats-line")).toHaveText(/^Running · /);
  await expect(page.getByTestId("comet")).toBeVisible();
  await expect(page.getByText("4/7")).toBeVisible();
  await expect(page.getByText("needs-answer · Waiting for your answer")).toBeVisible();
  await expect(page.getByText("Which option should the fake worker take?")).toBeVisible();
  await expect(page.getByRole("button", nav("Answer"))).toBeVisible();
  await expect(page.getByRole("link", { name: "07 fails" })).toBeVisible();

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
  await expect(page.getByText("Finished · $0.00 · 0s agent time")).toBeVisible();
  await expect(page.getByText("2 done")).toBeVisible();
  await expect(page.getByText("2/2")).toBeVisible();
  await expect(page.getByTestId("comet")).toHaveCount(0);
  const more = page.getByRole("button", { name: /^More stats/ });
  await expect(more).toHaveAttribute("aria-expanded", "false");
  await more.click();
  await expect(more).toHaveAttribute("aria-expanded", "true");
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
  await expect(page.getByText("0/0")).toBeVisible();
  await expect(page.getByRole("button", { name: /^More stats/ })).toHaveCount(0);

  await page.getByRole("link", nav("← Epics")).click();
  await page.getByRole("link", { name: "blocked-epic" }).click();
  await expect(page.getByRole("link", { name: "02 waiting after 01" })).toBeVisible();
  await expect(page.getByRole("link", { name: "01 unmerged ready" })).toBeVisible();
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
