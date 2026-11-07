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
  localStorage.clear();
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
  expect(screen.getByText("fresh", { selector: "span" })).toBeTruthy();
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
  expect(screen.getByText("Named", { selector: "span" })).toBeTruthy();
  cleanup();
  await mount({}, { slug: "slug", idea: "words" });
  type("Epic slug", "");
  expect(screen.getByText("words", { selector: "span" })).toBeTruthy();
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

const CONFLICT = "This draft changed on another device.";
const GONE = "This draft was deleted or started on another device.";

/** A form whose first save is refused with `kind`; `theirs` is what the server holds when read. */
async function refused(kind: "conflict" | "gone", theirs: Partial<DesignDraft> = {}) {
  vi.useFakeTimers();
  const base = fakeDrafts({ slug: "mine", idea: "my idea", name: "Named", project: "blog", images: ["a.png"] });
  const refuse = vi.fn(async (): Promise<DraftResult> => ({ kind }));
  const save = vi.fn(base.save).mockImplementationOnce(refuse);
  const server: DesignDraft = { id: "d1", project: "gx", slug: "theirs", idea: "their idea", images: ["b.png", "c.png"], revision: 9, edited_at: "", ...theirs };
  // The first read opens the form on the local draft; later reads see the server's copy.
  const read = vi.fn(async (_id: string): Promise<DraftResult> => ({ kind: "ok", draft: server })).mockImplementationOnce(base.client.read);
  const view = await mount({ drafts: { ...base.client, save, read } }, { slug: "mine", idea: "my idea", name: "Named", project: "blog", images: ["a.png"] });
  type("Rough idea", "latest");
  await wait(800);
  return { ...view, save, read };
}

test("a conflict raises the banner; typing sends nothing and Start is off", async () => {
  const { save } = await refused("conflict");
  expect(screen.getByText(CONFLICT)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Load theirs" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Keep mine" })).toBeTruthy();
  expect(save).toHaveBeenCalledTimes(1);
  type("Rough idea", "still typing");
  await wait(2000);
  expect(save).toHaveBeenCalledTimes(1);
  expect((screen.getByRole("button", { name: "Start design" }) as HTMLButtonElement).disabled).toBe(true);
});

test("Keep mine saves the latest local text over the server's revision and resumes", async () => {
  const { save } = await refused("conflict");
  type("Rough idea", "newest");
  await act(async () => click("Keep mine"));
  await wait(0);
  expect(save).toHaveBeenCalledTimes(2);
  expect(save).toHaveBeenLastCalledWith(expect.objectContaining({ id: "d1", idea: "newest", revision: 9 }));
  expect(screen.queryByText(CONFLICT)).toBeNull();
  expect((screen.getByRole("button", { name: "Start design" }) as HTMLButtonElement).disabled).toBe(false);
  type("Rough idea", "after");
  await wait(800);
  expect(save).toHaveBeenCalledTimes(3);
});

test("Load theirs replaces the text and image list and drops local edits", async () => {
  const { save } = await refused("conflict");
  await act(async () => click("Load theirs"));
  expect((screen.getByRole("textbox", { name: "Epic slug" }) as HTMLInputElement).value).toBe("theirs");
  expect((screen.getByRole("textbox", { name: "Rough idea" }) as HTMLTextAreaElement).value).toBe("their idea");
  expect(project()).toBe("gx");
  expect(screen.queryByText(CONFLICT)).toBeNull();
  await wait(2000);
  expect(save).toHaveBeenCalledTimes(1);
  // Their image list is what Discard now warns about.
  click("Discard");
  expect(screen.getByText("Discard this draft and its images?")).toBeTruthy();
});

