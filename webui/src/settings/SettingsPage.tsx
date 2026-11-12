import { Box, chakra, Text } from "@chakra-ui/react"
import { useState, type ReactNode } from "react"
import { Navigate, useNavigate, useParams } from "react-router"
import { EmptyState } from "../ui/empty-state/EmptyState"
import type { FetchEpicList } from "../epics/list"
import { useEpicList } from "../epics/list"
import { useDevice } from "../useWide"
import type { SettingsEntry, SettingsReadResponse } from "../generated/server"
import {
  GLOBAL_SECTIONS,
  projectPath,
  projectSections,
  SectionBody,
  sectionFromSlug,
  sectionPath,
  SettingsShellView,
} from "./SettingsShell"
import {
  entryLabel,
  GLOBAL_TARGET,
  groupBySection,
  postSettings,
  searchEntries,
  useSettings,
  type FetchSettings,
  type PostSettings,
} from "./settingsApi"
import { fetchAuthState, type AuthState } from "../auth/authState"
import { CER_LABEL, Fingerprint, PROFILE_LABEL, type CertInfo } from "../auth/cert"
import { SETTINGS_HREF } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { Button } from "../ui/button/Button"
import { Chip } from "../ui/chip/Chip"
import { Spinner } from "../ui/spinner/Spinner"
import { useFetched, type FetchedState } from "../useFetched"
import { useWrite } from "../useWrite"
import {
  endsInDays,
  fetchLogins,
  lastSeenText,
  renameLogin,
  revokeLogin,
  signOutThisDevice,
  type FetchLogins,
  type LoginWrite,
  type WebLoginEntry,
} from "./logins"
import { BackLink, Frame, Group, rowProps, SearchBox } from "./parts"
import type { WriteSetting } from "./SettingsRow"

const DAY_MS = 24 * 60 * 60 * 1000
const LOGINS_HREF = `${SETTINGS_HREF}/logins`

const Chevron = () => (
  <Text aria-hidden color="text.dim">
    ›
  </Text>
)

function LinkRow({ href, label, detail }: { href: string; label: string; detail?: ReactNode }) {
  return (
    <chakra.a href={href} {...rowProps}>
      <Box flex="1">{label}</Box>
      {detail && (
        <Text fontSize="sm" color="text.muted">
          {detail}
        </Text>
      )}
      <Chevron />
    </chakra.a>
  )
}

const sectionHref = (section: string, project?: string) => `#${sectionPath(section, project)}`

/** A phone search hit: opens the section page holding the row. */
function HitRows({
  entries,
  query,
  project,
}: {
  entries: SettingsEntry[]
  query: string
  project?: string
}) {
  return groupBySection(searchEntries(entries, query)).map((g) => (
    <Group key={g.section} label={`${project ?? "Global"} · ${g.section}`}>
      {g.entries.map((e) => (
        <LinkRow
          key={e.key}
          href={sectionHref(g.section, project)}
          label={entryLabel(e)}
          detail={e.description}
        />
      ))}
    </Group>
  ))
}

function ProjectHits({
  project,
  query,
  fetchSettings,
}: {
  project: string
  query: string
  fetchSettings?: FetchSettings
}) {
  const settings = useSettings(project, undefined, fetchSettings)
  return settings.data ? (
    <HitRows entries={settings.data.entries} query={query} project={project} />
  ) : null
}

