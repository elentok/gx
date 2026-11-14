import { expect, test } from "vitest"
import {
  HostedKindBinary,
  HostedKindCode,
  HostedKindHTML,
  HostedKindImage,
  HostedKindMarkdown,
  HostedKindMermaid,
  HostedKindPDF,
  HostedKindSVG,
  HostedKindText,
} from "../generated/ops"
import { MAX_PREVIEW_BYTES, previewNote, previewVerdict } from "./viewerBody"

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

test.each([
  [HostedKindCode, "code"],
  [HostedKindText, "text"],
  [HostedKindMarkdown, "markdown"],
  [HostedKindMermaid, "mermaid"],
  [HostedKindImage, "image"],
  [HostedKindSVG, "image"],
  [HostedKindHTML, "html"],
  [HostedKindPDF, "note"],
  [HostedKindBinary, "note"],
] as const)("a %s file gets the %s body", (kind, body) => {
  expect(previewVerdict({ kind, size: 10 }, false).body).toBe(body)
})

test("a missing file gets the missing body, whatever its kind", () => {
  expect(previewVerdict({ kind: HostedKindCode, size: 10 }, true).body).toBe("missing")
})

test("an over-2MB html file gets the too-large note", () => {
  const verdict = previewVerdict({ kind: HostedKindHTML, size: MAX_PREVIEW_BYTES + 1 }, false)
  expect(verdict.body).toBe("note")
})

test("a huge PDF shows the PDF note, not too large", () => {
  const note = previewNote({ kind: HostedKindPDF, size: 50 * MAX_PREVIEW_BYTES })
  expect(note?.title).toBeUndefined()
  expect(note?.text).toContain("PDF viewer")
})
