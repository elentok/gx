import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { renderThemed } from "../ui/render";
import { GearMenu } from "./GearMenu";

afterEach(cleanup);

function setup(snapshot: Record<string, unknown> = {}, props: Parameters<typeof GearMenu>[0] = {}) {
  const state = { snapshot, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <GearMenu {...props} />
    </ConnectionContext.Provider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Menu" }));
}

test("lists AFK, Vi mode, Settings, Restart server and the build in order, with no limits block", async () => {
  setup({ build: "v1.2.3" });
  const afk = await screen.findByRole("menuitemcheckbox", { name: "AFK" });
  const vi = screen.getByRole("menuitemcheckbox", { name: "Vi mode" });
  const settings = screen.getByRole("menuitem", { name: "Settings" });
  const restart = screen.getByRole("menuitem", { name: "Restart server" });
  const build = screen.getByTestId("server-build");
  expect(build.textContent).toBe("v1.2.3");
  expect(screen.queryByText("Claude limits")).toBeNull();
  const order = [afk, vi, settings, restart, build];
  order.slice(1).forEach((el, i) => expect(order[i].compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy());
});

test("the AFK and Vi mode items post the opposite of the snapshot", async () => {
  const postAfk = vi.fn(async () => {});
  const postViMode = vi.fn(async () => {});
  setup({ afk: true, vi_mode: false }, { postAfk, postViMode });
  const afk = await screen.findByRole("menuitemcheckbox", { name: "AFK" });
  expect(afk.getAttribute("aria-checked")).toBe("true");
  fireEvent.click(afk);
  await waitFor(() => expect(postAfk).toHaveBeenCalledWith(false));
  fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "Vi mode" }));
  await waitFor(() => expect(postViMode).toHaveBeenCalledWith(true));
});

test("choosing Settings closes the menu", async () => {
  setup();
  fireEvent.click(await screen.findByRole("menuitem", { name: "Settings" }));
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
});

test("Restart server opens the restart dialog", async () => {
  setup();
  fireEvent.click(await screen.findByRole("menuitem", { name: "Restart server" }));
  expect(await screen.findByRole("dialog")).toBeTruthy();
});
