import { act, cleanup } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { renderThemed } from "../ui/render";
import { usePageTitle } from "./title";

afterEach(cleanup);

function Probe() {
  usePageTitle({ kind: "Epics" });
  return null;
}

const session = (reason: string) => ({ actionable: { reason }, status: { state: "idle" } });

function setup(reasons: string[]) {
  let state = { snapshot: { sessions: reasons.map(session) }, connected: true } as unknown as ConnectionState;
  const subs = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => (subs.add(fn), () => subs.delete(fn)),
  } as unknown as Connection;
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <Probe />
    </ConnectionContext.Provider>,
  );
  return (next: string[]) =>
    act(() => {
      state = { ...state, snapshot: { sessions: next.map(session) } } as unknown as ConnectionState;
      subs.forEach((fn) => fn());
    });
}

test("the title carries the actionable count and follows live updates", () => {
  const update = setup(["a", "b"]);
  expect(document.title).toBe("(2) Epics · gx");

  update(["a"]);
  expect(document.title).toBe("(1) Epics · gx");

  update([]);
  expect(document.title).toBe("Epics · gx");
});
