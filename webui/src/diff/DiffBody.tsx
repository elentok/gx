import type { DiffLineAnnotation } from "@pierre/diffs"
import { PatchDiff } from "@pierre/diffs/react"
import { useMemo } from "react"

export type DiffView = "split" | "unified"

export type LineAnchor = { path: string }

// Pierre draws inside a shadow root. Custom properties pass through it, so the app's Chakra
// tokens reach it. The font-size variable alone has no effect, hence the rules on pre and code.
export const UNSAFE_CSS = `
.diffs-host { --diffs-font-family: var(--chakra-fonts-mono); --diffs-font-size: 12px; --diffs-line-height: 18px; }
pre, [data-diffs] { --diffs-bg: var(--chakra-colors-surface); font-size: 12px; line-height: 18px; }
code { font-size: 12px; line-height: 18px; }
`

/** One anchor per added and deleted line: deleted lines use the old number, added the new one. */
export function lineAnchors(patch: string, path: string): DiffLineAnnotation<LineAnchor>[] {
  const out: DiffLineAnnotation<LineAnchor>[] = []
  let oldN = 0
  let newN = 0
  let inHunk = false
  for (const line of patch.split("\n")) {
    const h = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line)
    if (h) {
      oldN = Number(h[1])
      newN = Number(h[2])
      inHunk = true
    } else if (!inHunk || line.startsWith("\\")) {
      continue
    } else if (line.startsWith("+")) {
      out.push({ side: "additions", lineNumber: newN++, metadata: { path } })
    } else if (line.startsWith("-")) {
      out.push({ side: "deletions", lineNumber: oldN++, metadata: { path } })
    } else if (line.startsWith(" ")) {
      oldN++
      newN++
    }
  }
  return out
}

/** The only place that touches Pierre. Takes the patch, the view and the file path. */
export function DiffBody({ patch, view, path }: { patch: string; view: DiffView; path: string }) {
  const lineAnnotations = useMemo(() => lineAnchors(patch, path), [patch, path])
  const options = useMemo(
    () => ({
      diffStyle: view,
      theme: "pierre-dark" as const,
      themeType: "dark" as const,
      disableFileHeader: true,
      unsafeCSS: UNSAFE_CSS,
    }),
    [view],
  )
  return <PatchDiff patch={patch} options={options} lineAnnotations={lineAnnotations} />
}
