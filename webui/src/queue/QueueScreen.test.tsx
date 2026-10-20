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
    expect(screen.getByRole("heading", { name: "Queue" })).toBeTruthy();
    expect(container.querySelectorAll("[role=progressbar]")).toHaveLength(snap.epics.length);
    for (const label of ["Running", "Parked", "Deadlocked", "Awaiting merge"]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
    expect(screen.getByText("Waiting for a slot")).toBeTruthy();
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
    expect(screen.getAllByRole("link")).toHaveLength(1);
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
