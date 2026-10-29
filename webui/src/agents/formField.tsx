import { Box, chakra, Text } from "@chakra-ui/react";

const focusRing = { borderColor: "tone.running.fg", boxShadow: "0 0 0 3px {colors.tone.running.bg}" };
export const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface",
  borderWidth: "1px",
  borderColor: "border",
  borderRadius: "6px",
  px: "3",
  _placeholder: { color: "text.dim" },
  _focusVisible: { outline: "none", ...focusRing },
} as const;

export function Field({ label, optional, caret, children }: { label: string; optional?: boolean; caret?: boolean; children: React.ReactNode }) {
  return (
    <chakra.label display="flex" flexDirection="column" gap="1.5" position="relative">
      <Text as="span" fontSize="13px" color="text.muted">
        {label}
        {optional && <i> · optional</i>}
      </Text>
      {children}
      {caret && (
        <Box aria-hidden position="absolute" right="3" bottom="0" h="40px" display="flex" alignItems="center" color="text.dim" pointerEvents="none">
          ▾
        </Box>
      )}
    </chakra.label>
  );
}
