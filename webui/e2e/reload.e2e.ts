import { test } from "./fixture";

// Smoke checks of the fixture's reload allowance, not catalog flows.

test("reload allowance: one allowed reload passes", async ({ page, start }) => {
  await start("empty", { reloads: 1 });
  await page.reload();
});

test("reload allowance: a second reload fails the test", async ({ page, start }) => {
  test.fail();
  await start("empty", { reloads: 1 });
  await page.reload();
  await page.reload();
});

test("reload allowance: a reload fails by default", async ({ page, start }) => {
  test.fail();
  await start("empty");
  await page.reload();
});
