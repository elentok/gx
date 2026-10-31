import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { PostPrompt } from "../design/prompt";
import type { SessionInfo } from "../generated/server";
import { renderThemed } from "../ui/render";
import { toaster } from "../ui/toaster";
import { WideContext } from "../useWide";
import { AgentComposer } from "./AgentComposer";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

const session = (state = "idle"): SessionInfo => ({
  id: "s-a",
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  status: { state } as SessionInfo["status"],
  status_changed_at: new Date().toISOString(),
  run_started_at: new Date().toISOString(),
  unread: {},
  actionable: {},
  auto_advance: true,
});

function render(over: { s?: SessionInfo; connected?: boolean; post?: PostPrompt; reason?: string; wide?: boolean } = {}) {
  return renderThemed(
    <WideContext.Provider value={over.wide ?? true}>
      <AgentComposer
        session={over.s ?? session()}
        connected={over.connected ?? true}
        post={over.post}
        reason={over.reason}
      />
    </WideContext.Provider>,
  );
}

const box = () => screen.getByRole("textbox", { name: "Message" });
const send = () => screen.getByRole("button", { name: "Send" });

test("the box is open with Send beside it and none of the design parts", () => {
  render({ reason: "Why it is off" });
  expect(box()).toBeTruthy();
  expect(send()).toBeTruthy();
  expect(screen.getByText("Why it is off")).toBeTruthy();
  for (const name of [/Auto/, /Continue/, /Interrupt/, /Note/]) {
    expect(screen.queryByRole("button", { name })).toBeNull();
  }
});

test("Send is off while empty and on once there is text", () => {
  render();
  expect((send() as HTMLButtonElement).disabled).toBe(true);
  fireEvent.change(box(), { target: { value: "hi" } });
  expect((send() as HTMLButtonElement).disabled).toBe(false);
});

test("Send is off when not connected, but the box stays editable", () => {
  render({ connected: false });
  fireEvent.change(box(), { target: { value: "hi" } });
  expect((box() as HTMLTextAreaElement).value).toBe("hi");
  expect((box() as HTMLTextAreaElement).disabled).toBe(false);
  expect((send() as HTMLButtonElement).disabled).toBe(true);
});

test("Send is off when the session is not promptable, but the box stays editable", () => {
  render({ s: session("working") });
  fireEvent.change(box(), { target: { value: "hi" } });
  expect((box() as HTMLTextAreaElement).value).toBe("hi");
  expect((send() as HTMLButtonElement).disabled).toBe(true);
});

test("a refused send shows its reason inline only, no toast, and keeps the draft", async () => {
  const toast = vi.spyOn(toaster, "create");
  const post = vi.fn(async () => ({ refused: true, message: "text may not start with / or !" })) as unknown as PostPrompt;
  render({ post });
  fireEvent.change(box(), { target: { value: "/x" } });
  fireEvent.click(send());
  expect(await screen.findByText("text may not start with / or !")).toBeTruthy();
  expect(toast).not.toHaveBeenCalled();
  expect((box() as HTMLTextAreaElement).value).toBe("/x");
  toast.mockRestore();
});

const HINT = "Codex skills start with $. Type $gx-... instead.";
const codex = (): SessionInfo => ({ ...session(), harness: "codex" });

test("the $ hint shows for a Codex /gx- draft, not for Claude or other drafts", () => {
  render({ s: codex() });
  fireEvent.change(box(), { target: { value: "/gx-design x" } });
  expect(screen.getByText(HINT)).toBeTruthy();
  expect(screen.queryByText("Draft saved in this browser")).toBeNull();
  fireEvent.change(box(), { target: { value: "/clear" } });
  expect(screen.queryByText(HINT)).toBeNull();
  fireEvent.change(box(), { target: { value: "$gx-design x" } });
  expect(screen.queryByText(HINT)).toBeNull();
  cleanup();
  render();
  fireEvent.change(box(), { target: { value: "/gx-design x" } });
  expect(screen.queryByText(HINT)).toBeNull();
});

test("the $ hint yields to a refusal line", async () => {
  const post = vi.fn(async () => ({ refused: true, message: "text may not start with / or !" })) as unknown as PostPrompt;
  render({ s: codex(), post });
  fireEvent.change(box(), { target: { value: "/gx-design x" } });
  fireEvent.click(send());
  expect(await screen.findByText("text may not start with / or !")).toBeTruthy();
  expect(screen.queryByText(HINT)).toBeNull();
});

test("Send is off while a send is pending, then clears the box", async () => {
  let release: (v: { refused: false }) => void = () => {};
  const post = vi.fn(() => new Promise<{ refused: false }>((r) => (release = r))) as unknown as PostPrompt;
  render({ post });
  fireEvent.change(box(), { target: { value: "hi" } });
  fireEvent.click(send());
  await waitFor(() => expect((send() as HTMLButtonElement).disabled).toBe(true));
  expect(post).toHaveBeenCalledWith(expect.objectContaining({ id: "s-a", text: "hi" }));
  release({ refused: false });
  await waitFor(() => expect((box() as HTMLTextAreaElement).value).toBe(""));
});

const ok = { refused: false } as Awaited<ReturnType<PostPrompt>>;
const cmdEnter = (el: HTMLElement, mod: "metaKey" | "ctrlKey" = "metaKey") =>
  fireEvent.keyDown(el, { key: "Enter", [mod]: true });

test("the placeholder names the session and no hairline tops the composer", () => {
  render();
  expect((box() as HTMLTextAreaElement).placeholder).toBe(`Message ${session().name || session().id}`);
  expect(getComputedStyle(screen.getByRole("form", { name: "Composer" })).borderTopWidth).not.toBe("1px");
});

test("Cmd or Ctrl+Enter sends like the Send button", async () => {
  for (const mod of ["metaKey", "ctrlKey"] as const) {
    const post = vi.fn<PostPrompt>(async () => ok);
    render({ post });
    fireEvent.change(box(), { target: { value: "hello" } });
    cmdEnter(box(), mod);
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    expect(post).toHaveBeenCalledWith({ id: "s-a", text: "hello" });
    await waitFor(() => expect((box() as HTMLTextAreaElement).value).toBe(""));
    cleanup();
    localStorage.clear();
  }
});

test("the key does nothing while Send is disabled", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post });
  cmdEnter(box());
  fireEvent.change(box(), { target: { value: "  " } });
  cmdEnter(box());
  cleanup();
  const offline = vi.fn<PostPrompt>(async () => ok);
  render({ post: offline, connected: false });
  fireEvent.change(box(), { target: { value: "hello" } });
  cmdEnter(box());
  await new Promise((r) => setTimeout(r, 20));
  expect(post).not.toHaveBeenCalled();
  expect(offline).not.toHaveBeenCalled();
});

test("plain Enter adds a newline and does not send", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post });
  fireEvent.change(box(), { target: { value: "hello" } });
  expect(fireEvent.keyDown(box(), { key: "Enter" })).toBe(true);
  await new Promise((r) => setTimeout(r, 20));
  expect(post).not.toHaveBeenCalled();
});

test("the narrow layout ignores the key", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post, wide: false });
  fireEvent.change(box(), { target: { value: "hello" } });
  cmdEnter(box());
  await new Promise((r) => setTimeout(r, 20));
  expect(post).not.toHaveBeenCalled();
});
