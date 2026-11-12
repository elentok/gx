import { Box, chakra, Text } from "@chakra-ui/react"
import type { ReactNode } from "react"
import { SETTINGS_HREF } from "../hrefs"

export const Group = ({ label, children }: { label?: string; children: ReactNode }) => (
  <Box mt="6">
    {label && (
      <Text
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
  <chakra.input
    data-mock-part="search"
    type="text"
    aria-label="Search settings"
    placeholder="Search all settings"
    value={value}
    onChange={(e) => onChange(e.target.value)}
    mt="2"
    w="100%"
    h="40px"
    px="3.5"
    borderWidth="1px"
    borderColor="border"
    borderRadius="10px"
    bg="surface.raised"
    color="text"
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
