import { Box, Text } from "@chakra-ui/react"

export const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface",
  borderWidth: "1px",
  borderColor: "border",
  borderRadius: "6px",
  px: "3",
  _placeholder: { color: "text.dim" },
} as const

export function Field({
  label,
  optional,
  caret,
  hideLabel,
  children,
}: {
  label: string
  optional?: boolean
  caret?: boolean
  /** Keeps the label for screen readers only. */
  hideLabel?: boolean
  children: React.ReactNode
}) {
  return (
    <Box as="label" display="flex" flexDirection="column" gap="1.5" position="relative">
      <Text
        as="span"
        fontSize="13px"
        color="text.muted"
        {...(hideLabel && {
          position: "absolute",
          w: "1px",
          h: "1px",
          overflow: "hidden",
          clipPath: "inset(50%)",
          whiteSpace: "nowrap",
        })}
      >
        {label}
        {optional && <i> · optional</i>}
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