export function SettingsHomeView({
  loginCount,
  totpOn,
  authenticatorMissing = false,
  entries = [],
  projects = [],
  fetchSettings,
  onSignOut,
}: {
  loginCount?: number
  totpOn: boolean
  /** TOTP is on but the saved secret was cleared, so the row adds one instead of resetting it. */
  authenticatorMissing?: boolean
  /** The global settings, for the section rows' changed chips and the search. */
  entries?: SettingsEntry[]
  projects?: string[]
  /** Reads each project's settings while searching. */
  fetchSettings?: FetchSettings
  onSignOut: () => void
}) {
  const [query, setQuery] = useState("")
  const searching = query.trim() !== ""
  const changed = new Map(groupBySection(entries).map((g) => [g.section, g.changed]))
  return (
    <Frame>
      <Box data-mock-part="page">
        <PageHeader title="Settings" icon={<PageIcon kind="settings" />} />
        <SearchBox value={query} onChange={setQuery} />
        {searching ? (
          <>
            <HitRows entries={entries} query={query} />
            {projects.map((p) => (
              <ProjectHits key={p} project={p} query={query} fetchSettings={fetchSettings} />
            ))}
            {projects.length === 0 && searchEntries(entries, query).length === 0 && (
              <EmptyState title="No settings match">Nothing matches “{query.trim()}”.</EmptyState>
            )}
          </>
        ) : (
          <>
            <Group label="Security">
              <LinkRow
                href={LOGINS_HREF}
                label="Web logins"
                detail={
                  loginCount === undefined
                    ? undefined
                    : `${loginCount} ${loginCount === 1 ? "device" : "devices"}`
                }
              />
              <LinkRow href={`${SETTINGS_HREF}/password`} label="Change password" />
              {totpOn && (
                <LinkRow
                  href={`${SETTINGS_HREF}/authenticator`}
                  label={authenticatorMissing ? "Add authenticator" : "Reset authenticator"}
                />
              )}
              <LinkRow href={`${SETTINGS_HREF}/certificate`} label="Certificate" />
            </Group>
            <Group label="Global settings">
              {GLOBAL_SECTIONS.map((s) => (
                <LinkRow
                  key={s}
                  href={sectionHref(s)}
                  label={s}
                  detail={changedText(changed.get(s), "changed")}
                />
              ))}
            </Group>
            {projects.length > 0 && (
              <Group label="Projects">
                {projects.map((p) => (
                  <LinkRow key={p} href={`#${projectPath(p)}`} label={p} />
                ))}
              </Group>
            )}
            <Group>
              <chakra.button
                type="button"
                onClick={onSignOut}
                {...rowProps}
                color="tone.deadlocked.fg"
              >
                Sign out of this device
              </chakra.button>
            </Group>
          </>
        )}
      </Box>
    </Frame>
  )
}

const changedText = (n: number | undefined, word: string) =>
  n ? <Chip tone="running">{`${n} ${word}`}</Chip> : undefined

/** A phone project's list of sections, each with how many of its keys the project overrides. */
export function ProjectSectionsView({
  project,
  entries,
}: {
  project: string
  entries: SettingsEntry[]
}) {
  return (
    <Frame>
      <Box data-mock-part="page">
        <BackLink />
        <PageHeader title={project} icon={<PageIcon kind="settings" />} />
        <Text mt="2" fontSize="sm" color="text.muted">
          Each setting uses the global value until you override it here.
        </Text>
        <Group label="Sections">
          {groupBySection(entries).map((g) => (
            <LinkRow
              key={g.section}
              href={sectionHref(g.section, project)}
              label={g.section}
              detail={
                g.entries.some((e) => e.identity) ? undefined : changedText(g.changed, "overridden")
              }
            />
          ))}
        </Group>
      </Box>
    </Frame>
  )
}

// The mock lists this device first, then the other logins newest to oldest.
const sortLogins = (logins: WebLoginEntry[]) =>
  [...logins].sort(
    (a, b) =>
      Number(b.this_device) - Number(a.this_device) ||
      Date.parse(b.last_used) - Date.parse(a.last_used),
  )

export function WebLoginsView({ logins, now }: { logins: WebLoginEntry[]; now: number }) {
  return (
    <Frame>
      <BackLink />
      <PageHeader title="Web logins" icon={<PageIcon kind="settings" />} />
      <Text mt="2" fontSize="sm" color="text.muted">
        A web login ends after 90 days without use.
      </Text>
      <Group>
        {sortLogins(logins).map((login) => {
          const endsIn = endsInDays(login.last_used, now)
          return (
            <chakra.a key={login.id} href={`${LOGINS_HREF}/${login.id}`} {...rowProps} py="1">
              <Box flex="1" minW="0">
                <Text fontWeight="600">{login.name}</Text>
                <Text fontSize="sm" color={endsIn === undefined ? "text.muted" : "tone.waiting.fg"}>
                  {endsIn === undefined
                    ? lastSeenText(login, now)
                    : `Ends in ${endsIn} ${endsIn === 1 ? "day" : "days"} unless used`}
                </Text>
              </Box>
              {login.this_device && <Chip tone="done">This device</Chip>}
              <Chevron />
            </chakra.a>
          )
        })}
      </Group>
    </Frame>
  )
}

