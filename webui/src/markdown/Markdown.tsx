import { Box, type BoxProps } from "@chakra-ui/react"
import { memo, type ReactNode, useMemo } from "react"
import ReactMarkdown, { type Components } from "react-markdown"
import remarkGfm from "remark-gfm"
import { FileLink } from "../files/FileLink"
import { useFileLinks, useFoundFile } from "../files/FileLinkContext"
import { detectPaths } from "../files/paths"
import { rawUrl } from "../files/rawUrl"
import { remarkFileCandidates } from "../files/remarkCandidates"

const inlineElements = ["strong", "em", "del", "code", "a", "span"]

const remarkPlugins = [remarkGfm]
const remarkPluginsWithFiles = [remarkGfm, remarkFileCandidates]

const hasScheme = /^[a-z][a-z0-9+.-]*:/i

/** The absolute path of a relative `src` next to `file`, or undefined for anything else. */
function resolveRelative(file: string, src: string): string | undefined {
  if (
    !src ||
    src.startsWith("/") ||
    src.startsWith("#") ||
    src.startsWith("~/") ||
    hasScheme.test(src)
  ) {
    return undefined
  }
  const parts = file.split("/").slice(0, -1)
  for (const seg of decode(src.split(/[?#]/)[0]).split("/")) {
    if (seg === "..") parts.pop()
    else if (seg !== "." && seg !== "") parts.push(seg)
  }
  return parts.join("/") || "/"
}

const siblingTicketLink = /^(?:issues\/)?(\d+)-[^/]*\.md$/

function decode(href: string) {
  try {
    return decodeURI(href)
  } catch {
    return href
  }
}

// A link target that is neither http(s) nor a sibling ticket: a file path, maybe with a `#heading`.
function FileTarget({ href, children }: { href: string; children?: ReactNode }) {
  const ctx = useFileLinks()
  const target = decode(href)
  const hash = target.indexOf("#")
  const pathPart = hash < 0 ? target : target.slice(0, hash)
  const heading = hash < 0 ? undefined : target.slice(hash + 1) || undefined
  const hit =
    pathPart && !hasScheme.test(pathPart)
      ? detectPaths(pathPart, { wholeText: true })[0]
      : undefined
  const found = useFoundFile(hit?.path)
  if (!pathPart) {
    if (!ctx?.file || !heading) return <>{children}</>
    return (
      <FileLink path={ctx.file} heading={heading}>
        {children}
      </FileLink>
    )
  }
  if (!found?.path) return <>{children}</>
  return (
    <FileLink path={found.path} line={hit?.line} heading={heading}>
      {children}
    </FileLink>
  )
}

function Candidate({
  path,
  line,
  children,
}: {
  path: string
  line?: number
  children?: ReactNode
}) {
  const found = useFoundFile(path)
  if (!found?.path) return <>{children}</>
  return (
    <FileLink path={found.path} line={line}>
      {children}
    </FileLink>
  )
}

function candidateProps(props: object) {
  const p = props as { "data-file-candidate"?: string; "data-line"?: string }
  const path = p["data-file-candidate"]
  if (!path) return null
  return { path, line: p["data-line"] ? Number(p["data-line"]) : undefined }
}

function buildComponents(epic: string | undefined, files: boolean): Components {
  const components: Components = {
    a: function MarkdownLink({ href, children }) {
      if (href && /^https?:\/\//i.test(href)) {
        return (
          <a href={href} target="_blank" rel="noopener noreferrer">
            {children}
          </a>
        )
      }
      const id = href?.match(siblingTicketLink)?.[1]
      if (epic && id) return <a href={`#/t/${epic}/${id}`}>{children}</a>
      if (files && href) return <FileTarget href={href}>{children}</FileTarget>
      return <>{children}</>
    },
  }
  if (files) {
    components.img = function MarkdownImage({ node: _node, src, alt }) {
      const ctx = useFileLinks()
      const abs = ctx?.file && typeof src === "string" ? resolveRelative(ctx.file, src) : undefined
      if (!abs) return <img src={src} alt={alt} />
      return <img src={rawUrl(abs)} alt={alt} />
    }
    components.span = function MarkdownSpan({ node: _node, children, ...rest }) {
      const c = candidateProps(rest)
      if (!c) return <span {...rest}>{children}</span>
      return <Candidate {...c}>{children}</Candidate>
    }
    components.code = function MarkdownCode({ node: _node, children, ...rest }) {
      const c = candidateProps(rest)
      if (!c) return <code {...rest}>{children}</code>
      return (
        <code>
          <Candidate {...c}>{children}</Candidate>
        </code>
      )
    }
  }
  return components
}

// Server text written in markdown, such as an epic summary or a ticket body. react-markdown builds
// React elements and ignores raw HTML, so the text cannot inject markup. `epic` is the "project:epic"
// address that sibling ticket links resolve against. `smallHeadings` shrinks its own `##` headings
// below the page's section headings. `inline` keeps only inline markup inside a span, for text that
// sits in a button or a line, such as an option label. It is memoized: parsing is slow, and a page
// that re-renders on each keystroke (a Grilling round) would re-parse every card.
export const Markdown = memo(function Markdown({
  children,
  epic,
  smallHeadings,
  inline,
  ...box
}: { children: string; epic?: string; smallHeadings?: boolean; inline?: boolean } & BoxProps) {
  const files = useFileLinks() !== null
  const components = useMemo(() => buildComponents(epic, files), [epic, files])
  return (
    <Box
      as={inline ? "span" : undefined}
      {...box}
      data-small-headings={smallHeadings ? "" : undefined}
      css={{
        "&[data-small-headings] h2": { fontSize: "md", fontWeight: "600" },
        // A wide table scrolls on its own; overflowWrap resets the chat's "anywhere" so cells do not squeeze.
        "& table": {
          borderCollapse: "collapse",
          marginBlock: "2",
          display: "block",
          maxWidth: "100%",
          overflowX: "auto",
          overflowWrap: "normal",
        },
        "& th, & td": {
          border: "1px solid {colors.border}",
          paddingInline: "2",
          paddingBlock: "1",
        },
        "& > :first-child": { marginTop: 0 },
        "& > :last-child": { marginBottom: 0 },
        "& p, & ul, & ol": { marginBlock: "2" },
        "& ul, & ol": { paddingInlineStart: "6" },
        "& ul": { listStyleType: "disc" },
        "& ol": { listStyleType: "decimal" },
        "& strong": { fontWeight: "600" },
        "& a": { color: "{colors.tone.done.fg}", textDecoration: "underline" },
        "& code": {
          background: "{colors.tone.running.bg}",
          color: "{colors.tone.running.fg}",
          borderRadius: "{radii.sm}",
          paddingInline: "1",
          fontSize: "0.9em",
        },
        "& pre": {
          background: "{colors.surface.raised}",
          borderRadius: "{radii.md}",
          paddingInline: "3",
          paddingBlock: "2",
          marginBlock: "2",
          // Leave room for macOS overlay scrollbars before the following paragraph.
          marginBottom: "4",
          overflowX: "auto",
          maxWidth: "100%",
          overflowWrap: "normal",
          lineHeight: "1.5",
        },
        "& pre code": { background: "none", color: "inherit", padding: 0, fontSize: "0.85em" },
      }}
    >
      <ReactMarkdown
        remarkPlugins={files ? remarkPluginsWithFiles : remarkPlugins}
        components={components}
        allowedElements={inline ? inlineElements : undefined}
        unwrapDisallowed={inline}
      >
        {children}
      </ReactMarkdown>
    </Box>
  )
})
