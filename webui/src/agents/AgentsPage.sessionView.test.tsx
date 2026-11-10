import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import type { SessionChatResponse, SessionRoundCard } from "../generated/server";
import { roundCards } from "../design/roundFixture";
import { chat, chatOf, render, session, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

test("the header names the session, status, dir and ticket, with no model", async () => {
  render([session("s-1", "p", { name: "one", dir: "/work/one", ticket: "gx:m/01" })], "/agents/s-1", true, chatOf(chat()));
  const header = within((await screen.findByRole("heading", { name: /one/ })).closest("header")!);
  expect(header.getByText("Idle")).toBeTruthy();
  expect(header.getByText("/work/one")).toBeTruthy();
  expect(header.getByText("gx:m/01")).toBeTruthy();
  expect(header.queryByText("Claude CLI default")).toBeNull();
  cleanup();
  render([session("s-2", "p", { name: "two", harness: "codex" })], "/agents/s-2", true, chatOf(chat()));
  const header2 = within((await screen.findByRole("heading", { name: /two/ })).closest("header")!);
  expect(header2.queryByText("Codex CLI default")).toBeNull();
});

test("the header subtitle says Chat before the project", async () => {
  render([session("s-1", "proj", { name: "one" })], "/agents/s-1", true, chatOf(chat()));
  await screen.findByRole("heading", { name: /one/ });
  expect(screen.getByTestId("page-header-subtitle").textContent).toBe("Chat · proj");
});

test("a Triage session links to its project's triage page and a plain one does not", async () => {
  render([session("s-1", "proj", { name: "one", flow: "triage", flow_key: "proj" })], "/agents/s-1", true, chatOf(chat()));
  const link = await screen.findByRole("link", { name: /Open triage page/ });
  expect(link.getAttribute("href")).toBe("#/triage/proj");
  expect(screen.queryByRole("link", { name: /Open design page/ })).toBeNull();
  cleanup();
  render([session("s-2", "proj", { name: "two" })], "/agents/s-2", true, chatOf(chat()));
  await screen.findByRole("heading", { name: /two/ });
  expect(screen.queryByRole("link", { name: /Open triage page/ })).toBeNull();
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

test("a round in the chat shows the question sheet with a Submit", async () => {
  const round = [{ label: "Pick", title: "Which?", kind: "options", options: [{ id: "a", label: "A" }] }] as unknown as SessionRoundCard[];
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat({ questions: roundCards(round) })));
  expect(await screen.findByText("hello there")).toBeTruthy();
  expect(await screen.findByRole("group", { name: "Questions" })).toBeTruthy();
  expect((screen.getByRole("button", { name: "Submit" }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.queryByRole("group", { name: "View" })).toBeNull();
});

test("the header has no model, no tier pill, and the menu's Agent block holds the tier", async () => {
  render([session("s-1", "p", { name: "one", tier: "strongest", model: "claude-opus-5", effort: "high" })], "/agents/s-1", true, chatOf(chat()));
  const header = within((await screen.findByRole("heading", { name: /one/ })).closest("header")!);
  expect(header.queryByText(/Claude Opus 5/)).toBeNull();
  expect(header.queryByText("Strongest")).toBeNull();
  expect(header.queryByText("switched by hand")).toBeNull();
  fireEvent.click(header.getByRole("button", { name: "More actions" }));
  expect(await screen.findByText("Agent")).toBeTruthy();
  expect(screen.getByText("Strongest")).toBeTruthy();
  expect(screen.queryByText("Effort")).toBeNull();
});

test("the header of a Codex session with a hand-switched model says so in the menu", async () => {
  render([session("s-1", "p", { name: "one", harness: "codex", tier: "worker", model: "gpt-5.5", effort: "xhigh", model_switched: true })], "/agents/s-1", true, chatOf(chat()));
  await screen.findByRole("heading", { name: /one/ });
  expect(screen.queryByText("Codex GPT-5.5")).toBeNull();
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

test("on a phone the chat header goes while the keyboard is open and returns when it closes", async () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  const vv = Object.assign(new EventTarget(), { height: 800, scale: 1 });
  Object.defineProperty(window, "visualViewport", { value: vv, configurable: true });
  Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
  const resize = (h: number) =>
    act(() => {
      vv.height = h;
      vv.dispatchEvent(new Event("resize"));
    });
  try {
    render([session("s-1", "p", { name: "one" })], "/agents/s-1", true, chatOf(chat()));
    const heading = await screen.findByRole("heading", { name: /one/ });
    act(() => screen.getByRole("textbox").focus());
    resize(350);
    await waitFor(() => expect(getComputedStyle(heading.closest("header")!).display).toBe("none"));
    act(() => (document.activeElement as HTMLElement).blur());
    resize(800);
    await waitFor(() => expect(getComputedStyle(heading.closest("header")!).display).not.toBe("none"));
  } finally {
    Object.defineProperty(window, "visualViewport", { value: undefined, configurable: true });
    vi.unstubAllGlobals();
  }
});
