import { act, cleanup, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { renderThemed } from "../ui/render";
import { MigratingStrip } from "./MigratingStrip";

afterEach(cleanup);

function setup(migrating: boolean) {
  let state = { snapshot: { migrating, seq: 1 }, connected: true, refresh: () => {} } as unknown as ConnectionState;
  const subs = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => (subs.add(fn), () => subs.delete(fn)),
    refresh: () => {},
  } as unknown as Connection;
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <MigratingStrip />
      <p>page body</p>
    </ConnectionContext.Provider>,
  );
  return (next: boolean) =>
    act(() => {
      state = { ...state, snapshot: { ...state.snapshot!, migrating: next } } as ConnectionState;
      subs.forEach((fn) => fn());
    });
}

test("shows the line while migrating and does not block the page", () => {
  setup(true);
  expect(screen.getByText("Migrating tickets")).toBeTruthy();
  expect(screen.getByText("page body")).toBeTruthy();
});

test("absent when not migrating", () => {
  setup(false);
  expect(screen.queryByText("Migrating tickets")).toBeNull();
  expect(screen.getByText("page body")).toBeTruthy();
});

test("goes when migration ends", () => {
  const set = setup(true);
  set(false);
  expect(screen.queryByText("Migrating tickets")).toBeNull();
});
