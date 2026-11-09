import { act, fireEvent, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SettingsEntry, SettingsWriteResult } from "../generated/server";
import { renderThemed } from "../ui/render";
import { SettingsRow } from "./SettingsRow";

const entry = (over: Partial<SettingsEntry>): SettingsEntry => ({
  key: "execution-queue.max-agents",
  path: ["execution-queue", "max-agents"],
  type: "number",
  section: "Execution queue",
  description: "Live agents.",
  set: false,
  ...over,
});

const setup = (e: SettingsEntry, result: SettingsWriteResult = {}, pendingRestart = false) => {
  const onWrite = vi.fn(async () => result);
  const view = renderThemed(<SettingsRow entry={e} pendingRestart={pendingRestart} onWrite={onWrite} />);
  return { ...view, onWrite };
};

const flush = () => act(async () => {});

describe("SettingsRow", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("a bool switch sends a set change at once", async () => {
    const { getByRole, onWrite } = setup(entry({ type: "bool", default: false }));
    fireEvent.click(getByRole("switch"));
    await flush();
    expect(onWrite).toHaveBeenCalledWith([{ key: "execution-queue.max-agents", value: true }]);
  });

  it("a number saves the draft as a number; Save is disabled until it differs", async () => {
    const { getByRole, onWrite } = setup(entry({ set: true, value: 6, default: 4 }));
    const save = getByRole("button", { name: "Save" }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.change(getByRole("textbox"), { target: { value: "8" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await flush();
    expect(onWrite).toHaveBeenCalledWith([{ key: "execution-queue.max-agents", value: 8 }]);
  });

  it("Cancel restores the saved value", () => {
    const { getByRole } = setup(entry({ type: "string", set: true, value: "a" }));
    const input = getByRole("textbox") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "b" } });
    fireEvent.click(getByRole("button", { name: "Cancel" }));
    expect(input.value).toBe("a");
  });

  it("shows a unit hint beside duration and size fields", () => {
    expect(setup(entry({ type: "duration" })).getByText("e.g. 30s, 5m, 2h")).toBeTruthy();
    expect(setup(entry({ type: "size" })).getByText("e.g. 50G")).toBeTruthy();
  });

  it("an enum segment click sends that option", async () => {
    const { getByRole, onWrite } = setup(entry({ type: "enum", enum: ["a", "b"], default: "a" }));
    fireEvent.click(getByRole("button", { name: "b" }));
    await flush();
    expect(onWrite).toHaveBeenCalledWith([{ key: "execution-queue.max-agents", value: "b" }]);
  });

  it("shows the dot and Reset link only for a key in the file; Reset sends a remove", async () => {
    const unset = setup(entry({ default: 4 }));
    expect(unset.queryByTitle("Changed from default")).toBeNull();
    expect(unset.queryByText(/Reset to default/)).toBeNull();
    unset.unmount();
    const { getByText, getByTitle, onWrite } = setup(entry({ set: true, value: 6, default: 4 }));
    expect(getByTitle("Changed from default")).toBeTruthy();
    fireEvent.click(getByText("Reset to default (4)"));
    await flush();
    expect(onWrite).toHaveBeenCalledWith([{ key: "execution-queue.max-agents", remove: true }]);
  });

  it("shows Saved, then removes it after about two seconds", async () => {
    const { getByRole, queryByText } = setup(entry({ type: "bool" }));
    fireEvent.click(getByRole("switch"));
    await flush();
    expect(queryByText("Saved")).toBeTruthy();
    await act(async () => {
      vi.advanceTimersByTime(2100);
    });
    expect(queryByText("Saved")).toBeNull();
  });

  it("a refused save keeps the draft, marks the field and shows the reason", async () => {
    const { getByRole, getByText, queryByText } = setup(entry({ set: true, value: 6 }), {
      refused: true,
      keys: ["execution-queue.max-agents"],
      reason: "must be at least 1",
    });
    const input = getByRole("textbox") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "0" } });
    fireEvent.click(getByRole("button", { name: "Save" }));
    await flush();
    expect(input.value).toBe("0");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(getByText("Not saved: must be at least 1")).toBeTruthy();
    expect(queryByText("Saved")).toBeNull();
  });

  it("does not mark a field that a multi-key refusal does not name", async () => {
    const { getByRole, queryByText } = setup(entry({}), { refused: true, keys: ["other.key"], reason: "bad" });
    fireEvent.change(getByRole("textbox"), { target: { value: "3" } });
    fireEvent.click(getByRole("button", { name: "Save" }));
    await flush();
    expect(queryByText(/Not saved/)).toBeNull();
  });

  it("marks a startup-only key Needs restart, and Restart needed to apply once pending", () => {
    const idle = setup(entry({ restart: true }));
    expect(idle.getByText("Needs restart")).toBeTruthy();
    idle.unmount();
    expect(setup(entry({ restart: true }), {}, true).getByText("Restart needed to apply")).toBeTruthy();
  });

  it("says Restart needed to apply after a save the server reports as pending", async () => {
    const { getByRole, getByText } = setup(entry({ restart: true, type: "string" }), { restart_needed: ["execution-queue.max-agents"] });
    fireEvent.change(getByRole("textbox"), { target: { value: "x" } });
    fireEvent.click(getByRole("button", { name: "Save" }));
    await flush();
    expect(getByText("Restart needed to apply")).toBeTruthy();
  });

  describe("lists", () => {
    const listEntry = (over: Partial<SettingsEntry>) => entry({ key: "server.tab-env", type: "string-list", set: true, value: ["A=1", "B=2"], ...over });

    it("add and remove send the whole list", async () => {
      const { getByRole, onWrite } = setup(listEntry({}));
      fireEvent.change(getByRole("textbox"), { target: { value: "C=3" } });
      fireEvent.click(getByRole("button", { name: "Add" }));
      await flush();
      expect(onWrite).toHaveBeenLastCalledWith([{ key: "server.tab-env", value: ["A=1", "B=2", "C=3"] }]);
      fireEvent.click(getByRole("button", { name: "Remove A=1" }));
      await flush();
      expect(onWrite).toHaveBeenLastCalledWith([{ key: "server.tab-env", value: ["B=2"] }]);
    });

    it("a number list refuses abc before the write and accepts 90", async () => {
      const { getByRole, getByText, onWrite } = setup(listEntry({ key: "budget.notification-thresholds", type: "number-list", value: [50] }));
      fireEvent.change(getByRole("textbox"), { target: { value: "abc" } });
      fireEvent.click(getByRole("button", { name: "Add" }));
      await flush();
      expect(getByText('"abc" is not a number')).toBeTruthy();
      expect(onWrite).not.toHaveBeenCalled();
      fireEvent.change(getByRole("textbox"), { target: { value: "90" } });
      fireEvent.click(getByRole("button", { name: "Add" }));
      await flush();
      expect(onWrite).toHaveBeenCalledWith([{ key: "budget.notification-thresholds", value: [50, 90] }]);
    });

    it("an empty set list shows as empty and removing the last tag sends []", async () => {
      const { getByRole, onWrite } = setup(listEntry({ value: ["x"] }));
      fireEvent.click(getByRole("button", { name: "Remove x" }));
      await flush();
      expect(onWrite).toHaveBeenCalledWith([{ key: "server.tab-env", value: [] }]);
    });

    it("an empty set list is an empty tag list with its dot and a Reset", () => {
      const { getByText, getByTitle, getByRole } = setup(listEntry({ value: [], default: ["a"] }));
      expect(getByText("Empty list")).toBeTruthy();
      expect(getByTitle("Changed from default")).toBeTruthy();
      expect(getByRole("button", { name: /Reset to default/ })).toBeTruthy();
    });
  });

  describe("secrets", () => {
    const secret = (set: boolean) => entry({ key: "notifications.telegram.bot-token", type: "secret", set });

    it("shows Set or Not set", () => {
      expect(setup(secret(true)).getByText("Set")).toBeTruthy();
      expect(setup(secret(false)).getByText("Not set")).toBeTruthy();
    });

    it("Replace sends the new value and the token never reaches the DOM", async () => {
      const { getByRole, getByLabelText, onWrite, container } = setup(secret(true));
      fireEvent.click(getByRole("button", { name: "Replace" }));
      fireEvent.change(getByLabelText(/New/), { target: { value: "tok-123" } });
      fireEvent.click(getByRole("button", { name: "Save" }));
      await flush();
      expect(onWrite).toHaveBeenCalledWith([{ key: "notifications.telegram.bot-token", value: "tok-123" }]);
      expect(container.innerHTML).not.toContain("tok-123");
    });

    it("Clear sends a remove", async () => {
      const { getByRole, onWrite } = setup(secret(true));
      fireEvent.click(getByRole("button", { name: "Clear" }));
      await flush();
      expect(onWrite).toHaveBeenCalledWith([{ key: "notifications.telegram.bot-token", remove: true }]);
    });

    it("a chat id string row shows in full", () => {
      const { getByRole } = setup(entry({ key: "notifications.telegram.chat-id", type: "string", set: true, value: "-1001234567890" }));
      expect((getByRole("textbox") as HTMLInputElement).value).toBe("-1001234567890");
    });
  });

  describe("risky keys", () => {
    const risky = (over: Partial<SettingsEntry>) => entry({ risky: true, confirm_text: "This is risky.", ...over });
    const cases: [string, Partial<SettingsEntry>, (v: ReturnType<typeof setup>) => void, unknown][] = [
      ["server.web.listen", { type: "string", default: "a" }, (v) => fireEvent.change(v.getByRole("textbox"), { target: { value: "b" } }), "b"],
      ["server.web.dev-url", { type: "string", default: "a" }, (v) => fireEvent.change(v.getByRole("textbox"), { target: { value: "b" } }), "b"],
      ["ticket-store.path", { type: "string", default: "a" }, (v) => fireEvent.change(v.getByRole("textbox"), { target: { value: "b" } }), "b"],
      ["server.tcp-listen", { type: "bool", default: false }, (v) => fireEvent.click(v.getByRole("switch")), true],
      ["server.web.dev-frontend", { type: "bool", default: false }, (v) => fireEvent.click(v.getByRole("switch")), true],
      ["server.web.totp", { type: "bool", default: true, set: true, value: true }, (v) => fireEvent.click(v.getByRole("switch")), false],
      ["server.web.allowed-hosts", { type: "string-list", default: [] }, (v) => {
        fireEvent.change(v.getByRole("textbox"), { target: { value: "h" } });
        fireEvent.click(v.getByRole("button", { name: "Add" }));
      }, ["h"]],
    ];

    it.each(cases)("%s asks before writing; Cancel writes nothing, Change it writes", async (key, over, act1, value) => {
      const v = setup(risky({ key, ...over }));
      act1(v);
      if (over.type === "string") fireEvent.click(v.getByRole("button", { name: "Save" }));
      await flush();
      expect(v.getByRole("dialog")).toBeTruthy();
      expect(v.onWrite).not.toHaveBeenCalled();
      fireEvent.click(within(v.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
      await flush();
      expect(v.queryByRole("dialog")).toBeNull();
      expect(v.onWrite).not.toHaveBeenCalled();

      act1(v);
      if (over.type === "string") fireEvent.click(v.getByRole("button", { name: "Save" }));
      await flush();
      fireEvent.click(v.getByRole("button", { name: "Change it" }));
      await flush();
      expect(v.onWrite).toHaveBeenCalledWith([{ key, value }]);
      expect(v.queryByRole("dialog")).toBeNull();
    });

    it("turning the one-time code on needs no dialog", async () => {
      const v = setup(risky({ key: "server.web.totp", type: "bool", default: true, set: true, value: false }));
      fireEvent.click(v.getByRole("switch"));
      await flush();
      expect(v.queryByRole("dialog")).toBeNull();
      expect(v.onWrite).toHaveBeenCalledWith([{ key: "server.web.totp", value: true }]);
    });

    it("shows the entry's confirm text, including that old ticket data is not moved", async () => {
      const text = "Your existing tickets are not moved.";
      const v = setup(risky({ key: "ticket-store.path", type: "string", confirm_text: text }));
      fireEvent.change(v.getByRole("textbox"), { target: { value: "/new" } });
      fireEvent.click(v.getByRole("button", { name: "Save" }));
      await flush();
      expect(v.getByRole("dialog").textContent).toContain("not moved");
    });
  });
});
