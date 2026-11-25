import { registerCustomCSSVariableTheme } from "@pierre/diffs"
import type { DiffLineAnnotation, FileDiffLoadedFiles, FileDiffMetadata } from "@pierre/diffs"
import { PatchDiff, Virtualizer } from "@pierre/diffs/react"
import { useMemo } from "react"
import type { DiffBlobsBody } from "./api"

export type DiffView = "split" | "unified"

export type LineAnchor = { path: string }

const token = (name: string) => `var(--chakra-colors-${name})`

// The theme's variables (Pierre's `--diffs-` prefix dropped) mapped to the app's Chakra tokens.
const THEME_TOKENS: Record<string, string> = {
  foreground: token("text-muted"),
  background: token("surface"),
  "token-comment": token("text-faint"),
  "token-parameter": token("text-muted"),
  "token-punctuation": token("text-dim"),
  "token-keyword": token("kind-design-fg"),
  "token-string": token("kind-implementation-fg"),
  "token-string-expression": token("kind-implementation-fg"),
  "token-function": token("tone-running-fg"),
  "token-constant": token("tone-waiting-fg"),
}

const emphasis = (tone: string) => `color-mix(in srgb, ${token(tone)} 30%, transparent)`

const HOST_VARS: Record<string, string> = {
  ...Object.fromEntries(Object.entries(THEME_TOKENS).map(([k, v]) => [`--diffs-${k}`, v])),
  "--diffs-bg": token("surface"),
  "--diffs-bg-context-gutter-override": token("surface"),
  "--diffs-addition-color-override": token("tone-done-fg"),
  "--diffs-deletion-color-override": token("tone-deadlocked-fg"),
  "--diffs-bg-addition-emphasis-override": emphasis("tone-done-fg"),
  "--diffs-bg-deletion-emphasis-override": emphasis("tone-deadlocked-fg"),
  "--diffs-font-family": "var(--chakra-fonts-mono)",
  "--diffs-font-size": "12.5px",
  "--diffs-line-height": "19px",
}

export const THEME_NAME = "gx"

// A second registration of one name logs an error, so this runs once at module load.
registerCustomCSSVariableTheme(THEME_NAME, THEME_TOKENS)

// Pierre draws inside a shadow root. Custom properties pass through it, so the app's Chakra
// tokens reach it. The font-size variable alone leaves `code` at 16px, hence the pre, code rule.
export const UNSAFE_CSS = `
:host { ${Object.entries(HOST_VARS)
  .map(([k, v]) => `${k}: ${v};`)
  .join(" ")} }
pre, code { font-size: 12.5px; line-height: 19px; }
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
      continue
    }
    // Before the first hunk, or a "\ No newline" marker: no line number moves.
    if (!inHunk || line.startsWith("\\")) continue
    if (line.startsWith("+")) {
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
      theme: THEME_NAME,
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
