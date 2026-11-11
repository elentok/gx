import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { DevState } from "../generated/server";
import { renderThemed } from "../ui/render";
import { toaster } from "../ui/toaster";
import { useDevToast } from "./useDevToast";

afterEach(() => {
  cleanup();
  toaster.remove();
  created.mockClear();
});

const TITLE = "Vite is down. gx serves the built files.";

function setup(first: DevState) {
  const postDevRestart = vi.fn();
  let state = { snapshot: { dev: { state: first } }, connected: true } as unknown as ConnectionState;
  const subs = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => (subs.add(fn), () => subs.delete(fn)),
  } as unknown as Connection;
  function Probe() {
    useDevToast({ postDevRestart });
    return null;
  }
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <Probe />
    </ConnectionContext.Provider>,
  );
  const set = (next: DevState) =>
    act(() => {
      state = { ...state, snapshot: { dev: { state: next } } } as unknown as ConnectionState;
      subs.forEach((fn) => fn());
    });
  return { set, postDevRestart };
}

const created = vi.spyOn(toaster, "create");
const shown = () => created.mock.calls.filter(([t]) => t.title === TITLE).length;

test("running to down fires the toast once", async () => {
  const { set } = setup("running");
  set("down");
  expect(await screen.findByText(TITLE)).toBeTruthy();
  expect(shown()).toBe(1);
});

test("a first snapshot already down fires none", () => {
  setup("down");
  expect(shown()).toBe(0);
});

test("down to down fires none", () => {
  const { set } = setup("running");
  set("down");
  set("down");
  expect(shown()).toBe(1);
});

test("down then running then down fires twice", () => {
  const { set } = setup("running");
  set("down");
  toaster.remove();
  set("running");
  set("down");
  expect(shown()).toBe(2);
});

test("Restart Vite calls its handler", async () => {
  const { set, postDevRestart } = setup("running");
  set("down");
  fireEvent.click(await screen.findByRole("button", { name: "Restart Vite" }));
  expect(postDevRestart).toHaveBeenCalledTimes(1);
});
