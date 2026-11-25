import { Box, chakra, Link, Text } from "@chakra-ui/react"
import { useState } from "react"
import type { EpicList } from "../epics/list"
import type { EpicBlocker } from "../generated/server"
import { epicHref } from "../hrefs"
import { Button } from "../ui/button/Button"
import { useWrite } from "../useWrite"
import { BlockerPicker } from "./BlockerPicker"
import { postBlockerRemove } from "./blockerWrite"
import type { EpicDetail } from "./detail"
import { blockerHint } from "./waiting"

/** One blocker as a pill; a refused stored entry is the raw text with no link. */
function Chip({ blocker, onRemove }: { blocker: EpicBlocker; onRemove: () => void }) {
  const label = blocker.invalid ? (blocker.ref ?? "") : (blocker.name ?? "")
  return (
    <Box
      data-testid={blocker.invalid ? "blocker-chip-error" : "blocker-chip"}
      display="inline-flex"
      alignItems="center"
      gap="1"
      borderWidth="1px"
      borderColor="border"
      borderRadius="full"
      fontSize="14px"
      pl="3"
      pr="1"
    >
      {blocker.invalid || !blocker.address ? (
        <Text as="span" color="tone.paused.text">
          {label}
        </Text>
      ) : (
        <Link href={epicHref(blocker.address)} textDecoration="none" color="text">
          {label}
        </Link>
      )}
      <chakra.button
        type="button"
        aria-label={`Remove ${label}`}
        color="text.dim"
        px="2"
        cursor="pointer"
        onClick={onRemove}
      >
        ×
      </chakra.button>
    </Box>
  )
}

/**
 * Under the header: why a Waiting epic does not move, then one chip per unreleased blocker.
 * Remove sends only the one entry, so the server keeps the ones this page does not show.
 */
export function BlockedBy({
  detail,
  address,
  list,
  refetch,
  readOnly,
}: {
  detail: EpicDetail
  address: string
  list: EpicList | null
  refetch: () => void
  readOnly?: boolean
}) {
  const { run } = useWrite()
  const [open, setOpen] = useState(false)
  const blockers = detail.blockers ?? []
  const hint = blockerHint(blockers)
  const chips = !readOnly
  if (!hint && !chips) return null
  return (
    <Box display="flex" flexDirection="column" mt="4" gap="3">
      {hint ? (
        <Text
          data-testid="blocked-by-hint"
          fontSize="14px"
          color={hint.stuck ? "tone.paused.text" : "text.dim"}
        >
          {hint.text}
        </Text>
      ) : null}
      {chips ? (
        <Box display="flex" flexWrap="wrap" alignItems="center" gap="2">
          <Text as="span" fontSize="14px" color="text.dim">
            Blocked by
          </Text>
          {blockers.map((b) => {
            const key = (b.invalid ? b.ref : b.address) ?? ""
            return (
              <Chip
                key={key}
                blocker={b}
                onRemove={() =>
                  void run(() => postBlockerRemove(address, key), { onDone: refetch })
                }
              />
            )
          })}
          <Button tone="muted" emphasis="ghost" size="sm" onClick={() => setOpen(!open)}>
            + Blocker
          </Button>
        </Box>
      ) : null}
      {chips && open ? (
        <BlockerPicker
          list={list}
          address={address}
          ticked={new Set(blockers.map((b) => b.address ?? ""))}
          refetch={refetch}
        />
      ) : null}
    </Box>
  )
}
