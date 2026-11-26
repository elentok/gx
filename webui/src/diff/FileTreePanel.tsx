import { Box, chakra } from "@chakra-ui/react"
import { forwardRef, useMemo } from "react"
import type { DiffFile } from "../generated/server"
import { TextBox } from "../ui/text-box/TextBox"
import { buildTree, filterFiles } from "./fileTree"

export const FileFilter = forwardRef<
  HTMLInputElement,
  { value: string; onChange: (v: string) => void }
>(function FileFilter({ value, onChange }, ref) {
  return (
    <TextBox
      kind="search"
      ref={ref}
      name="file-filter"
      data-testid="file-filter"
      label="Filter files"
      hideLabel
      placeholder="Filter files"
      mono
      mb="2"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  )
})

function Dot({ added }: { added: boolean }) {
  return (
    <Box
      as="span"
      data-testid="file-dot"
      data-tone={added ? "done" : "waiting"}
      aria-hidden="true"
      w="6px"
      h="6px"
      flexShrink={0}
      borderRadius="full"
      bg={added ? "tone.done.fg" : "tone.waiting.fg"}
    />
  )
}

function Counts({ added, deleted }: { added: number; deleted: number }) {
  if (added === 0 && deleted === 0) return null
  return (
    <Box as="span" data-testid="file-counts" flexShrink={0} fontSize="xs" color="text.faint">
      {added > 0 && <Box as="span" data-part="added" color="tone.done.fg">{`+${added}`}</Box>}
      {added > 0 && deleted > 0 && " "}
      {deleted > 0 && (
        <Box as="span" data-part="deleted" color="tone.deadlocked.fg">{`−${deleted}`}</Box>
      )}
    </Box>
  )
}

export function FileTree({
  files,
  filter,
  selected,
  onSelect,
}: {
  files: DiffFile[]
  filter: string
  selected: string | undefined
  onSelect: (path: string) => void
}) {
  const matches = useMemo(() => filterFiles(files, filter), [files, filter])
  const rows = useMemo(() => buildTree(matches), [matches])

  if (matches.length === 0) {
    return (
      <Box data-testid="file-tree" mt="2" px="2" py="3" fontSize="sm" color="text.muted">
        {`No files match "${filter}".`}
      </Box>
    )
  }
  return (
    <Box data-testid="file-tree" role="tree" aria-label="Changed files">
      {rows.map((row) => {
        const pl = `${row.depth * 14 + 8}px`
        if (row.kind === "dir") {
          return (
            <Box
              key={`d:${row.id}`}
              data-testid="tree-dir"
              display="flex"
              alignItems="center"
              minH="28px"
              pl={pl}
              pr="2"
              fontFamily="text"
              fontSize="sm"
              fontWeight="500"
              color="text.dim"
              overflow="hidden"
              textOverflow="ellipsis"
              whiteSpace="nowrap"
            >
              {row.name}
            </Box>
          )
        }
        const f = row.file
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
            display="flex"
            alignItems="center"
            gap="2"
            w="100%"
            minH="30px"
            pl={pl}
            pr="2"
            borderRadius="4px"
            fontFamily="text"
            fontSize="sm"
            textAlign="left"
            bg={on ? "sidebar.current" : "transparent"}
            color={on ? "text" : "text.muted"}
            _hover={on ? undefined : { bg: "surface.hover" }}
          >
            <Dot added={f.status.charAt(0).toUpperCase() === "A"} />
            <Box as="span" flex="1" minW="0" overflow="hidden" textOverflow="ellipsis">
              {row.name}
            </Box>
            <Counts added={f.added} deleted={f.deleted} />
          </chakra.button>
        )
      })}
    </Box>
  )
}
