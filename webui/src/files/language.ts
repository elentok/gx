/** Highlighter language names, keyed by lower-case file extension. */
const BY_EXTENSION: Record<string, string> = {
  go: "go",
  ts: "typescript",
  tsx: "typescript",
  mts: "typescript",
  js: "javascript",
  jsx: "javascript",
  mjs: "javascript",
  py: "python",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  css: "css",
  html: "xml",
  xml: "xml",
  sql: "sql",
  rs: "rust",
  toml: "ini",
  md: "markdown",
}

/** The highlighter language of a path, or undefined for an unknown or missing extension. */
export function languageOf(path: string): string | undefined {
  const name = path.slice(path.lastIndexOf("/") + 1)
  const dot = name.lastIndexOf(".")
  if (dot <= 0) return undefined
  return BY_EXTENSION[name.slice(dot + 1).toLowerCase()]
}
