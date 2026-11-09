import { afterEach, expect, test, vi } from "vitest";
import type { SettingsEntry } from "../generated/server";
import { changedCount, draftHint, fetchSettings, groupBySection, postSettings, removeChange, rowState, searchEntries, setChange } from "./settingsApi";

afterEach(() => vi.unstubAllGlobals());

const entry = (key: string, section: string, set: boolean): SettingsEntry => ({ key, path: key.split("."), type: "string", section, description: "", set });

test("groups entries by section in the order sent and counts the keys the file holds", () => {
  const groups = groupBySection([entry("a", "Budget", true), entry("b", "Server", false), entry("c", "Budget", false), entry("d", "Server", true)]);

  expect(groups.map((g) => [g.section, g.entries.map((e) => e.key), g.changed])).toEqual([
    ["Budget", ["a", "c"], 1],
    ["Server", ["b", "d"], 1],
  ]);
  expect(changedCount([entry("a", "x", true), entry("b", "x", true), entry("c", "x", false)])).toBe(2);
});

test("search matches name, key path and description case-insensitively; blank matches nothing", () => {
  const es = [
    { ...entry("execution-queue.max-agents", "Execution queue", false), description: "Live agents." },
    { ...entry("budget.soft", "Budget", false), description: "Soft limit." },
  ];
  const keys = (q: string) => searchEntries(es, q).map((e) => e.key);
  expect(keys("MAX AGENTS")).toEqual(["execution-queue.max-agents"]);
  expect(keys("budget.")).toEqual(["budget.soft"]);
  expect(keys("limit")).toEqual(["budget.soft"]);
  expect(keys("  ")).toEqual([]);
  expect(keys("nope")).toEqual([]);
});

test("a row is set when the file holds the key, even at its default value", () => {
  expect(rowState({ set: true })).toBe("set");
  expect(rowState({ set: false })).toBe("default");
});

test("change builders", () => {
  expect(setChange("a.b", 3)).toEqual({ key: "a.b", value: 3 });
  expect(removeChange("a.b")).toEqual({ key: "a.b", remove: true });
});

test("draft hints", () => {
  expect(draftHint("number-list", "1, 2.5,3")).toBeUndefined();
  expect(draftHint("number-list", "1, x")).toBe('"x" is not a number');
  expect(draftHint("duration", "1h30m")).toBeUndefined();
  expect(draftHint("duration", "5")).toBeDefined();
  expect(draftHint("size", "50G")).toBeUndefined();
  expect(draftHint("size", "500 MB")).toBeUndefined();
  expect(draftHint("size", "lots")).toBeDefined();
  expect(draftHint("string", "anything")).toBeUndefined();
});

test("fetchSettings asks for the target and throws on a failed status", async () => {
  const fetchMock = vi.fn(async (_url: string) => new Response(JSON.stringify({ entries: [] }), { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
  expect(await fetchSettings("my proj")).toEqual({ entries: [] });
  expect(fetchMock.mock.calls[0][0]).toBe("/v1/settings?target=my%20proj");

  vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 404 })));
  await expect(fetchSettings("nope")).rejects.toThrow("HTTP 404");
});

test("postSettings sends the project only for a project target", async () => {
  const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => new Response("{}", { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);

  await postSettings("global", [removeChange("a")]);
  await postSettings("gx", [setChange("a", 1)]);

  const bodies = fetchMock.mock.calls.map((c) => JSON.parse(String(c[1]?.body)));
  expect(bodies).toEqual([{ changes: [{ key: "a", remove: true }] }, { project: "gx", changes: [{ key: "a", value: 1 }] }]);
});
