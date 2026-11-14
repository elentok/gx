import { Box } from "@chakra-ui/react"
import { CodeView } from "./CodeView"
import { MermaidDiagram } from "./MermaidDiagram"

/** A Mermaid file: the diagram, or its raw text as Source. */
export function MermaidView({ text, source }: { text: string; source: boolean }) {
  if (source) return <CodeView text={text} />
  return (
    <Box p="4" data-testid="mermaid-view">
      <MermaidDiagram source={text} />
    </Box>
  )
}
