import { Box, Text } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { CodeView } from "./CodeView"
import { renderMermaid } from "./renderMermaid"

type State =
  | { status: "pending" }
  | { status: "done"; svg: string }
  | { status: "failed"; error: string }

/** One diagram. A parse failure shows the error text and the source. */
export function MermaidDiagram({ source, onSettled }: { source: string; onSettled?: () => void }) {
  const [state, setState] = useState<State>({ status: "pending" })
  useEffect(() => {
    let live = true
    setState({ status: "pending" })
    renderMermaid(source).then(
      (svg) => live && setState({ status: "done", svg }),
      (e) =>
        live && setState({ status: "failed", error: e instanceof Error ? e.message : String(e) }),
    )
    return () => {
      live = false
    }
  }, [source])
  useEffect(() => {
    if (state.status !== "pending") onSettled?.()
  }, [state.status, onSettled])
  if (state.status === "failed") {
    return (
      <Box data-diagram="failed">
        <Text
          role="alert"
          p="3"
          color="tone.error.fg"
          fontFamily="mono"
          fontSize="12.5px"
          whiteSpace="pre-wrap"
        >
          {state.error}
        </Text>
        <CodeView text={source} />
      </Box>
    )
  }
  if (state.status === "pending") return <Box data-diagram="pending" minH="4" />
  return (
    <Box
      data-diagram="done"
      overflowX="auto"
      css={{ "& svg": { display: "block", marginInline: "auto" } }}
      // Mermaid's strict security level sanitizes the SVG it returns.
      dangerouslySetInnerHTML={{ __html: state.svg }}
    />
  )
}
