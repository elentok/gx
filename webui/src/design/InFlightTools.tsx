import { Box, chakra, Popover, Portal, Text } from "@chakra-ui/react"
import { useState } from "react"
import type { SessionInFlightTool } from "../generated/server"
import { withKeys } from "../uniqueKeys"
import { CallLabel } from "./CallLabel"

const VERBS: Record<string, string> = {
  shell: "running shell: ",
  agent: "waiting for sub-agent: ",
  search: "searching: ",
}

const KIND_WORDS: Record<string, string> = {
  shell: "shell",
  agent: "sub-agent",
  search: "search",
  other: "tool",
}

/** The Working row's tool in flight: its wording, then a "(+N more)" button that lists the older calls. */
export function InFlightTools({
  current,
  others = [],
  plainAgent,
}: {
  current: SessionInFlightTool
  others?: SessionInFlightTool[]
  /** An agent call reads its label alone. */
  plainAgent?: boolean
}) {
  const [open, setOpen] = useState(false)
  const verb = plainAgent && current.kind === "agent" ? "" : (VERBS[current.kind] ?? "")
  return (
    // With a path, no minW 0: the call shrinks before the text in front of it, down to its tool and file name.
    <Box
      as="span"
      display="flex"
      gap="1"
      {...(current.path ? { flexShrink: 1000 } : { minW: "0" })}
    >
      <CallLabel label={`${verb}${current.label}`} path={current.path} />
      {others.length > 0 && (
        <>
          {" "}
          <Popover.Root
            open={open}
            onOpenChange={(e) => setOpen(e.open)}
            positioning={{ placement: "top-start", gutter: 8 }}
            autoFocus={false}
            lazyMount
            unmountOnExit
          >
            <Popover.Anchor asChild>
              <chakra.button
                type="button"
                color="text.dim"
                flexShrink="0"
                whiteSpace="nowrap"
                textDecoration="underline dotted"
                cursor="pointer"
                onClick={() => setOpen(true)}
                onMouseEnter={() => setOpen(true)}
                onMouseLeave={() => setOpen(false)}
                onFocus={() => setOpen(true)}
                onBlur={() => setOpen(false)}
                onKeyDown={(e: React.KeyboardEvent) => e.key === "Escape" && setOpen(false)}
              >
                {`(+${others.length} more)`}
              </chakra.button>
            </Popover.Anchor>
            <Portal>
              <Popover.Positioner>
                <Popover.Content
                  aria-label="Other tools in flight"
                  bg="surface.raised"
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="12px"
                  boxShadow="lg"
                  color="text"
                  w="min(360px, calc(100vw - 32px))"
                  px="3"
                  py="2"
                >
                  <Box as="ul" listStyleType="none" m="0" p="0" fontSize="sm">
                    {withKeys(others, (o) => `${o.kind}|${o.label}`).map(({ key, item: t }) => (
                      <Box as="li" key={key} display="flex" gap="2">
                        <Text as="span" color="text.dim">
                          {KIND_WORDS[t.kind] ?? t.kind}
                        </Text>
                        <CallLabel label={t.label} path={t.path} wrap />
                      </Box>
                    ))}
                  </Box>
                </Popover.Content>
              </Popover.Positioner>
            </Portal>
          </Popover.Root>
        </>
      )}
    </Box>
  )
}