const Fact = ({ label, value }: { label: string; value: string }) => (
  <Box {...rowProps} cursor="default" _hover={{}}>
    <Box flex="1">{label}</Box>
    <Text fontSize="sm" color="text.muted">
      {value}
    </Text>
  </Box>
)

const signedInText = (created?: string) =>
  created
    ? new Date(created).toLocaleDateString("en-US", {
        month: "short",
        day: "numeric",
        year: "numeric",
      })
    : "Unknown"

// The mock counts in days from one day on; lastSeenText would say "10w".
function lastUsedText(lastUsed: string, now: number): string {
  const days = Math.floor((now - Date.parse(lastUsed)) / DAY_MS)
  return days >= 1
    ? `${days} ${days === 1 ? "day" : "days"} ago`
    : lastSeenText({ last_used: lastUsed, last_ip: "" }, now)
}

export function WebLoginView({
  login,
  now,
  onRename,
  onSignOut,
  pending = false,
  startConfirming = false,
}: {
  login: WebLoginEntry
  now: number
  onRename: (name: string) => void
  onSignOut: () => void
  pending?: boolean
  /** Opens with the confirm step showing, for the gallery. */
  startConfirming?: boolean
}) {
  const [confirming, setConfirming] = useState(startConfirming)
  return (
    <Frame>
      <BackLink />
      <PageHeader title={login.name} icon={<PageIcon kind="settings" />} />
      <Group>
        <Box data-mock-part="login-facts">
          <Fact label="Signed in" value={signedInText(login.created)} />
          <Fact label="Last used" value={lastUsedText(login.last_used, now)} />
          <Fact label="Last IP" value={login.last_ip || "Unknown"} />
        </Box>
      </Group>
      <Box mt="6">
        <chakra.label data-mock-part="name" display="block">
          <Text fontSize="sm" color="text.muted" mb="2">
            Name
          </Text>
          <chakra.input
            key={login.name}
            type="text"
            defaultValue={login.name}
            w="100%"
            h="40px"
            px="3"
            borderWidth="1px"
            borderColor="border"
            borderRadius="8px"
            bg="surface.raised"
            color="text"
            onBlur={(e) => {
              const name = e.currentTarget.value.trim()
              if (name && name !== login.name) onRename(name)
            }}
          />
          <Text fontSize="xs" color="text.dim" mt="1">
            Saved when you leave the field.
          </Text>
        </chakra.label>
      </Box>
      <Group>
        <chakra.button
          type="button"
          data-mock-part="sign-out"
          onClick={() => setConfirming(true)}
          {...rowProps}
          color="tone.deadlocked.fg"
        >
          Sign out this device
        </chakra.button>
      </Group>
      {confirming && (
        <Box data-mock-part="confirm" mt="4" display="flex" flexDirection="column" gap="3">
          <Text
            data-mock-part="note"
            fontSize="sm"
            bg="tone.waiting.bg"
            color="tone.waiting.fg"
            px="3"
            py="2"
            borderRadius="md"
          >
            It has to sign in again with the password and a code.
          </Text>
          <Box display="flex" gap="2">
            <Button tone="paused" disabled={pending} onClick={onSignOut}>
              Sign out
            </Button>
            <Button emphasis="ghost" tone="muted" onClick={() => setConfirming(false)}>
              Cancel
            </Button>
          </Box>
        </Box>
      )}
    </Frame>
  )
}

export function CertificateView({ cert }: { cert: CertInfo }) {
  return (
    <Frame>
      <BackLink />
      <PageHeader title="Certificate" icon={<PageIcon kind="settings" />} />
      <Box data-mock-part="downloads">
        <Group>
          <LinkRow href={cert.profile_path} label={PROFILE_LABEL} />
          <LinkRow href={cert.cer_path} label={`${CER_LABEL} (.cer)`} />
        </Group>
      </Box>
      <Box data-mock-part="fingerprint" mt="4">
        <Text fontSize="sm" color="text.dim" mb="1">
          SHA-256 fingerprint
        </Text>
        <Fingerprint value={cert.fingerprint} size="sm" />
      </Box>
    </Frame>
  )
}

export function CertificatePage({
  fetchAuth = fetchAuthState,
}: {
  fetchAuth?: () => Promise<AuthState>
}) {
  const auth = useFetched("settings-auth", fetchAuth)
  return (
    <DetailPage title={{ kind: "Settings", name: "Certificate" }}>
      {auth.data ? <CertificateView cert={auth.data} /> : <Spinner label="Loading" />}
    </DetailPage>
  )
}

