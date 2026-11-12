import { Box } from "@chakra-ui/react"
import { useMemo } from "react"
import { useIsTopLayer } from "../ui/modal/layers"
import { chordTable, displayKey, type Chord, type ChordContext } from "./chords"

const realTable = () => chordTable(() => {})

/**
 * The Chord box: lists the chords available in `ctx`, under the Leader key chip. It has no key
 * handling and no timer; whoever mounts it decides when it opens and closes. The default table's
 * Remove chord does nothing, so a caller that can remove passes its own table.
 */
export function ChordBox({
  leader,
  ctx,
  chords,
  onClose,
}: {
  leader: string | undefined
  ctx: ChordContext
  chords?: Chord[]
  onClose: () => void
}) {
  // Joins the layer stack while mounted so Esc handlers elsewhere defer to it.
  useIsTopLayer()
  const table = useMemo(() => chords ?? realTable(), [chords])
  const rows = table.filter((c) => c.available(ctx))

  return (
    <Box
      role="menu"
      aria-label="Chords"
      position="fixed"
      right="3"
      bottom="3"
      zIndex="popover"
      display="grid"
      gap="2px"
      minW="190px"
      p="2"
      bg="surface.raised"
      borderWidth="1px"
      borderColor="hairline"
      borderRadius="14px"
      boxShadow="0 10px 30px rgba(0, 0, 0, 0.55)"
      fontFamily="text"
      fontSize="15px"
      lineHeight="1.45"
      color="text.muted"
    >
      <Box
        as="span"
        justifySelf="start"
        ml="1"
        mb="1"
        px="10px"
        py="2px"
        borderRadius="full"
        fontFamily="condensed"
        lineHeight="normal"
        fontSize="12px"
        fontWeight="600"
        letterSpacing="0.1em"
        color="tone.running.fg"
        bg="tone.running.bg"
      >
        {displayKey(leader)}
      </Box>
      {rows.map((c) => (
        <Box
          key={c.key}
          role="menuitem"
          tabIndex={-1}
          display="flex"
          gap="10px"
          alignItems="baseline"
          px="10px"
          py="1"
          borderRadius="8px"
          cursor="pointer"
          _hover={{ bg: "sidebar.hover" }}
          onClick={() => {
            c.run()
            onClose()
          }}
        >
          <Box
            as="b"
            w="1ch"
            fontFamily="condensed"
            lineHeight="normal"
            fontSize="16px"
            fontWeight="700"
            color={c.danger ? "tone.paused.fg" : "text"}
          >
            {c.key}
          </Box>
          {c.label(ctx)}
        </Box>
      ))}
      <Box px="10px" pt="1" pb="2px" fontSize="12px" color="text.faint" textAlign="right">
        esc closes
      </Box>
    </Box>
  )
}
