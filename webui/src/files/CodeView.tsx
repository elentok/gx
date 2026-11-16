import { Box } from "@chakra-ui/react"
import { useEffect, useRef, useState } from "react"
import { tokenStyles } from "./tokenStyles"

/** The cited line as a 1-based line number, or undefined when it names no line of `total`. */
function citedLine(line: number | undefined, total: number): number | undefined {
  if (line === undefined || !Number.isInteger(line) || line < 1 || line > total) return undefined
  return line
}

/**
 * Numbered source lines, highlighted once the lazy highlighter loads. Props in, DOM out: the
 * numbers are drawn by CSS, so a selection copies the code only.
 */
export function CodeView({
  text,
  language,
  line,
}: {
  text: string
  language?: string
  line?: number
}) {
  const lines = text.split("\n")
  // A trailing newline ends the last line; it does not start another.
  if (lines.length > 1 && lines[lines.length - 1] === "") lines.pop()
  const [html, setHtml] = useState<{ text: string; language: string; lines: string[] }>()
  const cited = useRef<HTMLDivElement>(null)
  const mark = citedLine(line, lines.length)

  useEffect(() => {
    if (!language) return
    let live = true
    void import("./highlight").then(
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

  useEffect(() => {
    cited.current?.scrollIntoView?.({ block: "center" })
  }, [mark, html])

  const shown = html && html.text === text && html.language === language ? html.lines : undefined
  return (
    <Box
      data-testid="code-view"
      overflowX="auto"
      py="2"
      fontFamily="mono"
      fontSize="11.5px"
      lineHeight="1.5"
      css={tokenStyles}
    >
      <Box minW="max-content">
        {lines.map((src, i) => {
          const n = i + 1
          const isCited = n === mark
          return (
            <Box
              key={n}
              ref={isCited ? cited : undefined}
              data-line={n}
              aria-current={isCited ? "true" : undefined}
              whiteSpace="pre"
              // An empty line has no text to give the row its height.
              minH="1.5em"
              pl="calc(3.2em + 12px)"
              pr="3"
              position="relative"
              bg={isCited ? "sidebar.selected" : undefined}
              _before={{
                content: "attr(data-line)",
                position: "absolute",
                left: "0",
                width: "3.2em",
                textAlign: "right",
                color: "text.faint",
                userSelect: "none",
              }}
              {...(shown
                ? { dangerouslySetInnerHTML: { __html: shown[i] ?? "" } }
                : { children: src || undefined })}
            />
          )
        })}
      </Box>
    </Box>
  )
}
