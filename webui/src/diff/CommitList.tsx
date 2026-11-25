import { Box, chakra } from "@chakra-ui/react"
import { useState } from "react"
import type { DiffCommit } from "../generated/server"
import type { CommitGroup } from "./commitGroups"
import { UNCOMMITTED } from "./diffUrl"

export const UNCOMMITTED_LABEL = "Uncommitted changes"
export const TRUNCATED_TEXT = "Showing the first 500 commits"
export const ALL_COMMITS = "All commits"

const sameShas = (a: string[], b: string[]) =>
  a.length === b.length && a.every((s) => b.includes(s))

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
  waiting = false,
  children,
}: {
  active: boolean
  onClick: () => void
  indent?: boolean
  waiting?: boolean
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
      color={waiting ? "tone.waiting.fg" : active ? "text" : "text.muted"}
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
  uncommitted = false,
}: {
  groups: CommitGroup[]
  count: number
  truncated: boolean
  /** Worktree mode with changes since HEAD: adds the last row. It is not part of `count` but is in the header's number. */
  uncommitted?: boolean
  narrowing: string[] | undefined
  onNarrow: (shas: string[] | undefined) => void
}) {
  const [open, setOpen] = useState(false)
  const shown = count + (uncommitted ? 1 : 0)
  const isNarrowed = (shas: string[]) => !!narrowing && sameShas(shas, narrowing)
  return (
    <Box data-testid="commits-disclosure" mb="3">
      <chakra.button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        display="flex"
        alignItems="center"
        gap="2"
        w="100%"
        h="34px"
        px="2"
        borderRadius="6px"
        bg="surface.raised"
        fontSize="sm"
        color="text.muted"
      >
        <Box as="span" flex="1" textAlign="left">
          {`${shown} ${shown === 1 ? "commit" : "commits"}`}
        </Box>
        <Box as="span" aria-hidden="true">
          {open ? "▴" : "▾"}
        </Box>
      </chakra.button>
      {open && (
        <Box data-testid="commits-list" p="1" borderRadius="6px" bg="surface.raised">
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
            <Box as="span" flex="1" fontSize="md" fontWeight="700">
              {ALL_COMMITS}
            </Box>
            <Box as="span" fontSize="xs">
              {count}
            </Box>
          </Row>
          {groups.map((g) => {
            const shas = g.commits.map((c) => c.sha)
            return (
              <Box key={g.key} mt="3">
                <Row active={isNarrowed(shas)} onClick={() => onNarrow(shas)}>
                  <Box
                    as="span"
                    flex="1"
                    minW="0"
                    fontSize="md"
                    fontWeight="700"
                    color={g.kind === "design" ? "kind.design.fg" : "text"}
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
          {uncommitted && (
            <Box mt="3">
              <Row
                waiting
                active={isNarrowed([UNCOMMITTED])}
                onClick={() => onNarrow([UNCOMMITTED])}
              >
                <Box
                  as="span"
                  aria-hidden="true"
                  fontFamily="mono"
                  fontSize="xs"
                  color="text.faint"
                >
                  {"·······"}
                </Box>
                <Box as="span" flex="1" minW="0">
                  {UNCOMMITTED_LABEL}
                </Box>
              </Row>
            </Box>
          )}
        </Box>
      )}
    </Box>
  )
}
