import { act, renderHook, waitFor } from "@testing-library/react";
import { expect, test } from "vitest";
import type { Connection } from "../connection";
import { useDetail, type DetailResult } from "./detail";

const conn = {
  getState: () => ({ streams: 0 }),
  subscribe: () => () => {},
  onEvent: () => () => {},
} as unknown as Connection;

test("a refetch kept from the previous address does not drop the fetch of the current one", async () => {
  const fetcher = (address: string): Promise<DetailResult<string>> => Promise.resolve({ kind: "ok", detail: `detail of ${address}` });
  const { result, rerender } = renderHook(({ address }) => useDetail(address, conn, fetcher), { initialProps: { address: "a" } });
  await waitFor(() => expect(result.current.detail).toBe("detail of a"));
  const staleRefetch = result.current.refetch;

  rerender({ address: "b" });
  act(() => staleRefetch());

  await waitFor(() => expect(result.current.detail).toBe("detail of b"));
});
