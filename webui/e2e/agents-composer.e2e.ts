import type { Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { expect, nav, test, type Fixture } from "./fixture";
import { ask, chatOf, composer, created, freshVisit, go, messageBox, REPLY, sendButton, sessions } from "./agentsKit";

test("76. Send and draft", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);

  // The draft survives a new tab of the same browser.
  await messageBox(page).fill("a draft to keep");
  const second = await freshVisit(page, `/#/agents/${idle.id}`);
  await expect(messageBox(second)).toHaveValue("a draft to keep");
  await second.close();

  await sendButton(page).click();
  await expect(messageBox(page)).toHaveValue("");
  await expect(page.getByText("a draft to keep", { exact: true })).toBeVisible();
  await expect.poll(() => fixture.cli("agents", "chat", idle.id)).toContain("a draft to keep");
});

test("78. Send refused", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await expect(page.getByText(REPLY)).toBeVisible();

  // The page keeps its old chat while the chat read fails, so the CLI prompt moves the chat under it.
  let failing = true;
  await page.route("**/v1/sessions/chat*", (route) => (failing ? route.fulfill({ status: 500, body: "boom" }) : route.continue()));
  await ask(fixture, idle.id, "moved from the CLI");
  await expect(page.getByText("moved from the CLI", { exact: true })).toHaveCount(0);

  await messageBox(page).fill("my draft");
  failing = false;
  await sendButton(page).click();
  await expect(composer(page).getByText("the conversation moved; review it first")).toBeVisible();
  await expect(page.getByText("moved from the CLI", { exact: true })).toBeVisible();
  await expect(messageBox(page)).toHaveValue("my draft");
  expect(await fixture.cli("agents", "chat", idle.id)).not.toContain("my draft");

  // A send that starts with ! is refused by prefix and keeps the draft.
  await messageBox(page).fill("!ls");
  await sendButton(page).click();
  await expect(composer(page).getByText("text may not start with !")).toBeVisible();
  await expect(messageBox(page)).toHaveValue("!ls");
});

// 79 "Keyboard send" (x3) moved to AgentComposer.test.tsx and design/Composer.test.tsx ("design page: the key ...").

test("91. Removed session draft", async ({ page, start }) => {
  const fixture = await start("agents");
  const exited = (await sessions(fixture)).find((s) => s.status.state === "exited")!;
  await page.setViewportSize({ width: 1280, height: 800 });
  const draftKey = `gx.draft.composer.${exited.id}`;
  const stored = () => page.evaluate((k) => localStorage.getItem(k), draftKey);

  // The draft is written in a tab that is then closed.
  const tab = await freshVisit(page, `/#/agents/${exited.id}`);
  await messageBox(tab).fill("a draft for a dead session");
  await expect(composer(tab).getByText("Exited. Restore it in gx, Agents tab.")).toBeVisible();
  await expect.poll(stored).toBe("a draft for a dead session");
  await tab.close();

  // Removing the session and opening the Epics page drops the draft.
  await fixture.cli("agents", "remove", exited.id);
  await go(page, "#/");
  await expect.poll(stored).toBeNull();

  // A new session starts empty. Ids are random, so it cannot reuse the removed one's label.
  const before = await sessions(fixture);
  await fixture.cli("agents", "create", "--project", "demo", "--name", exited.id, "--prompt", "hello");
  const made = await created(fixture, before);
  await go(page, `#/agents/${made.id}`);
  await expect(messageBox(page)).toHaveValue("");
  await expect(composer(page).getByText("Draft saved in this browser")).toHaveCount(0);
});

