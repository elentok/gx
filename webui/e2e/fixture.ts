import { execFile, spawn } from "node:child_process";
import { rename, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import { expect, test as base, type Page } from "@playwright/test";

const run = promisify(execFile);

// A reload drops this window property, so a flow that reloads fails its test.
const NO_RELOAD = "__gxE2eNoReload";

interface FixtureOutput {
  web_url: string;
  env: string[];
  gx: string;
  hold_file: string;
  disk_dir: string;
}

export interface Fixture {
  /** Runs a gx CLI verb against this fixture's server and no other. */
  cli(...args: string[]): Promise<string>;
  /** Like `cli`, with extra environment variables. */
  cliEnv(extra: Record<string, string>, ...args: string[]): Promise<string>;
  /** Lets the workers frozen by `start(scenario, { hold: true })` finish. */
  release(): Promise<void>;
  /** Sets the free space the server reads from the fake disk, a size string such as `15G`. */
  setDiskFree(size: string): Promise<void>;
  /** Sets what the next cleanup does: a size string frees to it, `fail` fails, empty does nothing. Set it before free. */
  setDiskClean(behaviour: string): Promise<void>;
}

// A rename keeps the server from reading a half-written control file.
async function writeControl(dir: string, name: string, value: string): Promise<void> {
  const tmp = join(dir, `${name}.e2e-tmp`);
  await writeFile(tmp, `${value}\n`);
  await rename(tmp, join(dir, name));
}

/** Starts a fresh fixture server for `scenario` and signs the page in. */
async function startFixture(
  scenario: string,
  page: Page,
  hold: boolean,
  autoAdvanceDelay?: string,
  config: Record<string, string> = {},
): Promise<{ fixture: Fixture; stop: () => Promise<void> }> {
  const proc = spawn(
    process.env.GX_E2E_GXFIXTURE!,
    [
      "--gx",
      process.env.GX_E2E_GX!,
      "--fakeclaude",
      process.env.GX_E2E_FAKECLAUDE!,
      ...(hold ? ["--hold"] : []),
      ...(autoAdvanceDelay ? ["--auto-advance-delay", autoAdvanceDelay] : []),
      ...Object.entries(config).flatMap(([key, value]) => ["--config", `${key}=${value}`]),
      scenario,
    ],
    { stdio: ["ignore", "pipe", "inherit"] },
  );
  const exited = new Promise<void>((resolve) => proc.once("exit", () => resolve()));
  const out = await new Promise<FixtureOutput>((resolve, reject) => {
    let buf = "";
    proc.stdout!.on("data", (chunk: Buffer) => {
      buf += chunk;
      const line = buf.split("\n")[0];
      if (buf.includes("\n")) resolve(JSON.parse(line));
    });
    proc.once("exit", (code) => reject(new Error(`gxfixture exited with ${code} before it printed its output`)));
  });
  const env = { ...process.env, ...Object.fromEntries(out.env.map((kv) => [kv.slice(0, kv.indexOf("=")), kv.slice(kv.indexOf("=") + 1)])) };
  const fixture: Fixture = {
    async cli(...args) {
      const { stdout } = await run(out.gx, args, { env, cwd: tmpdir() });
      return stdout;
    },
    async cliEnv(extra, ...args) {
      const { stdout } = await run(out.gx, args, { env: { ...env, ...extra }, cwd: tmpdir() });
      return stdout;
    },
    async release() {
      await rm(out.hold_file, { force: true });
    },
    setDiskFree: (size) => writeControl(out.disk_dir, "free", size),
    setDiskClean: (behaviour) => writeControl(out.disk_dir, "clean", behaviour),
  };
  // The first visit trades the token in web_url for the web cookie.
  await page.goto(out.web_url);
  await page.evaluate((key) => ((window as unknown as Record<string, unknown>)[key] = true), NO_RELOAD);
  return {
    fixture,
    stop: async () => {
      proc.kill("SIGTERM");
      await exited;
    },
  };
}

/**
 * `start(scenario)` boots a fixture server and opens the page on it. Every test
 * gets its own server, stopped when the test ends.
 */
export const test = base.extend<{
  start: (
    scenario: string,
    opts?: { hold?: boolean; autoAdvanceDelay?: string; config?: Record<string, string> },
  ) => Promise<Fixture>;
}>({
  start: async ({ page }, use) => {
    const stops: Array<() => Promise<void>> = [];
    await use(async (scenario, opts) => {
      const { fixture, stop } = await startFixture(scenario, page, opts?.hold ?? false, opts?.autoAdvanceDelay, opts?.config);
      stops.push(stop);
      return fixture;
    });
    const survived = await page.evaluate((key) => (window as unknown as Record<string, unknown>)[key] === true, NO_RELOAD);
    await Promise.all(stops.map((stop) => stop()));
    expect(survived, "the page was reloaded during the flow").toBe(true);
  },
});

/** Matches a role's accessible name exactly. */
export const nav = (name: string) => ({ name, exact: true });

/** The `gx agents list` rows that mention `slug` (tab-separated, the session id first). */
export const sessionRows = async (fixture: Fixture, slug: string) =>
  (await fixture.cli("agents", "list")).split("\n").filter((l) => l.includes(slug));

/** A section of the Epics page, expanded if it starts collapsed. */
export async function section(page: Page, title: string) {
  const region = page.getByRole("region", { name: new RegExp(`^${title}`) });
  const header = region.getByRole("button", { name: new RegExp(`^${title}`) });
  if ((await header.getAttribute("aria-expanded")) === "false") await header.click();
  return region;
}

/** The Epics page card whose title link is `epic`. */
export const card = (page: Page, epic: string) =>
  page.locator("[data-kind]").filter({ has: page.getByRole("link", { name: epic, exact: true }) });

/** Holds every request to `url` until the returned release runs, so a test can see a button while its write is pending. */
export async function holdRoute(page: Page, url: string): Promise<() => void> {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  await page.route(url, async (route) => {
    await gate;
    await route.continue();
  });
  return release;
}

/** The toasts on screen. */
export const toasts = (page: Page) => page.getByRole("region", { name: /^Notifications/ }).getByRole("status");

export { expect };
