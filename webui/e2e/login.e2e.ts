import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

const codeField = (page: Page) => page.getByLabel(/Code from your authenticator app/);

async function signIn(page: Page, password: string, code: string) {
  await page.getByLabel("Password").fill(password);
  await codeField(page).fill(code);
  await page.getByRole("button", { name: "Sign in" }).click();
}

test("102. Login", async ({ page, start }) => {
  const fixture = await start("web-login");
  await page.evaluate(() => (window.location.hash = "#/stats"));
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();

  await signIn(page, fixture.password, "000000");
  await expect(page.getByText("Wrong password or code")).toBeVisible();

  await signIn(page, fixture.password, await fixture.nextTotpCode());
  await expect(page.getByRole("heading", { name: "Sign in" })).toHaveCount(0);
  expect(page.url()).toMatch(/#\/stats$/);
  await expect(page.getByRole("link", { name: "Stats", exact: true })).toBeVisible();
});

test("103. Login rate limit", async ({ page, start }) => {
  const fixture = await start("web-login");
  for (let i = 0; i < 5; i++) {
    await signIn(page, fixture.password, "000000");
    await expect(page.getByText("Wrong password or code")).toBeVisible();
  }

  await signIn(page, fixture.password, "000000");
  await expect(page.getByText(/^Too many tries\. Try again in \d+ minutes?\.$/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Sign in" })).toBeDisabled();
});

test("104. Certificate help", async ({ page, start }) => {
  const fixture = await start("web-login");
  const help = page.locator('[data-mock-part="trust-help"]');
  await expect(help.getByText("Browser warns about this page?")).toBeVisible();
  await expect(help.getByRole("link")).toHaveCount(0);
  await help.getByText("Browser warns about this page?").click();

  const downloads = [
    { name: "iPhone or iPad profile", type: "application/x-apple-aspen-config" },
    { name: "Mac certificate", type: "application/pkix-cert" },
  ];
  for (const { name, type } of downloads) {
    const href = await help.getByRole("link", { name }).getAttribute("href");
    const res = await page.context().request.get(new URL(href!, page.url()).href);
    expect(res.ok(), `${name} downloads`).toBe(true);
    expect(res.headers()["content-type"]).toContain(type);
    expect((await res.body()).length).toBeGreaterThan(0);
  }

  const printed = (await fixture.cli("server", "status")).match(/fingerprint: (.+)/)?.[1].trim();
  expect(printed, "gx server status prints the fingerprint").toBeTruthy();
  const shown = await help.locator("p + div > span").allInnerTexts();
  expect(shown.join(" ")).toBe(printed);
});
