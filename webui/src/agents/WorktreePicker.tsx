import { Box, chakra, Text } from "@chakra-ui/react"
import { useEffect, useRef, useState, type KeyboardEvent } from "react"
import { Field, field } from "./formField"

export interface Option {
  key: string
  label: string
  path: string
  /** Set on the "+ Create worktree" row. */
  create?: string
}

export const baseName = (path: string) => path.replace(/\/+$/, "").split("/").pop() ?? path

/** A searchable combobox: filter `rows`; with `canCreate`, a typed name offers "+ Create worktree". Arrow keys move, Enter picks. */
export function WorktreePicker({
  rows,
  picked,
  creating = "",
  onPick,
  label = "Worktree",
  canCreate = true,
}: {
  label?: string
  canCreate?: boolean
  rows: Option[]
  picked: Option | undefined
  /** The new worktree's name once "+ Create worktree" was chosen. */
  creating?: string
  onPick: (o: Option) => void
}) {
  // The text being typed; null shows the current choice.
  const [query, setQuery] = useState<string | null>(null)

  const [active, setActive] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)
  const listId = `combo-${label.toLowerCase()}`

  const q = query?.trim() ?? ""
  const options: Option[] = []
  if (canCreate && q && !rows.some((r) => r.key !== "root" && r.label === q)) {
    options.push({
      key: "create",
      label: `+ Create worktree “${q}”`,
      path: "",
      create: q,
    })
  }
  options.push(...rows.filter((r) => !q || r.label.toLowerCase().includes(q.toLowerCase())))

  const at = Math.min(active, Math.max(options.length - 1, 0))
  useEffect(() => {
    listRef.current?.children[at]?.scrollIntoView?.({ block: "nearest" })
  }, [at, query])

  const pick = (o: Option) => {
    onPick(o)
    setQuery(null)
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault()
      if (query === null) setQuery("")
      else if (options.length) {
        setActive((at + (e.key === "ArrowDown" ? 1 : options.length - 1)) % options.length)
      }
    } else if (e.key === "Enter" && query !== null) {
      e.preventDefault()
      if (options[at]) pick(options[at])
    } else if (e.key === "Escape" && query !== null) {
      e.stopPropagation()
      setQuery(null)
    }
  }

  return (
    <Field label={label} caret>
      <chakra.input
        role="combobox"
        aria-label={label}
        aria-expanded={query !== null}
        aria-controls={listId}
        aria-activedescendant={query !== null && options[at] ? `${listId}-${at}` : undefined}
        autoComplete="off"
        autoCorrect="off"
        autoCapitalize="off"
        spellCheck={false}
        {...field}
        h="40px"
        pr="8"
        value={query ?? (creating || picked?.label || "")}
        onFocus={() => setQuery((q) => q ?? "")}
        onBlur={() => setQuery(null)}
        onChange={(e) => {
          setQuery(e.target.value)
          setActive(0)
        }}
        onKeyDown={onKeyDown}
      />
      {query !== null && (
        <Box
          ref={listRef}
          id={listId}
          role="listbox"
          aria-label={`${label}s`}
          position="absolute"
          top="100%"
          left="0"
          right="0"
          zIndex="1"
          maxH="200px"
          overflowY="auto"
          bg="surface.raised"
          borderWidth="1px"
          borderColor="border"
          borderRadius="8px"
        >
          {options.map((o, i) => (
            <chakra.div
              key={o.key}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={o.create ? false : o.path === picked?.path && !creating}
              display="flex"
              gap="3"
              px="3"
              py="2"
              cursor="pointer"
              color={o.create ? "tone.done.fg" : "text"}
              bg={i === at ? "surface" : o.create ? "surface" : undefined}
              _hover={{ bg: "surface" }}
              // Keeps the input focused so its blur does not close the list before the click lands.
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => pick(o)}
            >
              <span>{o.label}</span>
              {o.path && (
                <Text as="span" ml="auto" fontSize="xs" color="text.dim" truncate>
                  {o.path}
                </Text>
              )}
            </chakra.div>
          ))}
        </Box>
      )}
    </Field>
  )
}
