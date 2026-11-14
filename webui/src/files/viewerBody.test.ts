import { expect, test } from "vitest"
import { HostedKindCode, HostedKindImage, HostedKindPDF } from "../generated/ops"
import { MAX_PREVIEW_BYTES, previewNote } from "./viewerBody"

test.each([
  [MAX_PREVIEW_BYTES - 1, false],
  [MAX_PREVIEW_BYTES, false],
  [MAX_PREVIEW_BYTES + 1, true],
])("a %i byte file is too large: %s", (size, tooLarge) => {
  expect(previewNote({ kind: HostedKindCode, size })?.title === "Too large to show").toBe(tooLarge)
  expect(previewNote({ kind: HostedKindImage, size })?.title === "Too large to show").toBe(tooLarge)
})

test("the limit is 2 MB", () => {
  expect(MAX_PREVIEW_BYTES).toBe(2 * 1024 * 1024)
})

test("a huge PDF shows the PDF note, not too large", () => {
  const note = previewNote({ kind: HostedKindPDF, size: 50 * MAX_PREVIEW_BYTES })
  expect(note?.title).toBeUndefined()
  expect(note?.text).toContain("PDF viewer")
})
