import { ChakraProvider } from "@chakra-ui/react";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { FixAgent } from "../epic/fixAgent";
import type { MigrationFailure } from "../generated/server";
import { DetailPage } from "../page/DetailPage";
import { renderThemed } from "../ui/render";
import { system } from "../ui/theme";
import { toaster } from "../ui/toaster";
import { FailedMigrationStrip } from "./FailedMigrationStrip";

afterEach(() => {
  cleanup();
  toaster.remove();
});

const fail = (project: string, error: string): MigrationFailure => ({ project, error, retry_command: "gx tickets migrate" });

function setup(failures: MigrationFailure[], ui: (fix?: FixAgent) => React.ReactNode, fixAgent?: FixAgent) {
  let state = { snapshot: { migration_failures: failures, seq: 1 }, connected: true, refresh: () => {} } as unknown as ConnectionState;
  const subs = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => (subs.add(fn), () => subs.delete(fn)),
    refresh: () => {},
  } as unknown as Connection;
  const { rerender } = renderThemed(<ConnectionContext.Provider value={conn}>{ui(fixAgent)}</ConnectionContext.Provider>);
  return {
    rerender: (node: React.ReactNode) =>
      rerender(
        <ChakraProvider value={system}>
          <ConnectionContext.Provider value={conn}>{node}</ConnectionContext.Provider>
        </ChakraProvider>,
      ),
    set: (next: MigrationFailure[]) =>
      act(() => {
        state = { ...state, snapshot: { ...state.snapshot!, migration_failures: next } } as ConnectionState;
        subs.forEach((fn) => fn());
      }),
  };
}

test("two failures render two rows with name, error and retry command", () => {
  setup([fail("gx", "bad schema"), fail("web", "disk full")], (f) => <FailedMigrationStrip fixAgent={f} />);
  expect(screen.getByText("gx: migration failed")).toBeTruthy();
  expect(screen.getByText("bad schema")).toBeTruthy();
  expect(screen.getByText("web: migration failed")).toBeTruthy();
  expect(screen.getByText("disk full")).toBeTruthy();
  expect(screen.getAllByText("gx tickets migrate")).toHaveLength(2);
  expect(screen.getAllByRole("button", { name: "Fix agent" })).toHaveLength(2);
});

test("Fix agent opens the dialog and Start posts the project", async () => {
  const calls: unknown[] = [];
  setup([fail("gx", "bad schema")], (f) => <FailedMigrationStrip fixAgent={f} />, async (body) => {
    calls.push(body);
    return { remote_control: false };
  });
  fireEvent.click(screen.getByRole("button", { name: "Fix agent" }));
  fireEvent.click(await screen.findByRole("button", { name: "Start gx agent" }));
  await waitFor(() => expect(calls).toEqual([{ project: "gx" }]));
});

test("a name-taken refusal shows the already-fixing toast", async () => {
  setup([fail("gx", "bad schema")], (f) => <FailedMigrationStrip fixAgent={f} />, async () => ({
    remote_control: false,
    refused: true,
    reason: "name-taken",
    message: "an agent is already fixing gx",
  }));
  fireEvent.click(screen.getByRole("button", { name: "Fix agent" }));
  fireEvent.click(await screen.findByRole("button", { name: "Start gx agent" }));
  expect(await screen.findByText("an agent is already fixing gx")).toBeTruthy();
});

test("a row disappears when the snapshot drops it", () => {
  const { set } = setup([fail("gx", "bad schema"), fail("web", "disk full")], () => <FailedMigrationStrip />);
  set([fail("web", "disk full")]);
  expect(screen.queryByText("gx: migration failed")).toBeNull();
  expect(screen.getByText("web: migration failed")).toBeTruthy();
  set([]);
  expect(screen.queryByText("web: migration failed")).toBeNull();
});

test("the page shell shows the strip on two different pages", () => {
  const page = (active: "epics" | "agents") => (
    <DetailPage active={active} title={{ kind: "Epics" }} mode="wide">
      <p>{active} body</p>
    </DetailPage>
  );
  const { rerender } = setup([fail("gx", "bad schema")], () => page("epics"));
  expect(screen.getByText("gx: migration failed")).toBeTruthy();
  rerender(page("agents"));
  expect(screen.getByText("epics body".replace("epics", "agents"))).toBeTruthy();
  expect(screen.getByText("gx: migration failed")).toBeTruthy();
});
