import { chromium, type Page } from "@playwright/test";
import { expect, nav, test, type Fixture } from "./fixture";

/** The ids `gx server web-logins` prints, one per line, the id first. */
const loginIds = async (fixture: Fixture) =>
  (await fixture.cli("server", "web-logins")).split("\n").filter(Boolean).map((line) => line.split(/\s+/)[0]);

const loginRows = (page: Page) => page.locator('a[href^="#/settings/logins/"]');

async function openWebLogins(page: Page) {
  await page.getByRole("button", nav("Menu")).click();
  await page.getByRole("menuitem", nav("Settings")).click();
  await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
  await page.getByRole("link", { name: /^Web logins/ }).click();
  await expect(page.getByRole("heading", { name: "Web logins" })).toBeVisible();
}

test("105. Settings and web logins", async ({ page, start }) => {
  await start("web-logins");
  await page.getByRole("button", nav("Menu")).click();
  await page.getByRole("menuitem", nav("Settings")).click();
  await expect(page.getByText("Security")).toBeVisible();
  await expect(page.getByRole("link", { name: /^Web logins/ })).toContainText("4 devices");

  await page.getByRole("link", { name: /^Web logins/ }).click();
  await expect(loginRows(page)).toHaveCount(4);
  await expect(page.getByText("This device")).toHaveCount(1);
  await expect(page.getByText(/^Ends in \d+ days? unless used$/)).toHaveCount(1);
  await expect(page.getByText(/^Ends in 20 days unless used$/)).toBeVisible();

  const other = loginRows(page).filter({ hasNot: page.getByText("This device") }).first();
  await other.click();
  await expect(page.getByRole("button", { name: "Sign out this device" })).toBeVisible();
  const name = page.getByLabel(/^Name/);
  await name.fill("Kitchen tablet");
  await name.blur();
  await page.getByRole("link", { name: "‹ Settings" }).click();
  await page.getByRole("link", { name: /^Web logins/ }).click();
  await expect(loginRows(page).filter({ hasText: "Kitchen tablet" })).toHaveCount(1);
});

test("106. Sign out another device", async ({ page, start }) => {
  const fixture = await start("web-logins");
  const before = await loginIds(fixture);

  const browser = await chromium.launch();
  const second = await browser.newContext({ ignoreHTTPSErrors: true });
  try {
    const other = await second.newPage();
    await other.goto(page.url());
    await other.evaluate(() => ((window as unknown as Record<string, unknown>).__second = true));
    await other.getByLabel("Password").fill(fixture.password);
    await other.getByLabel(/Code from your authenticator app/).fill(await fixture.nextTotpCode());
    await other.getByRole("button", { name: "Sign in" }).click();
    await expect(other.getByRole("heading", { name: "Sign in" })).toHaveCount(0);

    const [id] = (await loginIds(fixture)).filter((i) => !before.includes(i));
    expect(id, "the second browser has a web login").toBeTruthy();

    await openWebLogins(page);
    await expect(loginRows(page)).toHaveCount(5);
    await page.locator(`a[href$="/${id}"]`).click();
    await page.getByRole("button", { name: "Sign out this device" }).click();
    await page.getByRole("button", { name: "Sign out", exact: true }).click();

    await expect(other.getByRole("heading", { name: "Sign in" })).toBeVisible();
    expect(await other.evaluate(() => (window as unknown as Record<string, unknown>).__second), "the second page was not reloaded").toBe(true);
    expect(await loginIds(fixture)).not.toContain(id);
  } finally {
    await browser.close();
  }
});

test("107. Sign out of this device", async ({ page, start }) => {
  const fixture = await start("web-logins");
  const before = await loginIds(fixture);
  expect(before).toHaveLength(4);

  await page.getByRole("button", nav("Menu")).click();
  await page.getByRole("menuitem", nav("Settings")).click();
  await page.getByRole("button", { name: "Sign out of this device" }).click();
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  expect(await loginIds(fixture)).toHaveLength(3);
});
