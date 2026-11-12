import { Box, chakra, Text } from "@chakra-ui/react"
import { useState } from "react"
import { Link } from "react-router"
import type { SettingsEntry } from "../generated/server"
import { SETTINGS_HREF } from "../hrefs"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { useDevice } from "../useWide"
import { Chip } from "../ui/chip/Chip"
import { entryLabel, groupBySection, searchEntries } from "./settingsApi"
import { Group } from "./parts"
import { DEV_FRONTEND_KEY } from "../dev"
import { DevFrontendRow } from "./DevFrontendRow"
import { SettingsRow, type WriteSetting } from "./SettingsRow"
import { ChecksEditor } from "./ChecksEditor"
import { CHECKS_KEY } from "./checks"

/** The global sections, in sidebar order. */
export const GLOBAL_SECTIONS = [
  "Execution queue",
  "Budget",
  "Notifications",
  "Server",
  "Web UI",
  "Keybindings",
  "CPU watchdog",
  "Process watchdog",
  "Disk space",
  "Agents",
  "Models",
  "Pull requests",
  "Evolve and recovery",
  "Ticket store",
] as const

export const sectionSlug = (section: string) => section.toLowerCase().replace(/[^a-z0-9]+/g, "-")

export const projectPath = (project: string) => `/settings/p/${encodeURIComponent(project)}`

export const sectionPath = (section: string, project?: string) =>
  `${project === undefined ? "/settings" : projectPath(project)}/s/${sectionSlug(section)}`

export const sectionFromSlug = (
  slug: string | undefined,
  sections: readonly string[] = GLOBAL_SECTIONS,
) => sections.find((s) => sectionSlug(s) === slug)

/** A project's sidebar sections in the order the server sent them: Project, the overridable ones, Checks. */
export const projectSections = (entries: SettingsEntry[]) =>
  groupBySection(entries).map((g) => g.section)

const IDENTITY_KEYS = ["name", "repo", "vcs"]

const NavLink = chakra(Link)

const navLink = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "1.5",
  px: "2.5",
  py: "1.5",
  borderRadius: "7px",
  color: "text.muted",
  fontWeight: "600",
  fontSize: "sm",
  cursor: "pointer",
  _hover: { bg: "surface.raised" },
} as const

const GroupLabel = ({ children }: { children: string }) => (
  <Text
    fontSize="11px"
    letterSpacing="0.08em"
    textTransform="uppercase"
    color="text.muted"
    mx="2"
    mt="3"
    mb="0.5"
  >
    {children}
  </Text>
)

const CountChip = ({ n }: { n: number }) => (
  <chakra.span
    fontSize="11px"
    fontWeight="600"
    px="7px"
    py="1px"
    borderRadius="full"
    bg="tone.running.bg"
    color="tone.running.fg"
  >
    {n}
  </chakra.span>
)

export interface ShellProps {
  entries: SettingsEntry[]
  projects: string[]
  /** The project on the page; global settings when absent. */
  project?: string
  /** Called with a picker value: "global" or a project name. */
  onScope?: (target: string) => void
  /** The section on the page; the first section when absent. */
  section?: string
  totpOn: boolean
  authenticatorMissing?: boolean
  onSignOut: () => void
  /** Startup-only keys changed since the server started. */
  pendingRestart?: string[]
  onWrite: WriteSetting
  /** The project's repo holds .gx/checks.json, which wins over its checks setting. */
  repoChecksFile?: boolean
}

function Sidebar({
  entries,
  projects,
  project,
  onScope,
  section,
  totpOn,
  authenticatorMissing,
  onSignOut,
  query,
  onQuery,
}: ShellProps & { query: string; onQuery: (q: string) => void }) {
  const changed = new Map(groupBySection(entries).map((g) => [g.section, g.changed]))
  const sections = project === undefined ? GLOBAL_SECTIONS : projectSections(entries)
  return (
    <chakra.nav data-mock-part="sidebar" display="grid" gap="0.5" alignSelf="start">
      <chakra.select
        data-mock-part="scope-picker"
        aria-label="Settings scope"
        w="100%"
        h="40px"
        px="3"
        borderWidth="1px"
        borderColor="border"
        borderRadius="8px"
        bg="surface.raised"
        color="text"
        value={project ?? "global"}
        onChange={(e) => onScope?.(e.target.value)}
      >
        <option value="global">Global settings</option>
        {projects.map((p) => (
          <option key={p} value={p}>
            Project: {p}
          </option>
        ))}
      </chakra.select>
      <chakra.input
        data-mock-part="search"
        type="text"
        aria-label="Search settings"
        placeholder="Search all settings"
        value={query}
        onChange={(e) => onQuery(e.target.value)}
        mt="2"
        w="100%"
        h="40px"
        px="3.5"
        borderWidth="1px"
        borderColor="border"
        borderRadius="10px"
        bg="surface.raised"
        color="text"
      />
      <GroupLabel>{project ?? "Global"}</GroupLabel>
      {sections.map((s) => (
        <NavLink
          key={s}
          to={sectionPath(s, project)}
          {...navLink}
          aria-current={s === section ? "page" : undefined}
          {...(s === section && { bg: "tone.running.bg", color: "tone.running.fg" })}
        >
          {s}
          {(changed.get(s) ?? 0) > 0 && <CountChip n={changed.get(s) ?? 0} />}
        </NavLink>
      ))}
      {project === undefined && (
        <>
          <GroupLabel>Account</GroupLabel>
          <AccountLink href={`${SETTINGS_HREF}/logins`}>Web logins</AccountLink>
          <AccountLink href={`${SETTINGS_HREF}/password`}>Password</AccountLink>
          {totpOn && (
            <AccountLink href={`${SETTINGS_HREF}/authenticator`}>
              {authenticatorMissing ? "Add authenticator" : "Authenticator"}
            </AccountLink>
          )}
          <AccountLink href={`${SETTINGS_HREF}/certificate`}>Certificate</AccountLink>
          <chakra.button
            type="button"
            onClick={onSignOut}
            {...navLink}
            color="tone.deadlocked.fg"
            textAlign="left"
          >
            Sign out of this device
          </chakra.button>
        </>
      )}
    </chakra.nav>
  )
}

