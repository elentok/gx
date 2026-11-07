import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { DraftSwitcher } from "./DraftSwitcher";

afterEach(cleanup);

const item = (id: string, over = {}) => ({ id, project: "gx", slug: id, idea: "", image_count: 1, edited_at: new Date().toISOString(), ...over });

test("the menu loads on open, picks a draft and starts a new one", async () => {
  const load = vi.fn(async () => [item("a"), item("b", { image_count: 3 })]);
  const onPick = vi.fn();
  const onNew = vi.fn();
  renderThemed(<DraftSwitcher label="a" currentId="a" load={load} onPick={onPick} onNew={onNew} onRename={() => {}} onDelete={async () => {}} />);
  expect(load).not.toHaveBeenCalled();
  await act(async () => void fireEvent.click(screen.getByRole("button", { name: /a/ })));
  expect(load).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("menuitem", { name: /a.*editing now/ })).toBeTruthy();
  await act(async () => void fireEvent.click(screen.getByRole("menuitem", { name: /3 images/ })));
  expect(onPick).toHaveBeenCalledWith("b");
  expect(screen.queryByRole("menu")).toBeNull();
  await act(async () => void fireEvent.click(screen.getByRole("button", { name: /a/ })));
  await act(async () => void fireEvent.click(screen.getByRole("menuitem", { name: "New draft" })));
  expect(onNew).toHaveBeenCalled();
});

test("an open menu dims the form behind it with a scrim", async () => {
  renderThemed(<DraftSwitcher label="a" currentId="a" load={async () => [item("a")]} onPick={() => {}} onNew={() => {}} onRename={() => {}} onDelete={async () => {}} />);
  expect(screen.queryByTestId("switcher-scrim")).toBeNull();
  await act(async () => void fireEvent.click(screen.getByRole("button", { name: /a/ })));
  expect(screen.getByTestId("switcher-scrim")).toBeTruthy();
});

test("choosing the open draft changes nothing", async () => {
  const onPick = vi.fn();
  renderThemed(<DraftSwitcher label="a" currentId="a" load={async () => [item("a")]} onPick={onPick} onNew={() => {}} onRename={() => {}} onDelete={async () => {}} />);
  await act(async () => void fireEvent.click(screen.getByRole("button", { name: /a/ })));
  await act(async () => void fireEvent.click(screen.getByRole("menuitem", { name: /editing now/ })));
  expect(onPick).not.toHaveBeenCalled();
});
