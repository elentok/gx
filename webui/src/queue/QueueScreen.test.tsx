import { act, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import canned from "../canned-snapshot.json";
import { OFFLINE_DELAY_MS, type ConnectionState } from "../connection";
import type { Snapshot } from "../snapshot";
import { renderThemed } from "../ui/render";
import { REPAIR_HINT } from "../view";
import { QueueScreen } from "./QueueScreen";

const snap = canned as unknown as Snapshot;
const state = (patch: Partial<ConnectionState> = {}): ConnectionState => ({
  snapshot: snap,
  connected: true,
  loadedOnce: true,
  tick: 0,
  ...patch,
});

describe("QueueScreen", () => {
  it("renders a card per epic with its state badge", () => {
    const { container } = renderThemed(<QueueScreen state={state()} />);
    expect(screen.queryByRole("heading", { name: "Queue" })).toBeNull();
    expect(container.querySelectorAll("[role=progressbar]")).toHaveLength(snap.epics.length);
    for (const label of ["Running", "Parked", "Deadlocked", "Awaiting merge"]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
    expect(screen.getByText("Waiting for a slot")).toBeTruthy();
  });

  it("labels a Merge blocked card with its reason and a Merging card with its live agent", () => {
    const base = snap.epics[0];
    const blocked = { ...base, address: "gx:blocked", name: "blocked-epic", state: "merge-blocked", active: [], parked: [], reason: "rebase clash in a.go" };
    const merging = {
      ...base,
      address: "gx:merging",
      name: "merging-epic",
      state: "merging",
      parked: [],
      reason: undefined,
      active: [{ address: "gx:merging/09", title: "Merge agent", status: "claimed", answerable: false, answered: false, needs_approval: false }],
    };
    const epics = [blocked, merging] as unknown as Snapshot["epics"];
    const { container } = renderThemed(<QueueScreen state={state({ snapshot: { ...snap, epics } })} />);
    expect(screen.getByText("Merge blocked")).toBeTruthy();
    expect(screen.getByText("rebase clash in a.go")).toBeTruthy();
    expect(screen.getByText("Merging")).toBeTruthy();
    expect(screen.getByText("Merge agent").closest("[data-tone]")?.querySelector("[role=img]")).not.toBeNull();
    expect(container.querySelectorAll("[data-live]")).toHaveLength(1);
  });

  it("shows the server build in the header, and nothing when the server sends none", () => {
    const { unmount } = renderThemed(<QueueScreen state={state({ snapshot: { ...snap, build: "v1.2.3" } })} />);
    expect(screen.getByTestId("server-build").textContent).toBe("v1.2.3");
    unmount();
    renderThemed(<QueueScreen state={state({ snapshot: { ...snap, build: undefined } })} />);
    expect(screen.queryByTestId("server-build")).toBeNull();
  });

  it("shows the Paused or Draining chip and the Live or Offline dot in the top bar", () => {
    const paused = renderThemed(<QueueScreen state={state({ snapshot: { ...snap, mode: "paused" } })} />);
    expect(paused.getByText("Paused")).toBeTruthy();
    expect(paused.getByRole("status", { name: "Live" })).toBeTruthy();
    expect(paused.getByRole("link", { name: "Queue" }).getAttribute("aria-current")).toBe("page");
    paused.unmount();
    const draining = renderThemed(<QueueScreen state={state({ snapshot: { ...snap, mode: "draining" }, connected: false })} />);
    expect(draining.getByText("Draining")).toBeTruthy();
    expect(draining.getByRole("status", { name: "Offline" })).toBeTruthy();
  });

  it("shows the deadlock reason clamped to two lines", () => {
    renderThemed(<QueueScreen state={state()} />);
    const reason = screen.getByText("blocked cycle");
    expect(reason.hasAttribute("data-reason")).toBe(true);
    expect(getComputedStyle(reason).getPropertyValue("-webkit-line-clamp")).toBe("2");
  });

  it("links an answerable row to its ticket route, unclipped", () => {
    renderThemed(<QueueScreen state={state()} />);
    expect(screen.getByRole("link", { name: "Answer →" }).getAttribute("href")).toBe("#/t/gx:wait/03");
    const summary = screen.getByText("Which option do you want?");
    expect(getComputedStyle(summary).getPropertyValue("-webkit-line-clamp")).not.toBe("2");
  });

  it("shows a repair row's reason and TUI hint with no link", () => {
    renderThemed(<QueueScreen state={state()} />);
    expect(screen.getByText("worktree has uncommitted changes")).toBeTruthy();
    expect(screen.getByText(REPAIR_HINT)).toBeTruthy();
    expect(screen.queryByRole("link", { name: /Answer/ })).not.toBeNull();
    expect(screen.getByText("worktree has uncommitted changes").closest("[data-tone]")?.querySelector("a")).toBeNull();
  });

  it("shows a needs-approval row linking to its ticket page", () => {
    renderThemed(<QueueScreen state={state()} />);
    const row = screen.getByText("Second active").closest("[data-tone]")!;
    expect(row.textContent).toContain("needs approval");
    expect(screen.getByRole("link", { name: "Review →" }).getAttribute("href")).toBe("#/t/gx:run2/01");
    expect(screen.getByText("Active one").closest("[data-tone]")?.querySelector("a")).toBeNull();
  });

  it("runs the orbit round running epics and spins only working tickets", () => {
    const { container } = renderThemed(<QueueScreen state={state()} />);
    const running = snap.epics.filter((e) => e.state === "running").length;
    expect(container.querySelectorAll("[data-live]")).toHaveLength(running);
    const activeRow = screen.getByText("Active one").closest("[data-tone]")!;
    expect(activeRow.querySelector("[role=img]")).not.toBeNull();
    const approvalRow = screen.getByText("Second active").closest("[data-tone]")!;
    expect(approvalRow.querySelector("[role=img]")).toBeNull();
  });

  it("shows an answered row in the waiting tone with its queue position", () => {
    renderThemed(<QueueScreen state={state()} />);
    const state_ = screen.getByText("Answered · #2 in queue");
    expect(state_.closest("[data-tone]")?.getAttribute("data-tone")).toBe("waiting");
  });

  it("renders finished rows without progress bars", () => {
    renderThemed(<QueueScreen state={state()} />);
    expect(screen.getByText("Recently finished")).toBeTruthy();
    expect(screen.getAllByText(/finished .* ago|finished just now/).length).toBe(snap.recently_finished.length);
  });

  it("marks finished one-offs and shows no done/total for them", () => {
    renderThemed(<QueueScreen state={state()} />);
    expect(screen.getAllByText("one-off")).toHaveLength(1);
    expect(screen.getAllByText("4/4")).toHaveLength(1);
    expect(screen.queryByText("0/0")).toBeNull();
  });

  it("links the epic name on a card and a finished row to the epic page, not the whole card, and not for one-offs", () => {
    renderThemed(<QueueScreen state={state()} />);
    const card = screen.getByText("Active one").closest("[data-tone]")!;
    expect(screen.getByRole("link", { name: "run" }).getAttribute("href")).toBe("#/e/gx:run");
    expect(card.parentElement?.tagName).not.toBe("A");
    expect(screen.getByRole("link", { name: "old" }).getAttribute("href")).toBe("#/e/gx:old");
    expect(screen.queryByRole("link", { name: "fix-flaky-test" })).toBeNull();
    expect(screen.getByText("fix-flaky-test")).toBeTruthy();
  });

  it("shows All quiet only after the first snapshot", () => {
    const empty = { ...snap, epics: [], recently_finished: [] };
    const before = renderThemed(<QueueScreen state={state({ snapshot: null, loadedOnce: false })} />);
    expect(screen.queryByText("All quiet")).toBeNull();
    before.unmount();
    renderThemed(<QueueScreen state={state({ snapshot: empty })} />);
    expect(screen.getByText("All quiet")).toBeTruthy();
    expect(screen.getByText(/Nothing is queued/)).toBeTruthy();
  });

  it("shows the Paused chip only when the queue is paused", () => {
    renderThemed(<QueueScreen state={state({ snapshot: { ...snap, mode: "paused" } })} />);
    expect(screen.getByText("Paused")).toBeTruthy();
  });

  it("shows the Offline strip when disconnected", () => {
    renderThemed(<QueueScreen state={state({ connected: false })} />);
    expect(screen.getByText("Offline, reconnecting…")).toBeTruthy();
  });
});

describe("App", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("shows the Offline strip after 5s of failed fetches, keeping no data", async () => {
    const fetchSnapshot = vi.fn().mockRejectedValue(new Error("down"));
    renderThemed(<App deps={{ fetchSnapshot }} />);
    expect(screen.queryByText("Offline, reconnecting…")).toBeNull();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(OFFLINE_DELAY_MS + 100);
    });
    expect(screen.getByText("Offline, reconnecting…")).toBeTruthy();
    expect(screen.queryByText("All quiet")).toBeNull();
  });
});
