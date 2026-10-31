import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import type { SessionChatResponse, SessionInfo } from "../generated/server";
import { accepted, chat, chatOf, composerBox, prompts, refused, render, sendButton, session, stubPrompt, type, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

test("Send is off while Working, with the Working reason, and the box stays editable", async () => {
  render([session("s-1", "p", { status: { state: "working" } })], "/agents/s-1", true, chatOf(chat()));
  await type("hi");
  expect(sendButton().disabled).toBe(true);
  expect((await composerBox()).value).toBe("hi");
  expect(screen.getByText("Working. Send turns on when the session is idle.")).toBeTruthy();
});

test("the reason line names each state in order", async () => {
  const { update } = render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  await composerBox();
  update({ connected: false });
  expect(screen.getByText("Offline. Send turns on when gx reconnects.")).toBeTruthy();
  expect(sendButton().disabled).toBe(true);
  cleanup();
  render([session("s-2", "p", { harness: "other" })], "/agents/s-2", true, chatOf(chat()));
  expect(await screen.findByText("Sending works for Claude and Codex sessions only")).toBeTruthy();
  cleanup();
  render([session("s-3", "p", { status: { state: "exited" } })], "/agents/s-3", true, chatOf(chat()));
  expect(await screen.findByText("Exited. Restore it in gx, Agents tab.")).toBeTruthy();
  cleanup();
  render([session("s-4", "p", { status: { state: "rate-limited" } })], "/agents/s-4", true, chatOf(chat()));
  expect(await screen.findByText("Rate limited. Send turns on when the session is idle.")).toBeTruthy();
});

test("Idle with text enables Send; accept clears the draft and the box", async () => {
  prompts.length = 0;
  stubPrompt(accepted);
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  expect(sendButton().disabled).toBe(true);
  await type("hello");
  expect(screen.getByText("Draft saved in this browser")).toBeTruthy();
  expect(localStorage.getItem("gx.draft.composer.s-1")).toBe("hello");
  fireEvent.click(sendButton());
  await vi.waitFor(() => expect(localStorage.getItem("gx.draft.composer.s-1")).toBeNull());
  expect(prompts).toEqual([expect.objectContaining({ id: "s-1", text: "hello" })]);
  expect((await composerBox()).value).toBe("");
  expect(screen.queryByText("Draft saved in this browser")).toBeNull();
});

test("a refusal keeps the draft and shows its reason in red, inline", async () => {
  stubPrompt(refused("prefix-not-allowed", "Messages starting with / or ! are not sent."));
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  await type("/clear");
  fireEvent.click(sendButton());
  const line = await screen.findAllByText("Messages starting with / or ! are not sent.");
  expect(getComputedStyle(line[0]).color).not.toBe(getComputedStyle(screen.getByText("hello there")).color);
  expect((await composerBox()).value).toBe("/clear");
  expect(localStorage.getItem("gx.draft.composer.s-1")).toBe("/clear");
});

test("stale refetches the chat and keeps the draft", async () => {
  stubPrompt(refused("stale"));
  let calls = 0;
  render([session("s-1", "p")], "/agents/s-1", true, {
    chat: async () => {
      calls++;
      return { kind: "ok", detail: chat() };
    },
  });
  await type("keep me");
  await vi.waitFor(() => expect(calls).toBe(1));
  fireEvent.click(sendButton());
  await vi.waitFor(() => expect(calls).toBe(2));
  expect((await composerBox()).value).toBe("keep me");
});

test("a Codex session shows its chat, and Send follows the idle rule", async () => {
  prompts.length = 0;
  stubPrompt(accepted);
  render([session("s-2", "p", { harness: "codex" })], "/agents/s-2", true, chatOf(chat()));
  expect(await screen.findByText("hello there")).toBeTruthy();
  expect(screen.queryByText(/Chat works for/)).toBeNull();
  expect(screen.queryByText(/Sending works for/)).toBeNull();
  await type("hi");
  expect(sendButton().disabled).toBe(false);
  cleanup();
  render([session("s-3", "p", { harness: "codex", status: { state: "working" } })], "/agents/s-3", true, chatOf(chat()));
  await type("hi");
  expect(sendButton().disabled).toBe(true);
  expect(screen.getByText("Working. Send turns on when the session is idle.")).toBeTruthy();
});

test("a harness with no chat loads no chat, shows the notice, Show screen opens the screen modal and the composer says why it is off", async () => {
  const chatCalls: unknown[] = [];
  const screenCalls: string[] = [];
  render([session("s-2", "p", { harness: "other" })], "/agents/s-2", true, {
    chat: async () => {
      chatCalls.push(1);
      return { kind: "ok", detail: chat() };
    },
    screen: async (target) => {
      screenCalls.push(target);
      return { kind: "ok", detail: { rows: ["$ codex"] } } as never;
    },
  });
  expect(await screen.findByText("Chat works for Claude and Codex sessions only.")).toBeTruthy();
  expect(chatCalls).toEqual([]);
  expect(screen.getByText("Sending works for Claude and Codex sessions only")).toBeTruthy();
  expect(sendButton().disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Show screen" }));
  expect((await screen.findByLabelText("Screen rows")).textContent).toBe("$ codex");
  expect(screenCalls).toEqual(["s-2"]);
});

test.each([
  { kind: "crashed", over: { status: { state: "exited" }, exit: "crashed" }, label: "Agent crashed", buttons: [] },
  { kind: "blocked", over: { status: { state: "blocked" } }, label: "Waiting at a prompt in the terminal", buttons: ["Screen"] },
  { kind: "rate-limit", over: { status: { state: "rate-limited" } }, label: "Rate limited", buttons: [] },
] as { kind: string; over: Partial<SessionInfo>; label: string; buttons: string[] }[])(
  "a non-Claude session with a $kind failure shows the banner with only its buttons",
  async (c) => {
    render([session("s-2", "p", { ...c.over, harness: "other", failure: { kind: c.kind, label: c.label } })], "/agents/s-2", true);
    const banner = within(await screen.findByRole("region", { name: "Session failure" }));
    expect(banner.getByText(c.label)).toBeTruthy();
    expect(banner.queryAllByRole("button").map((b) => b.textContent)).toEqual(c.buttons);
    expect(screen.queryByRole("button", { name: "Restore" })).toBeNull();
    expect(screen.queryByRole("button", { name: "New session" })).toBeNull();
    expect(screen.getByText("Chat works for Claude and Codex sessions only.")).toBeTruthy();
  },
);

test.each(["auth", "stalled"])("a non-Claude session with a %s failure, which needs a transcript, or none, shows no banner", async (kind) => {
  render([session("s-2", "p", { harness: "other", failure: { kind, label: "x" } })], "/agents/s-2", true);
  await screen.findByText("Chat works for Claude and Codex sessions only.");
  expect(screen.queryByRole("region", { name: "Session failure" })).toBeNull();
  cleanup();
  render([session("s-2", "p", { harness: "other" })], "/agents/s-2", true);
  await screen.findByText("Chat works for Claude and Codex sessions only.");
  expect(screen.queryByRole("region", { name: "Session failure" })).toBeNull();
});

test("a design session shows its plain chat with no sheet, and links to its map's design page", async () => {
  const round = [{ id: "q1", header: "Pick", question: "Which one?", options: [] }] as unknown as SessionChatResponse["round"];
  render([session("s-1", "p", { design: true, ticket: "gx:map/01" })], "/agents/s-1", true, chatOf(chat({ round })));
  expect(await screen.findByText("hello there")).toBeTruthy();
  expect(screen.queryByText("Which one?")).toBeNull();
  expect(screen.getByRole("link", { name: "Open design page →" }).getAttribute("href")).toBe("#/d/gx:map");
  cleanup();
  render([session("s-1", "p", { ticket: "gx:map/01" })], "/agents/s-1", true, chatOf(chat()));
  await screen.findByText("hello there");
  expect(screen.queryByRole("link", { name: "Open design page →" })).toBeNull();
});

test("the draft is back after a remount, and one written through the store shows in the box", async () => {
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  await type("draft");
  cleanup();
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  expect((await composerBox()).value).toBe("draft");
  cleanup();
  localStorage.setItem("gx.draft.composer.s-2", "from the design page");
  render([session("s-2", "p")], "/agents/s-2", true, chatOf(chat()));
  expect((await composerBox()).value).toBe("from the design page");
});

const failureCases: {
  kind: string;
  over: Partial<SessionInfo>;
  label: string;
  text?: string;
  buttons: string[];
  reason?: string;
  sendOff: boolean;
}[] = [
  { kind: "crashed", over: { status: { state: "exited" }, exit: "crashed" }, label: "Agent crashed", buttons: [], reason: "Exited. Restore it in gx, Agents tab.", sendOff: true },
  { kind: "ended", over: { status: { state: "exited" }, exit: "clean" }, label: "Session ended", buttons: [], reason: "Exited. Restore it in gx, Agents tab.", sendOff: true },
  { kind: "auth", over: {}, label: "Not signed in", text: "Run /login", buttons: ["Retry"], sendOff: false },
  { kind: "stalled", over: { status: { state: "working" } }, label: "No progress for 15 min", buttons: ["Screen"], reason: "Working. Send turns on when the session is idle.", sendOff: true },
  { kind: "blocked", over: { status: { state: "blocked" } }, label: "Waiting at a prompt in the terminal", buttons: ["Screen"], reason: "Blocked. Send turns on when the session is idle.", sendOff: true },
  { kind: "rate-limit", over: { status: { state: "rate-limited" } }, label: "Rate limited", text: "resets at 5pm", buttons: [], reason: "Rate limited. Send turns on when the session is idle.", sendOff: true },
];

test.each(failureCases)("$kind failure: label, text, only its buttons, and the composer state", async (c) => {
  const failure = { kind: c.kind, label: c.label, text: c.text };
  render([session("s-1", "p", { ...c.over, failure })], "/agents/s-1", true, chatOf(chat()));
  const banner = within(await screen.findByRole("region", { name: "Session failure" }));
  expect(banner.getByText(c.label)).toBeTruthy();
  if (c.text) expect(banner.getByText(c.text)).toBeTruthy();
  expect(banner.queryAllByRole("button").map((b) => b.textContent)).toEqual(c.buttons);
  expect(screen.queryByRole("button", { name: "Restore" })).toBeNull();
  expect(screen.queryByRole("button", { name: "New session" })).toBeNull();
  expect(screen.getByText("hello there")).toBeTruthy();
  await type("hi");
  if (c.reason) expect(screen.getByText(c.reason)).toBeTruthy();
  expect(sendButton().disabled).toBe(c.sendOff);
});

test("Retry on an auth failure posts continue with the last message id", async () => {
  prompts.length = 0;
  stubPrompt(accepted);
  render([session("s-1", "p", { failure: { kind: "auth", label: "Not signed in" } })], "/agents/s-1", true, chatOf(chat()));
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
  await vi.waitFor(() => expect(prompts).toEqual([expect.objectContaining({ id: "s-1", text: "continue", last_message: "m1" })]));
});

const driven = [
  { kind: "Queue", over: { queue: true } },
  { kind: "Design", over: { design: true } },
];

test.each(driven)("a $kind agent opens with the composer off; confirm turns it on", async (c) => {
  prompts.length = 0;
  stubPrompt(accepted);
  render([session("s-1", "p", c.over)], "/agents/s-1", true, chatOf(chat()));
  expect(await screen.findByText("gx is driving this agent, so the composer is off.")).toBeTruthy();
  await type("hi");
  expect(sendButton().disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Turn on composer" }));
  const dialog = within(await screen.findByRole("dialog", { name: `Turn on composer for ${c.kind} agent` }));
  fireEvent.click(dialog.getByRole("button", { name: "Turn on composer" }));
  await vi.waitFor(() => expect(sendButton().disabled).toBe(false));
  expect(screen.queryByText("gx is driving this agent, so the composer is off.")).toBeNull();
  fireEvent.click(sendButton());
  await vi.waitFor(() => expect(prompts).toEqual([expect.objectContaining({ id: "s-1", text: "hi" })]));
});

test("Cancel keeps a Queue agent's composer off", async () => {
  render([session("s-1", "p", { queue: true })], "/agents/s-1", true, chatOf(chat()));
  await type("hi");
  fireEvent.click(await screen.findByRole("button", { name: "Turn on composer" }));
  fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(sendButton().disabled).toBe(true);
  expect(screen.getByText("gx is driving this agent, so the composer is off.")).toBeTruthy();
});

test("a plain session shows no driven line", async () => {
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  await composerBox();
  expect(screen.queryByRole("button", { name: "Turn on composer" })).toBeNull();
});
