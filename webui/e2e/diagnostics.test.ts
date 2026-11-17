// @vitest-environment node
import { describe, expect, it } from "vitest"
import { withState } from "./diagnostics"

describe("withState", () => {
  it("returns the value of run on success", async () => {
    const value = await withState(
      "wait",
      async () => "unused",
      async () => 42,
    )
    expect(value).toBe(42)
  })

  it("adds the label, the original message and the state read to a failure", async () => {
    const original = new Error("not shown")
    const error = await withState(
      "Codex weekly",
      async () => "popover: Claude 40%",
      async () => {
        throw original
      },
    ).catch((e: Error) => e)
    expect(error.message).toContain("Codex weekly")
    expect(error.message).toContain("not shown")
    expect(error.message).toContain("last state read:")
    expect(error.message).toContain("popover: Claude 40%")
    expect(error.cause).toBe(original)
  })

  it("keeps the original error when read throws", async () => {
    const error = await withState(
      "wait",
      async () => {
        throw new Error("page closed")
      },
      async () => {
        throw new Error("original")
      },
    ).catch((e: Error) => e)
    expect(error.message).toContain("original")
    expect(error.message).toContain("last state read:")
    expect(error.message).toContain("reading the state failed: page closed")
  })
})
