import { Box, chakra } from "@chakra-ui/react"
import type { KeyboardEvent } from "react"

export function Switch({
  on,
  disabled,
  onToggle,
}: {
  on: boolean
  disabled: boolean
  onToggle: () => void
}) {
  return (
    <chakra.button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label="Run on this machine"
      data-mock-part="switch"
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation()
        onToggle()
      }}
      onKeyDown={(e: KeyboardEvent) => e.stopPropagation()}
      flex="none"
      w="40px"
      h="22px"
      borderRadius="full"
      position="relative"
      bg={on ? "tone.running.fg" : "border"}
    >
      <Box
        position="absolute"
        top="2px"
        left={on ? "20px" : "2px"}
        w="18px"
        h="18px"
        borderRadius="full"
        bg="surface"
      />
    </chakra.button>
  )
}
