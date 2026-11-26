import { Box, Text } from "@chakra-ui/react"

/** The label around a dropdown. A text box labels itself. */
export function Field({
  label,
  caret,
  children,
}: {
  label: string
  caret?: boolean
  children: React.ReactNode
}) {
  return (
    <Box as="label" display="flex" flexDirection="column" gap="1.5" position="relative">
      <Text as="span" fontSize="13px" color="text.muted">
        {label}
      </Text>
      {children}
      {caret && (
        <Box
          aria-hidden
          position="absolute"
          right="3"
          bottom="0"
          h="40px"
          display="flex"
          alignItems="center"
          color="text.dim"
          pointerEvents="none"
        >
          ▾
        </Box>
      )}
    </Box>
  )
}
