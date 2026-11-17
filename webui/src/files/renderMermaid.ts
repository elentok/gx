let loaded: Promise<typeof import("pele/lazy")> | undefined

// Loaded on first use so the Pele chunk is requested only when a diagram is shown.
function loadPele() {
  loaded ??= import("pele/lazy")
  return loaded
}

/** The SVG markup of one diagram. Rejects with the parse error. */
export async function renderMermaid(source: string): Promise<string> {
  const pele = await loadPele()
  // Natural size, scrolled by the diagram box, and no flip of a wide LR chart to TB on a narrow page.
  const { svg } = await pele.renderAsync(source, { responsive: false, autoDirection: false })
  return svg
}
