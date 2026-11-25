import type { DiffFile } from "../generated/server"

export type TreeRow =
  | { kind: "dir"; id: string; depth: number; name: string }
  | { kind: "file"; id: string; depth: number; name: string; file: DiffFile }

interface Node {
  name: string
  path: string
  children: Map<string, Node>
  file?: DiffFile
}

const byName = (a: Node, b: Node) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0)

function sorted(n: Node): Node[] {
  const kids = [...n.children.values()]
  const dirs = kids.filter((k) => k.children.size > 0).sort(byName)
  const files = kids.filter((k) => k.children.size === 0).sort(byName)
  return [...dirs, ...files]
}

/**
 * Port of the TUI's ui/tree BuildEntriesFromPaths: directories before files, each sorted by name,
 * and a chain of directories that each hold one directory folds into one row ("a/b/c").
 * A directory id is the path of the last directory in its chain.
 */
export function buildTree(files: DiffFile[]): TreeRow[] {
  const root: Node = { name: "", path: "", children: new Map() }
  for (const file of files) {
    const parts = file.path.split("/")
    let cur = root
    parts.forEach((name, i) => {
      let next = cur.children.get(name)
      if (!next) {
        next = { name, path: parts.slice(0, i + 1).join("/"), children: new Map() }
        cur.children.set(name, next)
      }
      cur = next
    })
    cur.file = file
  }
  const rows: TreeRow[] = []
  const walk = (parent: Node, depth: number) => {
    for (const child of sorted(parent)) {
      if (child.children.size === 0) {
        if (child.file) {
          rows.push({ kind: "file", id: child.path, depth, name: child.name, file: child.file })
        }
        continue
      }
      let dir = child
      const names = [dir.name]
      while (dir.children.size === 1 && !dir.file) {
        const only = [...dir.children.values()][0]
        if (only.children.size === 0) break
        names.push(only.name)
        dir = only
      }
      rows.push({ kind: "dir", id: dir.path, depth, name: names.join("/") })
      walk(dir, depth + 1)
    }
  }
  walk(root, 0)
  return rows
}

/** Files whose path contains the text, case-insensitive. */
export function filterFiles(files: DiffFile[], text: string): DiffFile[] {
  const q = text.trim().toLowerCase()
  return q ? files.filter((f) => f.path.toLowerCase().includes(q)) : files
}

/** The files in tree order: the order Prev and Next walk. */
export function treeOrder(files: DiffFile[]): DiffFile[] {
  return buildTree(files).flatMap((r) => (r.kind === "file" ? [r.file] : []))
}
