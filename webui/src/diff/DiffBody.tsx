import type { DiffLineAnnotation, FileDiffLoadedFiles, FileDiffMetadata } from "@pierre/diffs"
import { PatchDiff, Virtualizer } from "@pierre/diffs/react"
import { useMemo } from "react"
import type { DiffBlobsBody } from "./api"

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

/**
 * The only place that touches Pierre. Takes the patch, the view and the file path.
 * `loadBlobs` reads both sides of the file at the resolved SHAs; Pierre calls it on the first click
 * on an expand row. A narrowed view has no loader, and a patch without one has no expand rows.
 * A large file is drawn in a virtualized 70vh box so the page never mounts thousands of rows.
 */
export function DiffBody({
  patch,
  view,
  path,
  large = false,
  narrowed = false,
  loadBlobs,
}: {
  patch: string
  view: DiffView
  path: string
  large?: boolean
  narrowed?: boolean
  loadBlobs?: () => Promise<DiffBlobsBody>
}) {
  const lineAnnotations = useMemo(() => lineAnchors(patch, path), [patch, path])
  const expandable = !narrowed && !!loadBlobs
  const options = useMemo(
    () => ({
      diffStyle: view,
      theme: "pierre-dark" as const,
      themeType: "dark" as const,
      disableFileHeader: true,
      unsafeCSS: UNSAFE_CSS,
      ...(expandable && loadBlobs
        ? {
            loadDiffFiles: async (meta: FileDiffMetadata): Promise<FileDiffLoadedFiles> => {
              const blobs = await loadBlobs()
              const newFile = { name: meta.name, contents: blobs.new.text ?? "" }
              if (meta.type === "rename-pure") return { oldFile: null, newFile }
              return {
                oldFile: { name: meta.prevName ?? meta.name, contents: blobs.old.text ?? "" },
                newFile,
              }
            },
          }
        : { hunkSeparators: "simple" as const }),
    }),
    [view, expandable, loadBlobs],
  )
  const diff = <PatchDiff patch={patch} options={options} lineAnnotations={lineAnnotations} />
  if (!large) return diff
  return <Virtualizer style={{ height: "70vh", overflow: "auto" }}>{diff}</Virtualizer>
}
