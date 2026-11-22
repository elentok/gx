import { Box, Text } from "@chakra-ui/react"
import { useEffect, useLayoutEffect, useRef, useState } from "react"
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
  return <DoneDiagram svg={state.svg} />
}

/** The rendered SVG, parsed and attached before paint so the diagram is in the DOM on the render that shows it. */
function DoneDiagram({ svg }: { svg: string }) {
  const host = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    // The SVG is rendered by our own Pele renderer, not taken from outside. This insert is not a
    // sanitiser: DOMParser does not make untrusted markup safe.
    const doc = new DOMParser().parseFromString(svg, "text/html")
    host.current?.replaceChildren(...Array.from(doc.body.childNodes))
  }, [svg])
  return (
    <Box
      ref={host}
      data-diagram="done"
      overflowX="auto"
      css={{
        "& svg": { display: "block", marginInline: "auto" },
        // Pele reads these CSS variables, so the diagram takes the app's dark palette.
        "--pele-bg": "{colors.surface}",
        "--pele-fg": "{colors.text}",
        "--pele-muted": "{colors.text.dim}",
        "--pele-line": "{colors.text.dim}",
        "--pele-surface": "{colors.surface.raised}",
        "--pele-surface-alt": "{colors.sidebar}",
        "--pele-border": "{colors.border}",
        "--pele-accent": "{colors.tone.running.fg}",
      }}
    />
  )
}
