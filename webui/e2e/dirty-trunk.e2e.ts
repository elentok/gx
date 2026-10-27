import { expect, nav, test, toasts } from "./fixture";

const EPIC = "#/e/demo:dirty-epic";

test("58. Dirty trunk on the Epic page", async ({ page, start }) => {
  await start("dirty-trunk");
  await page.evaluate((hash) => (location.hash = hash), EPIC);

  await expect(page.getByTestId("stats-line")).toHaveText(/^Paused · /);
  // The notice holds the short reason, then the clashing files, then the buttons in that order.
  const notice = page.getByRole("region", nav("Merge blocked"));
  await expect(notice.locator("[data-reason]")).toHaveText("Can't merge: Trunk has local changes");
  const files = notice.getByTestId("land-block-files");
  await expect(files.getByRole("listitem")).toHaveText(["same.txt"]);

  const stash = notice.getByRole("button", nav("Stash, merge and restore"));
  await expect(stash).toHaveAttribute("title", "Set your changes aside, merge, then put them back. Conflicts are left for you to fix.");
  const again = notice.getByRole("button", nav("Merge again"));
  await expect(again).toBeVisible();
  const [reason, list, stashBox, againBox] = await Promise.all(
    [notice.locator("[data-reason]"), files, stash, again].map((l) => l.boundingBox().then((b) => b!)),
  );
  expect(list.y).toBeGreaterThanOrEqual(reason.y + reason.height);
  expect(stashBox.y).toBeGreaterThanOrEqual(list.y + list.height);
  const sameRow = Math.abs(stashBox.y - againBox.y) < stashBox.height;
  expect(sameRow ? stashBox.x < againBox.x : stashBox.y < againBox.y).toBe(true);
});

// --hold freezes the worker on the conflict ticket, so the epic stays mid-run.
test("59. Stash, merge and restore", async ({ page, start }) => {
  const fixture = await start("dirty-trunk", { hold: true });
  await page.evaluate((hash) => (location.hash = hash), EPIC);

  await page.getByRole("button", nav("Stash, merge and restore")).click();

  await expect(page.getByText("Resolve the conflicts on main")).toBeVisible();
  await expect(page.getByTestId("stats-line")).toHaveText(/^Running · /);
  await expect(page.getByRole("button", nav("Stash, merge and restore"))).toHaveCount(0);
  await expect(page.getByText("Awaiting merge")).toHaveCount(0);
  await expect(page.getByText("Finished", { exact: true })).toHaveCount(0);

  await fixture.release();
});

test("60. Merge again on a dirty trunk", async ({ page, start }) => {
  await start("dirty-trunk");
  await page.evaluate((hash) => (location.hash = hash), EPIC);

  await page.getByRole("button", nav("Merge again")).click();

  const warning = toasts(page).filter({ hasText: /^uncommitted changes on main .* clash with same\.txt/ });
  await expect(warning).toBeVisible();
  await expect(warning.getByRole("link", nav("Open the paused epic"))).toHaveAttribute("href", EPIC);

  // The page shows the fresh state, not the stale Awaiting merge.
  await expect(page.getByTestId("stats-line")).toHaveText(/^Paused · /);
  await expect(page.getByTestId("land-block-files").getByRole("listitem")).toHaveText(["same.txt"]);
});
