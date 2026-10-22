import { renderHook, waitFor } from "@testing-library/react";
import { expect, test } from "vitest";
import type { EpicList } from "./epics/list";
import { useEpicList } from "./epics/list";
import { readPageCache, writePageCache } from "./pageCache";

test("an entry is only returned for the id that wrote it", () => {
  writePageCache("epic", { n: 1 }, "gx:a");
  expect(readPageCache("epic", "gx:a")).toEqual({ n: 1 });
  expect(readPageCache("epic", "gx:b")).toBeNull();
});

test("broken storage reads as empty", () => {
  sessionStorage.setItem("gx.page.epic", "{not json");
  expect(readPageCache("epic")).toBeNull();
});

test("the epic list repaints from the cache before the fetch returns, then saves the fresh list", async () => {
  const old = { epics: [], follow_ups: [{ project: "old", open: 1 }] } as unknown as EpicList;
  const fresh = { epics: [], follow_ups: [{ project: "fresh", open: 2 }] } as unknown as EpicList;
  writePageCache("epics", old);
  const { result } = renderHook(() => useEpicList(() => new Promise<EpicList>((r) => setTimeout(() => r(fresh), 20))));
  expect(result.current.list).toEqual(old);
  await waitFor(() => expect(result.current.list).toEqual(fresh));
  expect(readPageCache("epics")).toEqual(fresh);
});
