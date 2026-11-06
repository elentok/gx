import { cleanup, fireEvent, screen } from "@testing-library/react";
import { describe, expect, test, vi } from "vitest";
import { accepted, chat, chatOf, composerBox, prompts, refused, render, session, stubPrompt, type, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

const promoteButton = () => screen.getByRole("button", { name: "Promote to Design" }) as HTMLButtonElement;
const findPromote = () => screen.findByRole("button", { name: "Promote to Design" }) as Promise<HTMLButtonElement>;

// wide=true is the wide layout, false the phone one.
describe.each([true, false])("wide=%s", (wide) => {
  const open = (s = session("s-1", "p"), c = chat()) => render([s], "/agents/s-1", true, chatOf(c), wide);

  test("a plain session with messages shows an enabled button", async () => {
    open();
    expect((await findPromote()).disabled).toBe(false);
  });

  test("a Design agent and a Queue agent show no button", async () => {
    open(session("s-1", "p", { design: true }));
    await composerBox();
    expect(screen.queryByRole("button", { name: "Promote to Design" })).toBeNull();
    cleanup();
    open(session("s-1", "p", { queue: true }));
    await composerBox();
    expect(screen.queryByRole("button", { name: "Promote to Design" })).toBeNull();
  });

  test("it is disabled with a reason while the chat has no message and while the composer cannot send", async () => {
    open(session("s-1", "p"), chat({ messages: [] }));
    expect((await findPromote()).disabled).toBe(true);
    expect(screen.getByText("No messages yet. Promote turns on after the first message.")).toBeTruthy();
    cleanup();
    open(session("s-1", "p", { status: { state: "working" } }));
    await screen.findByText("hello there");
    expect(promoteButton().disabled).toBe(true);
    expect(promoteButton().title).toBe("Working. Send turns on when the session is idle.");
  });

  test("it is disabled with the exited reason for an exited session", async () => {
    open(session("s-1", "p", { status: { state: "exited" } }));
    expect((await findPromote()).disabled).toBe(true);
    expect(promoteButton().title).toBe("Exited. Restore it in gx, Agents tab.");
  });

  test("a click posts promote with the last message and leaves the draft alone", async () => {
    prompts.length = 0;
    stubPrompt(accepted);
    open();
    await type("half written");
    fireEvent.click(await findPromote());
    await vi.waitFor(() => expect(prompts).toEqual([expect.objectContaining({ id: "s-1", promote: true, last_message: expect.any(String) })]));
    expect((await composerBox()).value).toBe("half written");
    expect(localStorage.getItem("gx.draft.composer.s-1")).toBe("half written");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  test("a refusal shows its reason beside the button", async () => {
    stubPrompt(refused("stale", "the conversation moved"));
    open();
    fireEvent.click(await findPromote());
    expect(await screen.findByText(/conversation moved|stale/i)).toBeTruthy();
  });
});
