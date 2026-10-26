import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection } from "../connection";
import { renderThemed } from "../ui/render";
import type { OpenDesign } from "./open";
import { NewDesignForm } from "./NewDesignForm";

afterEach(cleanup);

const KEY = "gx.newDesign.draft";
const saved = () => JSON.parse(localStorage.getItem(KEY) ?? "null");
const ok: OpenDesign = async () => ({ refused: false }) as never;

function mount(over: Partial<Parameters<typeof NewDesignForm>[0]> = {}) {
  const onClose = vi.fn();
  const onStarted = vi.fn();
  renderThemed(
    <ConnectionContext.Provider value={{ refresh: () => {} } as Connection}>
      <NewDesignForm projects={["gx", "blog"]} open={ok} onStarted={onStarted} onClose={onClose} {...over} />
    </ConnectionContext.Provider>,
  );
  return { onClose, onStarted };
}
const type = (name: string, value: string) => fireEvent.change(screen.getByRole(name === "Project" ? "combobox" : "textbox", { name }), { target: { value } });
const click = (name: string) => fireEvent.click(screen.getByRole("button", { name }));
const project = () => (screen.getByRole("combobox", { name: "Project" }) as HTMLSelectElement).value;

test("fields are named and inputs are 16px", () => {
  mount();
  const dialog = screen.getByRole("dialog", { name: "New Design" });
  expect(dialog.getAttribute("aria-modal")).toBe("true");
  for (const name of ["Epic slug", "Rough idea"]) expect(getComputedStyle(screen.getByRole("textbox", { name })).fontSize).toBe("16px");
  expect(getComputedStyle(screen.getByRole("combobox", { name: "Project" })).fontSize).toBe("16px");
  for (const name of ["Start design", "Discard", "Close"]) expect(screen.getByRole("button", { name })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
});

test("one project hides the select", () => {
  mount({ projects: ["gx"] });
  expect(screen.queryByRole("combobox", { name: "Project" })).toBeNull();
});

test("starting project: the page filter, then the first", () => {
  mount({ filterProject: "blog" });
  expect(project()).toBe("blog");
  cleanup();
  mount();
  expect(project()).toBe("gx");
});

test("starting project: a draft with text wins over the filter", () => {
  localStorage.setItem(KEY, JSON.stringify({ slug: "x", idea: "", project: "gx" }));
  mount({ filterProject: "blog" });
  expect(project()).toBe("gx");
});

test("starting project: an empty draft yields to the filter, and to the first when All projects", () => {
  localStorage.setItem(KEY, JSON.stringify({ slug: " ", idea: "", project: "blog" }));
  mount({ filterProject: "gx" });
  expect(project()).toBe("gx");
  cleanup();
  mount();
  expect(project()).toBe("blog");
});

test("starting project skips a project that is gone", () => {
  localStorage.setItem(KEY, JSON.stringify({ slug: "x", project: "old" }));
  mount({ filterProject: "blog" });
  expect(project()).toBe("blog");
});

test("every change saves the draft, and a new mount restores it", () => {
  mount();
  type("Epic slug", "fresh");
  type("Rough idea", "an idea");
  type("Project", "blog");
  expect(saved()).toEqual({ slug: "fresh", idea: "an idea", project: "blog" });
  cleanup();
  mount();
  expect((screen.getByRole("textbox", { name: "Epic slug" }) as HTMLInputElement).value).toBe("fresh");
  expect((screen.getByRole("textbox", { name: "Rough idea" }) as HTMLTextAreaElement).value).toBe("an idea");
  expect(project()).toBe("blog");
});

test("Close, Escape and a backdrop click close and keep the draft", () => {
  const { onClose } = mount();
  type("Epic slug", "fresh");
  click("Close");
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  fireEvent.click(screen.getByRole("dialog").parentElement!);
  expect(onClose).toHaveBeenCalledTimes(3);
  expect(saved().slug).toBe("fresh");
});

test("Discard with an empty draft closes at once", () => {
  const { onClose } = mount();
  click("Discard");
  expect(onClose).toHaveBeenCalled();
  expect(screen.queryByText("Discard this draft?")).toBeNull();
  expect(saved()).toBeNull();
});

test("Discard with text asks first; Keep returns, Discard clears", () => {
  const { onClose } = mount();
  type("Rough idea", "words");
  click("Discard");
  expect(screen.getByText("Discard this draft?")).toBeTruthy();
  expect(onClose).not.toHaveBeenCalled();
  click("Keep");
  expect(screen.queryByText("Discard this draft?")).toBeNull();
  expect(saved().idea).toBe("words");
  click("Discard");
  click("Discard");
  expect(onClose).toHaveBeenCalled();
  expect(saved()).toBeNull();
});

test("Start design posts, clears the draft and reports the address", async () => {
  const open = vi.fn(ok);
  const { onStarted } = mount({ open });
  type("Epic slug", " fresh ");
  type("Rough idea", "an idea");
  await act(async () => click("Start design"));
  expect(open).toHaveBeenCalledWith({ epic: "fresh", new_design: true, idea: "an idea", project: "gx" });
  expect(onStarted).toHaveBeenCalledWith("gx:fresh", undefined);
  expect(saved()).toBeNull();
});

test("a start_error clears the draft and hands the reason to the page", async () => {
  const { onStarted } = mount({ open: async () => ({ epic_created: true, start_error: "agent binary missing" }) as never });
  type("Epic slug", "fresh");
  type("Rough idea", "an idea");
  await act(async () => click("Start design"));
  expect(onStarted).toHaveBeenCalledWith("gx:fresh", "agent binary missing");
  expect(saved()).toBeNull();
});

test("a refusal shows in the dialog and keeps the draft", async () => {
  const { onStarted } = mount({ open: async () => ({ refused: true, message: "name taken" }) as never });
  type("Epic slug", "fresh");
  await act(async () => click("Start design"));
  expect(screen.getByText("name taken")).toBeTruthy();
  expect(onStarted).not.toHaveBeenCalled();
  expect(saved().slug).toBe("fresh");
});

test("Tab wraps inside the dialog", () => {
  mount();
  const start = screen.getByRole("button", { name: "Start design" });
  start.focus();
  fireEvent.keyDown(start, { key: "Tab" });
  expect(document.activeElement).toBe(screen.getByRole("combobox", { name: "Project" }));
  fireEvent.keyDown(document.activeElement!, { key: "Tab", shiftKey: true });
  expect(document.activeElement).toBe(start);
});

function stubPhone() {
  const listeners = new Set<() => void>();
  const vv = { height: 700, offsetTop: 0, addEventListener: (_: string, f: () => void) => listeners.add(f), removeEventListener: (_: string, f: () => void) => listeners.delete(f) };
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  vi.stubGlobal("visualViewport", vv);
  vi.stubGlobal("innerHeight", 800);
  return {
    resize: (height: number, offsetTop = 0) => act(() => ((vv.height = height), (vv.offsetTop = offsetTop), listeners.forEach((f) => f()))),
  };
}

test("a narrow viewport renders the bottom sheet with a grab handle, Close, and a 1:2 footer", () => {
  stubPhone();
  mount();
  expect(screen.getByTestId("grab-handle")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Close" })).toBeTruthy();
  expect(getComputedStyle(screen.getByRole("button", { name: "Discard" })).flexGrow).toBe("1");
  expect(getComputedStyle(screen.getByRole("button", { name: "Start design" })).flexGrow).toBe("2");
  vi.unstubAllGlobals();
});

test("the sheet is 88% high and follows the visual viewport while the keyboard is open", () => {
  const { resize } = stubPhone();
  mount();
  const dialog = screen.getByRole("dialog");
  expect(dialog.style.height || getComputedStyle(dialog).height).toBe("692px");
  resize(800);
  expect(getComputedStyle(dialog).height).toBe("704px");
  resize(400);
  expect(getComputedStyle(dialog).height).toBe("392px");
  vi.unstubAllGlobals();
});

test("the overlay covers the visual viewport, so the keyboard does not hide the sheet", () => {
  const { resize } = stubPhone();
  mount();
  const overlay = screen.getByRole("dialog").parentElement!;
  resize(400, 120);
  expect(getComputedStyle(overlay).top).toBe("120px");
  expect(getComputedStyle(overlay).height).toBe("400px");
  vi.unstubAllGlobals();
});

test("a wide viewport has no grab handle", () => {
  mount();
  expect(screen.queryByTestId("grab-handle")).toBeNull();
});
