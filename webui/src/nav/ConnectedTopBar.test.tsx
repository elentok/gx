import { cleanup, fireEvent, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { renderThemed } from "../ui/render";
import { ConnectedTopBar } from "./ConnectedTopBar";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("mounts the usage block in the open menu", async () => {
  const summary = { mode: "spending", limits: { windows: [] }, spend: { today: 0, since_sunday: 0, since_first: 0 } };
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(summary), { status: 200 })));
  const state = { connected: true, loadedOnce: true, tick: 0, snapshot: { tickets: [], epics: [] } } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  renderThemed(
    <MemoryRouter>
      <ConnectionContext.Provider value={conn}>
        <ConnectedTopBar />
      </ConnectionContext.Provider>
    </MemoryRouter>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Menu" }));
  expect(await screen.findByText("gx spend (est.)")).toBeTruthy();
});
