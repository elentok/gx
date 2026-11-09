// Fetch rules of the service worker, kept free of `self` so vitest can drive
// them with a fake `caches` and `fetch`. The worker entry only wires events.

export const SHELL_TIMEOUT_MS = 1500;

export interface FetchEventLike {
  request: Request;
  respondWith(response: Promise<Response>): void;
  waitUntil(work: Promise<unknown>): void;
}

export interface WorkerDeps {
  caches: Pick<CacheStorage, "open">;
  fetch: (input: string, init?: RequestInit) => Promise<Response>;
  cacheName: string;
  shellTimeoutMs?: number;
}

// Every cache the worker opens starts with this, so Force refresh can delete
// exactly the app shell caches and nothing else's.
export const CACHE_PREFIX = "gx-shell-";

export interface LifecycleDeps {
  caches: Pick<CacheStorage, "open" | "keys" | "delete">;
  fetch: (input: string, init?: RequestInit) => Promise<Response>;
  origin: string;
  buildId: string;
  // Site-relative URLs to precache; "/" is the shell.
  urls: string[];
}

export function cacheNameFor(buildId: string): string {
  return CACHE_PREFIX + buildId;
}

// Fetch everything before opening the cache, so one failed asset leaves no
// half-set cache behind; the shell goes in last, after the assets it loads.
export async function precache(deps: LifecycleDeps): Promise<void> {
  const fetched = await Promise.all(
    deps.urls.map(async (path) => {
      const url = deps.origin + path;
      const response = await deps.fetch(url, { cache: "reload" });
      if (response.status !== 200) throw new Error(`precache ${url}: status ${response.status}`);
      return { url, response };
    }),
  );
  const cache = await deps.caches.open(cacheNameFor(deps.buildId));
  const shellURL = deps.origin + "/";
  const ordered = [...fetched.filter((f) => f.url !== shellURL), ...fetched.filter((f) => f.url === shellURL)];
  for (const { url, response } of ordered) await cache.put(url, response);
}

export async function pruneOldCaches(deps: Pick<LifecycleDeps, "caches" | "buildId">): Promise<void> {
  const current = cacheNameFor(deps.buildId);
  const names = await deps.caches.keys();
  await Promise.all(names.filter((n) => n.startsWith(CACHE_PREFIX) && n !== current).map((n) => deps.caches.delete(n)));
}

const STALE_WHILE_REVALIDATE = /^\/(manifest\.webmanifest|pwa-icon[^/]*\.png|apple-touch-icon\.png|favicon\.png)$/;
const ASSET_REF = /(?:src|href)="(\/assets\/[^"]+)"/g;

// A refreshed index.html is stored only when every asset it references is
// already cached, so an offline open never loads a shell whose scripts are
// missing; otherwise the old copy stays.
async function refreshShell(cache: Cache, shellURL: string, origin: string, response: Response) {
  const html = await response.text();
  for (const match of html.matchAll(ASSET_REF)) {
    if (!(await cache.match(origin + match[1]))) return;
  }
  await cache.put(shellURL, new Response(html, { headers: response.headers }));
}

const TIMED_OUT = Symbol("timed out");

export function createFetchHandler(deps: WorkerDeps) {
  const timeoutMs = deps.shellTimeoutMs ?? SHELL_TIMEOUT_MS;
  const open = () => deps.caches.open(deps.cacheName);

  async function shell(event: FetchEventLike, shellURL: string, origin: string): Promise<Response> {
    const cache = await open();
    const network = deps.fetch(shellURL, { cache: "no-cache" }).then(async (r) => {
      if (r.status === 200) await refreshShell(cache, shellURL, origin, r.clone());
      return r;
    });
    // A slow answer still refreshes the copy; keep the worker alive for it.
    event.waitUntil(network.catch(() => undefined));
    const timeout = new Promise<typeof TIMED_OUT>((resolve) => setTimeout(() => resolve(TIMED_OUT), timeoutMs));
    let first: Response | typeof TIMED_OUT | Error;
    try {
      first = await Promise.race([network, timeout]);
    } catch (e) {
      first = e instanceof Error ? e : new Error(String(e));
    }
    if (first instanceof Response && first.status === 200) return first;
    const cached = await cache.match(shellURL);
    if (cached) return cached;
    // No copy to fall back on: hand back whatever the network gives.
    if (first instanceof Error) throw first;
    if (first === TIMED_OUT) return network;
    return first;
  }

  async function cacheFirst(request: Request): Promise<Response> {
    const cache = await open();
    const cached = await cache.match(request.url);
    if (cached) return cached;
    const response = await deps.fetch(request.url);
    if (response.status === 200) await cache.put(request.url, response.clone());
    return response;
  }

  async function staleWhileRevalidate(event: FetchEventLike): Promise<Response> {
    const url = event.request.url;
    const cache = await open();
    const cached = await cache.match(url);
    const refresh = deps.fetch(url).then(async (r) => {
      if (r.status === 200) await cache.put(url, r.clone());
      return r;
    });
    if (!cached) return refresh;
    event.waitUntil(refresh.catch(() => undefined));
    return cached;
  }

  return function onFetch(event: FetchEventLike): void {
    const { request } = event;
    if (request.method !== "GET") return;
    const url = new URL(request.url);
    const path = url.pathname;
    if (path.startsWith("/v1/")) return;
    if (request.mode === "navigate" || path === "/" || path === "/index.html") {
      event.respondWith(shell(event, url.origin + "/", url.origin));
    } else if (path.startsWith("/assets/")) {
      event.respondWith(cacheFirst(request));
    } else if (STALE_WHILE_REVALIDATE.test(path)) {
      event.respondWith(staleWhileRevalidate(event));
    }
  };
}