const AccountLink = ({ href, children }: { href: string; children: string }) => (
  <chakra.a href={href} {...navLink}>
    {children}
    <Text aria-hidden color="text.dim">
      ›
    </Text>
  </chakra.a>
)

/** A fact about the project: label, value and a read-only chip, with no control. */
const IdentityRow = ({ entry }: { entry: SettingsEntry }) => (
  <Box
    px="3.5"
    py="3"
    display="flex"
    gap="3"
    alignItems="center"
    _notFirst={{ borderTopWidth: "1px", borderTopColor: "hairline" }}
  >
    <Text fontWeight="600" color="text.muted">
      {entry.key === "repo" ? "Repository" : entry.key === "vcs" ? "VCS" : entryLabel(entry)}
    </Text>
    <Text fontWeight="700" overflowWrap="anywhere" minW="0">
      {String(entry.value ?? entry.default ?? "")}
    </Text>
    <Chip fontSize="xs">🔒 read-only</Chip>
  </Box>
)

/** One row; the dev-frontend key gets its wrapper, which supplies the lock reason. */
function settingsRow(
  e: SettingsEntry,
  pendingRestart: string[] | undefined,
  onWrite: WriteSetting,
  project: string | undefined,
) {
  const Row = e.key === DEV_FRONTEND_KEY ? DevFrontendRow : SettingsRow
  return (
    <Row
      key={e.key}
      entry={e}
      pendingRestart={pendingRestart?.includes(e.key) ?? false}
      onWrite={onWrite}
      project={project}
    />
  )
}

type SectionBodyProps = Pick<
  ShellProps,
  "entries" | "project" | "pendingRestart" | "onWrite" | "repoChecksFile"
> & { section: string }

/** One section's rows: Identity and Merging, the Checks editor, or plain rows. Shared by the sidebar shell and the phone page. */
export function SectionBody({
  entries,
  project,
  section,
  pendingRestart,
  onWrite,
  repoChecksFile,
}: SectionBodyProps) {
  const row = (e: SettingsEntry) => settingsRow(e, pendingRestart, onWrite, project)
  const sectionEntries = entries.filter((e) => e.section === section)
  if (project !== undefined && section === "Project") {
    return (
      <>
        <Group label="Identity">
          {sectionEntries
            .filter((e) => IDENTITY_KEYS.includes(e.key))
            .map((e) => (
              <IdentityRow key={e.key} entry={e} />
            ))}
        </Group>
        <Group label="Merging">
          {sectionEntries.filter((e) => !IDENTITY_KEYS.includes(e.key)).map(row)}
        </Group>
      </>
    )
  }
  if (project !== undefined && section === "Checks") {
    return (
      <ChecksEditor
        key={JSON.stringify(sectionEntries[0]?.value ?? null)}
        entry={sectionEntries.find((e) => e.key === CHECKS_KEY)}
        repoChecksFile={repoChecksFile ?? false}
        onWrite={onWrite}
      />
    )
  }
  return <Group>{sectionEntries.map(row)}</Group>
}

export function SettingsShellView(props: ShellProps) {
  const { pendingRestart, onWrite } = props
  const device = useDevice()
  const [query, setQuery] = useState("")
  const searching = query.trim() !== ""
  const section =
    props.section ??
    (props.project === undefined ? GLOBAL_SECTIONS[0] : projectSections(props.entries)[0])
  const row = (e: SettingsEntry) => settingsRow(e, pendingRestart, onWrite, props.project)
  const hits = searchEntries(props.entries, query)
  const scopeName = props.project === undefined ? "global settings" : `project ${props.project}`
  return (
    <Box
      display="grid"
      gridTemplateColumns={`${device === "desktop" ? 230 : 200}px minmax(0, 1fr)`}
      gap="14px"
      alignItems="start"
    >
      <Sidebar {...props} section={section} query={query} onQuery={setQuery} />
      <Box data-mock-part="page" maxW="720px">
        <PageHeader title={searching ? "Search" : section} icon={<PageIcon kind="settings" />} />
        {!searching && <SectionBody {...props} section={section} />}
        {searching && hits.length === 0 && (
          <EmptyState title="No settings match">
            Nothing in {scopeName} matches “{query.trim()}”.
          </EmptyState>
        )}
        {searching && hits.length > 0 && (
          <>
            <Text
              mt="4"
              fontSize="xs"
              letterSpacing="0.08em"
              textTransform="uppercase"
              color="text.muted"
              px="3"
            >
              {hits.length} {hits.length === 1 ? "match" : "matches"} in {scopeName}
            </Text>
            {groupBySection(hits).map((g) => (
              <Group key={g.section} label={g.section}>
                {g.entries.map(row)}
              </Group>
            ))}
          </>
        )}
      </Box>
    </Box>
  )
}
