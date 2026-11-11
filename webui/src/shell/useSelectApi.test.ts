import { act, renderHook } from "@testing-library/react";
import { expect, test } from "vitest";
import type { SelectRow } from "../select/selectModel";
import { useSelectApi } from "./SidebarSelect";

const row = (key: string): SelectRow => ({ key, kind: "epic", visible: true, selectable: true });
const names = new Map<string, string>();

function setup(initial: SelectRow[]) {
  return renderHook(({ rows }) => useSelectApi(rows, names, true), { initialProps: { rows: initial } });
}

test("a run that moves every selected row out of the list ends the mode once the list refreshes", () => {
  const { result, rerender } = setup([row("a"), row("b")]);
  act(() => result.current.enter());
  act(() => result.current.toggle("a"));
  act(() => result.current.toggle("b"));

  // The run finishes while the rows still list a and b; the refetch drops them afterwards.
  act(() => result.current.afterRun());
  expect(result.current.state.on).toBe(true);
  rerender({ rows: [] });

  expect(result.current.state).toEqual({ on: false, keys: [] });
});

test("a run's pending end does not carry into the next select session", () => {
  const { result, rerender } = setup([row("a")]);
  act(() => result.current.enter());
  act(() => result.current.toggle("a"));
  act(() => result.current.afterRun());
  act(() => result.current.leave());
  act(() => result.current.enter());
  rerender({ rows: [row("a")] });

  expect(result.current.state).toEqual({ on: true, keys: [] });
});

test("a run that leaves the selected rows in the list keeps the mode and the selection", () => {
  const { result, rerender } = setup([row("a")]);
  act(() => result.current.enter());
  act(() => result.current.toggle("a"));
  act(() => result.current.afterRun());
  rerender({ rows: [row("a")] });

  expect(result.current.state).toEqual({ on: true, keys: ["a"] });
});
