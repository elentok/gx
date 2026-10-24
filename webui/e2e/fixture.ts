import { execFile, spawn } from "node:child_process";
import { rm } from "node:fs/promises";
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
}

export interface Fixture {
  /** Runs a gx CLI verb against this fixture's server and no other. */
  cli(...args: string[]): Promise<string>;
  /** Lets the workers frozen by `start(scenario, { hold: true })` finish. */
  release(): Promise<void>;
}

/** Starts a fresh fixture server for `scenario` and signs the page in. */
async function startFixture(
  scenario: string,
  page: Page,
  hold: boolean,
): Promise<{ fixture: Fixture; stop: () => Promise<void> }> {
  const proc = spawn(
    process.env.GX_E2E_GXFIXTURE!,
    ["--gx", process.env.GX_E2E_GX!, "--fakeclaude", process.env.GX_E2E_FAKECLAUDE!, ...(hold ? ["--hold"] : []), scenario],
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
      const { stdout } = await run(out.gx, args, { env });
      return stdout;
    },
    async release() {
      await rm(out.hold_file, { force: true });
    },
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
export const test = base.extend<{ start: (scenario: string, opts?: { hold?: boolean }) => Promise<Fixture> }>({
  start: async ({ page }, use) => {
    const stops: Array<() => Promise<void>> = [];
    await use(async (scenario, opts) => {
      const { fixture, stop } = await startFixture(scenario, page, opts?.hold ?? false);
      stops.push(stop);
      return fixture;
    });
    const survived = await page.evaluate((key) => (window as unknown as Record<string, unknown>)[key] === true, NO_RELOAD);
    await Promise.all(stops.map((stop) => stop()));
    expect(survived, "the page was reloaded during the flow").toBe(true);
  },
});

export { expect };
