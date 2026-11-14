import { Box, Text } from "@chakra-ui/react"
import { Spinner } from "../ui/spinner/Spinner"
import { CodeView } from "./CodeView"
import { languageOf } from "./language"
import { rawUrl } from "./rawUrl"

/**
 * An HTML file: a live page in a sandboxed frame, or its source as text.
 * The sandbox stays exactly `allow-scripts`; the server answers the stable raw link with a redirect
 * to a token route, so the frame's own CSS, script and data load through the token.
 */
export function HtmlView({ path, text, source }: { path: string; text?: string; source: boolean }) {
  if (source) {
    if (text === undefined) return <Spinner label="Loading file" />
    return <CodeView text={text} language={languageOf(path)} />
  }
  return (
    <Box display="flex" flexDirection="column" height="100%" minHeight="240px">
      <Text px="4" py="2" fontSize="12px" color="text.muted">
        Sandboxed page, scripts allowed, no access to gx
      </Text>
      <iframe
        sandbox="allow-scripts"
        title={path.split("/").pop()}
        src={rawUrl(path)}
        style={{ flex: 1, width: "100%", minHeight: "240px", border: 0 }}
      />
    </Box>
  )
}
