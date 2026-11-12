import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, expect, test, vi } from "vitest"
import { useFetched } from "./useFetched"

beforeEach(() => sessionStorage.clear())

const focus = () => act(() => void window.dispatchEvent(new Event("focus")))

test("refetchOnFocus refetches when the window regains focus", async () => {
  const fetcher = vi.fn().mockResolvedValueOnce("one").mockResolvedValueOnce("two")
  const { result } = renderHook(() => useFetched("k", fetcher, undefined, "", undefined, true))
  await waitFor(() => expect(result.current.data).toBe("one"))
  focus()
  await waitFor(() => expect(result.current.data).toBe("two"))
  expect(fetcher).toHaveBeenCalledTimes(2)
})

test("without refetchOnFocus a focus event does not refetch", async () => {
  const fetcher = vi.fn().mockResolvedValue("one")
  const { result } = renderHook(() => useFetched("k2", fetcher))
  await waitFor(() => expect(result.current.data).toBe("one"))
  focus()
  expect(fetcher).toHaveBeenCalledTimes(1)
})
