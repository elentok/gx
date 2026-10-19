import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  BACKOFF_MAX_MS,
  createConnection,
  OFFLINE_DELAY_MS,
  TICK_MS,
  WATCHDOG_MS,
} from "./connection";
import type { Snapshot } from "./snapshot";

interface FakeStream {
  url: string;
  signal: AbortSignal;
  push(chunk: string): void;
  end(): void;
}

function setup(opts: { snapshotFails?: () => boolean } = {}) {
  const log: string[] = [];
  const streams: FakeStream[] = [];
  let seq = 0;
  const conn = createConnection({
    fetchSnapshot: async () => {
      log.push("snapshot");
      if (opts.snapshotFails?.()) throw new Error("down");
      return { seq: ++seq, tickets: [] } as Snapshot;
    },
    openStream: async (url, signal) => {
      log.push(`events ${url}`);
      const queue: (string | null)[] = [];
      let wake: (() => void) | undefined;
      const s: FakeStream = {
        url,
        signal,
        push(c) {
          queue.push(c);
          wake?.();
        },
        end() {
          queue.push(null);
          wake?.();
        },
      };
      streams.push(s);
      return (async function* () {
        for (;;) {
          while (queue.length === 0) await new Promise<void>((r) => (wake = r));
          const c = queue.shift()!;
          if (c === null) return;
          yield c;
        }
      })();
    },
  });
  return { conn, log, streams };
}

function setVisibility(v: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { value: v, configurable: true });
  document.dispatchEvent(new Event("visibilitychange"));
}

beforeEach(() => {
  vi.useFakeTimers();
  setVisibility("visible");
});
afterEach(() => vi.useRealTimers());

test("connects snapshot first, then events since the snapshot seq", async () => {
  const { conn, log } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  expect(log).toEqual(["snapshot", "events /v1/events?since=1"]);
  expect(conn.getState().loadedOnce).toBe(true);
  conn.stop();
});

test("an event frame refetches the snapshot without reconnecting", async () => {
  const { conn, log, streams } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  streams[0].push("id: 2\nevent: ticket\ndata: {}\n\n");
  await vi.advanceTimersByTimeAsync(0);
  expect(log).toEqual(["snapshot", "events /v1/events?since=1", "snapshot"]);
  expect(conn.getState().snapshot?.seq).toBe(2);
  conn.stop();
});

test("a stream drop reconnects after 1s (a good snapshot resets backoff), snapshot before events", async () => {
  const { conn, log, streams } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  for (let i = 0; i < 6; i++) {
    const before = streams.length;
    streams[before - 1].end();
    await vi.advanceTimersByTimeAsync(999);
    expect(streams.length).toBe(before);
    await vi.advanceTimersByTimeAsync(1);
    expect(streams.length).toBe(before + 1);
  }
  expect(log.filter((l) => l === "snapshot").length).toBe(7);
  expect(log[log.length - 2]).toBe("snapshot");
  expect(log[log.length - 1]).toMatch(/^events/);
  conn.stop();
});

test("failing snapshots back off 1s, 2s, 4s up to 15s and a good one resets", async () => {
  let failing = true;
  const { conn, log } = setup({ snapshotFails: () => failing });
  const snaps = () => log.filter((l) => l === "snapshot").length;
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  expect(snaps()).toBe(1);
  let expectedWaits = [1000, 2000, 4000, 8000, BACKOFF_MAX_MS, BACKOFF_MAX_MS];
  for (const wait of expectedWaits) {
    const n = snaps();
    await vi.advanceTimersByTimeAsync(wait - 1);
    expect(snaps()).toBe(n);
    await vi.advanceTimersByTimeAsync(1);
    expect(snaps()).toBe(n + 1);
  }
  failing = false;
  await vi.advanceTimersByTimeAsync(BACKOFF_MAX_MS);
  expect(conn.getState().loadedOnce).toBe(true);
  conn.stop();
});

test("45s without stream data is a drop; heartbeats keep it alive", async () => {
  const { conn, streams } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  await vi.advanceTimersByTimeAsync(WATCHDOG_MS - 1000);
  streams[0].push(": heartbeat\n\n");
  await vi.advanceTimersByTimeAsync(WATCHDOG_MS - 1);
  expect(streams[0].signal.aborted).toBe(false);
  await vi.advanceTimersByTimeAsync(1);
  expect(streams[0].signal.aborted).toBe(true);
  await vi.advanceTimersByTimeAsync(1000);
  expect(streams.length).toBe(2);
  conn.stop();
});

test("connected turns false only after 5s down and recovers on reconnect", async () => {
  let failing = false;
  const { conn, streams } = setup({ snapshotFails: () => failing });
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  expect(conn.getState().connected).toBe(true);
  failing = true;
  streams[0].end();
  await vi.advanceTimersByTimeAsync(OFFLINE_DELAY_MS - 1);
  expect(conn.getState().connected).toBe(true);
  await vi.advanceTimersByTimeAsync(1);
  expect(conn.getState().connected).toBe(false);
  failing = false;
  await vi.advanceTimersByTimeAsync(BACKOFF_MAX_MS);
  expect(conn.getState().connected).toBe(true);
  expect(streams.length).toBe(2);
  conn.stop();
});

test("a quick reconnect never flips connected", async () => {
  const { conn, streams } = setup();
  const seen: boolean[] = [];
  conn.subscribe(() => seen.push(conn.getState().connected));
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  streams[0].end();
  await vi.advanceTimersByTimeAsync(1000);
  await vi.advanceTimersByTimeAsync(OFFLINE_DELAY_MS);
  expect(seen).not.toContain(false);
  expect(conn.getState().connected).toBe(true);
  conn.stop();
});

test("becoming visible or online refreshes immediately", async () => {
  const { conn, log } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  setVisibility("hidden");
  setVisibility("visible");
  await vi.advanceTimersByTimeAsync(0);
  expect(log.filter((l) => l === "snapshot").length).toBe(2);
  window.dispatchEvent(new Event("online"));
  await vi.advanceTimersByTimeAsync(0);
  expect(log.filter((l) => l === "snapshot").length).toBe(3);
  expect(log[log.length - 1]).toBe("events /v1/events?since=3");
  conn.stop();
});

test("the 60s tick runs only while the tab is visible", async () => {
  const { conn } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(TICK_MS);
  expect(conn.getState().tick).toBe(1);
  setVisibility("hidden");
  await vi.advanceTimersByTimeAsync(TICK_MS * 3);
  expect(conn.getState().tick).toBe(1);
  setVisibility("visible");
  await vi.advanceTimersByTimeAsync(TICK_MS);
  expect(conn.getState().tick).toBe(2);
  conn.stop();
});

test("the last snapshot stays on drop", async () => {
  const { conn, streams } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  streams[0].end();
  await vi.advanceTimersByTimeAsync(OFFLINE_DELAY_MS);
  expect(conn.getState().snapshot?.seq).toBeGreaterThanOrEqual(1);
  conn.stop();
});
