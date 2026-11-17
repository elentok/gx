import { Box, chakra, Text } from "@chakra-ui/react"
import type { RefObject } from "react"
import { MascotToggle } from "../shell/MascotToggle"
import { DraftIcon } from "../shell/SidebarRows"

export const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface.raised",
  borderWidth: "1px",
  borderColor: "hairline",
  borderRadius: "8px",
  _placeholder: { color: "text.dim" },
} as const

interface Props {
  label: string
  project: string
  projects: string[]
  slug: string
  slugError: string | undefined
  slugInput: RefObject<HTMLInputElement | null>
  status: "none" | "saving" | "saved" | "unsaved"
  onRename: () => void
  onPickProject: (project: string) => void
  onSlug: (slug: string) => void
}

/** The draft page's title row (name, rename, save status) and the project / epic slug bar. */
export function DraftHeader({
  label,
  project,
  projects,
  slug,
  slugError,
  slugInput,
  status,
  onRename,
  onPickProject,
  onSlug,
}: Props) {
  return (
    <>
      <Box display="flex" alignItems="center" gap="2" pt="3" flexShrink="0">
        <MascotToggle />
        <DraftIcon />
        <Text
          as="h1"
          fontSize="lg"
          fontWeight="600"
          color={slug.trim() ? "text" : "text.dim"}
          minW="0"
          truncate
        >
          {label}
        </Text>
        <chakra.button
          type="button"
          aria-label="Rename"
          title="Rename"
          onClick={onRename}
          color="text.dim"
          h="32px"
          w="32px"
          flexShrink="0"
          borderRadius="8px"
          _hover={{ bg: "surface", color: "text" }}
        >
          <svg
            width="14"
            height="14"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden
          >
            <path d="M3 13l.6-3 6.9-6.9a1.3 1.3 0 0 1 1.8 0l.6.6a1.3 1.3 0 0 1 0 1.8L6 12.4z" />
          </svg>
        </chakra.button>
        <Box flex="1" />
        {status !== "none" && (
          <Text fontSize="xs" color="text.dim" flexShrink="0" whiteSpace="nowrap">
            {status === "saved" && (
              <chakra.span aria-hidden color="tone.done.fg">
                ✓{" "}
              </chakra.span>
            )}
            {status === "saving" ? "Saving..." : status === "unsaved" ? "Not saved" : "Saved"}
          </Text>
        )}
      </Box>
      <Box
        display="flex"
        alignItems="center"
        gap="2"
        py="3"
        borderBottomWidth="1px"
        borderColor="hairline"
        flexShrink="0"
      >
        {projects.length > 1 && (
          <>
            <chakra.select
              aria-label="Project"
              value={project}
              onChange={(e) => onPickProject(e.target.value)}
              fontSize="16px"
              h="40px"
              px="3"
              borderRadius="8px"
              bg="tone.running.bg"
              color="tone.running.fg"
              fontWeight="600"
            >
              {projects.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </chakra.select>
            <Text color="text.dim" fontSize="lg">
              /
            </Text>
          </>
        )}
        <chakra.input
          aria-label="Epic slug"
          aria-invalid={!!slugError}
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
          spellCheck={false}
          ref={slugInput}
          {...field}
          flex="1"
          minW="0"
          h="40px"
          px="3"
          fontWeight="600"
          placeholder="my-epic"
          value={slug}
          onChange={(e) => onSlug(e.target.value)}
        />
      </Box>
    </>
  )
}
