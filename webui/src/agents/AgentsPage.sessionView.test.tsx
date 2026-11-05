import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import type { SessionChatResponse } from "../generated/server";
import { chat, chatOf, render, session, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

test("the header names the session, status, dir and ticket, with the joined model name", async () => {
  render([session("s-1", "p", { name: "one", dir: "/work/one", ticket: "gx:m/01" })], "/agents/s-1", true, chatOf(chat()));
  const header = within((await screen.findByRole("heading", { name: /one/ })).closest("header")!);
  expect(header.getByText("Idle")).toBeTruthy();
  expect(header.getByText("/work/one")).toBeTruthy();
  expect(header.getByText("gx:m/01")).toBeTruthy();
  expect(header.getByText("Claude CLI default")).toBeTruthy();
  cleanup();
  render([session("s-2", "p", { name: "two", harness: "codex" })], "/agents/s-2", true, chatOf(chat()));
  expect(await screen.findByText("Codex CLI default")).toBeTruthy();
});

test("renders messages, the truncation note and the background strip", async () => {
  render(
    [session("s-1", "p")],
    "/agents/s-1",
    true,
    chatOf(chat({ truncated: true, background: [{ kind: "shell", label: "tail -f" }] as SessionChatResponse["background"] })),
  );
  expect(await screen.findByText("hello there")).toBeTruthy();
  expect(screen.getByText("Earlier messages hidden")).toBeTruthy();
  expect(screen.getByRole("list", { name: "Running in background" })).toBeTruthy();
});

test("a busy session shows a plain Working row, and an empty chat says so", async () => {
  render([session("s-1", "p", { status: { state: "working" } })], "/agents/s-1", true, chatOf(chat()));
  expect((await screen.findByRole("status")).textContent).toBe("Working");
  cleanup();
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat({ messages: [] })));
  expect(await screen.findByText("No messages yet")).toBeTruthy();
});

test("a round in the chat shows no question sheet", async () => {
  const round = [{ label: "Pick", title: "Which?", options: [{ label: "A" }] }] as unknown as SessionChatResponse["round"];
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat({ round })));
  expect(await screen.findByText("hello there")).toBeTruthy();
  expect(screen.queryByText("Which?")).toBeNull();
  expect(screen.queryByRole("group", { name: "View" })).toBeNull();
});

test("the header reads the model and effort as one line, with no tier pill, and the menu's Agent block holds the tier", async () => {
  render([session("s-1", "p", { name: "one", tier: "strongest", model: "claude-opus-5", effort: "high" })], "/agents/s-1", true, chatOf(chat()));
  const header = within((await screen.findByRole("heading", { name: /one/ })).closest("header")!);
  expect(header.getByText("Claude Opus 5").parentElement!.textContent).toBe("Claude Opus 5 · high");
  expect(header.queryByText("Strongest")).toBeNull();
  expect(header.queryByText("switched by hand")).toBeNull();
  fireEvent.click(header.getByRole("button", { name: "More actions" }));
  expect(await screen.findByText("Agent")).toBeTruthy();
  expect(screen.getByText("Strongest")).toBeTruthy();
  expect(screen.queryByText("Effort")).toBeNull();
});

test("the header of a Codex session with a hand-switched model says so in the menu", async () => {
  render([session("s-1", "p", { name: "one", harness: "codex", tier: "worker", model: "gpt-5.5", effort: "xhigh", model_switched: true })], "/agents/s-1", true, chatOf(chat()));
  expect(await screen.findByText("Codex GPT-5.5")).toBeTruthy();
  expect(screen.queryByText("switched by hand")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "More actions" }));
  expect(await screen.findByText("switched by hand")).toBeTruthy();
});

test("a Queue agent's header still has the menu, with its Agent block and no Stop or Remove", async () => {
  render([session("s-1", "p", { name: "q", queue: true, tier: "worker" })], "/agents/s-1", true, chatOf(chat()));
  await screen.findByRole("heading", { name: /q/ });
  fireEvent.click(screen.getByRole("button", { name: "More actions" }));
  expect(await screen.findByText("Tier")).toBeTruthy();
  expect(screen.queryByRole("menuitem", { name: "Stop" })).toBeNull();
  expect(screen.queryByRole("menuitem", { name: "Remove" })).toBeNull();
});

test("a Queue agent's session offers no Screen and no Type field", async () => {
  render([session("s-1", "p", { name: "q", queue: true })], "/agents/s-1", true, chatOf(chat()));
  await screen.findByRole("heading", { name: /q/ });
  expect(screen.queryByRole("button", { name: /Screen/ })).toBeNull();
  expect(screen.queryByRole("textbox", { name: "Text to type" })).toBeNull();
});

test("a failed re-read keeps the last chat", async () => {
  let calls = 0;
  const { emit } = render([session("s-1", "p")], "/agents/s-1", true, {
    chat: async () => {
      if (++calls > 1) throw new Error("500");
      return { kind: "ok", detail: chat() };
    },
  });
  expect(await screen.findByText("hello there")).toBeTruthy();
  emit("s-1");
  await vi.waitFor(() => expect(calls).toBe(2));
  expect(screen.getByText("hello there")).toBeTruthy();
});
