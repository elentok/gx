import { act, fireEvent, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CauseRow, Health, HealthCauses, HealthDisk, HealthReport } from "../generated/server";
import { ticketHref } from "../hrefs";
import { renderThemed } from "../ui/render";
import { Tray } from "../ui/tray/Tray";

const trayHealth: Health = {
  cpu: { level: 3, state: "high", read_at: "" },
  memory: { level: 2, state: "normal", read_at: "" },
  disk: { level: 1, state: "normal", read_at: "" },
};

const GIB = 1024 ** 3;
const GB = 1e9;

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
  disks: { warn_below: "20G", clean_below: "10G", state: "unknown", level: 0, reason: "waiting", disks: [] },
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

  describe("cause lists", () => {
    const causeRow = (over: Partial<CauseRow>): CauseRow => ({ kind: "process", label: "x", cpu: 0, mem_bytes: 0, ...over });
    const withCauses = (causes: HealthCauses, over: Partial<HealthReport> = {}) => {
      respondWith({ ...report(), causes, ...over });
    };
    const part = (name: string) => document.querySelector(`[data-part='${name}']`)!;

    it("lists the CPU and memory rows with right-aligned numbers", async () => {
      withCauses({
        cpu: { rows: [causeRow({ label: "node", cpu: 42.4 }), causeRow({ label: "other users", kind: "other", cpu: 9 })] },
        memory: { rows: [causeRow({ label: "claude (not gx)", kind: "claude", mem_bytes: 2.5 * GIB })] },
      });
      await openTray();
      const cpuRows = part("cpu-causes").querySelectorAll("[data-part='cause-row']");
      expect(cpuRows).toHaveLength(2);
      expect(cpuRows[0].textContent).toContain("node");
      const cpuValue = cpuRows[0].querySelector("[data-part='cause-value']")!;
      expect(cpuValue.textContent).toBe("42%");
      expect(getComputedStyle(cpuValue).textAlign).toBe("right");
      const memValue = part("memory-causes").querySelector("[data-part='cause-value']")!;
      expect(memValue.textContent).toBe("2.5 GB");
      expect(getComputedStyle(memValue).textAlign).toBe("right");
      expect(part("memory-causes").textContent).toContain("claude (not gx)");
    });

    it("links an agent row to its ticket page with the busiest process in grey", async () => {
      withCauses({
        cpu: { rows: [causeRow({ kind: "agent", label: "gx:foo/03", ticket: "gx:foo/03", busiest: "go test", cpu: 30 })] },
        memory: { rows: [] },
      });
      await openTray();
      const row = part("cpu-causes").querySelector("[data-part='cause-row']")!;
      const link = row.querySelector("a")!;
      expect(link.textContent).toBe("gx:foo/03");
      expect(link.getAttribute("href")).toBe(ticketHref("gx:foo/03"));
      expect(row.textContent).toContain("gx:foo/03 · go test");
      expect(row.querySelector("[data-part='cause-busiest']")!.textContent).toBe("go test");
    });

    it("colors the agent link like the other cause rows", async () => {
      withCauses({
        cpu: { rows: [causeRow({ kind: "agent", label: "gx:foo/03", ticket: "gx:foo/03", busiest: "go test", cpu: 30 }), causeRow({ kind: "other", label: "launchd", cpu: 5 })] },
        memory: { rows: [] },
      });
      await openTray();
      const rows = part("cpu-causes").querySelectorAll("[data-part='cause-row']");
      const link = rows[0].querySelector("a")!;
      const plain = rows[1].querySelector("[data-part='cause-label']")!;
      // A browser's default link color beats the inherited one, so the link needs its own rule.
      const ownColor = [...document.styleSheets]
        .flatMap((s) => [...s.cssRules])
        .filter((r): r is CSSStyleRule => r instanceof CSSStyleRule && link.classList.contains(r.selectorText.replace(/^\./, "").split(/[:\s]/)[0]))
        .map((r) => r.style.color)
        .find(Boolean);
      expect(ownColor).toBe("var(--chakra-colors-text-dim)");
      expect(getComputedStyle(link).color).toBe(getComputedStyle(plain).color);
    });

    it("shows a session name as plain text", async () => {
      withCauses({ cpu: { rows: [causeRow({ kind: "agent", label: "design-1", cpu: 5 })] }, memory: { rows: [] } });
      await openTray();
      const row = part("cpu-causes").querySelector("[data-part='cause-row']")!;
      expect(row.textContent).toContain("design-1");
      expect(row.querySelector("a")).toBeNull();
    });

    it("replaces both lists with 'processes unavailable'", async () => {
      withCauses({ cpu: { marker: "unavailable", rows: [] }, memory: { marker: "unavailable", rows: [] } });
      await openTray();
      expect(part("cpu-causes").textContent).toBe("processes unavailable");
      expect(part("memory-causes").textContent).toBe("processes unavailable");
    });

    it("shows the lists whatever the meter state", async () => {
      const r = report({ state: "unknown", reason: "waiting" });
      r.causes = { cpu: { rows: [causeRow({ label: "node", cpu: 10 })] }, memory: { rows: [causeRow({ label: "node", mem_bytes: GIB })] } };
      respondWith(r);
      await openTray();
      expect(part("memory-causes").textContent).toContain("node");
    });

    it("shows 'reading…' and makes one extra call with wait, whose answer replaces the list", async () => {
      const reading = report();
      reading.causes = { cpu: { marker: "reading", rows: [] }, memory: { rows: [] } };
      const ready = report();
      ready.causes = { cpu: { rows: [causeRow({ label: "node", cpu: 40 })] }, memory: { rows: [] } };
      let release: () => void = () => {};
      const gate = new Promise<void>((resolve) => (release = resolve));
      fetchMock.mockImplementation(async (url: string) => {
        if (url.includes("wait")) {
          await gate;
          return new Response(JSON.stringify(ready));
        }
        return new Response(JSON.stringify(reading));
      });
      await openTray();
      await waitFor(() => expect(part("cpu-causes").textContent).toBe("reading…"));
      expect(fetchMock.mock.calls.map((c) => c[0])).toEqual(["/v1/health", "/v1/health?wait"]);
      release();
      await waitFor(() => expect(part("cpu-causes").textContent).toContain("node"));
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });

    it("cuts a long label with an ellipsis", async () => {
      withCauses({ cpu: { rows: [causeRow({ label: "a-very-long-process-name-".repeat(6), cpu: 3 })] }, memory: { rows: [] } });
      await openTray();
      const label = part("cpu-causes").querySelector("[data-part='cause-label']")!;
      const style = getComputedStyle(label);
      expect(style.textOverflow).toBe("ellipsis");
      expect(style.overflow).toBe("hidden");
      expect(style.whiteSpace).toBe("nowrap");
    });
  });

  describe("disk row", () => {
    const disk = (over: Partial<HealthDisk> = {}): HealthDisk => ({
      id: "d1",
      name: "fixture disk",
      free_bytes: 15 * GB,
      total_bytes: 100 * GB,
      state: "normal",
      held: false,
      projects: ["gx", "dotfiles"],
      read_at: "2026-01-01T10:00:00Z",
      ...over,
    });
    // The meter and worst disk are the server's; by default the last disk is the worst.
    const withDisks = (disks: HealthDisk[], meter: Partial<HealthReport["disks"]> = {}) => {
      const worst = disks[disks.length - 1];
      const r = report();
      r.disks = { warn_below: "20G", clean_below: "10G", state: worst?.state ?? "unknown", level: 3, worst_id: worst?.id, disks, ...meter };
      respondWith(r);
    };

    it("shows the server's worst disk even when another has less free space", async () => {
      withDisks(
        [disk({ id: "d1", name: "big", free_bytes: 300 * GB, total_bytes: 1000 * GB, state: "high" }), disk({ id: "d2", name: "small", free_bytes: 3 * GB, total_bytes: 10 * GB })],
        { worst_id: "d1", state: "high" },
      );
      const { getAllByText, getByText } = await openTray();
      expect(getAllByText("300G free")).toHaveLength(2);
      expect(getAllByText("3G free")).toHaveLength(1);
      expect(getByText("High")).toBeTruthy();
    });

    it("says why the disk meter has no reading", async () => {
      withDisks([], { state: "unknown", reason: "waiting", worst_id: undefined });
      const { getByText } = await openTray();
      expect(getByText("No disk reading yet · warns under 20G, cleans under 10G")).toBeTruthy();
    });

    it("lists every disk with free space, bar, state and projects, plus thresholds", async () => {
      withDisks([disk(), disk({ id: "d2", name: "work", free_bytes: 8 * GB, state: "high", projects: ["site"], held: false })]);
      const { getByText, getAllByText, container } = await openTray();
      expect(getByText("warns under 20G, cleans under 10G")).toBeTruthy();
      // The row's value is the worst disk's free space, repeated in that disk's entry.
      expect(getAllByText("8G free")).toHaveLength(2);
      expect(getByText("15G free")).toBeTruthy();
      expect(getByText("fixture disk")).toBeTruthy();
      expect(getByText("gx, dotfiles")).toBeTruthy();
      const entries = container.ownerDocument.querySelectorAll("[data-part='disk-entry']");
      expect(entries).toHaveLength(2);
      expect(entries[1].getAttribute("data-state")).toBe("high");
      expect(entries[0].querySelector("[data-part='disk-bar']")?.getAttribute("data-percent")).toBe("85");
    });

    it("tells a held disk to use the banner and has no button", async () => {
      withDisks([disk({ held: true, state: "critical" })]);
      const { getByText, queryByRole } = await openTray();
      expect(getByText("Dispatch held for gx, dotfiles. Clean now is in the banner.")).toBeTruthy();
      expect(queryByRole("button", { name: /clean/i })).toBeNull();
    });

    it("shows a disk's own age only past 10 seconds", async () => {
      withDisks([
        disk({ read_at: "2026-01-01T09:59:17Z" }),
        disk({ id: "d2", name: "fresh", read_at: "2026-01-01T09:59:57Z" }),
      ]);
      const { getByText, queryByText } = await openTray();
      expect(getByText("read 45s ago")).toBeTruthy();
      expect(queryByText("read 5s ago")).toBeNull();
    });
  });

  it.each([
    ["waiting", "Waiting for a second reading"],
    ["unavailable", "Not available in this build"],
    ["error", "The last read failed"],
  ] as const)("says why a %s row has no reading", async (reason, copy) => {
    const r = report({ state: "unknown", reason });
    r.cpu = { ...r.cpu, state: "unknown", reason };
    respondWith(r);
    const { getAllByText } = await openTray();
    expect(getAllByText(copy)).toHaveLength(2);
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

describe("Tray offline", () => {
  beforeEach(() => sessionStorage.clear());

  const openWith = async (connected: boolean) => {
    const view = renderThemed(<Tray health={trayHealth} connected={connected} />);
    fireEvent.click(view.getByRole("button", { name: /^System health/ }));
    return view;
  };

  it("fades the Tray while offline and keeps its levels, with no offline text of its own", () => {
    const { getByRole } = renderThemed(<Tray health={trayHealth} connected={false} />);
    const tray = getByRole("button", { name: /^System health/ });
    expect(tray.hasAttribute("data-stale")).toBe(true);
    expect(tray.querySelector("[data-meter=C]")!.getAttribute("data-state")).toBe("high");
    expect(tray.textContent).toBe("CMD");
  });

  it("is not faded while connected", () => {
    const { getByRole } = renderThemed(<Tray health={trayHealth} />);
    expect(getByRole("button", { name: /^System health/ }).hasAttribute("data-stale")).toBe(false);
  });

  it("keeps the last values offline, counts the age up, stops polling and fetches on reconnect", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    let setConnected: (c: boolean) => void = () => {};
    const Harness = () => {
      const [connected, set] = useState(true);
      setConnected = set;
      return <Tray health={trayHealth} connected={connected} />;
    };
    const view = renderThemed(<Harness />);
    fireEvent.click(view.getByRole("button", { name: /^System health/ }));
    await view.findByText("gx server");
    act(() => setConnected(false));
    fetchMock.mockClear();
    await act(() => vi.advanceTimersByTimeAsync(3 * 60 * 1000));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(view.getByText("81% busy")).toBeTruthy();
    expect(view.getByText("Offline. Values from 3m ago.")).toBeTruthy();
    act(() => setConnected(true));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/v1/health"));
  });

  it("uses the cached response after a reload while offline", async () => {
    const first = await openTray();
    first.unmount();
    fetchMock.mockClear();
    const { findByText } = await openWith(false);
    expect(await findByText("81% busy")).toBeTruthy();
    expect(await findByText(/^Offline\. Values from/)).toBeTruthy();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("shows the snapshot states and says no values were fetched with no cache", async () => {
    const { findByText } = await openWith(false);
    expect(await findByText("Offline. No values were fetched.")).toBeTruthy();
    expect(document.querySelector("[data-part=cpu]")!.getAttribute("data-state")).toBe("high");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
