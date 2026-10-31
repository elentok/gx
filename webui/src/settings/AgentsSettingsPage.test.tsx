import { fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { renderThemed } from "../ui/render";
import { runnerNote, type DefaultHarnessStatus, type PostAgentSettings, type SetDefaultHarnessResult } from "./agents";
import { AgentsSettingsPage } from "./AgentsSettingsPage";

const status = (over: Partial<DefaultHarnessStatus> = {}): DefaultHarnessStatus => ({
  default: "claude",
  runner: "auto",
  projects: [
    { project: "gx", harness: "codex" },
    { project: "blog", harness: "" },
  ],
  ...over,
});
const result = (over: Partial<SetDefaultHarnessResult> = {}): SetDefaultHarnessResult => ({ changed: true, ...over });

const state: ConnectionState = {
  snapshot: undefined as unknown as ConnectionState["snapshot"],
  connected: true,
  loadedOnce: true,
  liveSnapshot: true,
  tick: 0,
  streams: 0,
};
const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;

function setup(initial: DefaultHarnessStatus, post: PostAgentSettings) {
  const fetchSettings = vi.fn().mockResolvedValue(initial);
  return {
    fetchSettings,
    ...renderThemed(
      <ConnectionContext.Provider value={conn}>
        <MemoryRouter>
          <AgentsSettingsPage fetchSettings={fetchSettings} post={post} />
        </MemoryRouter>
      </ConnectionContext.Provider>,
    ),
  };
}

describe("runnerNote", () => {
  it("names the runner only when Codex does not run the usual way", () => {
    expect(runnerNote("auto")).toBeUndefined();
    expect(runnerNote("pty")).toBeUndefined();
    expect(runnerNote("headless")).toBe("Codex runs on PTY");
    expect(runnerNote("herdr")).toBe("Codex runs on herdr with a reduced flow");
  });
});

describe("AgentsSettingsPage", () => {
  it("posts the other harness from the control", async () => {
    const post = vi.fn().mockResolvedValue(result());
    const { findByRole } = setup(status(), post);
    const claude = await findByRole("button", { name: "Claude" });
    expect(claude.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(await findByRole("button", { name: "Codex" }));
    await waitFor(() => expect(post).toHaveBeenCalledWith({ scope: "global", harness: "codex" }));
  });

  it("shows inheriting and overriding projects, and posts Override and Remove override", async () => {
    const post = vi.fn().mockResolvedValue(result());
    const { findByText, getByRole } = setup(status(), post);
    expect((await findByText("Inherits global (Claude)")).textContent).toBeTruthy();
    fireEvent.click(getByRole("button", { name: "Override" }));
    await waitFor(() => expect(post).toHaveBeenCalledWith({ scope: "project", project: "blog", harness: "codex" }));
    fireEvent.click(getByRole("button", { name: "Remove override" }));
    await waitFor(() => expect(post).toHaveBeenCalledWith({ scope: "project", project: "gx", harness: "" }));
  });

  it("overrides to the harness the global default is not", async () => {
    const post = vi.fn().mockResolvedValue(result());
    const { findByRole } = setup(status({ default: "codex", projects: [{ project: "blog", harness: "" }] }), post);
    fireEvent.click(await findByRole("button", { name: "Override" }));
    await waitFor(() => expect(post).toHaveBeenCalledWith({ scope: "project", project: "blog", harness: "claude" }));
  });

  it("shows the server's notice after a write", async () => {
    const notice = "Default is now Codex. The next design step will use it.";
    const post = vi.fn().mockResolvedValue(result({ notice }));
    const { findByRole, findByText } = setup(status(), post);
    fireEvent.click(await findByRole("button", { name: "Codex" }));
    expect((await findByText(notice)).textContent).toBe(notice);
  });

  it("shows a refusal inline and keeps the old selection", async () => {
    const post = vi.fn().mockResolvedValue(result({ refused: true, changed: false, message: "codex is not on PATH" }));
    const { findByRole, findByText } = setup(status(), post);
    fireEvent.click(await findByRole("button", { name: "Codex" }));
    await findByText("codex is not on PATH");
    expect((await findByRole("button", { name: "Claude" })).getAttribute("aria-pressed")).toBe("true");
  });

  it("shows the Codex setup hint only while Codex is in use", async () => {
    const hint = /gx codex setup/;
    const withCodex = setup(status(), vi.fn());
    await withCodex.findByText(hint);
    withCodex.unmount();
    const without = setup(status({ projects: [{ project: "blog", harness: "" }] }), vi.fn());
    await without.findByText("Inherits global (Claude)");
    expect(without.queryByText(hint)).toBeNull();
  });

  it("shows the runner note under the title", async () => {
    const { findByText } = setup(status({ runner: "headless" }), vi.fn());
    await findByText("Codex runs on PTY");
  });
});
