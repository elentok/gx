import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useComposerImages } from "./design/useComposerImages";
import { pruneDrafts, readRoundDraft, useComposerDraft, useImageDraft, writeRoundDraft } from "./drafts";

afterEach(() => vi.restoreAllMocks());

// A storage event from "another tab": the value is already written when the event fires.
function otherTabWrites(key: string, value: string | null) {
  if (value === null) localStorage.removeItem(key);
  else localStorage.setItem(key, value);
  window.dispatchEvent(new StorageEvent("storage", { key }));
}

function composerKey(sessionId: string): string {
  const { result } = renderHook(() => useComposerDraft(sessionId));
  act(() => result.current[1]("probe"));
  const key = Object.keys(localStorage).find((k) => localStorage.getItem(k) === "probe")!;
  act(() => result.current[1](""));
  return key;
}

describe("composer draft", () => {
  it("keeps a draft per session and reads it back", () => {
    const a = renderHook(() => useComposerDraft("a"));
    act(() => a.result.current[1]("hello"));
    expect(a.result.current[0]).toBe("hello");
    expect(renderHook(() => useComposerDraft("a")).result.current[0]).toBe("hello");
    expect(renderHook(() => useComposerDraft("b")).result.current[0]).toBe("");
  });

  it("removes the stored draft when the text is empty", () => {
    const { result } = renderHook(() => useComposerDraft("a"));
    act(() => result.current[1]("x"));
    act(() => result.current[1](""));
    expect(localStorage.length).toBe(0);
  });

  it("follows a storage event for its own key only", () => {
    const key = composerKey("a");
    const { result } = renderHook(() => useComposerDraft("a"));
    act(() => otherTabWrites(key, "from tab 2"));
    expect(result.current[0]).toBe("from tab 2");
    act(() => otherTabWrites(key, null));
    expect(result.current[0]).toBe("");
    act(() => otherTabWrites("unrelated", "x"));
    expect(result.current[0]).toBe("");
  });
});

describe("round draft", () => {
  it("is tagged with its round", () => {
    writeRoundDraft("a", "r1", { n: 1 });
    expect(readRoundDraft("a", "r1")).toEqual({ n: 1 });
    expect(readRoundDraft("a", "r2")).toBeNull();
    writeRoundDraft("a", "r1", null);
    expect(readRoundDraft("a", "r1")).toBeNull();
  });
});

describe("pruneDrafts", () => {
  it("deletes only the drafts of unlisted sessions", () => {
    for (const id of ["keep", "drop"]) {
      const { result } = renderHook(() => useComposerDraft(id));
      act(() => result.current[1](`text ${id}`));
      writeRoundDraft(id, "r", { id });
    }
    localStorage.setItem("other", "untouched");
    pruneDrafts(["keep"]);
    expect(renderHook(() => useComposerDraft("keep")).result.current[0]).toBe("text keep");
    expect(readRoundDraft("keep", "r")).toEqual({ id: "keep" });
    expect(renderHook(() => useComposerDraft("drop")).result.current[0]).toBe("");
    expect(readRoundDraft("drop", "r")).toBeNull();
    expect(localStorage.getItem("other")).toBe("untouched");
  });

  it("also deletes the image drafts of gone sessions", () => {
    for (const id of ["keep", "drop"]) {
      const { result } = renderHook(() => useImageDraft(id));
      act(() => result.current[1]((cur) => [...cur, { key: "k", id: "img" }]));
    }
    pruneDrafts(["keep"]);
    expect(renderHook(() => useImageDraft("keep")).result.current[0]).toHaveLength(1);
    expect(renderHook(() => useImageDraft("drop")).result.current[0]).toHaveLength(0);
  });
});

describe("image draft", () => {
  it("keeps ids and pending entries across a remount", () => {
    const { result } = renderHook(() => useImageDraft("a"));
    act(() => result.current[1]((cur) => [...cur, { key: "1", id: "x" }, { key: "2" }]));
    expect(renderHook(() => useImageDraft("a")).result.current[0]).toEqual([{ key: "1", id: "x" }, { key: "2" }]);
  });

  it("shows another tab's change, and a pending entry turning ready", () => {
    const { result } = renderHook(() => useImageDraft("a"));
    const key = "gx.draft.images.a";
    act(() => otherTabWrites(key, JSON.stringify([{ key: "1" }])));
    expect(result.current[0]).toEqual([{ key: "1" }]);
    act(() => otherTabWrites(key, JSON.stringify([{ key: "1", id: "x" }])));
    expect(result.current[0]).toEqual([{ key: "1", id: "x" }]);
  });
});

describe("composer images after a reload", () => {
  it("reads a pending entry as failed 'Upload cut off' with no retry, and ready once another tab finishes it", () => {
    localStorage.setItem("gx.draft.images.a", JSON.stringify([{ key: "1" }, { key: "2", id: "x" }]));
    const { result } = renderHook(() => useComposerImages("a"));
    expect(result.current.images[0]).toMatchObject({ state: "failed", message: "Upload cut off", canRetry: false });
    expect(result.current.failed).toBe(1);
    expect(result.current.readyIds).toEqual(["x"]);
    act(() => otherTabWrites("gx.draft.images.a", JSON.stringify([{ key: "1", id: "y" }, { key: "2", id: "x" }])));
    expect(result.current.failed).toBe(0);
    expect(result.current.readyIds).toEqual(["y", "x"]);
    expect(result.current.images[0].url).toContain("id=y");
  });
});

describe("throwing storage", () => {
  it("never throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
      throw new Error("denied");
    });
    const { result } = renderHook(() => useComposerDraft("a"));
    expect(() => act(() => result.current[1]("x"))).not.toThrow();
    expect(() => writeRoundDraft("a", "r", {})).not.toThrow();
    expect(readRoundDraft("a", "r")).toBeNull();
    expect(() => pruneDrafts([])).not.toThrow();
  });
});
