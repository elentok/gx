// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  CACHE_PREFIX,
  cacheNameFor,
  createFetchHandler,
  type FetchEventLike,
  precache,
  pruneOldCaches,
} from "./workerCore";

const ORIGIN = "https://gx.test";
const SHELL_URL = `${ORIGIN}/`;
const HTML = `<html><script src="/assets/app-1.js"></script><link href="/assets/app-1.css"></html>`;

function fakeCaches(failPut = false) {
  const store = new Map<string, Response>();
  const cache = {
    async match(req: string | Request) {
      const hit = store.get(typeof req === "string" ? req : req.url);
      return hit?.clone();
    },
    async put(req: string | Request, res: Response) {
      if (failPut) throw new Error("quota exceeded");
      store.set(typeof req === "string" ? req : req.url, res);
    },
  };
  return { store, caches: { open: async () => cache as unknown as Cache } };
}

type Fetch = (input: string, init?: RequestInit) => Promise<Response>;

function setup(fetchImpl: Fetch, opts: { failPut?: boolean } = {}) {
  const { store, caches } = fakeCaches(opts.failPut);
  const fetchSpy = vi.fn(fetchImpl);
  const onFetch = createFetchHandler({ caches, fetch: fetchSpy, cacheName: "t" });
  const send = (path: string, init: { method?: string; mode?: RequestMode } = {}) => {
    const request = new Request(ORIGIN + path, { method: init.method ?? "GET" });
    if (init.mode) Object.defineProperty(request, "mode", { value: init.mode });
    const waits: Promise<unknown>[] = [];
    let answer: Promise<Response> | undefined;
    const event: FetchEventLike = {
      request,
      respondWith: (p) => {
        answer = p;
      },
      waitUntil: (p) => {
        waits.push(p);
      },
    };
    onFetch(event);
    return { answer, settle: () => Promise.all(waits) };
  };
  return { store, fetchSpy, send };
}

const html = (body = HTML, status = 200) => new Response(body, { status });

async function seedAssets(store: Map<string, Response>) {
  store.set(`${ORIGIN}/assets/app-1.js`, new Response("js"));
  store.set(`${ORIGIN}/assets/app-1.css`, new Response("css"));
}

