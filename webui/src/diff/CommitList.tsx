import { Box, chakra } from "@chakra-ui/react"
import { useState } from "react"
import type { DiffCommit } from "../generated/server"
import type { CommitGroup } from "./commitGroups"

export const TRUNCATED_TEXT = "Showing the first 500 commits"
export const ALL_COMMITS = "All commits"

const sameShas = (a: string[], b: string[]) =>
  a.length === b.length && a.every((s) => b.includes(s))

/** What the narrowing reads as: its group's label, a single commit's subject, or a count. */
export function narrowingLabel(
  groups: CommitGroup[],
  narrowing: string[] | undefined,
): string | null {
  if (!narrowing?.length) return null
  const group = groups.find((g) =>
    sameShas(
      g.commits.map((c) => c.sha),
      narrowing,
    ),
  )
  if (group) return group.label
  if (narrowing.length === 1) {
    const commit = groups.flatMap((g) => g.commits).find((c) => c.sha === narrowing[0])
    if (commit) return commit.subject
  }
  return `${narrowing.length} ${narrowing.length === 1 ? "commit" : "commits"}`
}

const rowProps = {
  display: "flex",
  alignItems: "center",
  gap: "2",
  w: "100%",
  minH: "28px",
  px: "2",
  fontSize: "sm",
  textAlign: "left",
  borderRadius: "4px",
} as const

function Row({
  active,
  onClick,
  indent = false,
  children,
}: {
  active: boolean
  onClick: () => void
  indent?: boolean
  children: React.ReactNode
}) {
  return (
    <chakra.button
      type="button"
      aria-current={active || undefined}
      onClick={onClick}
      {...rowProps}
      pl={indent ? "6" : "2"}
      bg={active ? "tone.running.bg" : "transparent"}
      color={active ? "text" : "text.muted"}
      _hover={{ color: "text" }}
    >
      {children}
    </chakra.button>
  )
}

function CommitRow({
  commit,
  active,
  onClick,
}: {
  commit: DiffCommit
  active: boolean
  onClick: () => void
}) {
  return (
    <Row indent active={active} onClick={onClick}>
      <Box as="span" aria-hidden="true" fontFamily="mono" fontSize="xs" color="text.dim">
        {commit.sha.slice(0, 9)}
      </Box>
      <Box
        as="span"
        flex="1"
        minW="0"
        overflow="hidden"
        textOverflow="ellipsis"
        whiteSpace="nowrap"
      >
        {commit.subject}
      </Box>
      {commit.merge && (
        <Box
          as="span"
          fontSize="xs"
          color="text.muted"
          border="1px solid"
          borderColor="border"
          px="1"
        >
          merge
        </Box>
      )}
    </Row>
  )
}

/**
 * The disclosure above the file filter. `narrowing` is the SHAs the view is narrowed to; picking a
 * group or a commit reports its SHAs, "All commits" reports undefined.
 */
export function CommitList({
  groups,
  count,
  truncated,
  narrowing,
  onNarrow,
}: {
  groups: CommitGroup[]
  count: number
  truncated: boolean
  narrowing: string[] | undefined
  onNarrow: (shas: string[] | undefined) => void
}) {
  const [open, setOpen] = useState(false)
  const label = narrowingLabel(groups, narrowing)
  const isNarrowed = (shas: string[]) => !!narrowing && sameShas(shas, narrowing)
  return (
    <Box
      data-testid="commits-disclosure"
      mb="3"
      border="1px solid"
      borderColor="border"
      borderRadius="8px"
      bg="surface.raised"
    >
      <chakra.button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        display="flex"
        alignItems="center"
        gap="2"
        w="100%"
        minH="36px"
        px="3"
        fontSize="xs"
        letterSpacing="0.04em"
        color="text.muted"
      >
        <Box as="span" aria-hidden="true">
          {open ? "▾" : "▸"}
        </Box>
        <Box as="span">{`COMMITS BY TICKET (${count})`}</Box>
        <Box
          as="span"
          data-testid="commits-narrowing"
          flex="1"
          textAlign="right"
          color="tone.running.fg"
        >
          {label ?? ALL_COMMITS}
        </Box>
      </chakra.button>
      {open && (
        <Box px="1" pb="2">
          {truncated && (
            <Box
              data-testid="commits-truncated"
              px="2"
              py="1"
              fontSize="xs"
              color="tone.waiting.fg"
            >
              {TRUNCATED_TEXT}
            </Box>
          )}
          <Row active={!narrowing} onClick={() => onNarrow(undefined)}>
            <Box as="span" flex="1">
              {ALL_COMMITS}
            </Box>
            <Box as="span" fontSize="xs">
              {count}
            </Box>
          </Row>
          {groups.map((g) => {
            const shas = g.commits.map((c) => c.sha)
            return (
              <Box key={g.key}>
                <Row active={isNarrowed(shas)} onClick={() => onNarrow(shas)}>
                  <Box
                    as="span"
                    flex="1"
                    minW="0"
                    fontWeight="600"
                    color={g.kind === "design" ? "tone.design.fg" : "text"}
                  >
                    {g.label}
                  </Box>
                  <Box as="span" fontSize="xs">
                    {g.commits.length}
                  </Box>
                </Row>
                {g.commits.map((c) => (
                  <CommitRow
                    key={c.sha}
                    commit={c}
                    active={isNarrowed([c.sha]) && !isNarrowed(shas)}
                    onClick={() => onNarrow([c.sha])}
                  />
                ))}
              </Box>
            )
          })}
        </Box>
      )}
    </Box>
  )
}

/** The chip row under the ref fields. */
export function NarrowChip({ label, onClear }: { label: string; onClear: () => void }) {
  return (
    <Box data-testid="narrow-chip" mt="3" display="flex" alignItems="center" gap="2" fontSize="sm">
      <Box
        as="span"
        display="inline-flex"
        alignItems="center"
        gap="2"
        pl="3"
        pr="1"
        h="28px"
        border="1px solid"
        borderColor="border"
        borderRadius="full"
        color="text"
      >
        {`Showing: ${label}`}
        <chakra.button
          type="button"
          aria-label="Clear narrowing"
          onClick={onClear}
          w="22px"
          h="22px"
          borderRadius="full"
          color="text.muted"
        >
          ×
        </chakra.button>
      </Box>
    </Box>
  )
}
