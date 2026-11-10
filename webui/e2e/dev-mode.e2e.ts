import { writeFile } from "node:fs/promises";
import type { Page } from "@playwright/test";
import { CACHE_PREFIX } from "../src/pwa/workerCore";
import { freshVisit, go } from "./agentsKit";
import { expect, menu, menuButton, test, type Fixture } from "./fixture";

// The PWA step needs a registered worker, which only registers with the fixture's untrusted root ignored.
test.use({ serviceWorkers: "allow", launchOptions: { args: ["--ignore-certificate-errors"] } });

const LOADS = "__gxE2eLoads";
const loads = (page: Page) => page.evaluate((key) => Number(sessionStorage.getItem(key)), LOADS);

/** Marks the document so a later reload shows as a missing mark. */
const mark = (page: Page) => page.evaluate(() => ((window as any).__devMark = true));
const reloaded = (page: Page) => expect.poll(() => page.evaluate(() => (window as any).__devMark === undefined)).toBe(true);

// A ready Vite serves the fake's own page, never the gx UI: the page then shows "vitefake" and has no pill.
const onVite = (page: Page) => expect(page.locator("#root")).toHaveText("vitefake");

const devRow = (page: Page) => menu(page).getByRole("menuitemcheckbox", { name: "Dev mode", exact: true });
const pill = (page: Page, text: string) => page.locator('[data-part="dev-pill"]').filter({ hasText: new RegExp(`^${text}$`) });
const control = (fixture: Fixture, body: object) => writeFile(fixture.devControlFile, JSON.stringify(body));
const devKey = async (fixture: Fixture) => (await fixture.globalConfig())["server"]?.["web"]?.["dev-frontend"];

// Menu-less pages (the fake's own) leave the CLI, which writes the same setting the row does.
const setDev = (fixture: Fixture, on: boolean) => fixture.cli("server", "settings-set", "--set", `server.web.dev-frontend=${on}`);

async function flipMenuOn(page: Page) {
  await menuButton(page).click();
  await expect(devRow(page)).toHaveAttribute("aria-checked", "false");
  await devRow(page).click();
}

// Needs the server's reload event over SSE, the fake Vite's life cycle and a browser's service worker registry;
// DevPill, DevPopover, useDevToast and the settings row have vitest tests for each state alone.
test("Dev mode (menu flip, pill journey, settings row, PWA)", async ({ page, start }) => {
  // Menu: turning Dev mode on reloads every open page onto Vite; turning it off reloads them back.
  let fixture = await start("dev-mode", { shape: "off" });
  const other = await freshVisit(page, "#/");
  await expect(pill(page, "Dev")).toHaveCount(0);
  await mark(page);
  await mark(other);
  await flipMenuOn(page);
  await reloaded(page);
  await reloaded(other);
  await onVite(page);
  await onVite(other);
  expect(await devKey(fixture)).toBe(true);
  await setDev(fixture, false);
  // The fake's page has no event stream to hear a reload on, so these two load the built UI again by hand.
  await page.reload();
  await other.reload();
  await expect(menuButton(page)).toBeVisible();
  await expect(menuButton(other)).toBeVisible();
  await expect(pill(page, "Dev")).toHaveCount(0);
  expect(await loads(page)).toBe(2);
  await other.close();

  // Pill journey: installing, starting, then running swaps the page onto Vite's.
  fixture = await start("dev-mode", { shape: "installing" });
  await expect(pill(page, "Dev · installing")).toBeVisible();
  await control(fixture, { install: "ok", vite: "hold" });
  await expect(pill(page, "Dev · starting")).toBeVisible();
  expect(await loads(page)).toBe(0);
  await control(fixture, { vite: "ready" });
  await onVite(page);
  expect(await loads(page)).toBe(1);

  // Crash: down, a toast, and no reload. Turn off Dev mode in the popover: the key goes off and the pill goes away.
  fixture = await start("dev-mode", { shape: "starting" });
  await expect(pill(page, "Dev · starting")).toBeVisible();
  await control(fixture, { vite: "crash" });
  await expect(pill(page, "Dev · down")).toBeVisible();
  await expect(page.getByText("Vite is down. gx serves the built files.")).toBeVisible();
  expect(await loads(page), "the crash did not reload the page").toBe(0);
  await page.getByRole("button", { name: "Dev mode: Vite is down" }).click();
  await page.getByRole("button", { name: "Turn off Dev mode" }).click();
  await expect(pill(page, "Dev · down")).toHaveCount(0);
  expect(await devKey(fixture)).toBe(false);

  // Settings row: the switch turns Dev mode back on with no confirm dialog.
  await go(page, "#/settings/s/server");
  await page.getByRole("switch", { name: /dev.frontend/i }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect.poll(() => devKey(fixture)).toBe(true);

  // PWA: Dev mode unregisters the worker and clears the shell caches; off again lets a fresh one register.
  // Three reloads: the server's reload onto Vite, the dev worker's own while it unregisters, and the hand reload back.
  fixture = await start("dev-mode", { shape: "off", reloads: 3 });
  const workerState = () => page.evaluate(async () => (await navigator.serviceWorker.getRegistration())?.active?.state ?? "none");
  await expect.poll(workerState).toBe("activated");
  await page.evaluate(async (name) => (await caches.open(name)).put("/planted", new Response("x")), `${CACHE_PREFIX}planted`);
  await mark(page);
  await flipMenuOn(page);
  await reloaded(page);
  await onVite(page);
  await expect.poll(() => page.evaluate(async () => (await navigator.serviceWorker.getRegistrations()).length)).toBe(0);
  await expect.poll(() => page.evaluate((prefix) => caches.keys().then((keys) => keys.filter((k) => k.startsWith(prefix))), CACHE_PREFIX)).toEqual([]);
  await setDev(fixture, false);
  await page.reload();
  await expect.poll(workerState).toBe("activated");
});