test("93. Shared draft", async ({ page, start }) => {
  const fixture = await start("design", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/d/demo:design-epic");
  await page.getByRole("button", nav("Start design session")).click();
  const questions = page.getByRole("group", nav("Questions"));
  await expect(questions.getByText("Q8 - Which store?")).toBeVisible();
  const design = (await sessions(fixture)).find((s) => s.design)!;

  // A half-answered round survives opening the design page in a new tab.
  await questions.getByRole("button", { name: /SQLite/ }).click();
  const answered = (p: Page) => p.getByRole("button", { name: /^Question .*, answered$/ });
  await expect(answered(page)).toHaveCount(1);
  const fresh = await freshVisit(page, "/#/d/demo:design-epic");
  await expect(fresh.getByRole("group", nav("Questions"))).toBeVisible();
  await expect(answered(fresh)).toHaveCount(1);
  await fresh.close();

  // Text typed on either page shows live on the other.
  const agentsPage = await freshVisit(page, `/#/agents/${design.id}`);
  await page.getByRole("button", nav("Note")).click();
  const box = page.getByRole("textbox", { name: "Message" });
  await expect(box).toBeEnabled();
  await messageBox(agentsPage).fill("typed on Agents");
  await expect(box).toHaveValue("typed on Agents");
  await box.fill("typed on Design");
  await expect(messageBox(agentsPage)).toHaveValue("typed on Design");

  // Send answers sends it as the note, and both boxes are empty after.
  await questions.getByRole("button", { name: /Yes/ }).click();
  await page.getByRole("button", nav("Send answers")).click();
  await expect.poll(() => fixture.cli("agents", "chat", design.id)).toContain("typed on Design");
  await expect(messageBox(agentsPage)).toHaveValue("");
  await page.getByRole("button", nav("Note")).click();
  await expect(box).toHaveValue("");
  await agentsPage.close();
});

const WIDTHS = [
  { width: 1280, height: 800 },
  { width: 390, height: 844 },
];
const promoteButton = (page: Page) => page.getByRole("button", { name: "Promote to Design", exact: true });

/** Idle plain sessions of one harness, split by whether their chat has a message. */
async function plainIdle(fixture: Fixture, harness: "claude" | "codex") {
  const withMessages: SessionInfo[] = [];
  const empty: SessionInfo[] = [];
  for (const s of await sessions(fixture)) {
    if (s.status.state !== "idle" || s.design || s.queue || (s.harness === "codex") !== (harness === "codex")) continue;
    ((await chatOf(fixture, s.id)).messages.length > 0 ? withMessages : empty).push(s);
  }
  return { withMessages, empty };
}

test("185. Promote to Design button", async ({ page, start }) => {
  const fixture = await start("agents-promote");
  const claude = await plainIdle(fixture, "claude");
  const codex = await plainIdle(fixture, "codex");
  const all = await sessions(fixture);
  const exited = all.find((s) => s.status.state === "exited")!;
  const busy = all.find((s) => s.status.state === "working" && !s.design && !s.queue)!;

  // Each width promotes a session of its own, since a sent command keeps its session busy.
  for (const [i, size] of WIDTHS.entries()) {
    await page.setViewportSize(size);

    // Claude: the command is typed into the agent, the draft stays and no dialog opens.
    const session = claude.withMessages[i];
    await go(page, `#/agents/${session.id}`);
    await messageBox(page).fill("a draft to keep");
    await expect(promoteButton(page)).toBeEnabled();
    await promoteButton(page).click();
    await expect.poll(async () => fixture.input((await chatOf(fixture, session.id)).conversation_id)).toContain("/gx-promote-to-design");
    await expect(messageBox(page)).toHaveValue("a draft to keep");
    await expect(page.getByRole("dialog")).toHaveCount(0);

    // Codex: the same, with its `$` skill prefix.
    const codexSession = codex.withMessages[i];
    await go(page, `#/agents/${codexSession.id}`);
    await messageBox(page).fill("a codex draft");
    await promoteButton(page).click();
    await expect.poll(() => fixture.cli("agents", "chat", codexSession.id)).toContain("$gx-promote-to-design");
    await expect(messageBox(page)).toHaveValue("a codex draft");
    await expect(page.getByRole("dialog")).toHaveCount(0);

    // A session with no message, an exited one and a busy one are off, each with a reason.
    for (const off of [claude.empty[0], exited, busy]) {
      await go(page, `#/agents/${off.id}`);
      await expect(promoteButton(page)).toBeDisabled();
      await expect(promoteButton(page)).toHaveAttribute("title", /\S/);
    }
  }
});

test("186. No Promote to Design on driven agents", async ({ page, start }) => {
  const fixture = await start("agents-promote");
  const driven = (await sessions(fixture)).filter((s) => s.design || s.queue);
  expect(driven.some((s) => s.design)).toBe(true);
  expect(driven.some((s) => s.queue)).toBe(true);
  for (const size of WIDTHS) {
    await page.setViewportSize(size);
    for (const s of driven) {
      await go(page, `#/agents/${s.id}`);
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await expect(promoteButton(page)).toHaveCount(0);
    }
  }
});

// 114 "Read-only Queue agent" moved to AgentsPage.composer.test.tsx ("a $kind agent opens with the composer
// off; confirm turns it on", "Cancel keeps a Queue agent's composer off").
