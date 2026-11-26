import { Box, chakra, Text, VisuallyHidden } from "@chakra-ui/react"
import {
  useId,
  type AriaAttributes,
  type ComponentProps,
  type DOMAttributes,
  type InputHTMLAttributes,
  type KeyboardEvent,
  type Ref,
  type TextareaHTMLAttributes,
} from "react"

type NamingAria = "aria-label" | "aria-labelledby"

type PassedKey = "value" | "defaultValue" | "placeholder" | "disabled" | "required" | "autoFocus"

type CommonProps<E> = {
  label: string
  hideLabel?: boolean
  optional?: boolean
  hint?: string
  invalid?: boolean
  name: string
  ref?: Ref<E>
  role?: string
  // JSX accepts any undeclared hyphenated attribute, so these need `never` to be refused.
  "aria-label"?: never
  "aria-labelledby"?: never
  "data-focus-ring"?: never
} & Omit<AriaAttributes, NamingAria> &
  Omit<DOMAttributes<E>, "children" | "dangerouslySetInnerHTML">

type InputProps = CommonProps<HTMLInputElement> &
  Pick<InputHTMLAttributes<HTMLInputElement>, PassedKey | "minLength" | "list">

type ProseProps = CommonProps<HTMLTextAreaElement> &
  Pick<TextareaHTMLAttributes<HTMLTextAreaElement>, PassedKey | "minLength" | "rows"> & {
    kind: "prose"
    list?: string
    autoComplete?: never
    inputMode?: never
    verbatim?: never
    secret?: never
  }

type LineProps = InputProps &
  Pick<InputHTMLAttributes<HTMLInputElement>, "inputMode"> & {
    kind: "line"
    /** Keeps the text exactly as typed: no typing help. */
    verbatim?: boolean
    /** Hides the text. Also keeps it as typed. */
    secret?: boolean
    autoComplete?: never
  }

type SearchProps = InputProps & {
  kind: "search"
  autoComplete?: never
  inputMode?: never
  verbatim?: never
  secret?: never
}

type CredentialProps = InputProps &
  Pick<InputHTMLAttributes<HTMLInputElement>, "inputMode"> & {
    kind: "credential"
    autoComplete: "username" | "current-password" | "new-password" | "one-time-code"
    verbatim?: never
    secret?: never
  }

export type TextBoxProps = ProseProps | LineProps | SearchProps | CredentialProps

const look = {
  w: "100%",
  px: "3",
  py: "2",
  fontSize: "16px",
  color: "text",
  bg: "surface.raised",
  borderWidth: "1px",
  borderColor: "border",
  borderRadius: "8px",
  _placeholder: { color: "text.dim" },
  // The global ring is off for these boxes, so the border is the focus style.
  _focusVisible: { borderColor: "tone.running.fg" },
} as const

export function TextBox(props: TextBoxProps) {
  const id = useId()
  const hintId = `${id}-hint`
  const {
    kind,
    label,
    hideLabel,
    optional,
    hint,
    invalid,
    verbatim,
    secret,
    autoComplete,
    onKeyDown,
    "aria-describedby": describedBy,
    "aria-invalid": ariaInvalid,
    ...rest
  } = props
  const asTyped = kind === "search" || kind === "credential" || verbatim || secret
  const password = secret || autoComplete === "current-password" || autoComplete === "new-password"

  const forced = {
    ...look,
    ...(invalid && { borderColor: "tone.deadlocked.fg" }),
    "aria-invalid": invalid || ariaInvalid,
    "aria-describedby":
      kind === "credential" && hint ? [describedBy, hintId].filter(Boolean).join(" ") : describedBy,
    // The label's htmlFor needs an id no caller can duplicate.
    id,
    // The global ring would double the box's own focus style.
    "data-focus-ring": "none",
    // Saved-value popups cover the page; only a credential box wants AutoFill.
    autoComplete: autoComplete ?? "off",
    ...(asTyped && {
      // Queries, paths, codes and passwords must not get a capital first letter.
      autoCapitalize: "off",
      // Nor be rewritten into a dictionary word.
      autoCorrect: "off",
      // Nor be underlined as spelling mistakes.
      spellCheck: false,
    }),
  }

  const text = (
    <chakra.label htmlFor={id} fontSize="13px" color="text.muted">
      {label}
      {optional && <i aria-hidden="true"> · optional</i>}
    </chakra.label>
  )

  return (
    <Box display="flex" flexDirection="column" gap="1.5">
      {hideLabel ? <VisuallyHidden>{text}</VisuallyHidden> : text}
      {kind === "prose" ? (
        <chakra.textarea
          {...(rest as ComponentProps<"textarea">)}
          {...forced}
          onKeyDown={onKeyDown as ComponentProps<"textarea">["onKeyDown"]}
        />
      ) : (
        <chakra.input
          {...(rest as ComponentProps<"input">)}
          {...forced}
          // The type gives the search keyboard and clear button, or hides a password.
          type={kind === "search" ? "search" : password ? "password" : "text"}
          // The phone keyboard's Enter key says what it does.
          enterKeyHint={kind === "search" ? "search" : undefined}
          onKeyDown={(e: KeyboardEvent<HTMLInputElement>) => {
            ;(onKeyDown as ComponentProps<"input">["onKeyDown"])?.(e)
            // The browser would clear the text; the page decides what Escape does.
            if (kind === "search" && e.key === "Escape") e.preventDefault()
          }}
        />
      )}
      {hint && (
        <Text id={hintId} fontSize="xs" color="text.dim">
          {hint}
        </Text>
      )}
    </Box>
  )
}
