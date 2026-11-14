import { type ReactNode, useMemo } from "react"
import { FileLink } from "./FileLink"
import { useFileLinks, useFoundFile } from "./FileLinkContext"
import { detectPaths, type PathCandidate } from "./paths"

function Candidate({ hit, text }: { hit: PathCandidate; text: string }) {
  const found = useFoundFile(hit.path)
  const shown = text.slice(hit.start, hit.end)
  if (!found?.path) return <>{shown}</>
  return (
    <FileLink path={found.path} line={hit.line}>
      {shown}
    </FileLink>
  )
}

/** Plain text in which the paths the server found become file links. Everything else stays text. */
export function LinkedText({ children }: { children: string }) {
  const active = useFileLinks() !== null
  const hits = useMemo(() => (active ? detectPaths(children) : []), [active, children])
  if (hits.length === 0) return <>{children}</>
  const parts: ReactNode[] = []
  let at = 0
  for (const hit of hits) {
    parts.push(children.slice(at, hit.start))
    parts.push(<Candidate key={hit.start} hit={hit} text={children} />)
    at = hit.end
  }
  parts.push(children.slice(at))
  return <>{parts}</>
}
