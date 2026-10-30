import type { Page } from "@playwright/test";
import { expect, test, type Fixture } from "./fixture";

const GOOD_PASSWORD = "correct horse battery staple";

/** The Setup code `gx server status` prints while setup is pending. */
async function setupCode(fixture: Fixture): Promise<string> {
  const match = (await fixture.cli("server", "status")).match(/setup code: (\S+)/);
  if (!match) throw new Error("gx server status shows no pending setup code");
  return match[1];
}

const stepText = (page: Page, text: string) => page.getByText(text, { exact: false });
const signedIn = (page: Page) => expect(page.getByRole("heading", { name: "Epics" }).or(page.getByRole("link", { name: "Epics", exact: true })).first()).toBeVisible();

/** The setup starts with no secret, so the fixture takes the key step 3 shows. */
async function adoptKey(page: Page, fixture: Fixture) {
  fixture.useTotpSecret(await page.locator('[data-mock-part="totp-key"] code').innerText());
}

async function passwords(page: Page, password: string, again = password) {
  // The first field's label also holds its hint, so the fields are found by part.
  await page.locator('[data-mock-part="password"] input').fill(password);
  await page.locator('[data-mock-part="password-again"] input').fill(again);
}

test("99. First-run setup", async ({ page, start }) => {
  const fixture = await start("web-setup");
  await expect(stepText(page, "step 1 of 3")).toBeVisible();

  await page.getByLabel("Setup code").fill("AAAA-AAAA");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByText("That code is wrong or expired.")).toBeVisible();

  await page.getByLabel("Setup code").fill(await setupCode(fixture));
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(stepText(page, "step 2 of 3")).toBeVisible();
  await expect(page.getByText("At least 12 characters.")).toBeVisible();

  await passwords(page, "short");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByText("The password needs at least 12 characters.")).toBeVisible();
  await passwords(page, GOOD_PASSWORD, `${GOOD_PASSWORD}x`);
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByText("The two passwords don't match.")).toBeVisible();
  await passwords(page, GOOD_PASSWORD);
  await page.getByRole("button", { name: "Continue" }).click();

  await expect(stepText(page, "step 3 of 3")).toBeVisible();
  await expect(page.getByRole("link", { name: "Add to authenticator app" })).toHaveAttribute("href", /^otpauth:\/\/totp\//);
  await expect(page.getByText("Can't scan? Enter this key:")).toBeVisible();
  await adoptKey(page, fixture);

  await page.getByLabel("Enter the code it shows").fill("000000");
  await page.getByRole("button", { name: "Finish setup" }).click();
  await expect(page.getByText("Check the clock on your phone")).toBeVisible();
  expect(await fixture.cli("server", "status")).toContain("setup code:");

  await page.getByLabel("Enter the code it shows").fill(await fixture.nextTotpCode());
  await page.getByRole("button", { name: "Finish setup" }).click();
  await signedIn(page);
});

test("100. Setup from the QR link", async ({ page, start }) => {
  const fixture = await start("web-setup");
  const link = (await fixture.cli("server", "web-url")).trim().split(/\s+/).find((w) => w.includes("#/setup"));
  expect(link, "gx server web-url prints the setup link").toBeTruthy();

  // A hash-only navigation on the open page would not re-read the code, so the link opens in a new page.
  const opened = await page.context().newPage();
  await opened.goto(link!);
  await expect(opened.getByLabel("Setup code")).toHaveValue(await setupCode(fixture));
  expect(opened.url()).toMatch(/#\/setup$/);

  await opened.getByRole("button", { name: "Continue" }).click();
  await passwords(opened, GOOD_PASSWORD);
  await opened.getByRole("button", { name: "Continue" }).click();
  await adoptKey(opened, fixture);
  await opened.getByLabel("Enter the code it shows").fill(await fixture.nextTotpCode());
  await opened.getByRole("button", { name: "Finish setup" }).click();
  await signedIn(opened);
  // `#/` redirects to the Epics page, so the URL no longer holds `#/setup`.
  expect(opened.url()).toMatch(/#\/epics$/);
});

test("101. Setup and login with TOTP off", async ({ page, start }) => {
  const fixture = await start("web-setup", { config: { "server.web.totp": "false" } });
  await expect(stepText(page, "step 1 of 2")).toBeVisible();

  await page.getByLabel("Setup code").fill(await setupCode(fixture));
  await page.getByRole("button", { name: "Continue" }).click();
  await passwords(page, GOOD_PASSWORD);
  await page.getByRole("button", { name: "Finish setup" }).click();
  await signedIn(page);

  await page.evaluate(() => (window.location.hash = "#/settings"));
  await expect(page.getByRole("link", { name: "Change password" })).toBeVisible();
  await expect(page.getByText("Reset authenticator")).toHaveCount(0);

  await page.getByRole("button", { name: "Sign out of this device" }).click();
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  await expect(page.getByLabel(/Code from your authenticator app/)).toHaveCount(0);
  await page.getByLabel("Password").fill(GOOD_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await signedIn(page);
});
