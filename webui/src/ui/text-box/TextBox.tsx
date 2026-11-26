import { Box, chakra, Text, VisuallyHidden, type BoxProps } from "@chakra-ui/react"
import {
  useId,
  type AriaAttributes,
  type ComponentProps,
  type DOMAttributes,
  type InputHTMLAttributes,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
  type TextareaHTMLAttributes,
} from "react"
import { textBoxLook } from "./look"

type NamingAria = "aria-label" | "aria-labelledby"

type PassedKey = "value" | "defaultValue" | "placeholder" | "disabled" | "required" | "autoFocus"

/** Place the whole component: label, box and hint. */
const layoutKeys = [
  "w",
  "width",
  "minW",
  "minWidth",
  "maxW",
  "maxWidth",
  "m",
  "mx",
  "my",
  "mt",
  "mb",
  "ml",
  "mr",
  "ms",
  "me",
  "margin",
  "marginX",
  "marginY",
  "marginTop",
  "marginBottom",
  "marginLeft",
  "marginRight",
  "marginStart",
  "marginEnd",
  "marginInline",
  "marginInlineStart",
  "marginInlineEnd",
  "marginBlock",
  "marginBlockStart",
  "marginBlockEnd",
  "flex",
  "flexGrow",
  "flexShrink",
  "flexBasis",
  "gridArea",
  "gridColumn",
  "gridColumnStart",
  "gridColumnEnd",
  "gridRow",
  "gridRowStart",
  "gridRowEnd",
  "alignSelf",
  "justifySelf",
  "order",
] as const

/** Apply to the textarea itself. */
const textareaKeys = ["minH", "minHeight", "maxH", "maxHeight", "resize"] as const

type LayoutProps = Pick<BoxProps, (typeof layoutKeys)[number]>
type TextareaLayoutProps = Pick<BoxProps, (typeof textareaKeys)[number]>

type Look = "boxed" | "bare"
type Size = "sm" | "md" | "lg"

type Sized = {
  /** `boxed` is the default. */
  look?: Look
  /** `md` is the default. On a textarea the height comes from `rows` or the layout. */
  size?: Size
}

