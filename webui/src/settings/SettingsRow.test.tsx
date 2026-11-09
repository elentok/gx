import { act, fireEvent } from "@testing-library/react";
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
});
