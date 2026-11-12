import { Box, chakra, Text } from "@chakra-ui/react"
import type { Banner, DraftAutosave } from "./useDraftAutosave"

/** The refusal banner: what happened to the draft on another device and the ways out. */
export function DraftBanner({
  kind,
  actions,
}: {
  kind: NonNullable<Banner>
  actions: DraftAutosave["bannerActions"]
}) {
  const buttons =
    kind === "conflict"
      ? [
          { text: "Load theirs", onClick: actions.loadTheirs },
          { text: "Keep mine", onClick: actions.keepMine },
        ]
      : [{ text: "Keep as new draft", onClick: actions.keepAsNew }]
  return (
    <Box
      role="alert"
      display="flex"
      alignItems="center"
      flexWrap="wrap"
      gap="2"
      mx="3"
      mt="3"
      px="3"
      py="10px"
      borderRadius="8px"
      bg="tone.waiting.bg"
      color="tone.waiting.fg"
      fontSize="13px"
      flexShrink="0"
    >
      <Text flex="1" minW="0">
        {kind === "conflict"
          ? "This draft changed on another device."
          : "This draft was deleted or started on another device."}
      </Text>
      {buttons.map(({ text, onClick }) => (
        <chakra.button
          key={text}
          type="button"
          onClick={onClick}
          h="30px"
          px="10px"
          borderRadius="6px"
          borderWidth="1px"
          borderColor="currentColor"
          cursor="pointer"
        >
          {text}
        </chakra.button>
      ))}
    </Box>
  )
}
