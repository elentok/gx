import { chakra } from "@chakra-ui/react"
import type { ReactNode } from "react"
import { LuFileText } from "react-icons/lu"
import { useFileLinks } from "./FileLinkContext"
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
  const missing = useFileLinks()?.isMissing(path) ?? false
  return (
    <Anchor
      href={viewerHref(path, { line, heading })}
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
  )
}
