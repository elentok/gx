import { Box } from "@chakra-ui/react"
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { tokenStyles } from "../files/tokenStyles"
import { Button } from "../ui/button/Button"
import { withKeys } from "../uniqueKeys"
import { commandBlock } from "./taskCommand"

const COMMAND_LINE_LIMIT = 5

/** Read-only command text, lightly highlighted once the lazy highlighter loads. */
export function TaskCommand({ command }: { command: string }) {
  const { text, language } = useMemo(() => commandBlock(command), [command])
  const lines = text.split("\n")
  const [expanded, setExpanded] = useState(false)
  const [clipped, setClipped] = useState(false)
  const [html, setHtml] = useState<{ text: string; language: string; lines: string[] }>()
  const body = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let live = true
    void import("../files/highlight").then(
      (m) => {
        const out = m.highlightLines(text, language)
        if (live && out) setHtml({ text, language, lines: out })
      },
      () => {},
    )
    return () => {
      live = false
    }
  }, [text, language])

  const cut = !expanded && lines.length > COMMAND_LINE_LIMIT
  // jsdom has no layout, so this only fires in a browser: a long single line wrapped past five rows.
  // No dependency list: the highlighted lines and the text change the wrapping, so every render
  // re-measures, and an unchanged answer bails out of the state update.
  useLayoutEffect(() => {
    const el = body.current
    setClipped(!expanded && !!el && el.scrollHeight > el.clientHeight)
  })

  const shown = html && html.text === text && html.language === language ? html.lines : undefined
  const visible = cut ? lines.slice(0, COMMAND_LINE_LIMIT) : lines
  return (
    <Box
      data-testid="task-command"
      minW="0"
      mt="6px"
      mb="2px"
      bg="surface"
      border="1px solid"
      borderColor="border"
      borderRadius="8px"
      px="10px"
      py="8px"
      fontFamily="mono"
      fontSize="12px"
      lineHeight="1.5"
      color="text.dim"
      whiteSpace="pre-wrap"
      wordBreak="break-all"
      overflowWrap="anywhere"
      css={tokenStyles}
    >
      <Box
        ref={body}
        maxH={expanded ? undefined : "7.5em"}
        overflow={expanded ? undefined : "hidden"}
      >
        {withKeys(visible, (src) => src).map(({ item: src, index: line, key }) => (
          <Box
            key={key}
            minH="1.5em"
            {...(shown
              ? { dangerouslySetInnerHTML: { __html: shown[line] ?? "" } }
              : { children: src || undefined })}
          />
        ))}
      </Box>
      {cut && (
        <Box data-testid="task-command-more" color="text.faint" fontStyle="italic">
          {`# …${lines.length - COMMAND_LINE_LIMIT} more lines`}
        </Box>
      )}
      {(cut || clipped) && (
        <Button size="sm" emphasis="ghost" tone="running" onClick={() => setExpanded(true)}>
          {`Show all (${lines.length} lines)`}
        </Button>
      )}
    </Box>
  )
}
