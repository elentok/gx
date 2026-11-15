// Stands in for "shiki/langs" (see the alias in vite.config.ts). Pierre does
// `import("shiki/langs")` to look up a grammar, and the real module lists about 330 of them, so
// the build emitted a chunk for each. This keeps only the languages the diff view highlights.
// Any other file renders as plain text.
import bash from "@shikijs/langs/bash"
import css from "@shikijs/langs/css"
import go from "@shikijs/langs/go"
import html from "@shikijs/langs/html"
import javascript from "@shikijs/langs/javascript"
import json from "@shikijs/langs/json"
import jsx from "@shikijs/langs/jsx"
import markdown from "@shikijs/langs/markdown"
import python from "@shikijs/langs/python"
import tsx from "@shikijs/langs/tsx"
import typescript from "@shikijs/langs/typescript"
import yaml from "@shikijs/langs/yaml"

type Loader = () => Promise<{ default: unknown }>

const load = (grammar: unknown): Loader => {
  return () => Promise.resolve({ default: grammar })
}

export const bundledLanguages: Record<string, Loader> = {
  bash: load(bash),
  sh: load(bash),
  shell: load(bash),
  shellscript: load(bash),
  zsh: load(bash),
  json: load(json),
  yaml: load(yaml),
  yml: load(yaml),
  css: load(css),
  go: load(go),
  html: load(html),
  javascript: load(javascript),
  js: load(javascript),
  jsx: load(jsx),
  markdown: load(markdown),
  md: load(markdown),
  python: load(python),
  py: load(python),
  tsx: load(tsx),
  typescript: load(typescript),
  ts: load(typescript),
}
