import { Box, chakra, Text } from "@chakra-ui/react"
import type { ReactNode } from "react"
import { SETTINGS_HREF } from "../hrefs"
import { StateHeader } from "../page/StateHeader"
import { Spinner } from "../ui/spinner/Spinner"
import { TextBox } from "../ui/text-box/TextBox"

/** A settings page still loading: the page's own header over the spinner. */
export function LoadingState({ title }: { title: string }) {
  return (
    <>
      <StateHeader title={title} />
      <Spinner label="Loading" />
    </>
  )
}

export const Chevron = () => (
  <Text aria-hidden color="text.dim">
    ›
  </Text>
)

export const CountChip = ({ n, testId }: { n: number; testId?: string }) => (
  <chakra.span
    data-testid={testId}
    fontSize="11px"
    fontWeight="600"
    px="7px"
    py="1px"
    borderRadius="full"
    bg="tone.running.bg"
    color="tone.running.fg"
  >
    {n}
  </chakra.span>
)

/** The text with each case-insensitive occurrence of the query wrapped in a <mark>. A blank query leaves it plain. */
export function Highlight({ text, query }: { text: string; query?: string }) {
  const q = query?.trim().toLowerCase() ?? ""
  if (!q) return <>{text}</>
  const parts: ReactNode[] = []
  const lower = text.toLowerCase()
  let from = 0
  for (let at = lower.indexOf(q); at >= 0; at = lower.indexOf(q, from)) {
    if (at > from) parts.push(text.slice(from, at))
    parts.push(
      <chakra.mark key={at} bg="tone.waiting.bg" color="inherit" borderRadius="2px">
        {text.slice(at, at + q.length)}
      </chakra.mark>,
    )
    from = at + q.length
  }
  parts.push(text.slice(from))
  return <>{parts}</>
}

/** A section page block: a title bar with the changed-count chip (hidden at zero), then its rows. */
export const GroupBlock = ({
  title,
  changed,
  dimmed,
  part,
  children,
}: {
  title: string
  changed: number
  /** The block's body is faded, still editable. A channel whose switch is off. */
  dimmed?: boolean
  part?: string
  children: ReactNode
}) => (
  <Box
    mt="6"
    borderWidth="1px"
    borderColor="border"
    borderRadius="lg"
    bg="surface.raised"
    overflow="hidden"
    data-testid={`group-${title}`}
    data-mock-part={part}
    data-dimmed={dimmed ? "true" : undefined}
  >
    <Box
      display="flex"
      alignItems="center"
      justifyContent="space-between"
      gap="2"
      px="3.5"
      py="2"
      bg="surface"
    >
      <Text
        fontSize="xs"
        letterSpacing="0.06em"
        textTransform="uppercase"
        color="text.muted"
        fontWeight="600"
      >
        {title}
      </Text>
      {changed > 0 && <CountChip n={changed} testId={`group-count-${title}`} />}
    </Box>
    {dimmed ? <Box opacity={0.5}>{children}</Box> : children}
  </Box>
)

export const Group = ({
  label,
  part,
  children,
}: {
  label?: string
  part?: string
  children: ReactNode
}) => (
  <Box mt="6">
    {label && (
      <Text
        as="h2"
        fontSize="xs"
        letterSpacing="0.08em"
        textTransform="uppercase"
        color="text.muted"
        mb="2"
        px="3"
      >
        {label}
      </Text>
    )}
    <Box
      data-mock-part={part}
      borderWidth="1px"
      borderColor="border"
      borderRadius="lg"
      bg="surface.raised"
      overflow="hidden"
    >
      {children}
    </Box>
  </Box>
)

export const rowProps = {
  display: "flex",
  alignItems: "center",
  gap: "3",
  px: "4",
  py: "3",
  minH: "52px",
  color: "text",
  textAlign: "left",
  width: "100%",
  cursor: "pointer",
  bg: "transparent",
  _notFirst: { borderTopWidth: "1px", borderTopColor: "hairline" },
  _hover: { bg: "surface" },
} as const

/** The look of a settings text field; callers add width and border colour. */
export const inputProps = {
  h: "36px",
  px: "3",
  minW: "0",
  borderWidth: "1px",
  borderRadius: "8px",
  bg: "surface",
  color: "text",
} as const

export const SearchBox = ({
  value,
  onChange,
}: {
  value: string
  onChange: (q: string) => void
}) => (
  <TextBox
    kind="search"
    name="settings-search"
    data-mock-part="search"
    label="Search settings"
    hideLabel
    placeholder="Search all settings"
    value={value}
    onChange={(e) => onChange(e.currentTarget.value)}
    mt="2"
    w="100%"
  />
)

/** A list item as a chip with a remove button. */
export const TagChip = ({
  label,
  disabled,
  onRemove,
}: {
  label: string
  disabled?: boolean
  onRemove: () => void
}) => (
  <Box
    display="inline-flex"
    alignItems="center"
    gap="1.5"
    px="2.5"
    py="0.5"
    borderWidth="1px"
    borderColor="border"
    borderRadius="full"
    fontFamily="mono"
    fontSize="12.5px"
  >
    {label}
    <chakra.button
      type="button"
      aria-label={`Remove ${label}`}
      disabled={disabled}
      onClick={onRemove}
      color="text.dim"
    >
      ×
    </chakra.button>
  </Box>
)

/** One-of-several buttons; the chosen one reads as pressed. */
export function Segmented<T extends string>({
  label,
  options,
  value,
  disabled,
  onChange,
}: {
  label: string
  options: { value: T; label: string }[]
  value: T | string
  disabled?: boolean
  onChange: (value: T) => void
}) {
  return (
    <Box
      display="inline-flex"
      borderWidth="1px"
      borderColor="border"
      borderRadius="8px"
      overflow="hidden"
      role="group"
      aria-label={label}
    >
      {options.map((o) => (
        <chakra.button
          key={o.value}
          type="button"
          aria-pressed={o.value === value}
          disabled={disabled}
          onClick={() => onChange(o.value)}
          px="3"
          py="1.5"
          fontWeight="600"
          color="text.muted"
          _notFirst={{ borderLeftWidth: "1px", borderLeftColor: "border" }}
          _pressed={{ bg: "tone.running.bg", color: "tone.running.fg" }}
        >
          {o.label}
        </chakra.button>
      ))}
    </Box>
  )
}

export const ChangedDot = () => (
  <Box
    title="Changed from default"
    w="7px"
    h="7px"
    flexShrink="0"
    borderRadius="full"
    bg="tone.running.fg"
  />
)

/** The "Not saved: why" alert, or the "Saved" note once a write went through. */
export const SaveStatus = ({ errorText, saved }: { errorText?: string; saved: boolean }) => (
  <>
    {errorText && (
      <Text data-mock-part="error-message" role="alert" fontSize="xs" color="tone.deadlocked.fg">
        {errorText}
      </Text>
    )}
    {saved && !errorText && (
      <Text data-mock-part="saved-message" role="status" fontSize="xs" color="tone.done.fg">
        <span aria-hidden>✓ </span>Saved
      </Text>
    )}
  </>
)

export const Frame = ({ children }: { children: ReactNode }) => <Box maxW="560px">{children}</Box>

export const BackLink = ({
  href = SETTINGS_HREF,
  label = "Settings",
}: {
  href?: string
  label?: string
}) => (
  <chakra.a
    href={href}
    display="inline-flex"
    alignItems="center"
    h="8"
    px="3"
    fontSize="sm"
    fontWeight="600"
    color="text.muted"
    _hover={{ color: "text" }}
  >
    ‹ {label}
  </chakra.a>
)
