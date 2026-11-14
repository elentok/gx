import { Box } from "@chakra-ui/react"
import { useCallback, useEffect, useRef, useState } from "react"
import { Markdown } from "../markdown/Markdown"
import { CodeView } from "./CodeView"
import { FileLinkProvider } from "./FileLinkContext"
import { headingSlugs } from "./headingSlug"
import type { FileResolver } from "./resolve"

export function dirOf(path: string): string {
  const i = path.lastIndexOf("/")
  return i <= 0 ? "/" : path.slice(0, i)
}

/** A Markdown file, rendered or as numbered source. Links resolve against the file's own folder. */
export function MarkdownView({
  path,
  text,
  source,
  resolver,
  heading,
}: {
  path: string
  text: string
  source: boolean
  /** Tests inject a stub; the default is the real client. */
  resolver?: FileResolver
  /** Slug of the heading to scroll to once the text and its diagrams have rendered. */
  heading?: string
}) {
  const box = useRef<HTMLDivElement>(null)
  const [settled, setSettled] = useState(0)
  const onDiagramSettled = useCallback(() => setSettled((n) => n + 1), [])
  useEffect(() => {
    // `settled` re-runs this when a diagram finishes, since it changes the layout.
    void settled
    if (source || !heading || !box.current) return
    if (box.current.querySelector('[data-diagram="pending"]')) return
    const all = [...box.current.querySelectorAll("h1, h2, h3, h4, h5, h6")]
    const at = headingSlugs(all.map((h) => h.textContent ?? "")).indexOf(heading)
    all[at]?.scrollIntoView()
  }, [source, heading, settled, text])
  if (source) return <CodeView text={text} language="markdown" />
  return (
    <FileLinkProvider base={{ dir: dirOf(path) }} file={path} resolver={resolver}>
      <Box
        ref={box}
        p="4"
        data-testid="markdown-view"
        css={{
          "& h1, & h2, & h3, & h4": { fontWeight: "600", color: "{colors.tone.done.fg}" },
          "& h1": { fontSize: "lg", marginBlock: "2" },
          "& h2, & h3, & h4": { fontSize: "md", marginBlock: "2" },
        }}
      >
        <Markdown onDiagramSettled={onDiagramSettled}>{text}</Markdown>
      </Box>
    </FileLinkProvider>
  )
}
