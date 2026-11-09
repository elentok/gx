import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { Workspace } from "../generated/server";
import { renderThemed } from "../ui/render";
import { WriteError } from "../useWrite";
import { GearMenu } from "./GearMenu";
import { ShellContext } from "./ShellContext";

const showToast = vi.hoisted(() => vi.fn());
vi.mock("../ui/toaster", async (importOriginal) => ({ ...(await importOriginal<typeof import("../ui/toaster")>()), showToast }));

const install = vi.hoisted(() => ({ state: { mode: "none", desktopCaption: false } as { mode: string; desktopCaption: boolean }, promptInstall: vi.fn(async () => {}) }));
vi.mock("../pwa/install", () => ({ useInstall: () => ({ ...install.state, promptInstall: install.promptInstall }) }));

afterEach(() => {
  install.state = { mode: "none", desktopCaption: false };
  install.promptInstall.mockClear();
  cleanup();
  showToast.mockClear();
  window.location.hash = "";
});

const research: Workspace = { name: "research", sort: "activity" } as Workspace;

function setup(snapshot: Record<string, unknown> = {}, props: Parameters<typeof GearMenu>[0] = {}, closeSliding = () => {}) {
  const state = { snapshot, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const shell = { sidebarOpen: true, toggle: () => {}, close: () => {}, closeSliding, claimHeader: () => () => {} };
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <ShellContext.Provider value={shell}>
        <GearMenu {...props} />
      </ShellContext.Provider>
    </ConnectionContext.Provider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Menu" }));
}

test("lists AFK, Vi mode, Sort, Restart server and More settings in order, with no build or limits", async () => {
  setup({ build: "v1.2.3" }, { activeWorkspace: research });
  await screen.findByRole("menuitemcheckbox", { name: "AFK" });
  const items = [...screen.getByRole("menu").querySelectorAll('[role^="menuitem"]')].map((el) => el.textContent?.replace(/\s+/g, " ").trim());
  expect(items).toEqual(["AFK", "Vi mode", "Sort · research›", "Restart server", "More settings...", "Cleanup", "Tips"]);
  expect(screen.queryByTestId("server-build")).toBeNull();
  expect(screen.queryByText("Claude limits")).toBeNull();
});

test("one divider separates AFK, Vi mode and Sort from Restart server and More settings", async () => {
  setup({}, { activeWorkspace: research });
  await screen.findByRole("menuitemcheckbox", { name: "AFK" });
  const menu = screen.getByRole("menu");
  const separators = menu.querySelectorAll('[role="separator"]');
  expect(separators).toHaveLength(1);
  const sort = screen.getByRole("menuitem", { name: /Sort · research/ });
  const restart = screen.getByRole("menuitem", { name: "Restart server" });
  expect(sort.compareDocumentPosition(separators[0]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(restart.compareDocumentPosition(separators[0]) & Node.DOCUMENT_POSITION_PRECEDING).toBeTruthy();
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

test("Sort is disabled until the active workspace resolves", async () => {
  setup();
  const sort = await screen.findByRole("menuitem", { name: "Sort" });
  expect(sort.getAttribute("aria-disabled")).toBe("true");
});

test("the Sort submenu offers three radios with the stored value checked, and a pick posts it for the active workspace", async () => {
  const postSort = vi.fn(async () => {});
  setup({}, { activeWorkspace: research, postSort });
  fireEvent.click(await screen.findByRole("menuitem", { name: /Sort · research/ }));
  const radios = await screen.findAllByRole("menuitemradio");
  expect(radios.map((r) => [r.textContent, r.getAttribute("aria-checked")])).toEqual([
    ["Needs you first, then by name", "false"],
    ["By last activity, newest first", "true"],
    ["Alphabetically by name", "false"],
  ]);
  fireEvent.click(radios[2]);
  await waitFor(() => expect(postSort).toHaveBeenCalledWith("research", "name"));
});

test("a refused sort write shows the toast and the radio stays on the stored value", async () => {
  const postSort = vi.fn(async () => {
    throw new WriteError("no such workspace");
  });
  setup({}, { activeWorkspace: research, postSort });
  fireEvent.click(await screen.findByRole("menuitem", { name: /Sort · research/ }));
  fireEvent.click((await screen.findAllByRole("menuitemradio"))[0]);
  await waitFor(() => expect(showToast).toHaveBeenCalledWith({ type: "error", title: "no such workspace" }));
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  fireEvent.click(screen.getByRole("button", { name: "Menu" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: /Sort · research/ }));
  const checked = (await screen.findAllByRole("menuitemradio")).filter((r) => r.getAttribute("aria-checked") === "true");
  expect(checked.map((r) => r.textContent)).toEqual(["By last activity, newest first"]);
});

test("More settings... opens Settings and closes a sliding sidebar", async () => {
  const closeSliding = vi.fn();
  setup({}, {}, closeSliding);
  fireEvent.click(await screen.findByRole("menuitem", { name: "More settings..." }));
  expect(window.location.hash).toBe("#/settings");
  expect(closeSliding).toHaveBeenCalled();
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
});

test("Cleanup opens the Cleanup page and closes a sliding sidebar", async () => {
  const closeSliding = vi.fn();
  setup({}, {}, closeSliding);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Cleanup" }));
  expect(window.location.hash).toBe("#/cleanup");
  expect(closeSliding).toHaveBeenCalled();
});

test("Tips opens the Tips page and closes a sliding sidebar", async () => {
  const closeSliding = vi.fn();
  setup({}, {}, closeSliding);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Tips" }));
  expect(window.location.hash).toBe("#/tips");
  expect(closeSliding).toHaveBeenCalled();
});

test("Restart server opens the restart dialog", async () => {
  setup();
  fireEvent.click(await screen.findByRole("menuitem", { name: "Restart server" }));
  expect(await screen.findByRole("dialog")).toBeTruthy();
});

test("mode none shows no Install row", async () => {
  setup();
  await screen.findByRole("menuitem", { name: "Tips" });
  expect(screen.queryByRole("menuitem", { name: /Install app/ })).toBeNull();
});

test("prompt mode lists Install app last under its own separator, with the desktop caption, and a click prompts", async () => {
  install.state = { mode: "prompt", desktopCaption: true };
  setup();
  const row = await screen.findByRole("menuitem", { name: /Install app/ });
  expect(row.textContent).toContain("Chrome / Edge");
  const menu = screen.getByRole("menu");
  expect(menu.querySelectorAll('[role="separator"]')).toHaveLength(2);
  const items = [...menu.querySelectorAll('[role^="menuitem"]')];
  expect(items[items.length - 1]).toBe(row);
  fireEvent.click(row);
  expect(install.promptInstall).toHaveBeenCalledTimes(1);
});

test("prompt mode without the desktop caption shows a bare Install app row", async () => {
  install.state = { mode: "prompt", desktopCaption: false };
  setup();
  expect((await screen.findByRole("menuitem", { name: /Install app/ })).textContent?.trim()).toBe("Install app");
});

test("ios mode opens the Share steps sheet and Done closes it", async () => {
  install.state = { mode: "ios", desktopCaption: false };
  setup();
  fireEvent.click(await screen.findByRole("menuitem", { name: /Install app/ }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("Share");
  expect(dialog.textContent).toContain("Add to Home Screen");
  expect(install.promptInstall).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Done" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("mac-safari mode shows File then Add to Dock", async () => {
  install.state = { mode: "mac-safari", desktopCaption: false };
  setup();
  fireEvent.click(await screen.findByRole("menuitem", { name: /Install app/ }));
  const text = (await screen.findByRole("dialog")).textContent ?? "";
  expect(text.indexOf("File")).toBeGreaterThan(-1);
  expect(text.indexOf("Add to Dock")).toBeGreaterThan(text.indexOf("File"));
});

test("the Menu trigger is a burger: three horizontal lines and no circle", () => {
  setup();
  const svg = screen.getByRole("button", { name: "Menu", hidden: true }).querySelector("svg")!;
  expect(svg.querySelector("circle")).toBeNull();
  const paths = [...svg.querySelectorAll("path")].map((p) => p.getAttribute("d"));
  expect(paths).toHaveLength(3);
  for (const d of paths) expect(d).toMatch(/^M[\d.]+ [\d.]+h[\d.]+$/);
});
