import { createContext, useContext, useEffect, useRef, useSyncExternalStore } from "react";
import { apiFetch } from "./apiFetch";
import { fetchSnapshot,type Snapshot } from "./snapshot";
import type { SnapshotCache } from "./snapshotCache";

export const BACKOFF_MIN_MS = 1000;
export const BACKOFF_MAX_MS = 15000;
export const WATCHDOG_MS = 45000;
export const OFFLINE_DELAY_MS = 5000;
export const TICK_MS = 60000;

export interface ConnectionState {
  snapshot: Snapshot | null;
  /** False once the link has been down for OFFLINE_DELAY_MS; drives the Offline strip. */
  connected: boolean;
  loadedOnce: boolean;
  /** False until the current connection's first server snapshot lands; a cached copy does not count. */
  liveSnapshot: boolean;
  /** Bumps every TICK_MS while the tab is visible so relative times re-render. */
  tick: number;
  /** Counts event streams opened so far; a focus report counts only while a stream is open. */
  streams: number;
}

export interface ConnectionDeps {
  /** Names this tab on the event stream. Defaults to a random id per connection. */
  clientId?: string;
  fetchSnapshot?: () => Promise<Snapshot>;
  openStream?: (url: string, signal: AbortSignal) => Promise<AsyncIterable<string>>;
  /** Seeds the first render with the last snapshot and saves each new one. */
  cache?: SnapshotCache;
}

// The server's heartbeat is an SSE comment line, which EventSource never surfaces, so
// the stream is read as raw text: any chunk, comment or event, proves the link is alive.
async function openEventStream(url: string, signal: AbortSignal): Promise<AsyncIterable<string>> {
  const res = await apiFetch(url, { signal, headers: { Accept: "text/event-stream" } });
  if (!res.ok || !res.body) throw new Error(`events: HTTP ${res.status}`);
  const body = res.body;
  return (async function* () {
    const reader = body.pipeThrough(new TextDecoderStream()).getReader();
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) return;
        yield value;
      }
    } finally {
      void reader.cancel().catch(() => {});
    }
  })();
}

function eventAddress(block: string): string | undefined {
  const line = /^data: ?(.*)$/m.exec(block)?.[1];
  if (!line) return undefined;
  try {
    const address = JSON.parse(line).address;
    return typeof address === "string" && address !== "" ? address : undefined;
  } catch {
    return undefined;
  }
}

export type EventListener = (address: string | undefined) => void;

export type Connection = ReturnType<typeof createConnection>;

