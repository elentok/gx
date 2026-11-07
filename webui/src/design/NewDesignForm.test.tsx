import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection } from "../connection";
import type { DesignDraft, DesignDraftSaveRequest } from "../generated/server";
import { renderThemed } from "../ui/render";
import { WriteError } from "../useWrite";
import type { DraftResult } from "./drafts";
import type { OpenDesign } from "./open";
import { NewDesignForm, type DraftsClient } from "./NewDesignForm";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const ok: OpenDesign = async () => ({ refused: false }) as never;

/** An in-memory draft store: `latest` is the one the form opens on, if any. */
function fakeDrafts(latest?: Partial<DesignDraft>) {
  const stored: DesignDraft | undefined = latest && { id: "d1", project: "gx", slug: "", idea: "", images: [], revision: 3, edited_at: "", ...latest };
  let revision = stored?.revision ?? 0;
  const save = vi.fn(async (body: DesignDraftSaveRequest): Promise<DraftResult> => {
    revision++;
    return { kind: "ok", draft: { images: [], edited_at: "", ...body, id: body.id ?? "created", revision } };
  });
  const remove = vi.fn(async (): Promise<DraftResult> => ({ kind: "deleted" }));
  const client: DraftsClient = {
    list: async () => (stored ? [{ ...stored, image_count: 0 }] : []),
    read: async () => (stored ? { kind: "ok", draft: stored } : { kind: "gone" }),
    save,
    remove,
  };
  return { client, save, remove };
}

async function mount(over: Partial<Parameters<typeof NewDesignForm>[0]> = {}, latest?: Partial<DesignDraft>) {
  const onClose = vi.fn();
  const onStarted = vi.fn();
  const fake = fakeDrafts(latest);
  await act(async () => {
    renderThemed(
      <ConnectionContext.Provider value={{ refresh: () => {} } as Connection}>
        <NewDesignForm projects={["gx", "blog"]} open={ok} drafts={fake.client} onStarted={onStarted} onClose={onClose} {...over} />
      </ConnectionContext.Provider>,
    );
  });
  return { onClose, onStarted, ...fake };
}
const type = (name: string, value: string) => fireEvent.change(screen.getByRole(name === "Project" ? "combobox" : "textbox", { name }), { target: { value } });
const click = (name: string) => fireEvent.click(screen.getByRole("button", { name }));
const project = () => (screen.getByRole("combobox", { name: "Project" }) as HTMLSelectElement).value;
const wait = (ms: number) => act(async () => void vi.advanceTimersByTime(ms));

