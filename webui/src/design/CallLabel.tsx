import { Box, Text } from "@chakra-ui/react"

/**
 * A current call's label. `path` is the end of `label` that names a file: when the label does not fit,
 * the tool and the file name stay and the path is cut from its start. A label with no path is plain
 * text, cut at its end, or wrapped with `wrap`.
 */
export function CallLabel({ label, path, wrap }: { label: string; path?: string; wrap?: boolean }) {
  if (!path) {
    return (
      <Text
        as="span"
        {...(wrap ? { overflowWrap: "anywhere" } : { minW: "0", truncate: true as const })}
      >
        {label}
      </Text>
    )
  }
  const slash = Math.max(path.lastIndexOf("/"), 0)
  return (
    // A grid, not a flex row: its smallest width leaves the fr column out, so a parent that shrinks it
    // stops at the tool and the file name.
    <Box as="span" display="grid" gridTemplateColumns="auto minmax(0, 1fr) auto" whiteSpace="pre">
      <span data-part="call-tool">{label.slice(0, -path.length)}</span>
      {/* Right-to-left puts the ellipsis at the start; the isolated child keeps the path's own order. */}
      <Text as="span" data-part="call-dir" dir="rtl" minW="0" truncate>
        <bdi dir="ltr">{path.slice(0, slash)}</bdi>
      </Text>
      <Box
        as="span"
        data-part="call-file"
        {...(wrap && { whiteSpace: "pre-wrap", overflowWrap: "anywhere" })}
      >
        {path.slice(slash)}
      </Box>
    </Box>
  )
}
