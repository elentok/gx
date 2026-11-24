import { readFileSync } from "node:fs"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import { system } from "../src/ui/theme"

// Chakra returns nested emotion-style objects; this turns them into CSS text.
type Dict = { [key: string]: unknown }

// Where the committed files live, relative to the webui folder.
export const outDir = "../skills/gx-prototype/gx"

const LAYER_ORDER = "@layer reset, base, tokens, recipes;\n"

const kebab = (name: string): string => {
  if (name.startsWith("--")) return name
  const dashed = name.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`)
  return /^(webkit|moz|ms)-/.test(dashed) ? `-${dashed}` : dashed
}

const isDict = (v: unknown): v is Dict => typeof v === "object" && v !== null

// Keyframe steps ("0%", "from") are not selectors, so nothing inside @keyframes resolves "&".
function block(
  selector: string,
  body: Dict,
  parent: string,
  inKeyframes: boolean,
  depth: number,
): string {
  const pad = "  ".repeat(depth)
  const decls: string[] = []
  const nested: string[] = []
  const isAt = selector.startsWith("@")
  const resolved =
    isAt || inKeyframes
      ? selector
      : selector
          .split(/,(?![^(]*\))/)
          .map((s) => s.trim().replace(/&/g, parent).trim())
          .join(", ")
  const childParent = isAt ? parent : resolved === "&" ? parent : resolved
  const childKeyframes = inKeyframes || selector.startsWith("@keyframes")
  for (const [key, value] of Object.entries(body)) {
    if (isDict(value)) nested.push(block(key, value, childParent, childKeyframes, depth + 1))
    else decls.push(`${pad}  ${kebab(key)}: ${String(value)};`)
  }
  return [`${pad}${resolved} {`, ...decls, ...nested, `${pad}}`].join("\n")
}

export function toCss(tree: Dict): string {
  return Object.entries(tree)
    .map(([key, value]) => (isDict(value) ? block(key, value, "", false, 0) : ""))
    .join("\n")
}

export function tokensCss(): string {
  return `${LAYER_ORDER}${toCss(system.getTokenCss() as Dict)}\n`
}

const uiDir = resolve(dirname(fileURLToPath(import.meta.url)), "../src/ui")

// Keeps the rules of src/ui/fonts.css and swaps each file path for the font data.
export function fontsCss(): string {
  const source = readFileSync(resolve(uiDir, "fonts.css"), "utf8")
  const inlined = source.replace(/url\("\.\/fonts\/([^"]+\.woff2)"\)/g, (_m, file: string) => {
    const bytes = readFileSync(resolve(uiDir, "fonts", file))
    return `url("data:font/woff2;base64,${bytes.toString("base64")}")`
  })
  return `${LAYER_ORDER}${inlined}`
}

export function baseCss(): string {
  const preflight = toCss(system.getPreflightCss() as Dict)
  const global = toCss(system.getGlobalCss() as Dict)
  return `${LAYER_ORDER}${preflight}\n${global}\n`
}