test("a gone refusal raises the gone banner; Keep as new draft saves a new draft with no images", async () => {
  const { save } = await refused("gone");
  expect(screen.getByText(GONE)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Keep mine" })).toBeNull();
  type("Rough idea", "typed while gone");
  await wait(2000);
  expect(save).toHaveBeenCalledTimes(1);
  await act(async () => click("Keep as new draft"));
  await wait(0);
  expect(save).toHaveBeenCalledTimes(2);
  expect(save).toHaveBeenLastCalledWith({ id: undefined, slug: "mine", idea: "typed while gone", project: "blog", name: "Named", revision: 0 });
  expect(screen.queryByText(GONE)).toBeNull();
});

test("Keep mine against a draft that is gone raises the gone banner", async () => {
  const { read } = await refused("conflict");
  read.mockResolvedValueOnce({ kind: "gone" });
  await act(async () => click("Keep mine"));
  expect(screen.getByText(GONE)).toBeTruthy();
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
  expect(document.activeElement).toBe(screen.getByRole("button", { name: /fresh/, expanded: false }));
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

const UNSAVED_KEY = "gx.newDesign.unsaved";
const unsavedMap = () => JSON.parse(localStorage.getItem(UNSAVED_KEY) ?? "{}");

/** A form on draft d1 whose saves throw while `down.on` is set. */
async function serverDown() {
  vi.useFakeTimers();
  localStorage.clear();
  const base = fakeDrafts({ slug: "mine", idea: "my idea", project: "blog" });
  const down = { on: true };
  const save = vi.fn(async (body: DesignDraftSaveRequest) => {
    if (down.on) throw new WriteError("offline");
    return base.save(body);
  });
  const view = await mount({ drafts: { ...base.client, save } }, { slug: "mine", idea: "my idea", project: "blog" });
  return { ...view, save, remove: base.remove, down };
}

test("a failing save shows Not saved, and a keystroke and a 5 s tick each retry", async () => {
  const { save, down } = await serverDown();
  type("Rough idea", "one");
  await wait(800);
  expect(screen.getByText("Not saved")).toBeTruthy();
  expect(save).toHaveBeenCalledTimes(1);
  type("Rough idea", "two");
  await wait(800);
  expect(save).toHaveBeenCalledTimes(2);
  await wait(5000);
  expect(save).toHaveBeenCalledTimes(3);
  down.on = false;
  await wait(5000);
  expect(save).toHaveBeenCalledTimes(4);
  expect(screen.getByText("Saved")).toBeTruthy();
  expect(localStorage.getItem(UNSAVED_KEY)).toBeNull();
});

test("closing while unsaved keeps one entry for the draft", async () => {
  const { onClose } = await serverDown();
  type("Rough idea", "typed offline");
  await wait(800);
  click("Close");
  expect(onClose).toHaveBeenCalled();
  expect(unsavedMap()).toEqual({ d1: { revision: 3, project: "blog", slug: "mine", idea: "typed offline" } });
});

test("the next open saves a fallback entry with its revision and clears it", async () => {
  vi.useFakeTimers();
  localStorage.setItem(UNSAVED_KEY, JSON.stringify({ d1: { revision: 3, project: "blog", slug: "mine", idea: "kept", name: "N" } }));
  const { save } = await mount({}, { slug: "mine", idea: "old" });
  expect(save).toHaveBeenCalledWith({ id: "d1", project: "blog", slug: "mine", idea: "kept", name: "N", revision: 3 });
  expect(localStorage.getItem(UNSAVED_KEY)).toBeNull();
});

test.each([
  ["conflict", CONFLICT],
  ["gone", GONE],
] as const)("a fallback entry refused as %s raises its banner and keeps the text", async (kind, text) => {
  vi.useFakeTimers();
  localStorage.setItem(UNSAVED_KEY, JSON.stringify({ d1: { revision: 3, project: "blog", slug: "mine", idea: "kept" } }));
  const base = fakeDrafts({ slug: "mine", idea: "old" });
  await mount({ drafts: { ...base.client, save: vi.fn(async (): Promise<DraftResult> => ({ kind })) } });
  expect(screen.getByText(text)).toBeTruthy();
  expect((screen.getByRole("textbox", { name: "Rough idea" }) as HTMLTextAreaElement).value).toBe("kept");
  expect(Object.keys(unsavedMap())).toEqual(["d1"]);
});

test("a fallback entry survives a second failed open and another draft's entry stays beside it", async () => {
  vi.useFakeTimers();
  localStorage.setItem(
    UNSAVED_KEY,
    JSON.stringify({
      d1: { revision: 3, project: "blog", slug: "a", idea: "A text" },
      d2: { revision: 1, project: "gx", slug: "b", idea: "B text" },
    }),
  );
  const base = fakeDrafts({ slug: "a", idea: "old" });
  await mount({ drafts: { ...base.client, save: vi.fn(async () => Promise.reject(new WriteError("offline"))) } });
  expect(screen.getByText("Not saved")).toBeTruthy();
  expect(Object.keys(unsavedMap()).sort()).toEqual(["d1", "d2"]);
});

test("Discard removes the entry", async () => {
  const { remove } = await serverDown();
  type("Rough idea", "typed offline");
  await wait(800);
  expect(localStorage.getItem(UNSAVED_KEY)).not.toBeNull();
  click("Discard");
  click("Discard");
  await act(async () => {});
  expect(remove).toHaveBeenCalledWith("d1");
  expect(localStorage.getItem(UNSAVED_KEY)).toBeNull();
});

test("Start while Not saved leaves no entry, so no draft comes back", async () => {
  const { onStarted } = await serverDown();
  type("Rough idea", "typed offline");
  await wait(800);
  expect(localStorage.getItem(UNSAVED_KEY)).not.toBeNull();
  await act(async () => click("Start design"));
  expect(onStarted).toHaveBeenCalled();
  cleanup();
  expect(localStorage.getItem(UNSAVED_KEY)).toBeNull();
});

const LEGACY_KEY = "gx.newDesign.draft";

test("a stored legacy draft with text becomes a server draft with its project and the key is gone", async () => {
  localStorage.setItem(LEGACY_KEY, JSON.stringify({ slug: "old", idea: "old idea", project: "blog" }));
  const { save } = await mount();
  expect(save).toHaveBeenCalledWith({ project: "blog", slug: "old", idea: "old idea", revision: 0 });
  expect(localStorage.getItem(LEGACY_KEY)).toBeNull();
});

test("a refused legacy move keeps the key and the next open retries it", async () => {
  localStorage.setItem(LEGACY_KEY, JSON.stringify({ slug: "old", idea: "", project: "blog" }));
  const base = fakeDrafts();
  const refuse = vi.fn(async (): Promise<DraftResult> => ({ kind: "conflict" }));
  await mount({ drafts: { ...base.client, save: refuse } });
  expect(localStorage.getItem(LEGACY_KEY)).not.toBeNull();
  cleanup();
  await mount({ drafts: base.client });
  expect(base.save).toHaveBeenCalledTimes(1);
  expect(localStorage.getItem(LEGACY_KEY)).toBeNull();
});

test("a legacy key with no text is deleted and creates nothing", async () => {
  localStorage.setItem(LEGACY_KEY, JSON.stringify({ slug: " ", idea: "", project: "blog" }));
  const { save } = await mount();
  expect(save).not.toHaveBeenCalled();
  expect(localStorage.getItem(LEGACY_KEY)).toBeNull();
});

test("a second open after a successful move creates nothing", async () => {
  localStorage.setItem(LEGACY_KEY, JSON.stringify({ slug: "old", idea: "x", project: "gx" }));
  const base = fakeDrafts();
  await mount({ drafts: base.client });
  cleanup();
  await mount({ drafts: base.client });
  expect(base.save).toHaveBeenCalledTimes(1);
});

/** Two stored drafts: "alpha" (gx, 2 images, opened first) and "beta" (blog, 3 h old). */
function twoDrafts(aText: Partial<DesignDraft> = {}) {
  const a: DesignDraft = { id: "a", project: "gx", slug: "alpha", idea: "first idea", images: [], revision: 1, edited_at: new Date().toISOString(), ...aText };
  const b: DesignDraft = { id: "b", project: "blog", slug: "beta", idea: "second idea", images: [], revision: 1, edited_at: new Date(Date.now() - 3 * 3_600_000).toISOString() };
  const fake = fakeDrafts();
  const client: DraftsClient = {
    ...fake.client,
    list: async () => [
      { ...a, image_count: 2 },
      { ...b, image_count: 0 },
    ],
    read: async (id) => ({ kind: "ok", draft: id === "a" ? a : b }),
  };
  return { ...fake, client };
}
const openMenu = (name: RegExp) => act(async () => void fireEvent.click(screen.getByRole("button", { name })));
const pick = (name: string | RegExp) => act(async () => void fireEvent.click(screen.getByRole("menuitem", { name })));

test("the draft menu lists label, project, images and time, and marks the open draft", async () => {
  await mount({ drafts: twoDrafts().client });
  await openMenu(/alpha/);
  const rows = screen.getAllByRole("menuitem").map((r) => r.textContent);
  expect(rows[0]).toContain("alpha");
  expect(rows[0]).toContain("gx · 2 images · editing now");
  expect(rows[1]).toContain("beta");
  expect(rows[1]).toContain("blog · edited 3 h ago");
  expect(rows[2]).toContain("New draft");
});

test("choosing another draft saves the current one first, then shows the other's text and project", async () => {
  const { client, save } = twoDrafts();
  await mount({ drafts: client });
  type("Rough idea", "changed");
  await openMenu(/alpha/);
  await pick(/beta/);
  expect(save).toHaveBeenCalledWith(expect.objectContaining({ id: "a", idea: "changed" }));
  expect((screen.getByRole("textbox", { name: "Epic slug" }) as HTMLInputElement).value).toBe("beta");
  expect((screen.getByRole("textbox", { name: "Rough idea" }) as HTMLTextAreaElement).value).toBe("second idea");
  expect(project()).toBe("blog");
});

test("New draft shows a blank form with the slug field focused", async () => {
  await mount({ drafts: twoDrafts().client });
  await openMenu(/alpha/);
  await pick("New draft");
  expect((screen.getByRole("textbox", { name: "Epic slug" }) as HTMLInputElement).value).toBe("");
  expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Epic slug" }));
});

test("New draft on a blank draft refocuses the slug and creates nothing", async () => {
  const { save, remove } = await mount();
  await openMenu(/New draft/);
  screen.getByRole("textbox", { name: "Rough idea" }).focus();
  await pick("New draft");
  expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Epic slug" }));
  expect(save).not.toHaveBeenCalled();
  expect(remove).not.toHaveBeenCalled();
});

test("switching away from a blank saved draft deletes it and saves nothing", async () => {
  const { client, save, remove } = twoDrafts({ slug: "", idea: "" });
  await mount({ drafts: client });
  await openMenu(/New draft/);
  await pick(/beta/);
  expect(remove).toHaveBeenCalled();
  expect(save).not.toHaveBeenCalled();
});