export function createConnection(deps: ConnectionDeps = {}) {
  const getSnapshot = deps.fetchSnapshot ?? fetchSnapshot;
  const openStream = deps.openStream ?? openEventStream;
  const clientId = deps.clientId ?? `web-${randomId()}`;

  const cached = deps.cache?.load() ?? null;
  let state: ConnectionState = { snapshot: cached, connected: true, loadedOnce: cached !== null, liveSnapshot: false, tick: 0, streams: 0 };
  const listeners = new Set<() => void>();
  const eventListeners = new Set<EventListener>();
  let gen = 0;
  let abort: AbortController | null = null;
  let backoff = BACKOFF_MIN_MS;
  let live = false;
  let refreshing = false;
  let refreshQueued = false;
  let watchdog: ReturnType<typeof setTimeout> | undefined;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let offline: ReturnType<typeof setTimeout> | undefined;
  let ticker: ReturnType<typeof setInterval> | undefined;

  function set(patch: Partial<ConnectionState>) {
    state = { ...state, ...patch };
    if (patch.snapshot) deps.cache?.save(patch.snapshot);
    listeners.forEach((l) => l());
  }

  function teardown() {
    gen++;
    abort?.abort();
    abort = null;
    live = false;
    refreshing = false;
    refreshQueued = false;
    clearTimeout(watchdog);
    clearTimeout(retry);
  }

  // A torn-down stream makes the held snapshot stale, so it may no longer redirect or call a label gone.
  function dropStream() {
    teardown();
    if (state.liveSnapshot) set({ liveSnapshot: false });
  }

  function markUp() {
    clearTimeout(offline);
    offline = undefined;
    if (!state.connected) set({ connected: true });
  }

  function markDown() {
    if (offline || !state.connected) return;
    offline = setTimeout(() => {
      offline = undefined;
      set({ connected: false });
    }, OFFLINE_DELAY_MS);
  }

  function fail() {
    dropStream();
    markDown();
    retry = setTimeout(connect, backoff);
    backoff = Math.min(backoff * 2, BACKOFF_MAX_MS);
  }

  function arm(g: number) {
    clearTimeout(watchdog);
    watchdog = setTimeout(() => {
      if (g === gen) fail();
    }, WATCHDOG_MS);
  }

  // One fetch in flight, at most one queued behind it. Fetches run one after another, so an
  // older snapshot can never apply after a newer one.
  async function refresh(g: number) {
    if (refreshing) {
      refreshQueued = true;
      return;
    }
    refreshing = true;
    try {
      do {
        refreshQueued = false;
        const snap = await getSnapshot();
        if (g !== gen) return;
        set({ snapshot: snap });
      } while (refreshQueued);
    } catch {
      if (g === gen) fail();
    } finally {
      if (g === gen) refreshing = false;
    }
  }

  async function consume(g: number, since: number) {
    const ctl = new AbortController();
    abort = ctl;
    // Armed before the connect so a hung connect trips the watchdog too.
    arm(g);
    try {
      const stream = await openStream(`/v1/events?since=${since}&client=${encodeURIComponent(clientId)}`, ctl.signal);
      if (g !== gen) return;
      live = true;
      markUp();
      set({ streams: state.streams + 1 });
      arm(g);
      let buf = "";
      for await (const chunk of stream) {
        if (g !== gen) return;
        arm(g);
        const blocks = (buf + chunk).split("\n\n");
        buf = blocks.pop() ?? "";
        // Events carry no state: each one just means "snapshot is stale". The address, when
        // there is one, only lets a ticket page tell whether the event is about it.
        const events = blocks.filter((b) => /^event:/m.test(b));
        if (events.length > 0) void refresh(g);
        for (const b of events) eventListeners.forEach((l) => l(eventAddress(b)));
      }
      if (g === gen) fail();
    } catch {
      if (g === gen) fail();
    }
  }

  async function connect() {
    dropStream();
    const g = gen;
    let snap: Snapshot;
    try {
      snap = await getSnapshot();
    } catch {
      if (g === gen) fail();
      return;
    }
    if (g !== gen) return;
    backoff = BACKOFF_MIN_MS;
    set({ snapshot: snap, loadedOnce: true, liveSnapshot: true });
    void consume(g, snap.seq);
  }

  function startTick() {
    if (ticker === undefined) ticker = setInterval(() => set({ tick: state.tick + 1 }), TICK_MS);
  }

  function stopTick() {
    clearInterval(ticker);
    ticker = undefined;
  }

  function onVisibility() {
    if (document.visibilityState === "visible") {
      startTick();
      if (!live) void connect();
    } else {
      stopTick();
    }
  }

  function onOnline() {
    void connect();
  }

  return {
    clientId,
    getState: () => state,
    subscribe(l: () => void) {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    onEvent(l: EventListener) {
      eventListeners.add(l);
      return () => eventListeners.delete(l);
    },
    /** Re-reads the snapshot now, for a write whose event has not arrived yet. */
    refresh() {
      void refresh(gen);
    },
    start() {
      document.addEventListener("visibilitychange", onVisibility);
      window.addEventListener("online", onOnline);
      if (document.visibilityState === "visible") startTick();
      markDown();
      void connect();
    },
    stop() {
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("online", onOnline);
      teardown();
      stopTick();
      clearTimeout(offline);
      offline = undefined;
    },
  };
}

export function useConnection(deps?: ConnectionDeps): Connection {
  const ref = useRef<Connection>(undefined);
  ref.current ??= createConnection(deps);
  const conn = ref.current;
  useEffect(() => {
    conn.start();
    return () => conn.stop();
  }, [conn]);
  return conn;
}

export function useConnectionState(conn: Connection): ConnectionState {
  return useSyncExternalStore(conn.subscribe, conn.getState);
}

export const ConnectionContext = createContext<Connection | null>(null);

export function useConnectionContext(): Connection {
  const conn = useContext(ConnectionContext);
  if (!conn) throw new Error("ConnectionContext is missing");
  return conn;
}

// crypto.randomUUID exists only in secure contexts (HTTPS or localhost), and the
// web UI is often opened over plain HTTP from another device.
function randomId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}