interface SettingsPageProps {
  postChanges?: PostSettings
  fetchAuth?: () => Promise<AuthState>
  fetchList?: FetchLogins
  fetchSettings?: FetchSettings
  fetchEpics?: FetchEpicList
  signOut?: () => void | Promise<void>
}

type Settings = FetchedState<SettingsReadResponse>

/** What both pages read: the target's settings, the project names and a write that refreshes the settings. */
function useSettingsTarget(
  project: string | undefined,
  { fetchSettings, fetchEpics, postChanges = postSettings }: SettingsPageProps,
) {
  const target = project ?? GLOBAL_TARGET
  const settings = useSettings(target, undefined, fetchSettings)
  const { list } = useEpicList(fetchEpics)
  const projects = [...new Set(list?.epics.map((e) => e.project) ?? [])].sort()
  const onWrite: WriteSetting = async (changes) => {
    const res = await postChanges(target, changes)
    if (!res.refused) settings.retry()
    return res
  }
  return { settings, projects, onWrite }
}

/** The phone page of one section. */
function SectionPage({
  project,
  section,
  settings,
  onWrite,
}: {
  project?: string
  section: string
  settings: Settings
  onWrite: WriteSetting
}) {
  return (
    <DetailPage title={{ kind: "Settings", name: section }}>
      {settings.data ? (
        <Frame>
          <Box data-mock-part="page">
            <BackLink
              href={project === undefined ? SETTINGS_HREF : `#${projectPath(project)}`}
              label={project ?? "Settings"}
            />
            <PageHeader title={section} icon={<PageIcon kind="settings" />} />
            <SectionBody
              entries={settings.data.entries}
              project={project}
              section={section}
              pendingRestart={settings.data.pending_restart}
              repoChecksFile={settings.data.project?.repo_checks_file}
              onWrite={onWrite}
            />
          </Box>
        </Frame>
      ) : (
        <Spinner label="Loading" />
      )}
    </DetailPage>
  )
}

/** The sidebar shell, from 640px up. */
function ShellPage({
  project,
  slug,
  section,
  settings,
  projects,
  onWrite,
  auth,
  signOut = signOutThisDevice,
}: {
  project?: string
  slug?: string
  section?: string
  settings: Settings
  projects: string[]
  onWrite: WriteSetting
  auth?: AuthState | null
  signOut?: () => void | Promise<void>
}) {
  const navigate = useNavigate()
  return (
    <DetailPage title={{ kind: "Settings", name: section }}>
      {auth && settings.data ? (
        <SettingsShellView
          entries={settings.data.entries}
          projects={projects}
          project={project}
          onScope={(next) =>
            navigate(
              sectionPath(slug ?? GLOBAL_SECTIONS[0], next === GLOBAL_TARGET ? undefined : next),
            )
          }
          section={section}
          totpOn={auth.totp_on}
          authenticatorMissing={auth.setup_code_required}
          onSignOut={() => void signOut()}
          pendingRestart={settings.data.pending_restart}
          repoChecksFile={settings.data.project?.repo_checks_file}
          onWrite={onWrite}
        />
      ) : (
        <Spinner label="Loading" />
      )}
    </DetailPage>
  )
}

/** `/settings` and `/settings/s/:section`: the shell on wide screens; on a phone the home and one section page. */
function GlobalHomePage({ slug, ...props }: SettingsPageProps & { slug?: string }) {
  const { fetchAuth = fetchAuthState, fetchList = fetchLogins, signOut = signOutThisDevice } = props
  const auth = useFetched("settings-auth", fetchAuth)
  const logins = useFetched("settings-logins", fetchList)
  const { settings, projects, onWrite } = useSettingsTarget(undefined, props)
  const wide = useDevice() !== "phone"
  const section = sectionFromSlug(slug)
  if (wide && !section) return <Navigate to={sectionPath(GLOBAL_SECTIONS[0])} replace />
  if (!wide && section)
    return <SectionPage section={section} settings={settings} onWrite={onWrite} />
  if (wide) {
    return (
      <ShellPage
        slug={slug}
        section={section}
        settings={settings}
        projects={projects}
        onWrite={onWrite}
        auth={auth.data}
        signOut={signOut}
      />
    )
  }
  return (
    <DetailPage title={{ kind: "Settings" }}>
      {auth.data ? (
        <SettingsHomeView
          loginCount={logins.data?.length}
          entries={settings.data?.entries}
          projects={projects}
          fetchSettings={props.fetchSettings}
          totpOn={auth.data.totp_on}
          authenticatorMissing={auth.data.setup_code_required}
          onSignOut={() => void signOut()}
        />
      ) : (
        <Spinner label="Loading" />
      )}
    </DetailPage>
  )
}

