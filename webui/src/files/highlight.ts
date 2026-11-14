// Loaded with import() only: this module and highlight.js form their own build chunk.
import hljs from "highlight.js/lib/core"
import bash from "highlight.js/lib/languages/bash"
import css from "highlight.js/lib/languages/css"
import go from "highlight.js/lib/languages/go"
import ini from "highlight.js/lib/languages/ini"
import javascript from "highlight.js/lib/languages/javascript"
import json from "highlight.js/lib/languages/json"
import markdown from "highlight.js/lib/languages/markdown"
import python from "highlight.js/lib/languages/python"
import rust from "highlight.js/lib/languages/rust"
import sql from "highlight.js/lib/languages/sql"
import typescript from "highlight.js/lib/languages/typescript"
import xml from "highlight.js/lib/languages/xml"
import yaml from "highlight.js/lib/languages/yaml"

const languages = {
  bash,
  css,
  go,
  ini,
  javascript,
  json,
  markdown,
  python,
  rust,
  sql,
  typescript,
  xml,
  yaml,
}
for (const [name, def] of Object.entries(languages)) hljs.registerLanguage(name, def)

/**
 * One HTML string per source line. A token that spans lines is closed at each line end and
 * reopened on the next, so every line is balanced markup on its own.
 */
export function highlightLines(text: string, language: string): string[] | undefined {
  if (!hljs.getLanguage(language)) return undefined
  const html = hljs.highlight(text, { language, ignoreIllegals: true }).value
  const open: string[] = []
  return html.split("\n").map((line) => {
    const prefix = open.join("")
    for (const m of line.matchAll(/<span[^>]*>|<\/span>/g)) {
      if (m[0] === "</span>") open.pop()
      else open.push(m[0])
    }
    return prefix + line + "</span>".repeat(open.length)
  })
}
