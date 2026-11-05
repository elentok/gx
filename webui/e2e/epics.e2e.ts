import { card, expect, holdRoute, nav, section, test, toasts } from "./fixture";

test("2. Epic looks right", async ({ page, start, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  // 06 asks while 02 merges. With a live paused sibling the run only re-reads the epic on its 30s pause poll,
  // so 03 can start up to 30s after 02 merges, and 03's own merge waits out the debounce after that.
  test.setTimeout(180_000);
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
  // mixed-epic has no summary, so the header draws no description line.
  await expect(page.getByTestId("page-header-line")).toHaveCount(0);
  // The ⋯ menu holds only Copy epic ID, and it copies <project>:<name>.
  await page.getByRole("button", nav("More actions")).click();
  const menu = page.getByRole("menu", nav("More actions"));
  await expect(menu.getByRole("menuitem")).toHaveCount(1);
  await expect(menu.getByRole("menuitem", { name: /^Copy epic ID/ })).toContainText("demo:mixed-epic");
  await menu.getByRole("menuitem", { name: /^Copy epic ID/ }).click();
  await expect(menu).toHaveCount(0);
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("demo:mixed-epic");
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
  await expect(page.getByTestId("stats-line")).toHaveText(/^Running · /, { timeout: 30_000 });
  await expect(page.getByTestId("comet")).toBeVisible();
  // 02 then 03 merge one after the other, and every merge waits out the server's 3s finish debounce.
  await expect(page.getByText("4/7")).toBeVisible({ timeout: 120_000 });
  // The innermost block holding both the 06 link and the Answer button is its card.
  const asking = page
    .locator("div")
    .filter({ has: page.locator('a[href$="/t/demo:mixed-epic/06"]') })
    .filter({ has: page.getByRole("button", nav("Answer")) })
    .last();
  await expect(asking.getByText("needs answer")).toBeVisible({ timeout: 45_000 });
  await expect(asking.getByText("Which option should the fake worker take?")).toBeVisible();
  const answer = asking.getByRole("button", nav("Answer"));
  await expect(answer).toBeVisible();
  const title = (await asking.locator('a[href$="/t/demo:mixed-epic/06"]').boundingBox())!;
  const answerBox = (await answer.boundingBox())!;
  expect(answerBox.x).toBeGreaterThanOrEqual(title.x + title.width);
  const failed = page.locator('[data-tone="deadlocked"]').filter({ has: page.getByRole("link", { name: "fails" }) });
  await expect(failed.getByText("needs-repair · Run failed")).toBeVisible({ timeout: 45_000 });

  await page.getByRole("link", { name: /^Epics/ }).click();
  await expect(mixed.getByText("4/7")).toBeVisible();
  await expect(mixed.getByTestId("pill")).toBeVisible();
  await expect(mixed.getByTestId("reason")).toBeVisible();
});

test("7. Epic completion", async ({ page, start }) => {
  const fixture = await start("finishing", { hold: true });
  const progress = await section(page, "Implementing");
  await expect(progress.getByRole("link", { name: "finishing-epic" })).toBeVisible();
  await expect(card(page, "finishing-epic").getByText("1/2")).toBeVisible();

  // While held, the running ticket's card meta starts with "running · ".
  await card(page, "finishing-epic").getByRole("link", { name: "finishing-epic" }).click();
  await expect(page.locator('a[href$="/t/demo:finishing-epic/02"] [data-meta]')).toHaveText(/^running · /);
  await page.getByRole("link", { name: /^Epics/ }).click();

  await fixture.release();

  // The last ticket merges; the epic leaves Implementing for Finished.
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

test("6. Queue an epic, and outside changes show up", async ({ page, start }) => {
  const fixture = await start("ready");
  const ready = await section(page, "Ready to queue");
  await expect(ready.getByRole("link", { name: "ready-epic" })).toBeVisible();

  const release = await holdRoute(page, "**/v1/queue/add");
  await card(page, "ready-epic").getByRole("button", nav("Queue")).click();
  await expect(card(page, "ready-epic").getByRole("button")).toBeDisabled();
  release();

  await expect(ready.getByRole("link", { name: "ready-epic" })).toHaveCount(0);
  const progress = await section(page, "Implementing");
  await expect(progress.getByRole("link", { name: "ready-epic" })).toBeVisible();
  // The card moving is the feedback: no success toast.
  await expect(toasts(page)).toHaveCount(0);

  // A change made outside the page (CLI) moves the card too.
  await fixture.cli("server", "queue", "remove", "demo:ready-epic");
  await expect(ready.getByRole("link", { name: "ready-epic" })).toBeVisible();
  await fixture.cli("server", "queue", "add", "demo:ready-epic");

  await expect(progress.getByRole("link", { name: "ready-epic" })).toBeVisible();
  await expect(ready.getByRole("link", { name: "ready-epic" })).toHaveCount(0);
});
