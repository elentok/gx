import { Box, chakra, Text } from "@chakra-ui/react"
import { useState, type ReactNode } from "react"
import { Navigate, useNavigate, useParams } from "react-router"
import { EmptyState } from "../ui/empty-state/EmptyState"
import type { FetchEpicList } from "../epics/list"
import { useEpicList } from "../epics/list"
import { useDevice } from "../useWide"
import type { SettingsCategory, SettingsEntry, SettingsReadResponse } from "../generated/server"
import {
  projectPath,
  ScopePicker,
  SectionBody,
  SectionResults,
  sectionPath,
  SettingsShellView,
} from "./SettingsShell"
import {
  entryLabel,
  GLOBAL_TARGET,
  groupBySection,
  postSettings,
  resolveSection,
  searchEntries,
  treeSections,
  useSettings,
  type FetchSettings,
  type PostSettings,
} from "./settingsApi"
import { fetchAuthState, type AuthState } from "../auth/authState"
import { CER_LABEL, Fingerprint, PROFILE_LABEL, type CertInfo } from "../auth/cert"
import { SETTINGS_HREF } from "../hrefs"
import { refreshPushDevice } from "../push/deviceState"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { Button } from "../ui/button/Button"
import { Chip } from "../ui/chip/Chip"
import { StateHeader } from "../page/StateHeader"
import { TextBox } from "../ui/text-box/TextBox"
import { useFetched, type FetchedState } from "../useFetched"
import { useFetchedReady, useReady } from "../useReady"
import { useWrite } from "../useWrite"
import {
  endsInDays,
  fetchLogins,
  lastSeenText,
  removeLoginPush,
  renameLogin,
  revokeLogin,
  signOutThisDevice,
  type FetchLogins,
  type LoginWrite,
  type WebLoginEntry,
} from "./logins"
import {
  BackLink,
  CountChip,
  Chevron,
  Frame,
  Group,
  LoadingState,
  rowProps,
  SearchBox,
} from "./parts"
import type { WriteSetting } from "./SettingsRow"

const DAY_MS = 24 * 60 * 60 * 1000
const LOGINS_HREF = `${SETTINGS_HREF}/logins`

function LinkRow({
  href,
  label,
  detail,
  part,
}: {
  href: string
  label: string
  detail?: ReactNode
  part?: string
}) {
  return (
    <chakra.a href={href} data-mock-part={part} {...rowProps}>
      <Box flex="1" fontWeight="600">
        {label}
      </Box>
      {detail && (
        <Text fontSize="sm" color="text.muted">
          {detail}
        </Text>
      )}
      <Chevron />
    </chakra.a>
  )
}

const sectionHref = (slug: string, project?: string) => `#${sectionPath(slug, project)}`

const scopeProject = (picked: string) => (picked === GLOBAL_TARGET ? undefined : picked)
const homePath = (picked: string) => {
  const project = scopeProject(picked)
  return project === undefined ? "/settings" : projectPath(project)
}

