import { afterEach, expect, it, vi } from "vitest"
import { WriteError } from "../useWrite"
import { postWorkspace } from "./postWorkspace"

afterEach(() => vi.unstubAllGlobals())

it("posts the JSON body to the workspace route and returns the parsed result", async () => {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ ok: true }), { status: 200 }))
  vi.stubGlobal("fetch", fetchMock)
  await expect(postWorkspace("move", { name: "w", targets: ["k"] })).resolves.toEqual({ ok: true })
  expect(fetchMock).toHaveBeenCalledWith(
    "/v1/workspaces/move",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ name: "w", targets: ["k"] }),
    }),
  )
})

it("throws a WriteError with the body text on a non-OK response", async () => {
  vi.stubGlobal("fetch", async () => new Response(" nope \n", { status: 409 }))
  await expect(postWorkspace("create", { name: "w" })).rejects.toThrow(new WriteError("nope"))
})

it("falls back to the HTTP status when the body is empty", async () => {
  vi.stubGlobal("fetch", async () => new Response("", { status: 500 }))
  await expect(postWorkspace("create", { name: "w" })).rejects.toThrow("HTTP 500")
})
