import { Box, chakra, Text } from "@chakra-ui/react"
import { Fragment, useState, type ReactNode } from "react"
import { Link } from "react-router"
import type { SettingsCategory, SettingsEntry } from "../generated/server"
import { SETTINGS_HREF } from "../hrefs"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { useDevice } from "../useWide"
import { Chip } from "../ui/chip/Chip"
import { entryLabel, groupBySection, isChanged, searchEntries, treeSections } from "./settingsApi"
import { CountChip, Group, GroupBlock, SearchBox } from "./parts"
import { DEV_FRONTEND_KEY } from "../dev"
import { DevFrontendRow } from "./DevFrontendRow"
import { SettingsRow, type WriteSetting } from "./SettingsRow"
import { ChecksEditor } from "./ChecksEditor"
import { ModelsTable } from "./ModelsTable"
import { CHECKS_KEY } from "./checks"

export const projectPath = (project: string) => `/settings/p/${encodeURIComponent(project)}`

export const sectionPath = (slug: string, project?: string) =>
  `${project === undefined ? "/settings" : projectPath(project)}/s/${slug}`

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

export interface ShellProps {
  entries: SettingsEntry[]
  /** The page tree from the read response: categories and sections in page order. */
  categories: SettingsCategory[]
  projects: string[]
  /** The project on the page; global settings when absent. */
  project?: string
  /** Called with a picker value: "global" or a project name. */
  onScope?: (target: string) => void
  /** The section name on the page; the first section when absent. */
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

const ACCOUNT = "Account"

function Caret({ open }: { open: boolean }) {
  return (
    <Text as="span" aria-hidden color="text.dim" mr="1.5">
      {open ? "▾" : "▸"}
    </Text>
  )
}

/** The Global / project select above the sidebar and the phone home. */
export const ScopePicker = ({
  projects,
  project,
  onScope,
}: Pick<ShellProps, "projects" | "project" | "onScope">) => (
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
)

function Sidebar({
  entries,
  categories,
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
  const searching = query.trim() !== ""
  // While searching, the chips count hits and only sections with hits are listed.
  const counts = new Map(
    searching
      ? groupBySection(searchEntries(entries, query)).map((g) => [g.section, g.entries.length])
      : groupBySection(entries).map((g) => [g.section, g.changed]),
  )
  const shown = searching
    ? categories
        .map((c) => ({ ...c, sections: c.sections.filter((s) => (counts.get(s.name) ?? 0) > 0) }))
        .filter((c) => c.sections.length > 0)
    : categories
  const current = treeSections(categories).find((s) => s.name === section)?.category
  // Only the user's clicks are stored; they win over the default (the current category open) until the section changes.
  const [overrides, setOverrides] = useState<{ section?: string; open: Record<string, boolean> }>({
    open: {},
  })
  const open = overrides.section === section ? overrides.open : {}
  const isOpen = (name: string) => searching || (open[name] ?? name === current)
  const toggle = (name: string) =>
    setOverrides({ section, open: { ...open, [name]: !isOpen(name) } })
  return (
    <chakra.nav data-mock-part="sidebar" display="grid" gap="0.5" alignSelf="start">
      <ScopePicker projects={projects} project={project} onScope={onScope} />
      <SearchBox value={query} onChange={onQuery} />
      <CategoryNav
        categories={shown}
        counts={counts}
        project={project}
        section={section}
        searching={searching}
        isOpen={isOpen}
        toggle={toggle}
      />
      {project === undefined && !searching && (
        <AccountNav
          isOpen={isOpen(ACCOUNT)}
          onToggle={() => toggle(ACCOUNT)}
          totpOn={totpOn}
          authenticatorMissing={authenticatorMissing}
          onSignOut={onSignOut}
        />
      )}
    </chakra.nav>
  )
}

function NavHeading({
  name,
  count,
  open,
  showCount,
  onToggle,
}: {
  name: string
  count: number
  open: boolean
  showCount: boolean
  onToggle: () => void
}) {
  return (
    <chakra.button
      type="button"
      aria-expanded={open}
      onClick={onToggle}
      {...navLink}
      color="text.muted"
      fontSize="xs"
      letterSpacing="0.08em"
      textTransform="uppercase"
      textAlign="left"
    >
      <span>
        <Caret open={open} />
        {name}
      </span>
      {showCount && count > 0 && <CountChip n={count} />}
    </chakra.button>
  )
}

/** The collapsible categories and their section links; `counts` are changed counts, or hit counts while searching. */
function CategoryNav({
  categories,
  counts,
  project,
  section,
  searching,
  isOpen,
  toggle,
}: {
  categories: SettingsCategory[]
  counts: Map<string, number>
  project?: string
  section?: string
  searching: boolean
  isOpen: (name: string) => boolean
  toggle: (name: string) => void
}) {
  return categories.map((c) => (
    <Fragment key={c.name}>
      <NavHeading
        name={c.name}
        count={c.sections.reduce((n, s) => n + (counts.get(s.name) ?? 0), 0)}
        open={isOpen(c.name)}
        showCount={searching || !isOpen(c.name)}
        onToggle={() => toggle(c.name)}
      />
      {isOpen(c.name) &&
        c.sections.map((s) => (
          <NavLink
            key={s.slug}
            to={sectionPath(s.slug, project)}
            {...navLink}
            pl="6"
            aria-current={s.name === section ? "page" : undefined}
            {...(s.name === section && {
              bg: "tone.running.bg",
              color: "tone.running.fg",
            })}
          >
            {s.name}
            {(counts.get(s.name) ?? 0) > 0 && <CountChip n={counts.get(s.name) ?? 0} />}
          </NavLink>
        ))}
    </Fragment>
  ))
}

/** The global-only Account block: links to the account pages and sign out. */
function AccountNav({
  isOpen,
  onToggle,
  totpOn,
  authenticatorMissing,
  onSignOut,
}: Pick<ShellProps, "totpOn" | "authenticatorMissing" | "onSignOut"> & {
  isOpen: boolean
  onToggle: () => void
}) {
  return (
    <>
      <NavHeading name={ACCOUNT} count={0} open={isOpen} showCount={false} onToggle={onToggle} />
      {isOpen && (
        <>
          <AccountLink href={`${SETTINGS_HREF}/logins`}>Web logins</AccountLink>
          <AccountLink href={`${SETTINGS_HREF}/password`}>Password</AccountLink>
          {totpOn && (
            <AccountLink href={`${SETTINGS_HREF}/authenticator`}>
              {authenticatorMissing ? "Add authenticator" : "Authenticator"}
            </AccountLink>
          )}
          <AccountLink href={`${SETTINGS_HREF}/certificate`}>Certificate</AccountLink>
          <chakra.button type="button" onClick={onSignOut} {...navLink} pl="6" textAlign="left">
            Sign out of this device
          </chakra.button>
        </>
      )}
    </>
  )
}

const AccountLink = ({ href, children }: { href: string; children: string }) => (
  <chakra.a href={href} {...navLink} pl="6">
    {children}
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
      {entryLabel(entry)}
    </Text>
    <Text fontWeight="700" overflowWrap="anywhere" minW="0">
      {String(entry.value ?? entry.default ?? "")}
    </Text>
    <Chip fontSize="xs">🔒 read-only</Chip>
  </Box>
)

type RowProps = Pick<ShellProps, "project" | "pendingRestart" | "onWrite"> & { highlight?: string }

/** Builds one row per entry; the dev-frontend key gets its wrapper, which supplies the lock reason. */
const rowBuilder =
  ({ pendingRestart, onWrite, project, highlight }: RowProps) =>
  (e: SettingsEntry) => {
    const Row = e.key === DEV_FRONTEND_KEY ? DevFrontendRow : SettingsRow
    return (
      <Row
        key={e.key}
        entry={e}
        pendingRestart={pendingRestart?.includes(e.key) ?? false}
        onWrite={onWrite}
        project={project}
        highlight={highlight}
      />
    )
  }

type SectionBodyProps = Pick<
  ShellProps,
  "entries" | "categories" | "project" | "pendingRestart" | "onWrite" | "repoChecksFile"
> & { section: string }

/** The section's entries split by group: the response's group order, then any group the tree does not name. A group with no entry is absent. */
export function sectionGroups(
  categories: SettingsCategory[],
  section: string,
  sectionEntries: SettingsEntry[],
) {
  const tree = categories.flatMap((c) => c.sections).find((s) => s.name === section)?.groups ?? []
  const named = tree.map((g) => g.name)
  return [...new Set([...named, ...sectionEntries.map((e) => e.group)])]
    .map((name) => ({
      name,
      table: tree.find((g) => g.name === name)?.table ?? false,
      entries: sectionEntries.filter((e) => e.group === name),
    }))
    .filter((g) => g.entries.length > 0)
}

/** One section's titled group blocks, or the Checks editor. Shared by the sidebar shell and the phone page. */
export function SectionBody({
  entries,
  categories,
  project,
  section,
  pendingRestart,
  onWrite,
  repoChecksFile,
}: SectionBodyProps) {
  const row = rowBuilder({ pendingRestart, onWrite, project })
  const sectionEntries = entries.filter((e) => e.section === section)
  if (project !== undefined && sectionEntries.some((e) => e.key === CHECKS_KEY)) {
    return (
      <ChecksEditor
        key={JSON.stringify(sectionEntries[0]?.value ?? null)}
        entry={sectionEntries.find((e) => e.key === CHECKS_KEY)}
        repoChecksFile={repoChecksFile ?? false}
        onWrite={onWrite}
      />
    )
  }
  const groups = sectionGroups(categories, section, sectionEntries)
  return (
    <>
      {project !== undefined && groups.some((g) => g.table) && (
        <Text
          data-mock-part="override-note"
          mt="2"
          px="3"
          py="1.5"
          fontSize="13px"
          color="text.muted"
          bg="surface"
          borderWidth="1px"
          borderColor="border"
          borderRadius="8px"
        >
          Project overrides for {project}. An empty value uses the global setting.
        </Text>
      )}
      {groups.map((g) =>
        g.name === "" ? (
          <Group key="">{g.entries.map(row)}</Group>
        ) : (
          <GroupBlock key={g.name} title={g.name} changed={g.entries.filter(isChanged).length}>
            {g.table ? (
              <ModelsTable agent={g.name} entries={g.entries} project={project} onWrite={onWrite} />
            ) : (
              g.entries.map((e) =>
                project !== undefined && e.identity ? (
                  <IdentityRow key={e.key} entry={e} />
                ) : (
                  row(e)
                ),
              )
            )}
          </GroupBlock>
        ),
      )}
    </>
  )
}

export function SettingsShellView(props: ShellProps) {
  const device = useDevice()
  const [query, setQuery] = useState("")
  const searching = query.trim() !== ""
  const sections = treeSections(props.categories)
  const section = props.section ?? sections[0]?.name ?? ""
  const category = sections.find((s) => s.name === section)?.category
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
        {!searching && category && (
          <Text fontSize="xs" letterSpacing="0.08em" textTransform="uppercase" color="text.muted">
            {category}
          </Text>
        )}
        <PageHeader
          title={
            searching && hits.length > 0
              ? `${hits.length} results for "${query.trim()}"`
              : searching
                ? "Search"
                : section
          }
          icon={<PageIcon kind="settings" />}
        />
        {!searching && <SectionBody {...props} section={section} />}
        {searching && hits.length === 0 && (
          <EmptyState title="No settings match">
            Nothing in {scopeName} matches “{query.trim()}”.
          </EmptyState>
        )}
        {searching && hits.length > 0 && (
          <SearchResults {...props} hits={hits} query={query.trim()} />
        )}
      </Box>
    </Box>
  )
}

/** Every hit, split by section (with its category as a breadcrumb) and then by group. Hits are ordinary rows, so Models keys are not tables. */
function SearchResults({
  hits,
  query,
  categories,
  ...rest
}: ShellProps & { hits: SettingsEntry[]; query: string }) {
  const row = rowBuilder({ ...rest, highlight: query })
  return <SectionResults hits={hits} categories={categories} renderHit={(e) => row(e)} />
}

/** Hits split by section (with its category as a breadcrumb) and then by group; the caller renders each hit. Shared by the shell and the phone home. */
export function SectionResults({
  hits,
  categories,
  renderHit,
}: {
  hits: SettingsEntry[]
  categories: SettingsCategory[]
  /** `slug` is the hit's section slug. */
  renderHit: (entry: SettingsEntry, slug: string) => ReactNode
}) {
  const sections = treeSections(categories)
  return (
    <>
      {groupBySection(hits).map((g) => {
        const section = sections.find((s) => s.name === g.section)
        return (
          <Box key={g.section} mt="6" data-testid={`result-section-${g.section}`}>
            <Box display="flex" alignItems="baseline" gap="2" px="1">
              <Text as="h2" fontSize="md" fontWeight="700">
                {g.section}
              </Text>
              <Text fontSize="xs" color="text.muted">
                {section?.category}
              </Text>
            </Box>
            {sectionGroups(categories, g.section, g.entries).map((grp) => (
              <Box key={grp.name} mt="3">
                {grp.name !== "" && (
                  <Text
                    as="h3"
                    fontSize="xs"
                    letterSpacing="0.06em"
                    textTransform="uppercase"
                    color="text.muted"
                    fontWeight="600"
                    mb="1.5"
                    px="1"
                  >
                    {grp.name}
                  </Text>
                )}
                <Group>{grp.entries.map((e) => renderHit(e, section?.slug ?? ""))}</Group>
              </Box>
            ))}
          </Box>
        )
      })}
    </>
  )
}
