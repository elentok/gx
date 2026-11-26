/** What every typed-in surface shares; also for controls that must look like a text box. */
export const textBoxLook = {
  fontSize: "16px",
  color: "text",
  bg: "surface.raised",
  borderWidth: "1px",
  borderColor: "hairline",
  borderRadius: "8px",
  _placeholder: { color: "text.dim" },
} as const

/** The shared look as dropdowns wear it: sunk into the panel, with a stronger, tighter edge. */
export const dropdownLook = {
  ...textBoxLook,
  bg: "surface",
  borderColor: "border",
  borderRadius: "6px",
  px: "3",
} as const
