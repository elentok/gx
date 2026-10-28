import { card, expect, holdRoute, nav, section, test, toasts } from "./fixture";

test("1. Queue an epic", async ({ page, start }) => {
  await start("ready");
  const ready = await section(page, "Ready to queue");
  await expect(ready.getByRole("link", { name: "ready-epic" })).toBeVisible();

  const release = await holdRoute(page, "**/v1/queue/add");
  await card(page, "ready-epic").getByRole("button", nav("Queue")).click();
  await expect(card(page, "ready-epic").getByRole("button")).toBeDisabled();
  release();

  await expect(ready.getByRole("link", { name: "ready-epic" })).toHaveCount(0);
  const progress = await section(page, "In progress");
  await expect(progress.getByRole("link", { name: "ready-epic" })).toBeVisible();
  // The card moving is the feedback: no success toast.
  await expect(toasts(page)).toHaveCount(0);
});

test("2. Epic looks right", async ({ page, start }) => {
  // Recovery would fork an investigation off the failed ticket and turn it into a waiting parent.
  await start("mixed", { config: { "recovery.enabled": "false" } });
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
  // The innermost block holding both the 06 link and the Answer button is its card.
  const asking = page
    .locator("div")
    .filter({ has: page.locator('a[href$="/t/demo:mixed-epic/06"]') })
    .filter({ has: page.getByRole("button", nav("Answer")) })
    .last();
  await expect(asking.getByText("needs answer")).toBeVisible();
  await expect(asking.getByText("Which option should the fake worker take?")).toBeVisible();
  const answer = asking.getByRole("button", nav("Answer"));
  await expect(answer).toBeVisible();
  const title = (await asking.locator('a[href$="/t/demo:mixed-epic/06"]').boundingBox())!;
  const answerBox = (await answer.boundingBox())!;
  expect(answerBox.x).toBeGreaterThanOrEqual(title.x + title.width);
  const failed = page.locator('[data-tone="deadlocked"]').filter({ has: page.getByRole("link", { name: "fails" }) });
  await expect(failed.getByText("needs-repair · Run failed")).toBeVisible();

  await page.getByRole("link", { name: /^Epics/ }).click();
  await expect(mixed.getByText("4/7")).toBeVisible();
  await expect(mixed.getByTestId("pill")).toBeVisible();
  await expect(mixed.getByTestId("reason")).toBeVisible();
});

test("7. Epic completion", async ({ page, start }) => {
  const fixture = await start("finishing", { hold: true });
  const progress = await section(page, "In progress");
  await expect(progress.getByRole("link", { name: "finishing-epic" })).toBeVisible();
  await expect(card(page, "finishing-epic").getByText("1/2")).toBeVisible();

  // While held, the running ticket's card meta starts with "running · ".
  await card(page, "finishing-epic").getByRole("link", { name: "finishing-epic" }).click();
  await expect(page.locator('a[href$="/t/demo:finishing-epic/02"] [data-meta]')).toHaveText(/^running · /);
  await page.getByRole("link", { name: /^Epics/ }).click();

  await fixture.release();

  // The last ticket merges; the epic leaves In progress for Finished.
  await expect(progress.getByRole("link", { name: "finishing-epic" })).toHaveCount(0);
  const finished = await section(page, "Finished");
  await expect(finished.getByRole("link", { name: "finishing-epic" })).toBeVisible();
  await expect(card(page, "finishing-epic").getByText("2/2")).toBeVisible();
  await card(page, "finishing-epic").getByRole("link", { name: "finishing-epic" }).click();
  await expect(page.getByText("Merged · $0.00 · 0s agent time")).toBeVisible();
  await expect(page.getByText("2 done")).toBeVisible();
  await expect(page.getByText("2/2")).toBeVisible();
  await expect(page.getByTestId("comet")).toHaveCount(0);
  const more = page.getByRole("button", { name: /^More stats/ });
  await expect(more).toHaveAttribute("aria-expanded", "false");
  // The divider comes first and More stats last on the page.
  const divider = (await page.getByText("2 done", { exact: true }).boundingBox())!;
  const moreBox = (await more.boundingBox())!;
  const lastCard = (await page.locator('a[href*="/t/demo:finishing-epic/"]').last().boundingBox())!;
  expect(moreBox.y).toBeGreaterThan(lastCard.y + lastCard.height - 1);
  expect(divider.y).toBeLessThan(lastCard.y);
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

  await page.getByRole("link", { name: /^Epics/ }).click();
  await page.getByRole("link", { name: "blocked-epic" }).click();
  await expect(page.getByRole("link", { name: "02 waiting after 01" })).toBeVisible();
  await expect(page.getByRole("link", { name: "01 unmerged ready" })).toBeVisible();
});

test("54. Done list", async ({ page, start }) => {
  await start("many-done");
  await page.evaluate(() => (location.hash = "#/e/demo:many-done-epic"));
  const done = page.locator('[data-done-divider] ~ [data-tone="done"]');
  await expect(page.getByText("5 done")).toBeVisible();
  await expect(done).toHaveCount(3);
  // Newest first.
  await expect(done.nth(0)).toContainText("merged-e");
  await expect(done.nth(2)).toContainText("merged-c");

  await page.getByRole("button", nav("Show 2 more")).click();
  await expect(done).toHaveCount(5);
  await expect(done.nth(4)).toContainText("merged-a");
  await page.getByRole("button", nav("Show fewer")).click();
  await expect(done).toHaveCount(3);
  await expect(page.getByRole("button", nav("Show 2 more"))).toBeVisible();
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