test("fields are named and inputs are 16px", async () => {
  await mount();
  const dialog = screen.getByRole("dialog", { name: "New Design" });
  expect(dialog.getAttribute("aria-modal")).toBe("true");
  for (const name of ["Epic slug", "Rough idea"]) expect(getComputedStyle(screen.getByRole("textbox", { name })).fontSize).toBe("16px");
  expect(getComputedStyle(screen.getByRole("combobox", { name: "Project" })).fontSize).toBe("16px");
  for (const name of ["Start design", "Discard", "Close"]) expect(screen.getByRole("button", { name })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
});

test("one project hides the select", async () => {
  await mount({ projects: ["gx"] });
  expect(screen.queryByRole("combobox", { name: "Project" })).toBeNull();
});

test("the form shows a spinner, not its fields, while the drafts load", async () => {
  let release: () => void = () => {};
  const gate = new Promise<void>((r) => (release = r));
  const { client } = fakeDrafts();
  const onClose = vi.fn();
  await act(async () => {
    renderThemed(
      <ConnectionContext.Provider value={{ refresh: () => {} } as Connection}>
        <NewDesignForm projects={["gx"]} drafts={{ ...client, list: async () => (await gate, []) }} onStarted={() => {}} onClose={onClose} />
      </ConnectionContext.Provider>,
    );
  });
  expect(screen.queryByRole("textbox", { name: "Epic slug" })).toBeNull();
  expect(document.querySelector(".chakra-spinner")).toBeTruthy();
  await act(async () => release());
  expect(screen.getByRole("textbox", { name: "Epic slug" })).toBeTruthy();
});

test("starting project: the page filter, then the first", async () => {
  await mount({ filterProject: "blog" });
  expect(project()).toBe("blog");
  cleanup();
  await mount();
  expect(project()).toBe("gx");
});

test("starting project: a draft with text wins over the filter", async () => {
  await mount({ filterProject: "blog" }, { slug: "x", project: "gx" });
  expect(project()).toBe("gx");
});

test("starting project: an empty draft yields to the filter, and to the first when All projects", async () => {
  await mount({ filterProject: "gx" }, { slug: " ", project: "blog" });
  expect(project()).toBe("gx");
  cleanup();
  await mount({}, { slug: " ", project: "blog" });
  expect(project()).toBe("blog");
});

test("starting project skips a project that is gone", async () => {
  await mount({ filterProject: "blog" }, { slug: "x", project: "old" });
  expect(project()).toBe("blog");
});

test("the form opens on the latest draft's text and label", async () => {
  await mount({}, { slug: "fresh", idea: "an idea", project: "blog" });
  expect((screen.getByRole("textbox", { name: "Epic slug" }) as HTMLInputElement).value).toBe("fresh");
  expect((screen.getByRole("textbox", { name: "Rough idea" }) as HTMLTextAreaElement).value).toBe("an idea");
  expect(project()).toBe("blog");
  expect(screen.getByText("fresh", { selector: "p" })).toBeTruthy();
});

test("typing shows Saving..., saves 800 ms after the last keystroke with the revision, then shows Saved", async () => {
  vi.useFakeTimers();
  const { save } = await mount({}, { slug: "fresh", idea: "an idea" });
  type("Rough idea", "an idea!");
  expect(screen.getByText("Saving...")).toBeTruthy();
  await wait(700);
  type("Rough idea", "an idea!!");
  await wait(700);
  expect(save).not.toHaveBeenCalled();
  await wait(100);
  expect(save).toHaveBeenCalledTimes(1);
  expect(save).toHaveBeenLastCalledWith({ id: "d1", project: "gx", slug: "fresh", idea: "an idea!!", revision: 3, name: undefined });
  expect(screen.getByText("Saved")).toBeTruthy();
  expect(screen.queryByText("Saving...")).toBeNull();

  type("Rough idea", "more");
  await wait(800);
  expect(save).toHaveBeenLastCalledWith(expect.objectContaining({ idea: "more", revision: 4 }));
  expect(save.mock.calls[0]![0]).not.toHaveProperty("images");
});

test("a new draft is created by its first save and then updated by id", async () => {
  vi.useFakeTimers();
  const { save } = await mount();
  type("Epic slug", "fresh");
  await wait(800);
  expect(save).toHaveBeenLastCalledWith(expect.objectContaining({ id: undefined, slug: "fresh", revision: 0 }));
  type("Epic slug", "fresher");
  await wait(800);
  expect(save).toHaveBeenLastCalledWith(expect.objectContaining({ id: "created", slug: "fresher", revision: 1 }));
});

test("a blank form shows no status and saves nothing, even when typed in and cleared", async () => {
  vi.useFakeTimers();
  const { save, onClose } = await mount();
  expect(screen.queryByText("Saved")).toBeNull();
  expect(screen.queryByText("Saving...")).toBeNull();
  expect(screen.getByText("New draft")).toBeTruthy();
  type("Epic slug", "a");
  type("Epic slug", "");
  expect(screen.queryByText("Saving...")).toBeNull();
  await wait(800);
  click("Close");
  expect(onClose).toHaveBeenCalled();
  expect(save).not.toHaveBeenCalled();
});

test("the title is the name, else the slug, else the idea's first words", async () => {
  await mount({}, { name: "Named", slug: "slug", idea: "words" });
  expect(screen.getByText("Named", { selector: "p" })).toBeTruthy();
  cleanup();
  await mount({}, { slug: "slug", idea: "words" });
  type("Epic slug", "");
  expect(screen.getByText("words", { selector: "p" })).toBeTruthy();
});

test("Close, Escape and a backdrop click close and save at once", async () => {
  vi.useFakeTimers();
  const { onClose, save } = await mount();
  type("Epic slug", "fresh");
  click("Close");
  await wait(0);
  expect(save).toHaveBeenCalledTimes(1);
  type("Epic slug", "two");
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await wait(0);
  expect(save).toHaveBeenCalledTimes(2);
  type("Epic slug", "three");
  const backdrop = screen.getByRole("dialog").parentElement!;
  fireEvent.mouseDown(backdrop);
  fireEvent.click(backdrop);
  await wait(0);
  expect(save).toHaveBeenCalledTimes(3);
  expect(onClose).toHaveBeenCalledTimes(3);
});

test("leaving the page saves the pending text", async () => {
  vi.useFakeTimers();
  const { save } = await mount();
  type("Epic slug", "fresh");
  await act(async () => void window.dispatchEvent(new Event("pagehide")));
  expect(save).toHaveBeenCalledTimes(1);
  cleanup();
  expect(save).toHaveBeenCalledTimes(1);
});

test("unmounting with unsaved text saves it", async () => {
  vi.useFakeTimers();
  const { save } = await mount();
  type("Epic slug", "fresh");
  cleanup();
  await wait(0);
  expect(save).toHaveBeenCalledTimes(1);
});

test("a press in the sheet released on the backdrop keeps it open", async () => {
  const { onClose } = await mount();
  fireEvent.mouseDown(screen.getByRole("textbox", { name: "Rough idea" }));
  fireEvent.click(screen.getByRole("dialog").parentElement!);
  expect(onClose).not.toHaveBeenCalled();
});

test("Discard with an empty draft closes at once and deletes nothing", async () => {
  const { onClose, remove } = await mount();
  click("Discard");
  expect(onClose).toHaveBeenCalled();
  expect(screen.queryByText("Discard this draft and its images?")).toBeNull();
  expect(remove).not.toHaveBeenCalled();
});

test("Discard with text asks first; Keep returns, Discard deletes the saved draft", async () => {
  const { onClose, remove, save } = await mount({}, { slug: "fresh" });
  type("Rough idea", "words");
  click("Discard");
  expect(screen.getByText("Discard this draft and its images?")).toBeTruthy();
  expect(onClose).not.toHaveBeenCalled();
  click("Keep");
  expect(screen.queryByText("Discard this draft and its images?")).toBeNull();
  click("Discard");
  click("Discard");
  expect(onClose).toHaveBeenCalled();
  await act(async () => {});
  expect(remove).toHaveBeenCalledWith("d1");
  expect(save).not.toHaveBeenCalled();
});

test("Start design posts the draft id and reports the address, and stops saving", async () => {
  vi.useFakeTimers();
  const open = vi.fn(ok);
  const { onStarted, save } = await mount({ open }, { slug: "old", idea: "old idea" });
  type("Epic slug", " fresh ");
  type("Rough idea", "an idea");
  await act(async () => click("Start design"));
  expect(open).toHaveBeenCalledWith({ epic: "fresh", new_design: true, idea: "an idea", project: "gx", draft_id: "d1" });
  expect(onStarted).toHaveBeenCalledWith("gx:fresh", undefined);
  await wait(800);
  cleanup();
  expect(save).not.toHaveBeenCalled();
});

test("Start waits for a save in flight and sends the id it created; that save is not repeated", async () => {
  vi.useFakeTimers();
  const open = vi.fn(ok);
  const { client } = fakeDrafts();
  let release: () => void = () => {};
  const gate = new Promise<void>((r) => (release = r));
  const save = vi.fn(async (body: DesignDraftSaveRequest): Promise<DraftResult> => {
    await gate;
    return { kind: "ok", draft: { images: [], edited_at: "", ...body, id: "created", revision: 1 } };
  });
  await mount({ open, drafts: { ...client, save } });
  type("Epic slug", "fresh");
  type("Rough idea", "an idea");
  await wait(800);
  await act(async () => click("Start design"));
  expect(open).not.toHaveBeenCalled();
  await act(async () => release());
  expect(open).toHaveBeenCalledWith(expect.objectContaining({ draft_id: "created" }));
  await wait(2000);
  expect(save).toHaveBeenCalledTimes(1);
});

test("a refused Start keeps the text and the draft saves again", async () => {
  vi.useFakeTimers();
  const { save } = await mount({ open: async () => ({ refused: true, message: "epic exists" }) as never }, { slug: "old", idea: "old idea" });
  type("Epic slug", "fresh");
  await act(async () => click("Start design"));
  expect((screen.getByRole("textbox", { name: "Epic slug" }) as HTMLInputElement).value).toBe("fresh");
  await wait(0);
  expect(save).toHaveBeenCalledWith(expect.objectContaining({ id: "d1", slug: "fresh" }));
});

test("a start_error hands the reason to the page", async () => {
  const { onStarted } = await mount({ open: async () => ({ epic_created: true, start_error: "agent binary missing" }) as never });
  type("Epic slug", "fresh");
  type("Rough idea", "an idea");
  await act(async () => click("Start design"));
  expect(onStarted).toHaveBeenCalledWith("gx:fresh", "agent binary missing");
});

test("a refusal shows in the dialog and keeps saving the draft", async () => {
  const { onStarted } = await mount({ open: async () => ({ refused: true, message: "name taken" }) as never });
  type("Epic slug", "fresh");
  type("Rough idea", "an idea");
  await act(async () => click("Start design"));
  expect(await screen.findByText("name taken")).toBeTruthy();
  expect(onStarted).not.toHaveBeenCalled();
});

test("Start design is disabled while pending, and a server error shows its text in a toast", async () => {
  let fail: (e: Error) => void = () => {};
  const { onStarted } = await mount({ open: () => new Promise((_, reject) => (fail = reject)) as never });
  type("Epic slug", "fresh");
  type("Rough idea", "an idea");
  await act(async () => click("Start design"));
  expect((screen.getByRole("button", { name: "Start design" }) as HTMLButtonElement).disabled).toBe(true);
  await act(async () => fail(new WriteError("slug already exists")));
  expect(await screen.findByText("slug already exists")).toBeTruthy();
  expect(onStarted).not.toHaveBeenCalled();
});

test("Tab is trapped by the shared Modal, not by the form", async () => {
  await mount();
  type("Epic slug", "fresh");
  type("Rough idea", "an idea");
  const start = screen.getByRole("button", { name: "Start design" });
  start.focus();
  fireEvent.keyDown(start, { key: "Tab" });
  expect(document.activeElement).toBe(screen.getByRole("combobox", { name: "Project" }));
  fireEvent.keyDown(document.activeElement!, { key: "Tab", shiftKey: true });
  expect(document.activeElement).toBe(start);
});

test("Start is disabled with a reason for no slug and for no idea", async () => {
  await mount();
  const start = () => screen.getByRole("button", { name: "Start design" }) as HTMLButtonElement;
  expect(start().disabled).toBe(true);
  expect(screen.getByText("Name the epic to start.")).toBeTruthy();
  type("Rough idea", "an idea");
  expect(start().disabled).toBe(true);
  expect(screen.getByText("Name the epic to start.")).toBeTruthy();
  type("Epic slug", "fresh");
  expect(start().disabled).toBe(false);
  type("Rough idea", " ");
  expect(start().disabled).toBe(true);
  expect(screen.getByText("Write an idea to start.")).toBeTruthy();
});

test("the footer is Discard left, Start right, with no stretch", async () => {
  await mount();
  const discard = screen.getByRole("button", { name: "Discard" });
  const start = screen.getByRole("button", { name: "Start design" });
  expect(discard.compareDocumentPosition(start) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(getComputedStyle(start).flexGrow).not.toBe("2");
  expect(getComputedStyle(discard).flexGrow).not.toBe("1");
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

test("a narrow viewport renders the bottom sheet with a grab handle, Close, and a natural-width footer with the reason above it", async () => {
  stubPhone();
  await mount();
  expect(screen.getByTestId("grab-handle")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Close" })).toBeTruthy();
  const start = screen.getByRole("button", { name: "Start design" });
  const discard = screen.getByRole("button", { name: "Discard" });
  expect(getComputedStyle(discard).flexGrow).not.toBe("1");
  expect(getComputedStyle(start).flexGrow).not.toBe("2");
  const hint = screen.getByText("Name the epic to start.");
  expect(hint.compareDocumentPosition(discard) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  vi.unstubAllGlobals();
});

test("the sheet is 88% high and follows the visual viewport while the keyboard is open", async () => {
  const { resize } = stubPhone();
  await mount();
  const dialog = screen.getByRole("dialog");
  expect(dialog.style.height || getComputedStyle(dialog).height).toBe("692px");
  resize(800);
  expect(getComputedStyle(dialog).height).toBe("704px");
  resize(400);
  expect(getComputedStyle(dialog).height).toBe("392px");
  vi.unstubAllGlobals();
});

test("the overlay covers the visual viewport, so the keyboard does not hide the sheet", async () => {
  const { resize } = stubPhone();
  await mount();
  const overlay = screen.getByRole("dialog").parentElement!;
  resize(400, 120);
  expect(getComputedStyle(overlay).top).toBe("120px");
  expect(getComputedStyle(overlay).height).toBe("400px");
  vi.unstubAllGlobals();
});

test("a wide viewport has no grab handle", async () => {
  await mount();
  expect(screen.queryByTestId("grab-handle")).toBeNull();
});
