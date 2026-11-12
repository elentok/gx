import { Box, Spinner } from "@chakra-ui/react"
import type { DevState } from "../generated/server"
import type { Tone } from "../ui/tone"
import { useCoarsePointer } from "../useWide"

/** Each visible Dev state's pill text, popover status and tone; `off` shows nothing. */
export const DEV_LOOK: Partial<
  Record<DevState, { text: string; status: string; tone: Tone; busy?: boolean }>
> = {
  installing: {
    text: "Dev · installing",
    status: "Installing dependencies",
    tone: "muted",
    busy: true,
  },
  starting: { text: "Dev · starting", status: "Starting Vite", tone: "muted", busy: true },
  running: { text: "Dev", status: "Vite is running", tone: "done" },
  down: { text: "Dev · down", status: "Vite is down", tone: "deadlocked" },
}

/** The sidebar-footer pill for Dev mode; renders nothing while Dev mode is off. */
export function DevPill({ state }: { state: DevState }) {
  const coarse = useCoarsePointer()
  const look = DEV_LOOK[state]
  if (!look) return null
  return (
    <Box
      as="span"
      data-part="dev-pill"
      data-tone={look.tone}
      title={`Dev mode: ${look.status}`}
      display="inline-flex"
      alignItems="center"
      gap="6px"
      h={coarse ? "28px" : "22px"}
      px="8px"
      flex="none"
      whiteSpace="nowrap"
      borderRadius="full"
      fontSize="12px"
      fontWeight="600"
      letterSpacing="0.04em"
      color={`tone.${look.tone}.fg`}
      bg={`tone.${look.tone}.bg`}
    >
      {look.busy && <Spinner data-testid="dev-pill-spinner" boxSize="10px" borderWidth="2px" />}
      {look.text}
    </Box>
  )
}
