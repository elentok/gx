import { Box, Spinner as ChakraSpinner, Text } from "@chakra-ui/react"
import type { SessionChatResponse } from "../generated/server"
import { InFlightTools } from "./InFlightTools"

export interface WorkingStep {
  step?: string
  title?: string
}

// Inline, not the page-centered Spinner: this one sits at the bottom of the chat.
export function WorkingRow({
  step,
  title,
  current,
  others,
  inCard,
}: WorkingStep &
  Pick<SessionChatResponse, "current" | "others"> & {
    /** The header of the working card, whose rows already say which sub-agents run. */
    inCard?: boolean
  }) {
  return (
    <Box
      role="status"
      data-mock-part={inCard ? "working-line" : undefined}
      display="flex"
      alignItems="center"
      gap="2"
      fontSize="sm"
      color="text.muted"
      minW="0"
    >
      <ChakraSpinner size="sm" flexShrink="0" />
      {/* A flex row with minW 0, so a long step, title or tool label ends in an ellipsis instead of widening the page. */}
      {/* overflow hidden: a file name wider than the whole row is cut here too. */}
      <Text as="span" display="flex" minW="0" flex="1" overflow="hidden">
        <Text as="span" minW="0" truncate whiteSpace="pre">
          Working
          {step && `: ${step}`}
          {title && ` · ${title}`}
          {current && " · "}
        </Text>
        {current && <InFlightTools current={current} others={others} plainAgent={inCard} />}
      </Text>
    </Box>
  )
}
