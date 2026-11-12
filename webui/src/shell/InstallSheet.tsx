import { Box, Button, Heading, Text } from "@chakra-ui/react"
import type { ReactNode } from "react"
import type { InstallMode } from "../pwa/install"
import { BottomSheet } from "../ui/modal/BottomSheet"

type SheetMode = Exclude<InstallMode, "prompt" | "none">

const COPY: Record<SheetMode, { intro: string; steps: ReactNode[] }> = {
  ios: {
    intro: "Safari has no install button. Add gx from the Share menu.",
    steps: [
      <>
        Tap{" "}
        <Box as="span" aria-hidden borderWidth="1px" borderColor="border" borderRadius="sm" px="1">
          ⬆
        </Box>{" "}
        Share in Safari's toolbar
      </>,
      <>
        Scroll down and tap <strong>Add to Home Screen</strong>
      </>,
      <>
        Tap <strong>Add</strong>. Open gx from the new icon.
      </>,
    ],
  },
  "mac-safari": {
    intro: "Safari has no install button. Add gx from the File menu.",
    steps: [
      <>
        Choose <strong>File</strong> in the menu bar
      </>,
      <>
        Choose <strong>Add to Dock…</strong>
      </>,
      <>
        Click <strong>Add</strong>. Open gx from the Dock.
      </>,
    ],
  },
}

/** The manual install steps for a browser with no install prompt. */
export function InstallSheet({ mode, onClose }: { mode: SheetMode; onClose: () => void }) {
  const { intro, steps } = COPY[mode]
  return (
    <BottomSheet
      label="Install gx"
      onClose={onClose}
      p="4"
      display="flex"
      flexDirection="column"
      gap="2"
      desktop={{ maxW: "420px", p: "4", display: "flex", flexDirection: "column", gap: "2" }}
    >
      <Heading as="h3" fontSize="md">
        Install gx
      </Heading>
      <Text color="text.dim" fontSize="sm">
        {intro}
      </Text>
      <Box as="ol" listStyleType="none">
        {steps.map((step, i) => (
          <Box
            as="li"
            key={i}
            display="flex"
            gap="3"
            alignItems="center"
            py="2.5"
            borderTopWidth="1px"
            borderColor="border"
          >
            <Box
              aria-hidden
              w="26px"
              h="26px"
              borderRadius="full"
              bg="tone.running.bg"
              color="tone.running.fg"
              display="grid"
              placeItems="center"
              fontSize="sm"
              flexShrink="0"
            >
              {i + 1}
            </Box>
            <span>{step}</span>
          </Box>
        ))}
      </Box>
      <Button variant="outline" w="full" mt="2" onClick={onClose}>
        Done
      </Button>
    </BottomSheet>
  )
}
