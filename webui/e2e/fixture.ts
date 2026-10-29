import { execFile, spawn } from "node:child_process";
import { createHmac } from "node:crypto";
import { readdir, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import { expect, test as base, type Page } from "@playwright/test";

const run = promisify(execFile);

// A reload drops this window property, so a flow that reloads fails its test.
const NO_RELOAD = "__gxE2eNoReload";

const TOTP_PERIOD_S = 30;
const B32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";

/** The RFC 6238 six-digit code for `secret` (unpadded base32) at `step`. */
function totpCode(secret: string, step: number): string {
  let bits = "";
  for (const ch of secret.toUpperCase()) bits += B32.indexOf(ch).toString(2).padStart(5, "0");
  const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)));
  const msg = Buffer.alloc(8);
  msg.writeBigUInt64BE(BigInt(step));
  const sum = createHmac("sha1", key).update(msg).digest();
  const offset = sum[sum.length - 1] & 0x0f;
  return String(sum.readUInt32BE(offset) & 0x7fffffff).slice(-6).padStart(6, "0");
}

interface FixtureOutput {
  web_url: string;
  cookie_name: string;
  /** Empty in the `web-setup` and `web-login` scenarios, where the page starts signed out. */
  cookie: string;
  /** The web password; absent while the setup is pending (`web-setup`). */
  password?: string;
  /** Absent when the server runs with `server.web.totp=false`. */
  totp_secret?: string;
  /** The last TOTP step the fixture used. */
  totp_step?: number;
  env: string[];
  gx: string;
  hold_file: string;
  disk_dir: string;
  health_dir: string;
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
  /** Sets the fake CPU and memory the server reads, space-separated pairs such as `cpu=75 mem=85`. */
  setHealth(values: string): Promise<void>;
  /** Replaces the fake process table the popover's cause lists read: `proc`, `hidden` and `busy` lines. */
  setProcesses(lines: string): Promise<void>;
  /** Stops the fixture server mid-flow. Teardown skips a server that is already stopped. */
  stopServer(): Promise<void>;
  /** The argv of every fake claude launch, oldest first. A relaunch of a session moves its entry to the end. */
  launches(): Promise<string[][]>;
  /**
   * A TOTP code for the fixture's secret, returned only once a step later than the last one used has
   * begun (up to 30 seconds), since the server takes each step once. Every flow that needs a code uses it.
   */
  nextTotpCode(): Promise<string>;
  /** The web password the fixture's setup used; empty while the setup is pending. */
  readonly password: string;
  /** The TOTP secret; empty when the server runs with `server.web.totp=false`. */
  readonly totpSecret: string;
}

/** The value after `--name` in an argv, or undefined. */
export const flag = (args: string[], name: string) => {
  const at = args.indexOf(`--${name}`);
  return at < 0 ? undefined : args[at + 1];
};

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
  stallThreshold?: string,
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
      ...(stallThreshold ? ["--stall-threshold", stallThreshold] : []),
      ...Object.entries(config).flatMap(([key, value]) => ["--config", `${key}=${value}`]),
      scenario,
    ],
    { stdio: ["ignore", "pipe", "inherit"] },
  );
  const exited = new Promise<void>((resolve) => proc.once("exit", () => resolve()));
  let stopping: Promise<void> | undefined;
  const stop = () => {
    stopping ??= (async () => {
      if (proc.exitCode === null && proc.signalCode === null) proc.kill("SIGTERM");
      await exited;
    })();
    return stopping;
  };
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
  let lastStep = out.totp_step ?? 0;
  const fixture: Fixture = {
    password: out.password ?? "",
    totpSecret: out.totp_secret ?? "",
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
    setHealth: (values) => writeControl(out.health_dir, "system", values),
    setProcesses: (lines) => writeControl(out.health_dir, "processes", lines),
    stopServer: stop,
    async launches() {
      const dir = join(env.HOME!, ".claude", "agentfake");
      const files = (await readdir(dir).catch(() => [] as string[])).filter((n) => n.endsWith(".args"));
      const stamped = await Promise.all(files.map(async (n) => ({ n, at: (await stat(join(dir, n))).mtimeMs })));
      const ordered = stamped.sort((a, b) => a.at - b.at);
      return Promise.all(ordered.map(async ({ n }) => JSON.parse(await readFile(join(dir, n), "utf8")) as string[]));
    },
    async nextTotpCode() {
      if (!out.totp_secret) throw new Error("this fixture runs without TOTP");
      const deadline = Date.now() + (TOTP_PERIOD_S + 1) * 1000;
      for (;;) {
        const step = Math.floor(Date.now() / 1000 / TOTP_PERIOD_S);
        if (step > lastStep) {
          lastStep = step;
          return totpCode(out.totp_secret, step);
        }
        if (Date.now() > deadline) throw new Error("no new TOTP step began in time");
        await new Promise((resolve) => setTimeout(resolve, 250));
      }
    },
  };
  // The cookie is `__Host-`: set with the URL (secure, no domain), never a domain.
  // The web-setup and web-login scenarios start signed out, with no cookie.
  if (out.cookie) {
    await page.context().addCookies([{ name: out.cookie_name, value: out.cookie, url: out.web_url, secure: true }]);
  }
  await page.goto(out.web_url);
  await page.evaluate((key) => ((window as unknown as Record<string, unknown>)[key] = true), NO_RELOAD);
  return { fixture, stop };
}

/**
 * `start(scenario)` boots a fixture server and opens the page on it. Every test
 * gets its own server, stopped when the test ends.
 */
export const test = base.extend<{
  start: (
    scenario: string,
    opts?: { hold?: boolean; autoAdvanceDelay?: string; stallThreshold?: string; config?: Record<string, string> },
  ) => Promise<Fixture>;
}>({
  start: async ({ page }, use) => {
    const stops: Array<() => Promise<void>> = [];
    await use(async (scenario, opts) => {
      const { fixture, stop } = await startFixture(scenario, page, opts?.hold ?? false, opts?.autoAdvanceDelay, opts?.config, opts?.stallThreshold);
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
