import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { SessionInfo } from "../generated/server";
import { renderThemed } from "../ui/render";
import { toaster } from "../ui/toaster";
import { Composer } from "./Composer";
import type { PostPrompt } from "./prompt";

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

function render(over: { s?: SessionInfo; connected?: boolean; post?: PostPrompt; reason?: string } = {}) {
  return renderThemed(
    <Composer
      design={false}
      session={over.s ?? session()}
      connected={over.connected ?? true}
      post={over.post}
      reason={over.reason}
    />,
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
