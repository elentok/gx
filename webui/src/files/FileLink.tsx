import { chakra } from "@chakra-ui/react"
import { type ReactNode, useState } from "react"
import { LuFileText } from "react-icons/lu"
import { useCannotHover } from "../useWide"
import { FileHoverCard } from "./FileHoverCard"
import { useFileLinks } from "./FileLinkContext"
import { FileLinkSheet } from "./FileLinkSheet"
import { viewerHref } from "./viewerUrl"

const Anchor = chakra("a")

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
  const cannotHover = useCannotHover()
  const [sheet, setSheet] = useState(false)
  const sheetLink = cannotHover && !!ctx?.byPath(path)
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
              fontFamily: "{fonts.mono}",
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
