import { Box, Text } from "@chakra-ui/react"
import { useState } from "react"
import {
  SessionNoticeNoComposerMarker,
  SessionNoticeUntrustedRead,
  type SessionInfo,
} from "../generated/server"
import { Button } from "../ui/button/Button"

/** One fixed text per kind. A kind the web UI does not know yet shows nothing. */
export const noticeText: Record<string, string> = {
  [SessionNoticeUntrustedRead]:
    "gx typed while the terminal was still redrawing after a resize. Check that the prompt landed as you meant it.",
  [SessionNoticeNoComposerMarker]:
    "The first prompt was sent without seeing the agent's input box. If it looks wrong, check the terminal.",
}

/** The session's non-blocking terminal issues, each dismissible. The next prompt that works clears them on the server. */
export function SessionNotices({ session }: { session: SessionInfo }) {
  const [dismissed, setDismissed] = useState<string[]>([])
  const shown = (session.notices ?? []).filter((k) => noticeText[k] && !dismissed.includes(k))
  if (shown.length === 0) return null
  return (
    <Box display="flex" flexDirection="column" gap="1" flex="none">
      {shown.map((kind) => (
        <Box key={kind} role="status" display="flex" alignItems="center" gap="2">
          <Text fontSize="sm" color="fg.muted" flex="1">
            {noticeText[kind]}
          </Text>
          <Button emphasis="ghost" tone="muted" onClick={() => setDismissed((d) => [...d, kind])}>
            Dismiss
          </Button>
        </Box>
      ))}
    </Box>
  )
}
