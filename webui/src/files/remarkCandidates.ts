import { detectPaths } from "./paths"

type MdNode = {
  type: string
  value?: string
  children?: MdNode[]
  data?: { hName?: string; hProperties?: Record<string, string | number | undefined> }
}

// Links, code blocks and definitions keep their own text: a link target is checked by the `a` component.
const SKIP = new Set(["link", "linkReference", "code", "definition", "html"])

function candidateNode(text: string, path: string, line?: number): MdNode {
  return {
    type: "fileCandidate",
    data: { hName: "span", hProperties: { dataFileCandidate: path, dataLine: line } },
    children: [{ type: "text", value: text }],
  }
}

function walk(node: MdNode) {
  if (!node.children) return
  const out: MdNode[] = []
  for (const child of node.children) {
    if (SKIP.has(child.type)) {
      out.push(child)
    } else if (child.type === "inlineCode") {
      const hit = detectPaths(child.value ?? "", { wholeText: true })[0]
      if (hit) {
        child.data = {
          ...child.data,
          hProperties: { dataFileCandidate: hit.path, dataLine: hit.line },
        }
      }
      out.push(child)
    } else if (child.type === "text") {
      const text = child.value ?? ""
      let at = 0
      for (const hit of detectPaths(text)) {
        if (hit.start > at) out.push({ type: "text", value: text.slice(at, hit.start) })
        out.push(candidateNode(text.slice(hit.start, hit.end), hit.path, hit.line))
        at = hit.end
      }
      if (at === 0) out.push(child)
      else if (at < text.length) out.push({ type: "text", value: text.slice(at) })
    } else {
      walk(child)
      out.push(child)
    }
  }
  node.children = out
}

/** Marks path candidates in prose and inline code spans for the Markdown `span` and `code` overrides. */
export function remarkFileCandidates() {
  return (tree: MdNode) => walk(tree)
}
