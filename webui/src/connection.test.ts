import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { onSignedOut } from "./apiFetch";
import {
  BACKOFF_MAX_MS,
  createConnection,
  OFFLINE_DELAY_MS,
  TICK_MS,
  WATCHDOG_MS,
} from "./connection";
import type { Snapshot } from "./snapshot";

const emptySnapshot = (seq: number): Snapshot => ({
  seq,
  budget: { day: "", total: 0, soft_limit: 0, hard_limit: 0, budget_paused: false, hard_latched: false },
  health: {
    cpu: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
    memory: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
    disk: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
  },
  tickets: [],
  pending: [],
  sessions: [],
  epics: [],
  recently_finished: [],
});

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
    clientId: "tab-1",
    fetchSnapshot: async () => {
      log.push("snapshot");
      if (opts.snapshotFails?.()) throw new Error("down");
      return emptySnapshot(++seq);
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
  expect(log).toEqual(["snapshot", "events /v1/events?since=1&client=tab-1"]);
  expect(conn.getState().loadedOnce).toBe(true);
  conn.stop();
});

test("an event frame refetches the snapshot without reconnecting", async () => {
  const { conn, log, streams } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  streams[0].push("id: 2\nevent: ticket\ndata: {}\n\n");
  await vi.advanceTimersByTimeAsync(0);
  expect(log).toEqual(["snapshot", "events /v1/events?since=1&client=tab-1", "snapshot"]);
  expect(conn.getState().snapshot?.seq).toBe(2);
  conn.stop();
});

test("a stream drop reconnects after 1s (a good snapshot clears backoff), snapshot before events", async () => {
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

test("failing snapshots back off 1s, 2s, 4s up to 15s and a good one clears them", async () => {
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

test("becoming visible leaves a live stream alone, online reconnects", async () => {
  const { conn, log } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  setVisibility("hidden");
  setVisibility("visible");
  await vi.advanceTimersByTimeAsync(0);
  expect(log.filter((l) => l === "snapshot").length).toBe(1);
  window.dispatchEvent(new Event("online"));
  await vi.advanceTimersByTimeAsync(0);
  expect(log.filter((l) => l === "snapshot").length).toBe(2);
  expect(log[log.length - 1]).toBe("events /v1/events?since=2&client=tab-1");
  conn.stop();
});

test("becoming visible with a dead stream reconnects immediately", async () => {
  const { conn, log, streams } = setup();
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  setVisibility("hidden");
  streams[0].end();
  await vi.advanceTimersByTimeAsync(0);
  setVisibility("visible");
  await vi.advanceTimersByTimeAsync(0);
  expect(streams.length).toBe(2);
  expect(log.filter((l) => l === "snapshot").length).toBe(2);
  conn.stop();
});

test("a burst of events runs one refresh in flight plus one trailing, never out of order", async () => {
  const pending: { seq: number; resolve: () => void }[] = [];
  let calls = 0;
  const streams: { push(c: string): void }[] = [];
  const conn = createConnection({
    fetchSnapshot: async () => {
      const seq = ++calls;
      if (seq > 1) await new Promise<void>((resolve) => pending.push({ seq, resolve }));
      return emptySnapshot(seq);
    },
    openStream: async () => {
      const queue: string[] = [];
      let wake: (() => void) | undefined;
      streams.push({
        push(c) {
          queue.push(c);
          wake?.();
        },
      });
      return (async function* () {
        for (;;) {
          while (queue.length === 0) await new Promise<void>((r) => (wake = r));
          yield queue.shift()!;
        }
      })();
    },
  });
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  for (let i = 0; i < 5; i++) {
    streams[0].push(`id: ${i}\nevent: ticket\ndata: {}\n\n`);
    await vi.advanceTimersByTimeAsync(0);
  }
  expect(pending.map((p) => p.seq)).toEqual([2]);
  pending[0].resolve();
  await vi.advanceTimersByTimeAsync(0);
  expect(conn.getState().snapshot?.seq).toBe(2);
  expect(pending.map((p) => p.seq)).toEqual([2, 3]);
  pending[1].resolve();
  await vi.advanceTimersByTimeAsync(0);
  expect(conn.getState().snapshot?.seq).toBe(3);
  expect(pending.length).toBe(2);
  conn.stop();
});

test("a connect that never resolves trips the watchdog after 45s", async () => {
  const signals: AbortSignal[] = [];
  const conn = createConnection({
    fetchSnapshot: async () => emptySnapshot(1),
    openStream: (_url, signal) => {
      signals.push(signal);
      return new Promise<AsyncIterable<string>>(() => {});
    },
  });
  conn.start();
  await vi.advanceTimersByTimeAsync(WATCHDOG_MS - 1);
  expect(signals[0].aborted).toBe(false);
  await vi.advanceTimersByTimeAsync(1);
  expect(signals[0].aborted).toBe(true);
  expect(conn.getState().connected).toBe(false);
  await vi.advanceTimersByTimeAsync(1000);
  expect(signals.length).toBe(2);
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

test("liveSnapshot is false for a cached copy, true after the first fetch, false until a reconnect's snapshot lands", async () => {
  let failing = false;
  const streams: { end(): void }[] = [];
  const conn = createConnection({
    cache: { load: () => emptySnapshot(7), save: () => {} },
    fetchSnapshot: async () => {
      if (failing) throw new Error("down");
      return emptySnapshot(8);
    },
    openStream: async () => {
      let end!: () => void;
      const done = new Promise<void>((r) => (end = r));
      streams.push({ end });
      return (async function* () {
        await done;
      })();
    },
  });
  expect(conn.getState()).toMatchObject({ loadedOnce: true, liveSnapshot: false });
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  expect(conn.getState().liveSnapshot).toBe(true);

  failing = true;
  streams[0].end();
  await vi.advanceTimersByTimeAsync(1000);
  expect(conn.getState().liveSnapshot).toBe(false);

  failing = false;
  await vi.advanceTimersByTimeAsync(2000);
  expect(conn.getState().liveSnapshot).toBe(true);
  conn.stop();
});

test("a cached snapshot is the first state and each fresh one is saved", async () => {
  let saved: Snapshot | null = emptySnapshot(7);
  const conn = createConnection({
    cache: { load: () => saved, save: (s) => (saved = s) },
    fetchSnapshot: async () => emptySnapshot(8),
    openStream: async () => (async function* () {})(),
  });
  expect(conn.getState()).toMatchObject({ snapshot: { seq: 7 }, loadedOnce: true });
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  expect(saved?.seq).toBe(8);
  conn.stop();
});

test("a 401 on the event stream fires the signed-out signal", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 401 })));
  const signal = vi.fn();
  const off = onSignedOut(signal);
  const conn = createConnection({ fetchSnapshot: async () => emptySnapshot(1) });
  conn.start();
  await vi.advanceTimersByTimeAsync(0);
  expect(signal).toHaveBeenCalledTimes(1);
  conn.stop();
  off();
  vi.unstubAllGlobals();
});
