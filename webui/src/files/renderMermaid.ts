import { mermaidThemeVariables } from "../ui/theme"

let nextId = 0
let loaded: Promise<typeof import("mermaid").default> | undefined

// Loaded on first use so the Mermaid chunk is requested only when a diagram is shown.
function loadMermaid() {
  loaded ??= import("mermaid").then((m) => {
    m.default.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      // Natural size, and a line colour that reads on both the light and the dark page.
      theme: "base",
      themeVariables: mermaidThemeVariables,
      flowchart: { useMaxWidth: false },
    })
    return m.default
  })
  return loaded
}

/** The SVG markup of one diagram. Rejects with the parse error. */
export async function renderMermaid(source: string): Promise<string> {
  const mermaid = await loadMermaid()
  const { svg } = await mermaid.render(`mermaid-${nextId++}`, source)
  return svg
}
