import { writeFile } from "node:fs/promises"
import type { Page } from "@playwright/test"
import { CACHE_PREFIX } from "../src/pwa/workerCore"
import { freshVisit, go } from "./agentsKit"
import { expect, menu, menuButton, openMenu, test, type Fixture } from "./fixture"

// The PWA step needs a registered worker, which only registers with the fixture's untrusted root ignored.
test.use({ serviceWorkers: "allow", launchOptions: { args: ["--ignore-certificate-errors"] } })

const LOADS = "__gxE2eLoads"
const loads = (page: Page) => page.evaluate((key) => Number(sessionStorage.getItem(key)), LOADS)

/** Marks the document so a later reload shows as a missing mark. */
const mark = (page: Page) => page.evaluate(() => ((window as any).__devMark = true))
// A read landing mid-navigation throws "context destroyed", which is itself the reload.
const reloaded = (page: Page) =>
  expect
    .poll(() => page.evaluate(() => (window as any).__devMark === undefined).catch(() => true))
    .toBe(true)

// A ready Vite serves the fake's own page, never the gx UI: the page then shows "vitefake" and has no menu.
const onVite = (page: Page) => expect(page.locator("#root")).toHaveText("vitefake")

const devRow = (page: Page) =>
  menu(page).getByRole("menuitemcheckbox", { name: "Dev mode", exact: true })
const detailsRow = (page: Page) => menu(page).getByRole("menuitem", { name: /^Dev details/ })
const control = (fixture: Fixture, body: object) =>
  writeFile(fixture.devControlFile, JSON.stringify(body))
const devKey = async (fixture: Fixture) =>
  (await fixture.globalConfig())["server"]?.["web"]?.["dev-frontend"]

// Menu-less pages (the fake's own) leave the CLI, which writes the same setting the row does.
const setDev = (fixture: Fixture, on: boolean) =>
  fixture.cli("server", "settings-set", "--set", `server.web.dev-frontend=${on}`)

/** Closes the menu if open and waits until it is gone, so openMenu never sees the closing one. */
async function reopenMenu(page: Page) {
  // Escape does nothing while focus is outside the menu; the trigger toggles it.
  if (await menu(page).isVisible()) await menuButton(page).click()
  await expect(menu(page)).toBeHidden()
  await openMenu(page)
}

/** Reopens the menu fresh, then returns its "Dev details" row, whose hint is the Dev status. */
async function openDetails(page: Page) {
  await reopenMenu(page)
  return detailsRow(page)
}

async function flipMenuOn(page: Page) {
  await reopenMenu(page)
  await expect(devRow(page)).toHaveAttribute("aria-checked", "false")
  await devRow(page).click()
}

// Needs the server's reload event over SSE, the fake Vite's life cycle and a browser's service worker registry;
// DevPill, DevDetails, useDevToast and the settings row have vitest tests for each state alone.
test("Dev mode (menu flip, status journey, settings row, PWA)", async ({ page, start }) => {
  // Menu: turning Dev mode on reloads every open page onto Vite; turning it off reloads them back.
  let fixture = await start("dev-mode", { shape: "off" })
  const other = await freshVisit(page, "#/")
  await expect(await openDetails(page)).toHaveCount(0)
  await mark(page)
  await mark(other)
  await flipMenuOn(page)
  await reloaded(page)
  await reloaded(other)
  await onVite(page)
  await onVite(other)
  expect(await devKey(fixture)).toBe(true)
  await setDev(fixture, false)
  // The fake's page has no event stream to hear a reload on, so these two load the built UI again by hand.
  await page.reload()
  await other.reload()
  await expect(menuButton(page)).toBeVisible()
  await expect(menuButton(other)).toBeVisible()
  await expect(await openDetails(page)).toHaveCount(0)
  expect(await loads(page)).toBe(2)
  await other.close()

  // Status journey: installing, starting, then running swaps the page onto Vite's.
  fixture = await start("dev-mode", { shape: "installing" })
  await expect(await openDetails(page)).toContainText("Installing dependencies")
  await control(fixture, { install: "ok", vite: "hold" })
  await expect(await openDetails(page)).toContainText("Starting Vite")
  expect(await loads(page)).toBe(0)
  await control(fixture, { vite: "ready" })
  await onVite(page)
  expect(await loads(page)).toBe(1)

  // Crash: down, a toast, and no reload. The Dev details submenu offers Restart Vite, and the Dev mode row turns it off: the key goes off and the details row goes away.
  fixture = await start("dev-mode", { shape: "starting" })
  await expect(await openDetails(page)).toContainText("Starting Vite")
  await control(fixture, { vite: "crash" })
  await expect(await openDetails(page)).toContainText("Vite is down")
  await expect(page.getByText("Vite is down. gx serves the built files.")).toBeVisible()
  expect(await loads(page), "the crash did not reload the page").toBe(0)
  await detailsRow(page).click()
  await expect(page.getByRole("button", { name: "Restart Vite" })).toBeVisible()
  await devRow(page).click()
  await expect(detailsRow(page)).toHaveCount(0)
  expect(await devKey(fixture)).toBe(false)

  // Settings row: the switch turns Dev mode back on with no confirm dialog.
  await go(page, "#/settings/s/server")
  await page.getByRole("switch", { name: /dev.frontend/i }).click()
  await expect(page.getByRole("dialog")).toHaveCount(0)
  await expect.poll(() => devKey(fixture)).toBe(true)

  // PWA: Dev mode unregisters the worker and clears the shell caches; off again lets a fresh one register.
  // Three reloads: the server's reload onto Vite, the dev worker's own while it unregisters, and the hand reload back.
  fixture = await start("dev-mode", { shape: "off", reloads: 3 })
  const workerState = () =>
    page.evaluate(
      async () => (await navigator.serviceWorker.getRegistration())?.active?.state ?? "none",
    )
  await expect.poll(workerState).toBe("activated")
  await page.evaluate(
    async (name) => (await caches.open(name)).put("/planted", new Response("x")),
    `${CACHE_PREFIX}planted`,
  )
  await mark(page)
  await flipMenuOn(page)
  await reloaded(page)
  await onVite(page)
  await expect
    .poll(() =>
      page.evaluate(async () => (await navigator.serviceWorker.getRegistrations()).length),
    )
    .toBe(0)
  await expect
    .poll(() =>
      page.evaluate(
        (prefix) => caches.keys().then((keys) => keys.filter((k) => k.startsWith(prefix))),
        CACHE_PREFIX,
      ),
    )
    .toEqual([])
  await setDev(fixture, false)
  await page.reload()
  await expect.poll(workerState).toBe("activated")
})
