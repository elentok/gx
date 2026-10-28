import { act, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Health, HealthReport } from "../generated/server";
import { renderThemed } from "../ui/render";
import { Tray } from "../ui/tray/Tray";

const trayHealth: Health = {
  cpu: { level: 3, state: "high", read_at: "" },
  memory: { level: 2, state: "normal", read_at: "" },
  disk: { level: 1, state: "normal", read_at: "" },
};

const GIB = 1024 ** 3;

// The reports are read 2s before the server's own time.
const report = (over: Partial<HealthReport["memory"]> = {}, serverTime = "2026-01-01T10:00:02Z"): HealthReport => ({
  host: "macbook-pro.local",
  time: serverTime,
  cpu: {
    percent: 81,
    average: 76,
    level: 3,
    state: "high",
    thresholds: { high: 70, critical: 90 },
    read_at: "2026-01-01T10:00:00Z",
  },
  memory: {
    percent: 71,
    used_bytes: 22.7 * GIB,
    total_bytes: 32 * GIB,
    average: 70,
    pressure: "normal",
    level: 2,
    state: "normal",
    thresholds: { high: 80, critical: 90 },
    read_at: "2026-01-01T10:00:00Z",
    ...over,
  },
  disks: { warn_below: "20G", clean_below: "10G", disks: [] },
  causes: { cpu: { rows: [] }, memory: { rows: [] } },
});

let fetchMock: ReturnType<typeof vi.fn>;
const respondWith = (r: HealthReport) => fetchMock.mockImplementation(async () => new Response(JSON.stringify(r)));

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  respondWith(report());
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const openTray = async () => {
  const view = renderThemed(<Tray health={trayHealth} />);
  fireEvent.click(view.getByRole("button", { name: /^System health/ }));
  await view.findByText("gx server");
  return view;
};

describe("Tray popover", () => {
  it("opens on click and fetches /v1/health; Esc closes it", async () => {
    const { queryByText } = await openTray();
    expect(fetchMock).toHaveBeenCalledWith("/v1/health");
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await waitFor(() => expect(queryByText("gx server")).toBeNull());
  });

  it("closes on a click outside", async () => {
    const { queryByText } = await openTray();
    // Zag arms its outside listener a tick after opening, so a first press may be missed.
    await waitFor(() => {
      fireEvent.pointerDown(document.body);
      fireEvent.pointerUp(document.body);
      fireEvent.click(document.body);
      expect(queryByText("gx server")).toBeNull();
    });
  });

  it("shows the host in the header", async () => {
    const { getByText } = await openTray();
    expect(getByText("macbook-pro.local")).toBeTruthy();
  });

  it("shows the CPU and memory rows with averages and thresholds", async () => {
    const { getByText } = await openTray();
    expect(getByText("81% busy")).toBeTruthy();
    expect(getByText("1-minute average 76% · high at 70%, critical at 90%")).toBeTruthy();
    expect(getByText("71% used")).toBeTruthy();
    expect(getByText("22.7 of 32 GB · macOS pressure: normal · high at 80%, critical at 90%")).toBeTruthy();
  });

  it("leaves the pressure out when the response has none", async () => {
    respondWith(report({ pressure: undefined }));
    const { getByText } = await openTray();
    expect(getByText("22.7 of 32 GB · high at 80%, critical at 90%")).toBeTruthy();
  });

  it("polls every 5 seconds while open and stops when closed", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const { queryByText } = await openTray();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await waitFor(() => expect(queryByText("gx server")).toBeNull());
    await act(() => vi.advanceTimersByTimeAsync(15000));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("ages the footer from the server's times, whatever the browser clock says", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2030-06-01T00:00:00Z"));
    const { getByText } = await openTray();
    expect(getByText("Updated 2s ago")).toBeTruthy();
    await act(() => vi.advanceTimersByTimeAsync(3000));
    expect(getByText("Updated 5s ago")).toBeTruthy();
  });
});
