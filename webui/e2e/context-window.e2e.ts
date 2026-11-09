import { appendFile } from "node:fs/promises";
import { basename } from "node:path";
import type { Locator, Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { ask, chatOf, composer, freshVisit, go, row, sessions, sidebar } from "./agentsKit";
import { expect, nav, test, type Fixture } from "./fixture";

const MODEL = /^(Claude|Codex) \S+/;
// A NoChat session's model text is not a Claude or Codex name, so only a two-word label is pinned.
const NOCHAT_MODEL = /^\S+ \S+/;

const footer = (page: Page) => page.getByTestId("footer-line");
const badge = (page: Page) => footer(page).locator("[data-tone]");
const ring = (page: Page) => footer(page).getByTestId("context-ring");
const fillOf = async (page: Page) => Number(await ring(page).getAttribute("data-fill"));

/** An assistant line whose usage holds `tokens`; the badge follows the newest one. */
const usageLine = (tokens: number) =>
  JSON.stringify({
    type: "assistant",
    timestamp: new Date().toISOString(),
    message: { model: "claude-sonnet-4-5", usage: { input_tokens: tokens, output_tokens: 0 } },
  }) + "\n";

const conversationOf = async (fixture: Fixture, s: SessionInfo) => (await chatOf(fixture, s.id)).conversation_id;

test("202. Context badge (stories 1 to 28)", async ({ page, start }) => {
  test.setTimeout(240_000);
  const fixture = await start("context-window", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 900 });
  const all = await sessions(fixture);

  // The keyed transcripts name their sessions by conversation id.
  const keyed = async (key: string) => {
    const conv = basename(fixture.transcripts[key], ".jsonl");
    for (const s of all.filter((x) => !x.queue && !x.flow && x.harness === "claude")) {
      if ((await conversationOf(fixture, s)) === conv) return s;
    }
    throw new Error(`no session for ${key}`);
  };
  const plain = all.filter((s) => !s.queue && !s.flow);
  const open = async (s: SessionInfo) => {
    await go(page, `#/agents/${s.id}`);
    await expect(footer(page)).toBeVisible();
  };

  // Story 1, 4, 6, 24: the Agents composer footer holds the model and a green badge; the header holds no model.
  const green = await keyed("green");
  await open(green);
  await expect(composer(page)).toBeVisible();
  await expect(composer(page).getByTestId("footer-line")).toContainText(MODEL);
  await expect(page.getByTestId("page-header-meta")).not.toContainText(MODEL);
  await expect(badge(page)).toHaveText("40K");
  await expect(badge(page)).toHaveAttribute("data-tone", "done");

  // Stories 10, 13, 5, 4: the tooltip, then a usage line grows the count and the ring with no reload.
  const before = await fillOf(page);
  await badge(page).hover();
  await expect(page.getByRole("tooltip")).toHaveText("40K of 130K smart zone");
  await page.mouse.move(0, 0);
  // The server announces transcript growth only while a session works, so the line lands as part of a turn.
  await appendFile(fixture.transcripts.green, usageLine(70_400));
  await ask(fixture, green.id, "and again");
  await expect(badge(page)).toHaveText("70K");
  await expect.poll(() => fillOf(page)).toBeGreaterThan(before);
  await badge(page).hover();
  await expect(page.getByRole("tooltip")).toHaveText("70K of 130K smart zone");
  await page.mouse.move(0, 0);

  // Story 6: each level is its own tone.
  for (const [key, text, tone] of [
    ["yellow", "90K", "waiting"],
    ["orange", "128K", "paused"],
    ["red", "170K", "deadlocked"],
  ] as const) {
    await open(await keyed(key));
    await expect(badge(page)).toHaveText(text);
    await expect(badge(page)).toHaveAttribute("data-tone", tone);
  }

  // Stories 11 and 12: a fresh session shows the model alone.
  const noCount = plain.filter((s) => s.harness === "claude" && s.context_tokens == null);
  const chatLengths = await Promise.all(noCount.map(async (s) => (await chatOf(fixture, s.id)).messages.length));
  const freshSession = noCount[chatLengths.indexOf(0)];
  await open(freshSession);
  await expect(footer(page)).toContainText(MODEL);
  await expect(badge(page)).toHaveCount(0);

  // Story 14: a finished session keeps its last count.
  const finished = plain.find((s) => s.status.state === "exited" && s.context_tokens === 40_000)!;
  await open(finished);
  await expect(badge(page)).toHaveText("40K");

  // Story 15: /compact keeps the count until the next assistant message.
  const compact = await keyed("compact");
  await open(compact);
  await expect(badge(page)).toHaveText("90K");
  await fixture.controlClaude(await conversationOf(fixture, compact), { action: "usage", tokens: 20_000 });
  await ask(fixture, compact.id, "go on");
  await expect(badge(page)).toHaveText("20K");

  // Story 16: no transcript file, no badge, and the page works.
  const missing = plain.find((s) => s.harness === "claude" && s.context_tokens == null && s.id !== freshSession.id && s.id !== finished.id)!;
  await open(missing);
  await expect(footer(page)).toContainText(MODEL);
  await expect(badge(page)).toHaveCount(0);

  // Story 17: a Codex session shows the same badge and tooltip.
  await open(plain.find((s) => s.harness === "codex" && s.context_tokens === 90_000)!);
  await expect(badge(page)).toHaveText("90K");
  await badge(page).hover();
  await expect(page.getByRole("tooltip")).toHaveText("90K of 130K smart zone");
  await page.mouse.move(0, 0);

  // Story 27: a NoChat harness shows the model and no badge on Agents, Design and Triage, and its snapshot row has no count.
  const noChat = (await sessions(fixture)).filter((s) => !s.queue && s.harness === "nochat");
  expect(noChat.map((s) => s.flow ?? "agents").sort()).toEqual(["agents", "design", "triage"]);
  for (const s of noChat) expect(s.context_tokens ?? null).toBeNull();
  const noChatRoutes = { agents: `#/agents/${noChat.find((s) => !s.flow)!.id}`, design: "#/d/nochat:design-nochat", triage: "#/triage/nochat" };
  for (const route of Object.values(noChatRoutes)) {
    await go(page, route);
    // Design and Triage show a notice in place of the composer for a NoChat harness.
    await expect(footer(page)).toContainText(NOCHAT_MODEL);
    await expect(badge(page)).toHaveCount(0);
  }

  // Story 18: a Queue agent's session view has the badge; its sidebar row has none.
  const queue = all.find((s) => s.queue)!;
  await fixture.controlClaude(await conversationOf(fixture, queue), { action: "usage", tokens: 60_000 });
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === queue.id)!.context_tokens).toBe(60_000);
  // A Queue agent's transcript growth fires no event, so a fresh visit is what reads the count.
  const visit = await freshVisit(page, `#/agents/${queue.id}`);
  await expect(badge(visit)).toHaveText("60K");
  await sidebar(visit).getByRole("button", nav("Expand queue-epic")).click();
  await expect(row(visit, queue)).toBeVisible();
  await expect(row(visit, queue).getByTestId("context-ring")).toHaveCount(0);
  await visit.close();

  // Stories 2, 22, 23, 26, 28: the Design composer, Note closed then open, in dark scheme.
  const design = all.find((s) => s.flow === "design" && s.flow_key === "demo:design-epic")!;
  await go(page, "#/d/demo:design-epic");
  await expect(composer(page)).toBeVisible();
  await expect(badge(page)).toHaveText("128K");
  await expect(badge(page)).toHaveAttribute("data-tone", "paused");
  await noteClosedAboveButtons(page);
  await noteOpenInsideBox(page);
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(badge(page)).toBeVisible();
  const [text, box] = await badge(page).evaluate((el) => {
    let bg = "rgba(0, 0, 0, 0)";
    for (let p: Element | null = el; p && /rgba\(.*, 0\)$|transparent/.test(bg); p = p.parentElement) bg = getComputedStyle(p).backgroundColor;
    return [getComputedStyle(el).color, bg];
  });
  expect(text).not.toBe(box);
  await page.emulateMedia({ colorScheme: "light" });

  // Stories 3, 23, 25: the Triage composer, badge from a usage line, no model in the header.
  const triage = all.find((s) => s.flow === "triage")!;
  await go(page, "#/triage/demo");
  await expect(composer(page)).toBeVisible();
  await expect(footer(page)).toContainText(MODEL);
  await expect(badge(page)).toHaveCount(0);
  await expect(page.getByTestId("page-header-meta")).not.toContainText(MODEL);
  // An idle session's growth fires no event either, so a fresh visit reads the new count.
  await fixture.controlClaude(await conversationOf(fixture, triage), { action: "usage", tokens: 95_000 });
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === triage.id)!.context_tokens).toBe(95_000);
  const tri = await freshVisit(page, "#/triage/demo");
  await expect(badge(tri)).toHaveText("95K");
  await noteClosedAboveButtons(tri);

  // Story 14: a Triage and a Design that end hide the composer and keep the last count.
  await fixture.cli("agents", "stop", triage.id);
  await expect(composer(tri)).toHaveCount(0);
  await expect(footer(tri)).toContainText(MODEL);
  await expect(badge(tri)).toHaveText("95K");
  await fixture.cli("agents", "stop", design.id);
  await go(tri, "#/d/demo:design-epic");
  await expect(composer(tri)).toHaveCount(0);
  await expect(footer(tri)).toContainText(MODEL);
  await expect(badge(tri)).toHaveText("128K");
});

/** Story 23: with Note closed the footer line sits directly above the button row, with no border. */
async function noteClosedAboveButtons(page: Page) {
  const form = composer(page);
  const note = form.getByRole("button", nav("Note"));
  await expect(note).toBeVisible();
  await expect(footer(page)).toHaveCSS("border-top-width", "0px");
  await expect(footer(page)).toHaveCSS("border-bottom-width", "0px");
  const f = (await footer(page).boundingBox())!;
  const n = (await note.boundingBox())!;
  expect(f.y + f.height).toBeLessThanOrEqual(n.y);
  expect(n.y - (f.y + f.height)).toBeLessThan(24);
}

/** Story 22: with Note open the footer line is inside the text box. */
async function noteOpenInsideBox(page: Page) {
  await composer(page).getByRole("button", nav("Note")).click();
  const box: Locator = composer(page).getByRole("textbox", { name: "Message" });
  await expect(box).toBeVisible();
  const b = (await box.boundingBox())!;
  const f = (await footer(page).boundingBox())!;
  expect(f.y).toBeGreaterThanOrEqual(b.y - 1);
  expect(f.y + f.height).toBeLessThanOrEqual(b.y + b.height + 40);
}
