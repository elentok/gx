import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { ProjectWorktrees, SessionResult } from "../generated/server";
import { renderThemed } from "../ui/render";
import type { CreateSession, NewSessionBody } from "./newSession";
import { NewSessionForm } from "./NewSessionForm";

afterEach(cleanup);
beforeEach(() => localStorage.clear());

const KEY = "gx.newSession.draft";
const saved = () => JSON.parse(localStorage.getItem(KEY) ?? "null");
const trees: ProjectWorktrees[] = [
  { project: "gx", root: "/dev/gx", worktrees: ["/dev/gx-stats", "/dev/gx-stats-axis-labels"], tiers: { light: "haiku", worker: "sonnet", strongest: "opus" } },
  { project: "blog", root: "/dev/blog", worktrees: [], tiers: { light: "haiku", worker: "sonnet", strongest: "opus" } },
];
const accepted: SessionResult = { session: { id: "gx-7" } as never };

function mount(over: { create?: CreateSession; connected?: boolean; prefillProject?: string } = {}) {
  const posts: NewSessionBody[] = [];
  const create: CreateSession = over.create ?? (async (body) => (posts.push(body), accepted));
  const onCreated = vi.fn();
  const onClose = vi.fn();
  const state = { connected: over.connected ?? true } as ConnectionState;
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
  } as unknown as Connection;
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <NewSessionForm prefillProject={over.prefillProject} fetchTrees={async () => trees} create={create} onCreated={onCreated} onClose={onClose} />
    </ConnectionContext.Provider>,
  );
  return { posts, onCreated, onClose };
}

const ready = () => screen.findByRole("combobox", { name: "Worktree" }) as Promise<HTMLInputElement>;
const type = (name: string, value: string) => fireEvent.change(screen.getByRole(name === "Project" ? "combobox" : "textbox", { name }) as HTMLElement, { target: { value } });
const typeWorktree = (el: HTMLElement, value: string) => (fireEvent.focus(el), fireEvent.change(el, { target: { value } }));
const list = () => within(screen.getByRole("listbox", { name: "Worktrees" }));
const options = () => list().queryAllByRole("option").map((o) => o.textContent);
const submit = () => fireEvent.click(screen.getByRole("button", { name: "Create session" }));

test("fields and defaults: project prefilled, worktree is the project root", async () => {
  mount({ prefillProject: "blog" });
  const wt = await ready();
  expect((screen.getByRole("combobox", { name: "Project" }) as HTMLSelectElement).value).toBe("blog");
  expect(wt.value).toBe("Project root");
  for (const name of ["Name", "First prompt"]) expect(screen.getByRole("textbox", { name })).toBeTruthy();
  for (const name of ["Cancel", "Create session"]) expect(screen.getByRole("button", { name })).toBeTruthy();
});

test("the picker lists the root and worktrees, filters, and offers the create row", async () => {
  mount();
  const wt = await ready();
  fireEvent.focus(wt);
  expect(options()).toEqual(["Project root/dev/gx", "gx-stats/dev/gx-stats", "gx-stats-axis-labels/dev/gx-stats-axis-labels"]);
  typeWorktree(wt, "axis");
  expect(options()).toEqual(["+ Create worktree “axis”", "gx-stats-axis-labels/dev/gx-stats-axis-labels"]);
  typeWorktree(wt, "gx-stats");
  expect(options()[0]).toBe("gx-stats/dev/gx-stats");
  expect(options().some((o) => o?.startsWith("+ Create"))).toBe(false);
});

test("picking a dir posts dir", async () => {
  const { posts, onCreated } = mount();
  typeWorktree(await ready(), "gx-stats");
  fireEvent.click(list().getByRole("option", { name: /^gx-stats \/dev\/gx-stats$/ }));
  submit();
  await waitFor(() => expect(onCreated).toHaveBeenCalledWith("gx-7"));
  expect(posts).toEqual([{ project: "gx", dir: "/dev/gx-stats", name: undefined, prompt: undefined, tier: "worker" }]);
});

test("the default root posts its dir, with a typed name and prompt", async () => {
  const { posts, onCreated } = mount();
  await ready();
  type("Name", " mine ");
  type("First prompt", "do it");
  submit();
  await waitFor(() => expect(onCreated).toHaveBeenCalled());
  expect(posts).toEqual([{ project: "gx", dir: "/dev/gx", name: "mine", prompt: "do it", tier: "worker" }]);
});

test("the create row posts worktree", async () => {
  const { posts, onCreated } = mount();
  typeWorktree(await ready(), "fresh");
  fireEvent.click(list().getByRole("option", { name: /Create worktree/ }));
  submit();
  await waitFor(() => expect(onCreated).toHaveBeenCalled());
  expect(posts).toEqual([{ project: "gx", worktree: "fresh", name: undefined, prompt: undefined, tier: "worker" }]);
});

test("the tier picker lists three tiers with their models, defaults to worker and posts the pick", async () => {
  const { posts, onCreated } = mount();
  await ready();
  const picker = screen.getByRole("combobox", { name: "Tier" }) as HTMLSelectElement;
  expect([...picker.options].map((o) => o.textContent)).toEqual(["Light (haiku)", "Worker (sonnet)", "Strongest (opus)"]);
  expect(picker.value).toBe("worker");
  fireEvent.change(picker, { target: { value: "strongest" } });
  submit();
  await waitFor(() => expect(onCreated).toHaveBeenCalled());
  expect(posts[0].tier).toBe("strongest");
});

test("the Name placeholder follows the worktree", async () => {
  mount();
  const wt = await ready();
  expect(screen.getByRole("textbox", { name: "Name" }).getAttribute("placeholder")).toBe("gx");
  typeWorktree(wt, "fresh");
  fireEvent.click(list().getByRole("option", { name: /Create worktree/ }));
  expect(screen.getByRole("textbox", { name: "Name" }).getAttribute("placeholder")).toBe("fresh");
});

test("the draft survives a remount and clears on create and on discard", async () => {
  mount();
  await ready();
  type("Name", "keep");
  expect(saved().name).toBe("keep");
  cleanup();
  mount();
  await ready();
  expect((screen.getByRole("textbox", { name: "Name" }) as HTMLInputElement).value).toBe("keep");
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(saved()).toBeNull();
  cleanup();
  const { onCreated } = mount();
  await ready();
  type("Name", "again");
  submit();
  await waitFor(() => expect(onCreated).toHaveBeenCalled());
  expect(saved()).toBeNull();
});

test("a refusal shows in the form with input kept", async () => {
  const { onCreated } = mount({ create: async () => ({ refused: true, message: "bad worktree name" }) });
  typeWorktree(await ready(), "bad name");
  fireEvent.click(list().getByRole("option", { name: /Create worktree/ }));
  type("First prompt", "/x");
  submit();
  expect((await screen.findByRole("alert")).textContent).toBe("bad worktree name");
  expect((screen.getByRole("textbox", { name: "First prompt" }) as HTMLTextAreaElement).value).toBe("/x");
  expect((screen.getByRole("combobox", { name: "Worktree" }) as HTMLInputElement).value).toBe("bad name");
  expect(onCreated).not.toHaveBeenCalled();
  expect(saved().prompt).toBe("/x");
});

test("submit is disabled offline", async () => {
  mount({ connected: false });
  await ready();
  expect((screen.getByRole("button", { name: "Create session" }) as HTMLButtonElement).disabled).toBe(true);
});