/** The phone Settings home: scope picker, search, then every category with its sections as tiles. Account only in global scope. */
export function SettingsHomeView({
  loginCount,
  totpOn = false,
  authenticatorMissing = false,
  entries = [],
  categories = [],
  projects = [],
  project,
  onScope,
  onSignOut,
}: {
  loginCount?: number
  /** Account props: only the global home shows the Account block, so project pages omit them. */
  totpOn?: boolean
  /** TOTP is on but the saved secret was cleared, so the row adds one instead of resetting it. */
  authenticatorMissing?: boolean
  /** The picked scope's settings, for the tiles' changed chips and the search. */
  entries?: SettingsEntry[]
  /** The picked scope's page tree; the tiles come from it. */
  categories?: SettingsCategory[]
  projects?: string[]
  /** The picked project; global settings when absent. */
  project?: string
  onScope?: (target: string) => void
  onSignOut?: () => void
}) {
  const [query, setQuery] = useState("")
  const searching = query.trim() !== ""
  const changed = new Map(groupBySection(entries).map((g) => [g.section, g.changed]))
  const hits = searchEntries(entries, query)
  return (
    <Frame>
      <Box data-mock-part="page">
        <PageHeader title="Settings" icon={<PageIcon kind="settings" />} />
        <Box mt="2">
          <ScopePicker projects={projects} project={project} onScope={onScope} />
        </Box>
        <SearchBox value={query} onChange={setQuery} />
        {searching ? (
          hits.length === 0 ? (
            <EmptyState title="No settings match">
              Nothing in {project === undefined ? "global settings" : `project ${project}`} matches
              “{query.trim()}”.
            </EmptyState>
          ) : (
            <SectionResults
              hits={hits}
              categories={categories}
              renderHit={(e, slug) => (
                <LinkRow
                  key={e.key}
                  href={sectionHref(slug, project)}
                  label={entryLabel(e)}
                  detail={e.description}
                />
              )}
            />
          )
        ) : (
          <>
            {categories.map((c) => (
              <Group key={c.name} label={c.name}>
                {c.sections.map((s) => (
                  <LinkRow
                    key={s.slug}
                    href={sectionHref(s.slug, project)}
                    label={s.name}
                    detail={
                      changed.get(s.name) ? <CountChip n={changed.get(s.name) ?? 0} /> : undefined
                    }
                  />
                ))}
              </Group>
            ))}
            {project === undefined && (
              <Group label="Account" part="account-group">
                <LinkRow
                  href={LOGINS_HREF}
                  label="Web logins"
                  detail={
                    loginCount === undefined
                      ? undefined
                      : `${loginCount} ${loginCount === 1 ? "device" : "devices"}`
                  }
                />
                <LinkRow href={`${SETTINGS_HREF}/password`} label="Password" />
                {totpOn && (
                  <LinkRow
                    href={`${SETTINGS_HREF}/authenticator`}
                    label={authenticatorMissing ? "Add authenticator" : "Authenticator"}
                  />
                )}
                <LinkRow href={`${SETTINGS_HREF}/certificate`} label="Certificate" />
                <chakra.button
                  type="button"
                  data-mock-part="sign-out"
                  onClick={onSignOut}
                  {...rowProps}
                  color="tone.deadlocked.fg"
                >
                  Sign out of this device
                </chakra.button>
              </Group>
            )}
          </>
        )}
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
              {login.push_on && (
                <span data-mock-part="push-chip">
                  <Chip>Push on</Chip>
                </span>
              )}
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

const PUSH_CONTACT_KEY = "server.web.push-contact-email"

const pushSinceText = (since?: string) =>
  since ? new Date(since).toLocaleDateString("en-US", { month: "short", day: "numeric" }) : ""

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
  onRemovePush,
  pending = false,
  startConfirming = false,
}: {
  login: WebLoginEntry
  now: number
  onRename: (name: string) => void
  onSignOut: () => void
  onRemovePush: () => void
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
          {login.push_on && (
            <Box data-mock-part="push-row" {...rowProps} cursor="default" _hover={{}}>
              <Box flex="1">
                <Text fontWeight="600">Push</Text>
                <Text fontSize="sm" color="text.muted">
                  On since {pushSinceText(login.push_since)}
                </Text>
              </Box>
              <chakra.button
                type="button"
                disabled={pending}
                onClick={onRemovePush}
                color="tone.deadlocked.fg"
              >
                Remove
              </chakra.button>
            </Box>
          )}
        </Box>
      </Group>
      {login.push_on && (
        <Text mt="2" fontSize="sm" color="text.muted">
          Remove deletes only the push subscription. The login stays. Only that device can turn it
          back on.
        </Text>
      )}
      <Box mt="6">
        <Box data-mock-part="name">
          <TextBox
            key={login.name}
            kind="line"
            name="login-title"
            label="Title"
            placeholder="Title"
            hint="Saved when you leave the field."
            defaultValue={login.name}
            onBlur={(e) => {
              const title = e.currentTarget.value.trim()
              if (title && title !== login.name) onRename(title)
            }}
          />
        </Box>
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
  useFetchedReady(auth)
  return (
    <DetailPage title={{ kind: "Settings", name: "Certificate" }}>
      {auth.data ? <CertificateView cert={auth.data} /> : <LoadingState title="Certificate" />}
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
  useFetchedReady(settings)
  const { list } = useEpicList(fetchEpics)
  const projects = [...new Set(list?.epics.map((e) => e.project) ?? [])].sort()
  const onWrite: WriteSetting = async (changes) => {
    const res = await postChanges(target, changes)
    if (!res.refused) {
      settings.retry()
      // The server reloaded: the push card must see the new contact email.
      if (changes.some((c) => c.key === PUSH_CONTACT_KEY)) void refreshPushDevice()
    }
    return res
  }
  return { settings, projects, onWrite }
}

/** The phone drill-down page of one section: back arrow, category breadcrumb, then the groups. */
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
  const category = settings.data
    ? treeSections(settings.data.categories).find((s) => s.name === section)?.category
    : undefined
  return (
    <DetailPage title={{ kind: "Settings", name: section }}>
      {settings.data ? (
        <Frame>
          <Box data-mock-part="page">
            <Box display="flex" alignItems="center" gap="1">
              <chakra.a
                href={project === undefined ? SETTINGS_HREF : `#${projectPath(project)}`}
                aria-label={project === undefined ? "Back to Settings" : `Back to ${project}`}
                display="inline-flex"
                alignItems="center"
                justifyContent="center"
                w="8"
                h="8"
                fontSize="lg"
                fontWeight="600"
                color="text.muted"
                _hover={{ color: "text" }}
              >
                ‹
              </chakra.a>
              <Box flex="1" minW="0">
                <PageHeader title={section} icon={<PageIcon kind="settings" />} />
              </Box>
            </Box>
            <Text data-testid="breadcrumb" mt="2" px="3" fontSize="sm" color="text.muted">
              {category}
            </Text>
            <SectionBody
              entries={settings.data.entries}
              categories={settings.data.categories}
              project={project}
              section={section}
              pendingRestart={settings.data.pending_restart}
              repoChecksFile={settings.data.project?.repo_checks_file}
              notificationKinds={settings.data.notification_kinds}
              notificationChannels={settings.data.notification_channels}
              onWrite={onWrite}
            />
          </Box>
        </Frame>
      ) : (
        <LoadingState title={section} />
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
          categories={settings.data.categories}
          onScope={(next) => {
            // The other scope may not hold this section; its page redirects an unknown slug to its first.
            navigate(slug ? sectionPath(slug, scopeProject(next)) : homePath(next))
          }}
          section={section}
          totpOn={auth.totp_on}
          authenticatorMissing={auth.setup_code_required}
          onSignOut={() => void signOut()}
          pendingRestart={settings.data.pending_restart}
          repoChecksFile={settings.data.project?.repo_checks_file}
          notificationKinds={settings.data.notification_kinds}
          notificationChannels={settings.data.notification_channels}
          onWrite={onWrite}
        />
      ) : (
        <LoadingState title={section ?? "Settings"} />
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
  const navigate = useNavigate()
  const res = settings.data ? resolveSection(settings.data, slug) : undefined
  if (res?.redirect) return <Navigate to={sectionPath(res.redirect)} replace />
  const section = res?.section?.name
  if (wide && res && !section && res.first)
    return <Navigate to={sectionPath(res.first.slug)} replace />
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
          categories={settings.data?.categories}
          projects={projects}
          onScope={(next) => navigate(homePath(next))}
          totpOn={auth.data.totp_on}
          authenticatorMissing={auth.data.setup_code_required}
          onSignOut={() => void signOut()}
        />
      ) : (
        <LoadingState title="Settings" />
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
  const navigate = useNavigate()
  const res = settings.data ? resolveSection(settings.data, slug) : undefined
  if (res?.redirect) return <Navigate to={sectionPath(res.redirect, project)} replace />
  const section = res?.section?.name
  if (wide && res && !section && res.first)
    return <Navigate to={sectionPath(res.first.slug, project)} replace />
  if (settings.failed) {
    return (
      <DetailPage title={{ kind: "Settings" }}>
        <StateHeader title="Settings" />
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
          <SettingsHomeView
            project={project}
            projects={projects}
            entries={settings.data.entries}
            categories={settings.data.categories}
            onScope={(next) => navigate(homePath(next))}
          />
        ) : (
          <LoadingState title={project} />
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
  useFetchedReady(logins)
  return (
    <DetailPage title={{ kind: "Settings", name: "Web logins" }}>
      {logins.data ? (
        <WebLoginsView logins={logins.data} now={now()} />
      ) : (
        <LoadingState title="Web logins" />
      )}
    </DetailPage>
  )
}

export function WebLoginPage({
  fetchList = fetchLogins,
  rename = renameLogin,
  revoke = revokeLogin,
  removePush = removeLoginPush,
  signOut = signOutThisDevice,
  now = Date.now,
}: {
  fetchList?: FetchLogins
  rename?: LoginWrite
  revoke?: LoginWrite
  removePush?: LoginWrite
  signOut?: () => void | Promise<void>
  now?: () => number
}) {
  const { id = "" } = useParams()
  const logins = useFetched("settings-logins", fetchList)
  useFetchedReady(logins)
  const write = useWrite()
  const login = logins.data?.find((l) => l.id === id)
  return (
    <DetailPage title={{ kind: "Settings", name: login?.name ?? "Web login" }}>
      {!logins.data ? (
        <LoadingState title="Web login" />
      ) : !login ? (
        <Frame>
          <StateHeader title="Web login" />
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
          onRemovePush={() =>
            void write.run(() => removePush(login.id), {
              onDone: () => {
                logins.retry()
                if (login.this_device) {
                  void navigator.clearAppBadge?.()
                  void refreshPushDevice()
                }
              },
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
  useReady()
  return (
    <DetailPage title={{ kind: "Settings" }}>
      <Frame>
        <StateHeader title="Settings" />
        <BackLink />
      </Frame>
    </DetailPage>
  )
}
