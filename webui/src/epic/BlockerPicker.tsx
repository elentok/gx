import { Box, chakra, Text } from "@chakra-ui/react"
import { useState } from "react"
import { ReasonBlockedByCycle, type EpicBlockerResult } from "../generated/server"
import { slugOf } from "../slug"
import { TextBox } from "../ui/text-box/TextBox"
import { refusalText, useWrite } from "../useWrite"
import { candidates, cycleSentence } from "./blockers"
import { postBlockerAdd, postBlockerRemove } from "./blockerWrite"
import type { EpicList } from "../epics/list"

/**
 * The inline panel under the Blocked by row: one row per loop epic of the project, a click toggles
 * it. Each click sends the one add or remove, never the list, so the server keeps the entries the
 * page does not show. A refusal or failure stays in the open panel.
 */
export function BlockerPicker({
  list,
  address,
  ticked,
  refetch,
}: {
  list: EpicList | null
  address: string
  /** Addresses of the current blockers. */
  ticked: Set<string>
  refetch: () => void
}) {
  const [query, setQuery] = useState("")
  const { run, refusal, error } = useWrite<EpicBlockerResult>({ toastRefusal: false })
  const q = query.trim().toLowerCase()
  const rows = candidates(list, address).filter((c) => c.name.toLowerCase().includes(q))
  const cycle = refusal?.reason === ReasonBlockedByCycle && refusal.blocker && refusal.chain?.length
  const failure = cycle ? cycleSentence(refusal) : refusal ? refusalText(refusal) : error
  return (
    <Box
      data-testid="blocker-picker"
      bg="surface.raised"
      borderWidth="1px"
      borderColor="border"
      borderRadius="lg"
      p="2"
      maxW="380px"
    >
      <TextBox
        kind="search"
        name="blocker-search"
        label="Search epics"
        hideLabel
        placeholder="Search epics"
        mb="2"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") setQuery("")
        }}
      />
      {rows.map((c) => {
        const on = ticked.has(c.address)
        return (
          <chakra.button
            key={c.address}
            type="button"
            display="flex"
            justifyContent="space-between"
            w="full"
            textAlign="left"
            px="2"
            py="1"
            borderRadius="md"
            cursor="pointer"
            _hover={{ bg: "surface.hover" }}
            onClick={() =>
              void run(
                () =>
                  on ? postBlockerRemove(address, c.address) : postBlockerAdd(address, c.address),
                { onDone: refetch },
              )
            }
          >
            <span>{`${on ? "☑" : "☐"} ${c.name}`}</span>
            <Text as="span" fontSize="13px" color="text.dim">
              {c.state}
            </Text>
          </chakra.button>
        )
      })}
      <Text fontSize="13px" color="text.dim" px="2" py="1">
        {`${slugOf(address)}: this epic`}
      </Text>
      {failure ? (
        <Box
          role="alert"
          mt="2"
          fontSize="14px"
          bg="tone.paused.bg"
          color="tone.paused.text"
          borderRadius="md"
          px="3"
          py="2"
        >
          {failure}
        </Box>
      ) : null}
    </Box>
  )
}
