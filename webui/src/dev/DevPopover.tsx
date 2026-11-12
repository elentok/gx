import { Box, Text } from "@chakra-ui/react"
import { Button } from "../ui/button/Button"
import {
  postDevFrontend as defaultPostDevFrontend,
  postDevRestart as defaultPostDevRestart,
} from "../dev"
import type { DevStatus } from "../generated/server"
import { GxMenu, type MenuEntry } from "../ui/menu/Menu"
import { DEV_LOOK, DevPill } from "./DevPill"

interface Props {
  dev: DevStatus
  postDevFrontend?: (on: boolean) => void
  postDevRestart?: () => void
}

/** Status, reason, last Vite log lines, checkout path and actions; nothing while Dev mode is off. */
export function DevDetails({
  dev,
  postDevFrontend = defaultPostDevFrontend,
  postDevRestart = defaultPostDevRestart,
  showOff = true,
  showHeader = true,
}: Props & { showOff?: boolean; showHeader?: boolean }) {
  const look = DEV_LOOK[dev.state]
  if (!look) return null
  const log = dev.log_tail ?? []
  return (
    <Box px="3" py="2" w="300px" maxW="100%">
      {showHeader && (
        <Text
          as="div"
          fontSize="15px"
          fontWeight="600"
          color="text"
          display="flex"
          alignItems="center"
          gap="2"
        >
          Dev mode
          <Text
            as="span"
            data-testid="dev-popover-status"
            px="8px"
            h="20px"
            display="inline-flex"
            alignItems="center"
            borderRadius="full"
            fontSize="11px"
            letterSpacing="0.04em"
            color={`tone.${look.tone}.fg`}
            bg={`tone.${look.tone}.bg`}
          >
            {look.status}
          </Text>
        </Text>
      )}
      {dev.reason && dev.reason !== look.status && (
        <Text my="2" fontSize="13px" lineHeight="1.4" color="text.muted">
          {dev.reason}
        </Text>
      )}
      {log.length > 0 && (
        <Box
          data-testid="dev-popover-log"
          my="2"
          px="2"
          py="1.5"
          borderWidth="1px"
          borderColor="hairline"
          bg="surface"
          color="text.muted"
          borderRadius="6px"
          fontFamily="mono"
          fontSize="11px"
          lineHeight="1.5"
          whiteSpace="pre-wrap"
          wordBreak="break-all"
          overflow="hidden"
        >
          {log.map((line, i) => (
            <Box key={i} color={/error|exited/i.test(line) ? "tone.deadlocked.fg" : undefined}>
              {line}
            </Box>
          ))}
        </Box>
      )}
      {dev.checkout && (
        <Text
          data-testid="dev-popover-path"
          my="2"
          fontFamily="mono"
          fontSize="11px"
          color="text.dim"
          wordBreak="break-all"
        >
          Dev checkout: {dev.checkout}
        </Text>
      )}
      <Box display="flex" gap="1.5" flexWrap="wrap">
        {dev.state === "down" && (
          <Button
            size="sm"
            emphasis="secondary"
            borderColor="tone.running.fg"
            onClick={postDevRestart}
          >
            Restart Vite
          </Button>
        )}
        {showOff && (
          <Button size="sm" emphasis="secondary" onClick={() => postDevFrontend(false)}>
            Turn off Dev mode
          </Button>
        )}
      </Box>
    </Box>
  )
}

/** The Dev pill; a click opens the details. */
export function DevPopover(props: Props) {
  const { dev } = props
  const look = DEV_LOOK[dev.state]
  if (!look) return null
  const content = <DevDetails {...props} />
  const items: MenuEntry[] = [{ kind: "content", value: "dev", content }]
  return (
    <GxMenu
      placement="top-end"
      boundary={() =>
        document.querySelector('[data-part="sidebar-footer"]')?.parentElement ?? document.body
      }
      trigger={
        <button
          type="button"
          aria-label={`Dev mode: ${look.status}`}
          style={{
            background: "none",
            border: 0,
            padding: 0,
            cursor: "pointer",
          }}
        >
          <DevPill state={dev.state} />
        </button>
      }
      items={items}
    />
  )
}
