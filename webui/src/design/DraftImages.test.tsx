import { cleanup, screen } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import { galleryEntries } from "../gallery/registry"
import { renderThemed } from "../ui/render"

afterEach(cleanup)

test("the gallery renders the draft image row in each state, with its line", () => {
  const entry = galleryEntries.find((e) => e.name === "DraftImages")!
  const lines: (string | undefined)[] = [
    undefined,
    "1 of 10 images",
    "1 uploading",
    "2 images failed. Remove them or retry before starting.",
  ]
  expect(entry.cases.map((c) => c.label)).toEqual(["empty", "ready", "uploading", "failed"])
  entry.cases.forEach((c, i) => {
    renderThemed(<>{c.node}</>)
    if (lines[i]) expect(screen.getByText(lines[i]!)).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Add image" })).toBeNull()
    cleanup()
  })
})
