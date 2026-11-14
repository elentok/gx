import { chakra } from "@chakra-ui/react"
import { type ReactNode, useState } from "react"
import { LuFileText } from "react-icons/lu"
import { useCannotHover, useCoarsePointer } from "../useWide"
import { FileHoverCard } from "./FileHoverCard"
import { useFileLinks } from "./FileLinkContext"
import { FileLinkSheet } from "./FileLinkSheet"
import { viewerHref } from "./viewerUrl"

const Anchor = chakra("a")

/** What a file link opens: hover card, phone sheet, or nothing (a plain link). Touch or no-hover gets the sheet. */
export function usePreviewMode(path: string): "card" | "sheet" | "none" {
  const ctx = useFileLinks()
  const coarse = useCoarsePointer()
  const cannotHover = useCannotHover()
  if (!ctx?.byPath(path)) return "none"
  return coarse || cannotHover ? "sheet" : "card"
}

/** Link look A: a dotted mono link with a file icon. `path` is the canonical path from the server. */
export function FileLink({
  path,
  line,
  heading,
  children,
}: {
  path: string
  line?: number
  heading?: string
  children: ReactNode
}) {
  const ctx = useFileLinks()
  const missing = ctx?.isMissing(path) ?? false
  const [sheet, setSheet] = useState(false)
  const sheetLink = usePreviewMode(path) === "sheet"
  return (
    <>
      <FileHoverCard path={path} line={line} heading={heading}>
        <Anchor
          href={viewerHref(path, { line, heading })}
          onClick={
            sheetLink
              ? (e) => {
                  e.preventDefault()
                  void ctx?.recheck(path)
                  setSheet(true)
                }
              : undefined
          }
          data-file-link=""
          data-missing={missing ? "" : undefined}
          // `&&` outranks Markdown's own `& a` underline.
          css={{
            "&&": {
              fontFamily: "mono",
              fontSize: "0.9em",
              textDecoration: "underline dotted",
              textUnderlineOffset: "3px",
            },
            "&&[data-missing]": { opacity: 0.55, textDecoration: "none" },
          }}
        >
          <chakra.span display="inline-block" marginEnd="1" verticalAlign="-0.1em" aria-hidden>
            <LuFileText />
          </chakra.span>
          {children}
        </Anchor>
      </FileHoverCard>
      {sheet && (
        <FileLinkSheet path={path} line={line} heading={heading} onClose={() => setSheet(false)} />
      )}
    </>
  )
}
