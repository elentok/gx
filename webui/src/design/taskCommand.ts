export type CommandBlock = { text: string; language: "bash" | "python" }

const PY_HEAD = String.raw`^\s*(?:\S*\/)?python3?(?:\.\d+)?`

/** The body of a quoted argument starting at `s[0]`, unquoted; undefined when the quote never closes. */
function unquote(s: string): string | undefined {
  const q = s[0]
  if (q !== "'" && q !== '"') return undefined
  let out = ""
  for (let i = 1; i < s.length; i++) {
    const c = s[i]
    if (c === q) return out
    if (q === '"' && c === "\\" && i + 1 < s.length) {
      const n = s[i + 1]
      if (n === '"' || n === "\\" || n === "$" || n === "`") {
        out += n
        i++
        continue
      }
    }
    out += c
  }
  return undefined
}

function pythonScript(command: string): string | undefined {
  const c = new RegExp(`${PY_HEAD}(?:\\s+-\\S+)*?\\s+-c\\s+`).exec(command)
  if (c) return unquote(command.slice(c.index + c[0].length))
  const h = new RegExp(`${PY_HEAD}[^\\n]*?<<-?\\s*(['"]?)(\\w+)\\1[^\\n]*\\n`).exec(command)
  if (!h) return undefined
  const rest = command.slice(h.index + h[0].length)
  const end = new RegExp(`^\\s*${h[2]}\\s*$`, "m").exec(rest)
  return end ? rest.slice(0, end.index).replace(/\n$/, "") : undefined
}

/** What the expanded row shows for a command: a Python script body, else the full text as bash. */
export function commandBlock(command: string): CommandBlock {
  const script = pythonScript(command)
  if (script !== undefined && script.trim() !== "") return { text: script, language: "python" }
  return { text: command, language: "bash" }
}
