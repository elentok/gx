import { act, cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "./connection";
import { NodeMissingToast } from "./NodeMissingToast";
import { renderThemed } from "./ui/render";
import { toaster } from "./ui/toaster";

afterEach(() => {
  cleanup();
  toaster.remove();
});

function setup(missing?: boolean) {
  let state = { snapshot: { node_missing: missing }, connected: true } as unknown as ConnectionState;
  const subs = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => (subs.add(fn), () => subs.delete(fn)),
  } as unknown as Connection;
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <NodeMissingToast />
    </ConnectionContext.Provider>,
  );
  return (next?: boolean) =>
    act(() => {
      state = { ...state, snapshot: { ...state.snapshot!, node_missing: next } } as ConnectionState;
      subs.forEach((fn) => fn());
    });
}

test("missing node shows a toast that stays", async () => {
  setup(true);
  expect(await screen.findByText("node is not on the gx server's PATH")).toBeTruthy();
  expect(toaster.getVisibleToasts()[0]?.duration).toBe(Infinity);
});

test("node coming back removes the toast", async () => {
  const setMissing = setup(true);
  await screen.findByText("node is not on the gx server's PATH");

  setMissing(undefined);
  await waitFor(() => expect(toaster.getVisibleToasts()).toHaveLength(0));
});

test("node on the PATH, no toast", () => {
  setup();
  expect(screen.queryByText("node is not on the gx server's PATH")).toBeNull();
});
