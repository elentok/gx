export type PathCandidate = {
  /** Offsets into the text, covering the path and its `:line[:col]` suffix. */
  start: number
  end: number
  /** The path without the line suffix. */
  path: string
  line?: number
}

export type DetectOptions = {
  /** The text is an inline code span or a Markdown link target: the whole text is one path, spaces allowed. */
  wholeText?: boolean
}

const OPENERS = "([\"'`"
const TRAILING = ".,;:)!?"
const EXTENSION = /\.[A-Za-z][A-Za-z0-9]{0,7}$/
const LINE_SUFFIX = /:(\d+)(?::\d+)?$/

const isSpace = (c: string) => /\s/.test(c)

/** Splits `text:42` and `text:42:7` into the path and the line. */
function parseCandidate(raw: string): { path: string; line?: number; length: number } | null {
  let end = raw.length
  while (end > 0 && TRAILING.includes(raw[end - 1])) end--
  let body = raw.slice(0, end)
  if (body.includes("://")) return null
  let line: number | undefined
  const suffix = LINE_SUFFIX.exec(body)
  if (suffix) {
    line = Number(suffix[1])
    body = body.slice(0, suffix.index)
  }
  const absolute = body.startsWith("/") || body.startsWith("~/")
  if (body.length < 2 || !(absolute || body.includes("/") || EXTENSION.test(body))) return null
  return { path: body, line, length: end }
}

function scanLine(line: string, offset: number, out: PathCandidate[]) {
  let i = 0
  while (i < line.length) {
    const startsHere = i === 0 || isSpace(line[i - 1]) || OPENERS.includes(line[i - 1])
    if (!startsHere || isSpace(line[i]) || OPENERS.includes(line[i])) {
      i++
      continue
    }
    let j = i
    while (j < line.length && !isSpace(line[j]) && !OPENERS.includes(line[j])) j++
    const found = parseCandidate(line.slice(i, j))
    if (found) {
      out.push({
        start: offset + i,
        end: offset + i + found.length,
        path: found.path,
        line: found.line,
      })
    }
    i = j
  }
}

/**
 * Finds the strings in text that might be file paths. It does not check that a
 * file exists: false hits such as `e.g.` stay, and the server check removes them.
 */
export function detectPaths(text: string, options: DetectOptions = {}): PathCandidate[] {
  const out: PathCandidate[] = []
  if (options.wholeText) {
    const lead = text.length - text.trimStart().length
    const trimmed = text.trim()
    const found = parseCandidate(trimmed)
    if (found) {
      out.push({
        start: lead,
        end: lead + found.length,
        path: found.path,
        line: found.line,
      })
    }
    return out
  }
  let offset = 0
  let fenced = false
  for (const line of text.split("\n")) {
    if (line.trimStart().startsWith("```")) fenced = !fenced
    else if (!fenced) scanLine(line, offset, out)
    offset += line.length + 1
  }
  return out
}
