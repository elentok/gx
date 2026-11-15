export const fieldLook = {
  fontSize: "16px",
  color: "text",
  bg: "surface.raised",
  borderColor: "hairline",
} as const

/** On the prompt box element that takes focus and keydown: the textarea, or the editor's contenteditable. */
export const PROMPT_BOX_ATTR = "data-prompt-box"
/** Finds a prompt box of either kind; a plain textarea stays in the list for boxes outside PromptBox. */
export const PROMPT_BOX_SELECTOR = `textarea, [${PROMPT_BOX_ATTR}]`
/** Selector for the prompt box (either kind) with this `name`. */
export const promptBoxNamed = (name: string) =>
  `textarea[name=${name}], [${PROMPT_BOX_ATTR}][name=${name}]`
