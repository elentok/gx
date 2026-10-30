import { chromium, type Browser, type Page } from "@playwright/test";
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

/** Opens the login page in a fresh browser context and submits the password and the next code. */
async function signInElsewhere(browser: Browser, url: string, password: string, code: string) {
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const other = await context.newPage();
  await other.goto(url);
  await other.getByLabel("Password").fill(password);
  await other.getByLabel(/Code from your authenticator app/).fill(code);
  await other.getByRole("button", { name: "Sign in" }).click();
  return other;
}

const signedInPage = (other: Page) => expect(other.getByRole("heading", { name: "Sign in" })).toHaveCount(0);

async function openAccountPage(page: Page, link: string) {
  await page.getByRole("button", nav("Menu")).click();
  await page.getByRole("menuitem", nav("Settings")).click();
  await page.getByRole("link", { name: link }).click();
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
    await expect(page.locator(`a[href$="/${id}"]`), "a UI sign-in is named from the user agent").toContainText(/Chrome on (Mac|Linux|Windows)/);
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

test("108. Change password", async ({ page, start }) => {
  const fixture = await start("web-logins");
  const newPassword = "a brand new passphrase";

  await openAccountPage(page, "Change password");
  await expect(page.getByText("The 3 other devices will be signed out.")).toBeVisible();
  await page.getByLabel("Current password").fill(fixture.password);
  await page.getByLabel("Code from your authenticator app").fill(await fixture.nextTotpCode());
  await page.locator('[data-mock-part="new-password"] input').fill(newPassword);
  await page.locator('[data-mock-part="new-password-again"] input').fill(newPassword);
  await page.getByRole("button", { name: "Change password" }).click();
  await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();

  await openWebLogins(page);
  await expect(loginRows(page)).toHaveCount(1);

  const browser = await chromium.launch();
  try {
    const other = await signInElsewhere(browser, page.url(), newPassword, await fixture.nextTotpCode());
    await signedInPage(other);
  } finally {
    await browser.close();
  }
});

test("109. Reset authenticator", async ({ page, start }) => {
  // Six codes, each waits for a new 30 second TOTP step.
  test.setTimeout(240_000);
  const fixture = await start("web-logins");
  const oldSecret = fixture.totpSecret;

  const begin = async () => {
    await openAccountPage(page, "Reset authenticator");
    await page.getByLabel("Current password").fill(fixture.password);
    await page.getByLabel("Code from the current app").fill(await fixture.nextTotpCode());
    await page.getByRole("button", { name: "Next" }).click();
    await expect(page.locator('[data-mock-part="totp-qr"] svg')).toBeVisible();
    await expect(page.getByRole("link", { name: "Add to authenticator app" })).toHaveAttribute("href", /^otpauth:\/\/totp\//);
    await expect(page.getByText("The old app keeps working until this code checks out.")).toBeVisible();
    return page.locator('[data-mock-part="totp-key"] code').innerText();
  };

  const abandoned = await begin();
  expect(abandoned.replace(/\s/g, "")).not.toBe(oldSecret);
  await page.getByRole("link", { name: "‹ Settings" }).click();

  const browser = await chromium.launch();
  try {
    const stillOld = await signInElsewhere(browser, page.url(), fixture.password, await fixture.nextTotpCode());
    await signedInPage(stillOld);

    const newKey = await begin();
    await page.getByLabel("Code from the new app").fill("000000");
    await page.getByRole("button", { name: "Save new authenticator" }).click();
    await expect(page.getByText("That code doesn't match.")).toBeVisible();

    fixture.useTotpSecret(newKey);
    await page.getByLabel("Code from the new app").fill(await fixture.nextTotpCode());
    await page.getByRole("button", { name: "Save new authenticator" }).click();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();

    await openWebLogins(page);
    await expect(loginRows(page)).toHaveCount(1);

    fixture.useTotpSecret(oldSecret);
    const rejected = await signInElsewhere(browser, page.url(), fixture.password, await fixture.nextTotpCode());
    await expect(rejected.getByRole("heading", { name: "Sign in" })).toBeVisible();

    fixture.useTotpSecret(newKey);
    const accepted = await signInElsewhere(browser, page.url(), fixture.password, await fixture.nextTotpCode());
    await signedInPage(accepted);
  } finally {
    await browser.close();
  }
});

/** The Setup code a CLI reset prints. */
const printedSetupCode = (out: string) => {
  const code = out.match(/setup code: (\S+)/)?.[1];
  if (!code) throw new Error(`the CLI printed no setup code:\n${out}`);
  return code;
};

test("110. Web reset from the CLI", async ({ page, start }) => {
  const fixture = await start("web-logins");
  await expect(page.getByRole("heading", { name: "Sign in" })).toHaveCount(0);

  const code = printedSetupCode(await fixture.cli("server", "web-reset", "--yes"));
  await expect(page.getByText("step 1 of 3")).toBeVisible();

  const password = "a fresh passphrase 123";
  await page.getByLabel("Setup code").fill(code);
  await page.getByRole("button", { name: "Continue" }).click();
  await page.locator('[data-mock-part="password"] input').fill(password);
  await page.locator('[data-mock-part="password-again"] input').fill(password);
  await page.getByRole("button", { name: "Continue" }).click();
  fixture.useTotpSecret(await page.locator('[data-mock-part="totp-key"] code').innerText());
  await page.getByLabel("Enter the code it shows").fill(await fixture.nextTotpCode());
  await page.getByRole("button", { name: "Finish setup" }).click();
  await expect(page.getByRole("link", { name: "Epics", exact: true }).or(page.getByRole("heading", { name: "Epics" })).first()).toBeVisible();
});

test("111. CLI revoke", async ({ page, start }) => {
  const fixture = await start("web-logins");
  await openWebLogins(page);
  const href = await loginRows(page).filter({ has: page.getByText("This device") }).getAttribute("href");
  const id = href?.split("/").pop();
  expect(id, "the Web logins page links this device by id").toBeTruthy();

  await fixture.cli("server", "web-revoke", id!);
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  expect(await loginIds(fixture)).not.toContain(id);
});

test("112. Add the authenticator after a CLI reset", async ({ page: first, start }) => {
  // Two codes, each waits for a new 30 second TOTP step.
  test.setTimeout(180_000);
  const fixture = await start("web-login");
  const oldSecret = fixture.totpSecret;

  const out = await fixture.cli("server", "web-reset-authenticator");
  const code = printedSetupCode(out);
  expect(out, "the CLI prints no key").not.toContain(oldSecret);

  // The login page reads its flags on load, and the fixture fails a flow whose first page reloads, so it opens in a new page.
  const page = await first.context().newPage();
  await page.goto(first.url());
  await expect(page.getByLabel("Setup code")).toBeVisible();
  const order = await page.locator("label").allInnerTexts();
  expect(order.findIndex((t) => t.startsWith("Setup code"))).toBeLessThan(order.findIndex((t) => t.startsWith("Password")));

  const submit = async (setupCode: string) => {
    await page.getByLabel("Setup code").fill(setupCode);
    await page.getByLabel("Password").fill(fixture.password);
    const totp = page.getByLabel(/Code from your authenticator app/);
    if (await totp.count()) await totp.fill("000000");
    await page.getByRole("button", { name: "Sign in" }).click();
  };

  await submit("");
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Add gx to your authenticator app" })).toHaveCount(0);

  await submit(code);
  await expect(page.getByRole("heading", { name: "Add gx to your authenticator app" })).toBeVisible();
  await expect(page.locator('[data-mock-part="totp-qr"] svg')).toBeVisible();
  await expect(page.getByRole("link", { name: "Add to authenticator app" })).toHaveAttribute("href", /^otpauth:\/\/totp\//);
  const key = await page.locator('[data-mock-part="totp-key"] code').innerText();
  expect(key.replace(/\s/g, "")).not.toBe(oldSecret);

  await page.getByLabel("Enter the code it shows").fill("000000");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toHaveText("Wrong password or code");

  fixture.useTotpSecret(key);
  await page.getByLabel("Enter the code it shows").fill(await fixture.nextTotpCode());
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("link", { name: "Epics", exact: true }).or(page.getByRole("heading", { name: "Epics" })).first()).toBeVisible();

  const browser = await chromium.launch();
  try {
    fixture.useTotpSecret(oldSecret);
    const rejected = await signInElsewhere(browser, page.url(), fixture.password, await fixture.nextTotpCode());
    await expect(rejected.getByRole("heading", { name: "Sign in" })).toBeVisible();
  } finally {
    await browser.close();
  }
});
