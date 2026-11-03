export const fieldLook = {
  fontSize: "16px",
  color: "text",
  bg: "surface.raised",
  borderColor: "hairline",
  // Focus stays quiet: a mid-grey border and a faint blue halo, not Chakra's bright ring.
  _focusVisible: { outline: "none", borderColor: "border", boxShadow: "0 0 0 2px {colors.tone.running.bg}" },
} as const;
