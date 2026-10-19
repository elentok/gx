import { useEffect, useRef, useSyncExternalStore } from "react";
import { fetchSnapshot, type Snapshot } from "./snapshot";

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
  /** Bumps every TICK_MS while the tab is visible so relative times re-render. */
  tick: number;
}

export interface ConnectionDeps {
  fetchSnapshot?: () => Promise<Snapshot>;
  openStream?: (url: string, signal: AbortSignal) => Promise<AsyncIterable<string>>;
}

// The server's heartbeat is an SSE comment line, which EventSource never surfaces, so
// the stream is read as raw text: any chunk, comment or event, proves the link is alive.
async function openEventStream(url: string, signal: AbortSignal): Promise<AsyncIterable<string>> {
  const res = await fetch(url, { signal, headers: { Accept: "text/event-stream" } });
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

export function createConnection(deps: ConnectionDeps = {}) {
  const getSnapshot = deps.fetchSnapshot ?? fetchSnapshot;
  const openStream = deps.openStream ?? openEventStream;

  let state: ConnectionState = { snapshot: null, connected: true, loadedOnce: false, tick: 0 };
  const listeners = new Set<() => void>();
  let gen = 0;
  let abort: AbortController | null = null;
  let backoff = BACKOFF_MIN_MS;
  let watchdog: ReturnType<typeof setTimeout> | undefined;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let offline: ReturnType<typeof setTimeout> | undefined;
  let ticker: ReturnType<typeof setInterval> | undefined;

  function set(patch: Partial<ConnectionState>) {
    state = { ...state, ...patch };
    listeners.forEach((l) => l());
  }

  function teardown() {
    gen++;
    abort?.abort();
    abort = null;
    clearTimeout(watchdog);
    clearTimeout(retry);
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
    teardown();
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

  async function refresh(g: number) {
    try {
      const snap = await getSnapshot();
      if (g === gen) set({ snapshot: snap });
    } catch {
      if (g === gen) fail();
    }
  }

  async function consume(g: number, since: number) {
    const ctl = new AbortController();
    abort = ctl;
    try {
      const stream = await openStream(`/v1/events?since=${since}`, ctl.signal);
      if (g !== gen) return;
      markUp();
      arm(g);
      let buf = "";
      for await (const chunk of stream) {
        if (g !== gen) return;
        arm(g);
        const blocks = (buf + chunk).split("\n\n");
        buf = blocks.pop() ?? "";
        // Events carry no payload: each one just means "snapshot is stale".
        if (blocks.some((b) => /^event:/m.test(b))) void refresh(g);
      }
      if (g === gen) fail();
    } catch {
      if (g === gen) fail();
    }
  }

  async function connect() {
    teardown();
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
    set({ snapshot: snap, loadedOnce: true });
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
      void connect();
    } else {
      stopTick();
    }
  }

  function onOnline() {
    void connect();
  }

  return {
    getState: () => state,
    subscribe(l: () => void) {
      listeners.add(l);
      return () => listeners.delete(l);
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

export function useLiveConnection(deps?: ConnectionDeps): ConnectionState {
  const ref = useRef<ReturnType<typeof createConnection>>(undefined);
  ref.current ??= createConnection(deps);
  const conn = ref.current;
  useEffect(() => {
    conn.start();
    return () => conn.stop();
  }, [conn]);
  return useSyncExternalStore(conn.subscribe, conn.getState);
}