describe("service worker fetch rules", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("answers a fast 200 from the network and refreshes the copy", async () => {
    const { store, fetchSpy, send } = setup(async () => html());
    await seedAssets(store);
    store.set(SHELL_URL, html("old"));
    const { answer, settle } = send("/", { mode: "navigate" });
    expect(await (await answer)!.text()).toBe(HTML);
    await settle();
    expect(await store.get(SHELL_URL)!.text()).toBe(HTML);
    expect(fetchSpy).toHaveBeenCalledWith(SHELL_URL, { cache: "no-cache" });
  });

  it("still answers a 200 when the cache write rejects", async () => {
    const { store, send } = setup(async () => html(), { failPut: true });
    await seedAssets(store);
    const { answer, settle } = send("/", { mode: "navigate" });
    expect(await (await answer)!.text()).toBe(HTML);
    await settle();
  });

  it("answers from the cache after 1.5s, then refreshes when the slow request lands", async () => {
    let land!: (r: Response) => void;
    const { store, send } = setup(() => new Promise<Response>((r) => (land = r)));
    await seedAssets(store);
    store.set(SHELL_URL, html("old"));
    const { answer, settle } = send("/");
    await vi.advanceTimersByTimeAsync(1500);
    expect(await (await answer)!.text()).toBe("old");
    land(html());
    await settle();
    expect(await store.get(SHELL_URL)!.text()).toBe(HTML);
  });

  it("answers from the cache at once on a network error and refreshes nothing", async () => {
    const { store, send } = setup(async () => {
      throw new TypeError("refused");
    });
    store.set(SHELL_URL, html("old"));
    const { answer } = send("/");
    expect(await (await answer)!.text()).toBe("old");
    expect(await store.get(SHELL_URL)!.text()).toBe("old");
  });

  it("answers from the cache at once on a 502 and refreshes nothing", async () => {
    const { store, send } = setup(async () => html("bad gateway", 502));
    store.set(SHELL_URL, html("old"));
    const { answer, settle } = send("/");
    expect(await (await answer)!.text()).toBe("old");
    await settle();
    expect(await store.get(SHELL_URL)!.text()).toBe("old");
  });

  it("keeps the old copy when the new index references uncached assets", async () => {
    const { store, send } = setup(async () => html());
    store.set(SHELL_URL, html("old"));
    const { answer, settle } = send("/");
    expect(await (await answer)!.text()).toBe(HTML);
    await settle();
    expect(await store.get(SHELL_URL)!.text()).toBe("old");
  });

  it("leaves /v1 requests and non-GET requests alone", async () => {
    const { store, fetchSpy, send } = setup(async () => html());
    for (const path of ["/v1/snapshot", "/v1/events", "/v1/web/auth"]) {
      expect(send(path).answer).toBeUndefined();
    }
    expect(send("/", { method: "POST" }).answer).toBeUndefined();
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(store.size).toBe(0);
  });

  it("serves assets cache-first", async () => {
    const { store, fetchSpy, send } = setup(async () => new Response("net"));
    const first = send("/assets/app-2.js");
    expect(await (await first.answer)!.text()).toBe("net");
    expect(store.has(`${ORIGIN}/assets/app-2.js`)).toBe(true);
    const second = send("/assets/app-2.js");
    expect(await (await second.answer)!.text()).toBe("net");
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it("serves the manifest stale-while-revalidate", async () => {
    const { store, send } = setup(async () => new Response("new"));
    store.set(`${ORIGIN}/manifest.webmanifest`, new Response("old"));
    const { answer, settle } = send("/manifest.webmanifest");
    expect(await (await answer)!.text()).toBe("old");
    await settle();
    expect(await store.get(`${ORIGIN}/manifest.webmanifest`)!.text()).toBe("new");
  });

  it("gives no cached answer when there is no copy and the server is down", async () => {
    const { store, send } = setup(async () => {
      throw new TypeError("refused");
    });
    const { answer } = send("/");
    await expect(answer).rejects.toThrow("refused");
    expect(store.size).toBe(0);
  });
});

function namedCaches() {
  const all = new Map<string, Map<string, Response>>();
  const caches = {
    async open(name: string) {
      if (!all.has(name)) all.set(name, new Map());
      const store = all.get(name)!;
      return {
        async match(req: string) {
          return store.get(req)?.clone();
        },
        async put(req: string, res: Response) {
          store.set(req, res);
        },
      } as unknown as Cache;
    },
    async keys() {
      return [...all.keys()];
    },
    async delete(name: string) {
      return all.delete(name);
    },
  };
  return { all, caches };
}

describe("service worker lifecycle", () => {
  const urls = ["/", "/assets/app-1.js", "/assets/app-1.css", "/manifest.webmanifest", "/pwa-icon-192.png"];
  const body: Fetch = async (url) => new Response(url.endsWith("/") ? HTML : "x");
  const deps = (caches: ReturnType<typeof namedCaches>["caches"], fetch: Fetch, buildId = "b1") => ({
    caches,
    fetch,
    origin: ORIGIN,
    buildId,
    urls,
  });

  it("precaches the shell and every listed asset under the build's cache name", async () => {
    const { all, caches } = namedCaches();
    await precache(deps(caches, body));
    expect(cacheNameFor("b1").startsWith(CACHE_PREFIX)).toBe(true);
    expect([...all.get(cacheNameFor("b1"))!.keys()].sort()).toEqual(urls.map((u) => ORIGIN + u).sort());
  });

  it("fails the install and leaves no cache when an asset fetch fails", async () => {
    const { all, caches } = namedCaches();
    const failing: Fetch = async (url) => (url.endsWith(".css") ? new Response("", { status: 404 }) : body(url));
    await expect(precache(deps(caches, failing))).rejects.toThrow("app-1.css");
    expect(all.size).toBe(0);
    const throwing: Fetch = async (url) => {
      if (url.endsWith(".js")) throw new TypeError("offline");
      return body(url);
    };
    await expect(precache(deps(caches, throwing))).rejects.toThrow("offline");
    expect(all.size).toBe(0);
  });

  it("stores the shell after the assets it references", async () => {
    const { all, caches } = namedCaches();
    await precache(deps(caches, body));
    expect([...all.get(cacheNameFor("b1"))!.keys()].at(-1)).toBe(SHELL_URL);
  });

  it("keeps the old build's cache when the new build's install fails", async () => {
    const { all, caches } = namedCaches();
    await precache(deps(caches, body, "b1"));
    await expect(precache(deps(caches, async () => new Response("", { status: 500 }), "b2"))).rejects.toThrow();
    expect([...all.keys()]).toEqual([cacheNameFor("b1")]);
  });

  it("prunes older builds' prefixed caches, keeps the current one, and never touches others", async () => {
    const { all, caches } = namedCaches();
    await precache(deps(caches, body, "b1"));
    await precache(deps(caches, body, "b2"));
    await caches.open("someone-elses");
    await pruneOldCaches({ caches, buildId: "b2" });
    expect([...all.keys()].sort()).toEqual([cacheNameFor("b2"), "someone-elses"].sort());
  });

  it("a changed build replaces the copy", async () => {
    const { all, caches } = namedCaches();
    await precache(deps(caches, async (u) => new Response(u.endsWith("/") ? "<html>one</html>" : "x"), "b1"));
    await precache(deps(caches, async (u) => new Response(u.endsWith("/") ? "<html>two</html>" : "x"), "b2"));
    await pruneOldCaches({ caches, buildId: "b2" });
    expect(all.has(cacheNameFor("b1"))).toBe(false);
    expect(await all.get(cacheNameFor("b2"))!.get(SHELL_URL)!.text()).toBe("<html>two</html>");
  });
});
