import { Box, chakra } from "@chakra-ui/react"
import { useMemo, useState } from "react"
import type { DiffFile } from "../generated/server"
import { buildTree, filterFiles } from "./fileTree"

const STATUS_TONE: Record<string, string> = {
  A: "tone.done.fg",
  D: "tone.deadlocked.fg",
  R: "tone.waiting.fg",
}

function ChangeBar({ added, deleted }: { added: number; deleted: number }) {
  const total = added + deleted
  const addPct = total === 0 ? 0 : Math.round((added / total) * 100)
  return (
    <Box
      data-testid="change-bar"
      aria-label={`+${added} −${deleted}`}
      role="img"
      display="flex"
      w="48px"
      h="6px"
      flexShrink={0}
      borderRadius="3px"
      overflow="hidden"
      bg="border"
    >
      <Box data-part="added" w={`${addPct}%`} bg="tone.done.fg" />
      <Box data-part="deleted" w={`${total === 0 ? 0 : 100 - addPct}%`} bg="tone.deadlocked.fg" />
    </Box>
  )
}

export function FileFilter({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <chakra.input
      data-testid="file-filter"
      aria-label="Filter files"
      placeholder="Filter files"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      spellCheck={false}
      autoCapitalize="off"
      autoCorrect="off"
      w="100%"
      h="40px"
      px="3"
      fontSize="sm"
      fontFamily="mono"
      bg="surface.raised"
      color="text"
      border="1px solid"
      borderColor="border"
      borderRadius="8px"
      _focusVisible={{ outline: "none", borderColor: "text.dim" }}
    />
  )
}

const rowProps = {
  display: "flex",
  alignItems: "center",
  gap: "2",
  w: "100%",
  minH: "32px",
  pr: "2",
  fontSize: "sm",
  fontFamily: "mono",
  textAlign: "left",
} as const

export function FileTree({
  files,
  filter,
  selected,
  onSelect,
  dirCounts = true,
}: {
  dirCounts?: boolean
  files: DiffFile[]
  filter: string
  selected: string | undefined
  onSelect: (path: string) => void
}) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const matches = useMemo(() => filterFiles(files, filter), [files, filter])
  const rows = useMemo(() => buildTree(matches, collapsed), [matches, collapsed])

  const toggle = (id: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (!next.delete(id)) next.add(id)
      return next
    })

  if (matches.length === 0) {
    return (
      <Box data-testid="file-tree" mt="2" px="2" py="3" fontSize="sm" color="text.muted">
        {`No files match "${filter}".`}
      </Box>
    )
  }
  return (
    <Box data-testid="file-tree" mt="2" role="tree" aria-label="Changed files">
      {rows.map((row) => {
        const pl = `${row.depth * 16 + 8}px`
        if (row.kind === "dir") {
          return (
            <chakra.button
              key={`d:${row.id}`}
              type="button"
              role="treeitem"
              aria-expanded={row.expanded}
              data-testid="tree-dir"
              onClick={() => toggle(row.id)}
              {...rowProps}
              pl={pl}
              color="text.muted"
            >
              <Box as="span" aria-hidden="true" w="12px">
                {row.expanded ? "▾" : "▸"}
              </Box>
              <Box as="span" flex="1" minW="0" overflow="hidden" textOverflow="ellipsis">
                {row.name}
              </Box>
              {dirCounts && (
                <Box as="span" data-testid="dir-count" fontSize="xs">
                  {row.count}
                </Box>
              )}
            </chakra.button>
          )
        }
        const f = row.file
        const letter = f.status.charAt(0).toUpperCase()
        const on = f.path === selected
        return (
          <chakra.button
            key={`f:${row.id}`}
            type="button"
            role="treeitem"
            aria-selected={on}
            data-testid="tree-file"
            data-path={f.path}
            title={f.old_path ? `${f.old_path} → ${f.path}` : f.path}
            onClick={() => onSelect(f.path)}
            {...rowProps}
            pl={pl}
            bg={on ? "tone.running.bg" : "transparent"}
            color="text"
          >
            <Box
              as="span"
              data-testid="file-status"
              w="14px"
              fontFamily="mono"
              fontWeight="400"
              color={STATUS_TONE[letter] ?? "text.muted"}
            >
              {letter}
            </Box>
            <Box as="span" flex="1" minW="0" overflow="hidden" textOverflow="ellipsis">
              {row.name}
            </Box>
            <ChangeBar added={f.added} deleted={f.deleted} />
          </chakra.button>
        )
      })}
    </Box>
  )
}
