import { useEffect, useRef, useState } from "react"
import { type HostedResolved, HostedKindMarkdown } from "../generated/ops"
import { Markdown } from "../markdown/Markdown"
import { CodeView } from "./CodeView"
import { NoFileLinks, useFileLinks } from "./FileLinkContext"
import { previewCache } from "./previewCache"
import { fetchRawText } from "./rawText"

/**
 * The text body of a hover card: rendered Markdown, or plain numbered lines with no highlighter so
 * a hover never loads that chunk. Links stay plain text in a card. The text is fetched once per
 * path and mtime for the page's life; a 404 marks the file missing.
 */
export function FileHoverCardText({
  path,
  file,
  line,
}: {
  path: string
  file: HostedResolved
  line?: number
}) {
  const markMissing = useFileLinks()?.markMissing
  const [text, setText] = useState(() => previewCache.get<string>(path, file.modified))
  useEffect(() => {
    const cached = previewCache.get<string>(path, file.modified)
    if (cached !== undefined) {
      setText(cached)
      return
    }
    let live = true
    fetchRawText(path).then(
      (got) => {
        if (got === undefined) {
          markMissing?.(path)
          return
        }
        previewCache.set(path, file.modified, got)
        if (live) setText(got)
      },
      () => {},
    )
    return () => {
      live = false
    }
  }, [path, file.modified, markMissing])

  const holder = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (text === undefined || !line) return
    // The card is still being shown when the text arrives, and showing it resets the body's scroll.
    // So scroll again for a few frames, until the card has settled.
    let id = 0
    let frames = 0
    const tick = () => {
      holder.current?.querySelector("[aria-current]")?.scrollIntoView?.({ block: "center" })
      if (++frames < 12) id = requestAnimationFrame(tick)
    }
    id = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(id)
  }, [text, line])

  if (text === undefined) return null
  if (file.kind === HostedKindMarkdown) {
    return (
      <NoFileLinks>
        <Markdown card fontSize="sm">
          {text}
        </Markdown>
      </NoFileLinks>
    )
  }
  return (
    <div ref={holder}>
      <CodeView text={text} line={line} />
    </div>
  )
}
