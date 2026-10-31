import { act, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RestartCheckReport, RestartResult } from "../generated/server";
import { ChakraProvider } from "@chakra-ui/react";
import { useState, type ReactElement } from "react";
import { renderThemed } from "../ui/render";
import { system } from "../ui/theme";
import { NewVersionStrip, useBuildChanged } from "./NewVersionStrip";
import { RestartDialog, RESTART_TIMEOUT_MS } from "./RestartDialog";

const report = (over: Partial<RestartCheckReport> = {}): RestartCheckReport => ({
  status: "ok",
  binary: "/bin/gx",
  running_version: "v1",
  new_version: "v2",
  config: { ok: true },
  projects: [],
  agents: 0,
  instance: "i1",
  restart_log: "/state/restart.log",
  ...over,
});

const started: RestartResult = { started: true, refused: false, restarting: true, restart_log: "/state/restart.log" };

afterEach(() => vi.useRealTimers());

// rerender keeps the root, so the theme provider has to be repeated.
const themed = (ui: ReactElement) => <ChakraProvider value={system}>{ui}</ChakraProvider>;

function open(r: RestartCheckReport | Promise<RestartCheckReport>, extra: Partial<Parameters<typeof RestartDialog>[0]> = {}) {
  const check = vi.fn(() => Promise.resolve(r));
  const restart = vi.fn(() => Promise.resolve(started));
  const ui = renderThemed(<RestartDialog check={check} restart={restart} reload={vi.fn()} onClose={vi.fn()} {...extra} />);
  return { ...ui, check, restart };
}

describe("RestartDialog", () => {
  it("disables Restart while checking", () => {
    const { getByRole, getByText } = open(new Promise(() => {}));
    expect(getByText(/Checking/)).toBeTruthy();
    expect((getByRole("button", { name: "Restart" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("shows versions, agents and the migration warning when ready", async () => {
    const { findByText, getByRole, getByText } = open(report({ agents: 2 }));
    await findByText("Next version: v2");
    expect(getByText("Running version: v1")).toBeTruthy();
    expect(getByText(/2 agents are running. Agents keep running/)).toBeTruthy();
    expect(getByText(/migration runs while the agents work/)).toBeTruthy();
    expect((getByRole("button", { name: "Restart" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("says when the versions match", async () => {
    const { findByText } = open(report({ new_version: "v1" }));
    await findByText("Versions match");
  });

  it("labels the button Restart anyway when a dry run failed", async () => {
    const { findByRole, getByText } = open(report({ projects: [{ name: "p", ok: false, error: "boom", files: [] }] }));
    expect((await findByRole("button", { name: "Restart anyway" }) as HTMLButtonElement).disabled).toBe(false);
    expect(getByText("p: boom")).toBeTruthy();
  });

  it.each([
    ["unreadable", { status: "unreadable", error: "no such file" }, "no such file"],
    ["too old", { status: "too_old", error: "unknown command", hint: "that binary has no restart check; use `gx server restart` instead" }, "gx server restart"],
    ["bad config", { config: { ok: false, error: "bad toml" } }, "bad toml"],
  ])("disables Restart and names the error for %s", async (_n, over, text) => {
    const { findByText, getByRole } = open(report(over));
    expect((await findByText(new RegExp(text))).textContent).toContain("gx server restart");
    expect((getByRole("button", { name: "Restart" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("re-sends the confirmed values and shows Restarting", async () => {
    const { findByRole, restart, findByText } = open(report({ projects: [{ name: "p", ok: false, files: [] }] }));
    fireEvent.click(await findByRole("button", { name: "Restart anyway" }));
    await findByText(/Restarting/);
    expect(restart).toHaveBeenCalledWith({ new_version: "v2", config_ok: true, failing_projects: ["p"] });
  });

  it("shows the new values on a refusal and waits for a new click", async () => {
    const restart = vi
      .fn<() => Promise<RestartResult>>()
      .mockResolvedValueOnce({ started: false, refused: true, reason: "changed", message: "the check changed", report: report({ new_version: "v3" }) })
      .mockResolvedValue(started);
    const { findByRole, findByText, getByText, queryByText } = open(report(), { restart });
    fireEvent.click(await findByRole("button", { name: "Restart" }));
    await findByText("Next version: v3");
    expect(getByText("the check changed")).toBeTruthy();
    expect(queryByText(/Restarting/)).toBeNull();
    fireEvent.click(getByText("Restart"));
    await findByText(/Restarting/);
    expect(restart).toHaveBeenLastCalledWith(expect.objectContaining({ new_version: "v3" }));
  });

  it("counts a network error after the click as restarting", async () => {
    const restart = vi.fn(() => Promise.reject(new Error("network")));
    const { findByRole, findByText } = open(report(), { restart });
    fireEvent.click(await findByRole("button", { name: "Restart" }));
    await findByText(/Restarting/);
  });

  it("says the server did not return after 60 seconds, naming the log and gx server start", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const { findByRole, findByText, queryByText } = open(report());
    fireEvent.click(await findByRole("button", { name: "Restart" }));
    await findByText(/Restarting/);
    expect(queryByText(/did not return/)).toBeNull();
    await act(async () => {
      vi.advanceTimersByTime(RESTART_TIMEOUT_MS);
    });
    const msg = await findByText(/did not return/);
    expect(msg.textContent).toContain("/state/restart.log");
    expect(msg.textContent).toContain("gx server start");
    expect(msg.textContent).toContain("still be migrating");
  });

  it("reloads once when the live instance differs after the click", async () => {
    const reload = vi.fn();
    let setInstance: (i: string) => void = () => {};
    function Live() {
      const [i, set] = useState("i1");
      setInstance = set;
      return <RestartDialog check={() => Promise.resolve(report())} restart={() => Promise.resolve(started)} reload={reload} liveInstance={i} onClose={vi.fn()} />;
    }
    const { findByRole, findByText } = renderThemed(<Live />);
    fireEvent.click(await findByRole("button", { name: "Restart" }));
    await findByText(/Restarting/);
    expect(reload).not.toHaveBeenCalled();
    act(() => setInstance("i2"));
    await waitFor(() => expect(reload).toHaveBeenCalledOnce());
  });

  it("does not reload on a different instance before the click", async () => {
    const reload = vi.fn();
    const { findByText } = open(report(), { reload, liveInstance: "other" });
    await findByText("Next version: v2");
    expect(reload).not.toHaveBeenCalled();
  });
});

describe("NewVersionStrip", () => {
  function Probe({ build, reload }: { build?: string; reload: () => void }) {
    return useBuildChanged(build) ? <NewVersionStrip reload={reload} /> : null;
  }

  it("shows only once the build differs from the first one seen, and the button reloads", () => {
    const reload = vi.fn();
    const { queryByText, getByRole, rerender } = renderThemed(<Probe build="v1" reload={reload} />);
    expect(queryByText("New version, reload")).toBeNull();
    rerender(themed(<Probe build="v1" reload={reload} />));
    expect(queryByText("New version, reload")).toBeNull();
    rerender(themed(<Probe build="v2" reload={reload} />));
    expect(queryByText("New version, reload")).toBeTruthy();
    fireEvent.click(getByRole("button", { name: "Reload" }));
    expect(reload).toHaveBeenCalledOnce();
  });
});