/** `/settings/p/:project` and its sections: the shell on wide screens; on a phone the section list and one section page. */
function ProjectPage({
  project,
  slug,
  ...props
}: SettingsPageProps & { project: string; slug?: string }) {
  const { fetchAuth = fetchAuthState, signOut = signOutThisDevice } = props
  const auth = useFetched("settings-auth", fetchAuth)
  const { settings, projects, onWrite } = useSettingsTarget(project, props)
  const wide = useDevice() !== "phone"
  const sections = settings.data ? projectSections(settings.data.entries) : []
  const section = sectionFromSlug(slug, sections)
  if (wide && settings.data && !section)
    return <Navigate to={sectionPath(sections[0], project)} replace />
  if (settings.failed) {
    return (
      <DetailPage title={{ kind: "Settings" }}>
        <EmptyState title="Project not found">There is no project named “{project}”.</EmptyState>
      </DetailPage>
    )
  }
  if (!wide && section) {
    return <SectionPage project={project} section={section} settings={settings} onWrite={onWrite} />
  }
  if (!wide) {
    return (
      <DetailPage title={{ kind: "Settings", name: project }}>
        {settings.data ? (
          <ProjectSectionsView project={project} entries={settings.data.entries} />
        ) : (
          <Spinner label="Loading" />
        )}
      </DetailPage>
    )
  }
  return (
    <ShellPage
      project={project}
      slug={slug}
      section={section}
      settings={settings}
      projects={projects}
      onWrite={onWrite}
      auth={auth.data}
      signOut={signOut}
    />
  )
}

export function SettingsPage(props: SettingsPageProps) {
  const { project, section: slug } = useParams()
  return project === undefined ? (
    <GlobalHomePage slug={slug} {...props} />
  ) : (
    <ProjectPage project={project} slug={slug} {...props} />
  )
}

export function WebLoginsPage({
  fetchList = fetchLogins,
  now = Date.now,
}: {
  fetchList?: FetchLogins
  now?: () => number
}) {
  const logins = useFetched("settings-logins", fetchList)
  return (
    <DetailPage title={{ kind: "Settings", name: "Web logins" }}>
      {logins.data ? (
        <WebLoginsView logins={logins.data} now={now()} />
      ) : (
        <Spinner label="Loading" />
      )}
    </DetailPage>
  )
}

export function WebLoginPage({
  fetchList = fetchLogins,
  rename = renameLogin,
  revoke = revokeLogin,
  signOut = signOutThisDevice,
  now = Date.now,
}: {
  fetchList?: FetchLogins
  rename?: LoginWrite
  revoke?: LoginWrite
  signOut?: () => void | Promise<void>
  now?: () => number
}) {
  const { id = "" } = useParams()
  const logins = useFetched("settings-logins", fetchList)
  const write = useWrite()
  const login = logins.data?.find((l) => l.id === id)
  return (
    <DetailPage title={{ kind: "Settings", name: login?.name ?? "Web login" }}>
      {!logins.data ? (
        <Spinner label="Loading" />
      ) : !login ? (
        <Frame>
          <BackLink />
          <Text mt="4" color="text.muted">
            This web login no longer exists.
          </Text>
        </Frame>
      ) : (
        <WebLoginView
          login={login}
          now={now()}
          pending={write.pending}
          onRename={(name) =>
            void write.run(() => rename(login.id, name), {
              onDone: logins.retry,
            })
          }
          onSignOut={() =>
            login.this_device
              ? void signOut()
              : void write.run(() => revoke(login.id), {
                  onDone: () => {
                    window.location.hash = LOGINS_HREF
                  },
                })
          }
        />
      )}
    </DetailPage>
  )
}

/** The `/settings/*` catch-all: a settings URL nothing handles, so only the way back. */
export function UnknownSettingsPage() {
  return (
    <DetailPage title={{ kind: "Settings" }}>
      <Frame>
        <BackLink />
      </Frame>
    </DetailPage>
  )
}
