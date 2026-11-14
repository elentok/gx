import { Box } from "@chakra-ui/react"
import { Markdown } from "../markdown/Markdown"
import { CodeView } from "./CodeView"
import { FileLinkProvider } from "./FileLinkContext"
import type { FileResolver } from "./resolve"

export function dirOf(path: string): string {
  const i = path.lastIndexOf("/")
  return i <= 0 ? "/" : path.slice(0, i)
}

/** A Markdown file, rendered or as numbered source. Links resolve against the file's own folder. */
export function MarkdownView({
  path,
  text,
  source,
  resolver,
}: {
  path: string
  text: string
  source: boolean
  /** Tests inject a stub; the default is the real client. */
  resolver?: FileResolver
}) {
  if (source) return <CodeView text={text} language="markdown" />
  return (
    <FileLinkProvider base={{ dir: dirOf(path) }} file={path} resolver={resolver}>
      <Box
        p="4"
        data-testid="markdown-view"
        css={{
          "& h1, & h2, & h3, & h4": { fontWeight: "600", color: "{colors.tone.done.fg}" },
          "& h1": { fontSize: "lg", marginBlock: "2" },
          "& h2, & h3, & h4": { fontSize: "md", marginBlock: "2" },
        }}
      >
        <Markdown>{text}</Markdown>
      </Box>
    </FileLinkProvider>
  )
}
