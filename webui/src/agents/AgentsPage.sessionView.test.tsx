import { cleanup, screen, within } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import type { SessionChatResponse } from "../generated/server";
import { chat, chatOf, render, session, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

test("the header names the session, status, dir and ticket, with a harness chip for Claude and Codex", async () => {
  render([session("s-1", "p", { name: "one", dir: "/work/one", ticket: "gx:m/01" })], "/agents/s-1", true, chatOf(chat()));
  const header = within((await screen.findByRole("heading", { name: /one/ })).closest("header")!);
  expect(header.getByText("Idle")).toBeTruthy();
  expect(header.getByText("/work/one")).toBeTruthy();
  expect(header.getByText("gx:m/01")).toBeTruthy();
  expect(header.getByText("Claude")).toBeTruthy();
  expect(header.queryByText("Codex")).toBeNull();
  cleanup();
  render([session("s-2", "p", { name: "two", harness: "codex" })], "/agents/s-2", true, chatOf(chat()));
  expect(await screen.findByText("Codex")).toBeTruthy();
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

test("the header shows tier and model chips with a tier tooltip, and CLI default until a model is reported", async () => {
  render([session("s-1", "p", { name: "one", tier: "strongest", model: "claude-opus-5", effort: "high" })], "/agents/s-1", true, chatOf(chat()));
  const label = await screen.findByText("claude-opus-5 · high");
  expect(label.getAttribute("title")).toBe("Strongest tier");
  expect(screen.getByText("Strongest")).toBeTruthy();
  expect(screen.queryByText("switched by hand")).toBeNull();
  cleanup();
  render([session("s-1", "p", { name: "one", tier: "strongest", model: "claude-opus-5" })], "/agents/s-1", true, chatOf(chat()));
  expect(await screen.findByText("claude-opus-5")).toBeTruthy();
  cleanup();
  render([session("s-1", "p", { name: "one", tier: "worker" })], "/agents/s-1", true, chatOf(chat()));
  expect(await screen.findByText("Worker · CLI default")).toBeTruthy();
});

test("the header of a Codex session with a hand-switched model says so", async () => {
  render([session("s-1", "p", { name: "one", harness: "codex", tier: "worker", model: "gpt-5.5", effort: "xhigh", model_switched: true })], "/agents/s-1", true, chatOf(chat()));
  expect(await screen.findByText("Codex")).toBeTruthy();
  expect(screen.getByText("gpt-5.5 · xhigh")).toBeTruthy();
  expect(screen.getByText("switched by hand")).toBeTruthy();
});

test("a session with a model and no tier has a tooltip that names no tier", async () => {
  render([session("s-1", "p", { name: "one", model: "claude-sonnet-5-5" })], "/agents/s-1", true, chatOf(chat()));
  const label = await screen.findByText("claude-sonnet-5-5");
  expect(label.getAttribute("title")).toBeNull();
});

test("the header shows no model label when tier, model and effort are all empty", async () => {
  render([session("s-1", "p", { name: "one" })], "/agents/s-1", true, chatOf(chat()));
  const header = within((await screen.findByRole("heading", { name: /one/ })).closest("header")!);
  expect(header.getByText("Claude")).toBeTruthy();
  expect(header.queryByText(/CLI default|·/)).toBeNull();
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
