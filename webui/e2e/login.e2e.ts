import type { Page } from "@playwright/test"
import { expect, test } from "./fixture"

const codeField = (page: Page) => page.getByLabel(/Code from your authenticator app/)

async function signIn(page: Page, password: string, code: string) {
  await page.getByLabel("Password").fill(password)
  await codeField(page).fill(code)
  await page.getByRole("button", { name: "Sign in" }).click()
}

// The PWA steps need a registered worker, which only registers with the fixture's untrusted root ignored.
test.use({ serviceWorkers: "allow", launchOptions: { args: ["--ignore-certificate-errors"] } })

// Flows 103 (rate limit) and 104 (certificate help) run as steps of 102: same web-login scenario.
// The rate limit runs last, in a second browser context, because the lockout would block the sign-in.
test("102. Login (+103 rate limit, +104 certificate help)", async ({ page, start, browser }) => {
  const fixture = await start("web-login", { reloads: 1 })
  await page.evaluate(() => (window.location.hash = "#/stats"))
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible()

  // 104: certificate help
  const help = page.locator('[data-mock-part="trust-help"]')
  await expect(help.getByText("Browser warns about this page?")).toBeVisible()
  await expect(help.getByRole("link")).toHaveCount(0)
  await help.getByText("Browser warns about this page?").click()

  const downloads = [
    { name: "iPhone or iPad profile", type: "application/x-apple-aspen-config" },
    { name: "Mac certificate", type: "application/pkix-cert" },
  ]
  for (const { name, type } of downloads) {
    const href = await help.getByRole("link", { name }).getAttribute("href")
    const res = await page.context().request.get(new URL(href!, page.url()).href)
    expect(res.ok(), `${name} downloads`).toBe(true)
    expect(res.headers()["content-type"]).toContain(type)
    expect((await res.body()).length).toBeGreaterThan(0)
  }

  const printed = (await fixture.cli("server", "status")).match(/fingerprint: (.+)/)?.[1].trim()
  expect(printed, "gx server status prints the fingerprint").toBeTruthy()
  const shown = await help.locator("p + div > span").allInnerTexts()
  expect(shown.join(" ")).toBe(printed)

  // PWA: with a worker active, a signed-out reload shows the server's sign-in screen, not a cached page.
  const workerState = () =>
    page.evaluate(
      async () => (await navigator.serviceWorker.getRegistration())?.active?.state ?? "none",
    )
  await expect.poll(workerState).toBe("activated")
  await page.reload()
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible()

  // 102: sign in, hash preserved
  await signIn(page, fixture.password, "000000")
  await expect(page.getByText("Wrong password or code")).toBeVisible()

  await signIn(page, fixture.password, await fixture.nextTotpCode())
  await expect(page.getByRole("heading", { name: "Sign in" })).toHaveCount(0)
  expect(page.url()).toMatch(/#\/stats$/)
  await expect(page.getByRole("link", { name: "Stats and health", exact: true })).toBeVisible()

  // PWA: a 401 from a data route still signs the page out with the worker active.
  await page.context().clearCookies()
  await page.evaluate(() => (window.location.hash = "#/"))
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible()

  // 103: rate limit, last
  const context = await browser.newContext({ ignoreHTTPSErrors: true })
  try {
    const other = await context.newPage()
    await other.goto(page.url())
    for (let i = 0; i < 5; i++) {
      await signIn(other, fixture.password, "000000")
      await expect(other.getByText("Wrong password or code")).toBeVisible()
    }
    await signIn(other, fixture.password, "000000")
    await expect(other.getByText(/^Too many tries\. Try again in \d+ minutes?\.$/)).toBeVisible()
    await expect(other.getByRole("button", { name: "Sign in" })).toBeDisabled()
  } finally {
    await context.close()
  }
})
