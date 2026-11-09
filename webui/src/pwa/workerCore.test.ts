// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createFetchHandler, type FetchEventLike } from "./workerCore";

const ORIGIN = "https://gx.test";
const SHELL_URL = `${ORIGIN}/`;
const HTML = `<html><script src="/assets/app-1.js"></script><link href="/assets/app-1.css"></html>`;

function fakeCaches() {
  const store = new Map<string, Response>();
  const cache = {
    async match(req: string | Request) {
      const hit = store.get(typeof req === "string" ? req : req.url);
      return hit?.clone();
    },
    async put(req: string | Request, res: Response) {
      store.set(typeof req === "string" ? req : req.url, res);
    },
  };
  return { store, caches: { open: async () => cache as unknown as Cache } };
}

type Fetch = (input: string, init?: RequestInit) => Promise<Response>;

function setup(fetchImpl: Fetch) {
  const { store, caches } = fakeCaches();
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