type CommonProps<E> = LayoutProps & {
  label: string
  hideLabel?: boolean
  optional?: boolean
  hint?: string
  invalid?: boolean
  /** Monospace, for paths, refs and commands. */
  mono?: boolean
  /** Drawn inside the box's end edge, with room reserved. Only a button in it takes clicks. */
  endElement?: ReactNode
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
  Pick<InputHTMLAttributes<HTMLInputElement>, PassedKey | "minLength" | "list"> &
  Partial<Record<(typeof textareaKeys)[number] | "page", never>>

type ProseProps = CommonProps<HTMLTextAreaElement> &
  TextareaLayoutProps &
  Pick<TextareaHTMLAttributes<HTMLTextAreaElement>, PassedKey | "minLength" | "rows"> &
  (
    | (Sized & { page?: never })
    // The roomy spacing of a page that is one big textarea.
    | { look: "bare"; page: boolean; size?: "md" }
  ) & {
    kind: "prose"
    list?: string
    autoComplete?: never
    inputMode?: never
    verbatim?: never
    secret?: never
  }

type LineProps = InputProps &
  // `title` is the large page title.
  (Sized | { look: "bare"; size: "title" }) &
  Pick<InputHTMLAttributes<HTMLInputElement>, "inputMode"> & {
    kind: "line"
    /** Keeps the text exactly as typed: no typing help. */
    verbatim?: boolean
    /** Hides the text. Also keeps it as typed. */
    secret?: boolean
    autoComplete?: never
  }

type SearchProps = InputProps &
  Sized & {
    kind: "search"
    autoComplete?: never
    inputMode?: never
    verbatim?: never
    secret?: never
  }

type CredentialProps = InputProps &
  Sized &
  Pick<InputHTMLAttributes<HTMLInputElement>, "inputMode"> & {
    kind: "credential"
    autoComplete: "username" | "current-password" | "new-password" | "one-time-code"
    verbatim?: never
    secret?: never
  }

export type TextBoxProps = ProseProps | LineProps | SearchProps | CredentialProps

const looks = {
  boxed: textBoxLook,
  bare: { ...textBoxLook, bg: "transparent", borderWidth: "0" },
} as const

const heights: Record<Size, string> = { sm: "32px", md: "40px", lg: "44px" }

const titleStyle = {
  h: "36px",
  px: "1",
  fontSize: "20px",
  fontWeight: "bold",
  textOverflow: "ellipsis",
  // A background instead of a ring, and the whole text while editing.
  _focus: { bg: "surface.raised", textOverflow: "clip" },
} as const

const textareaPadding = {
  boxed: { px: "3", py: "2", lineHeight: "1.5" },
  bare: { px: "3", py: "2.5", lineHeight: "1.5" },
  page: { px: "4", py: "3", lineHeight: "1.6" },
} as const

// A 36px button and its gap from the box's edge.
const endRoom = "46px"

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
    look = "boxed",
    size = "md",
    page,
    mono,
    endElement,
    ...others
  } = props
  const asTyped = kind === "search" || kind === "credential" || verbatim || secret
  const password = secret || autoComplete === "current-password" || autoComplete === "new-password"
  const prose = kind === "prose"
  const title = size === "title"

  const rest: Record<string, unknown> = { ...others }
  const take = (keys: readonly string[]) =>
    Object.fromEntries(
      keys.flatMap((key) => {
        if (!(key in rest)) return []
        const value = rest[key]
        delete rest[key]
        return [[key, value]]
      }),
    )
  const layout: LayoutProps = take(layoutKeys)
  const areaLayout: TextareaLayoutProps = take(textareaKeys)
  // A flex parent may only shrink the textarea if the wrappers around it may shrink too.
  const shrinks = [areaLayout.minH, areaLayout.minHeight].some((v) => v === "0" || v === 0)
  const wrapMinH = shrinks ? "0" : undefined

  const forced = {
    w: "100%",
    ...looks[look],
    ...(prose
      ? {
          ...textareaPadding[page ? "page" : look],
          // Fills a parent that grows; `rows` still sets the starting height.
          flex: "1 1 auto",
          ...areaLayout,
        }
      : title
        ? titleStyle
        : { h: heights[size], px: "3", lineHeight: "1.5" }),
    ...(endElement !== undefined && { pe: endRoom }),
    ...(mono && { fontFamily: "mono" }),
    ...(invalid && { borderWidth: "1px", borderColor: "tone.deadlocked.fg" }),
    // theme.ts draws the ring: a title shows a background, and a bare prose box sits in a frame
    // that draws its own.
    "data-text-box-ring": title || (prose && look === "bare") ? undefined : "",
    // Opts out of the coarse-pointer font size in theme.ts.
    "data-text-box-title": title ? "" : undefined,
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
    <Box display="flex" flexDirection="column" gap="1.5" minH={wrapMinH} {...layout}>
      {hideLabel ? <VisuallyHidden>{text}</VisuallyHidden> : text}
      <Box
        position="relative"
        display="flex"
        flexDirection="column"
        flex="1 1 auto"
        minH={wrapMinH}
      >
        {prose ? (
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
        {endElement !== undefined && (
          <Box
            position="absolute"
            insetEnd="1.5"
            // A textarea holds it at the top; an input centers it.
            top={prose ? "1.5" : "0"}
            bottom={prose ? undefined : "0"}
            minW="36px"
            display="flex"
            alignItems="center"
            justifyContent="center"
            color="text.dim"
            // A click on a decoration reaches the box under it.
            pointerEvents="none"
            css={{ "& button": { pointerEvents: "auto" } }}
          >
            {endElement}
          </Box>
        )}
      </Box>
      {hint && (
        <Text id={hintId} fontSize="xs" color="text.dim">
          {hint}
        </Text>
      )}
    </Box>
  )
}
