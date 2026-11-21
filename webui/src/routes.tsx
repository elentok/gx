import { RestKindBedtime, RestKindBreak } from "./generated/server"
import { RestDemoPage } from "./RestDemoPage"
import { lazy } from "react"
import {
  createHashRouter,
  Navigate,
  redirect,
  useLocation,
  useParams,
  type RouteObject,
} from "react-router"
import type { AuthState } from "./auth/authState"
import { noteCatchAll } from "./routeSpans"
import { AppShell } from "./shell/AppShell"
import type { FetchLogins } from "./settings/logins"
import type { FetchSettings } from "./settings/settingsApi"
import { sectionPath } from "./settings/SettingsShell"
import type { FetchChat } from "./design/chat"
import type { DeleteDesign, OpenDesign, OpenFlow, ResumeSession, StopSession } from "./design/open"
import type { AbandonTriage } from "./triage/api"
import type { FetchTriage } from "./triage/useTriage"
import type { PostFocus } from "./design/focus"
import type { PostAnswer, PostPrompt } from "./design/prompt"
import type { FetchScreen } from "./design/screen"
import type { FetchMapImages } from "./design/mapImages"
import type { FetchEpicDetail } from "./epic/detail"
import type { FixAgent } from "./epic/fixAgent"
import type { SaveChecks } from "./checks/setup"
import type { FetchChecksStatus } from "./checks/status"
import type { FetchArchived } from "./archive/archive"
import type {
  FetchScheduleShow,
  FetchSchedules,
  PostCreate,
  PostDelete,
  PostEdit,
  PostRunNow,
  PostSwitch,
} from "./schedules/schedules"
import type { FetchBacklog, PostFinishMove, PostStartDesign } from "./backlog/backlog"
import type { FetchInitiative, FetchInitiatives } from "./initiatives/initiatives"
import { EpicsPage } from "./epics/EpicsPage"
import type { FetchEpicList } from "./epics/list"
import type { FetchCleanup, PostCleanup } from "./cleanup/cleanup"
import type { FetchTips } from "./tips/tips"
import type { FetchStats } from "./stats/stats"
import type { FetchDetail } from "./ticket/detail"
import type {
  FetchDiffCommitPatches,
  FetchDiffCommits,
  FetchDiffFiles,
  FetchDiffPatch,
} from "./diff/api"
import type { FetchFileResolve } from "./files/FileViewerPage"
import type { FetchFileText } from "./files/rawText"

const AgentsPage = lazy(() =>
  import("./agents/AgentsPage").then(({ AgentsPage }) => ({ default: AgentsPage })),
)
const ArchivePage = lazy(() =>
  import("./archive/ArchivePage").then(({ ArchivePage }) => ({ default: ArchivePage })),
)
const CleanupPage = lazy(() =>
  import("./cleanup/CleanupPage").then(({ CleanupPage }) => ({ default: CleanupPage })),
)
const DesignPage = lazy(() =>
  import("./design/DesignPage").then(({ DesignPage }) => ({ default: DesignPage })),
)
const DiffPage = lazy(() =>
  import("./diff/DiffPage").then(({ DiffPage }) => ({ default: DiffPage })),
)
const EpicPage = lazy(() =>
  import("./epic/EpicPage").then(({ EpicPage }) => ({ default: EpicPage })),
)
const FileViewerPage = lazy(() =>
  import("./files/FileViewerPage").then(({ FileViewerPage }) => ({ default: FileViewerPage })),
)
const BacklogPage = lazy(() =>
  import("./backlog/BacklogPage").then(({ BacklogPage }) => ({ default: BacklogPage })),
)
const InitiativesPage = lazy(() =>
  import("./initiatives/InitiativesPage").then(({ InitiativesPage }) => ({
    default: InitiativesPage,
  })),
)
const InitiativePage = lazy(() =>
  import("./initiatives/InitiativePage").then(({ InitiativePage }) => ({
    default: InitiativePage,
  })),
)
const SchedulePage = lazy(() =>
  import("./schedules/SchedulePage").then(({ SchedulePage }) => ({ default: SchedulePage })),
)
const SchedulesPage = lazy(() =>
  import("./schedules/SchedulesPage").then(({ SchedulesPage }) => ({ default: SchedulesPage })),
)
const StatsPage = lazy(() =>
  import("./stats/StatsPage").then(({ StatsPage }) => ({ default: StatsPage })),
)
const TicketPage = lazy(() =>
  import("./ticket/TicketPage").then(({ TicketPage }) => ({ default: TicketPage })),
)
const TipsPage = lazy(() =>
  import("./tips/TipsPage").then(({ TipsPage }) => ({ default: TipsPage })),
)
const TriagePage = lazy(() =>
  import("./triage/TriagePage").then(({ TriagePage }) => ({ default: TriagePage })),
)

const DraftPage = lazy(() =>
  import("./design/DraftPage").then(({ DraftPage }) => ({ default: DraftPage })),
)

const ChatDraftPage = lazy(() =>
  import("./design/ChatDraftPage").then(({ ChatDraftPage }) => ({ default: ChatDraftPage })),
)

const accountPagesModule = () => import("./settings/AccountPages")
const AuthenticatorPage = lazy(() =>
  accountPagesModule().then(({ AuthenticatorPage }) => ({ default: AuthenticatorPage })),
)
const ChangePasswordPage = lazy(() =>
  accountPagesModule().then(({ ChangePasswordPage }) => ({ default: ChangePasswordPage })),
)

const settingsPagesModule = () => import("./settings/SettingsPage")
const CertificatePage = lazy(() =>
  settingsPagesModule().then(({ CertificatePage }) => ({ default: CertificatePage })),
)
const SettingsPage = lazy(() =>
  settingsPagesModule().then(({ SettingsPage }) => ({ default: SettingsPage })),
)
const UnknownSettingsPage = lazy(() =>
  settingsPagesModule().then(({ UnknownSettingsPage }) => ({ default: UnknownSettingsPage })),
)
const WebLoginPage = lazy(() =>
  settingsPagesModule().then(({ WebLoginPage }) => ({ default: WebLoginPage })),
)
const WebLoginsPage = lazy(() =>
  settingsPagesModule().then(({ WebLoginsPage }) => ({ default: WebLoginsPage })),
)

function TicketRoute({
  fetchDetail,
  fetchScreen,
}: {
  fetchDetail?: FetchDetail
  fetchScreen?: FetchScreen
}) {
  // An address holds a slash (project:epic/NN), so it arrives as the splat.
  const address = useParams()["*"] ?? ""
  return <TicketPage address={address} fetchDetail={fetchDetail} fetchScreen={fetchScreen} />
}

function EpicRoute({
  fetchDetail,
  fetchList,
  openDesign,
  fixAgent,
  readOnly,
}: {
  fetchDetail?: FetchEpicDetail
  fetchList?: FetchEpicList
  openDesign?: OpenDesign
  fixAgent?: FixAgent
  readOnly?: boolean
}) {
  return (
    <EpicPage
      readOnly={readOnly}
      address={useParams()["*"] ?? ""}
      fetchDetail={fetchDetail}
      fetchList={fetchList}
      openDesign={openDesign}
      fixAgent={fixAgent}
    />
  )
}

function ScheduleRoute({ fetchers }: { fetchers: RouteFetchers }) {
  const address = useParams()["*"]
  return (
    <SchedulePage
      key={address}
      address={address && address !== "new" ? address : undefined}
      fetchList={fetchers.schedules}
      fetchShow={fetchers.scheduleShow}
      create={fetchers.createSchedule}
      edit={fetchers.editSchedule}
      runNow={fetchers.runNowSchedule}
      switchTo={fetchers.switchSchedule}
      remove={fetchers.deleteSchedule}
    />
  )
}

function DesignRoute({ fetchers }: { fetchers: RouteFetchers }) {
  // A Design's address holds a colon (project:epic), so it arrives as the splat.
  const address = useParams()["*"]
  const startFailure = (useLocation().state as { startFailure?: string } | null)?.startFailure
  if (!address) return <Navigate to="/epics" replace />
  return (
    <DesignPage
      address={address}
      startFailure={startFailure}
      fetchList={fetchers.epicList}
      deps={{
        fetchDetail: fetchers.epic,
        fetchChat: fetchers.chat,
        post: fetchers.prompt,
        answer: fetchers.answer,
        open: fetchers.openDesign,
        remove: fetchers.deleteDesign,
        stop: fetchers.stop,
        resume: fetchers.resume,
        fetchScreen: fetchers.screen,
        focus: fetchers.focus,
        fetchMapImages: fetchers.mapImages,
      }}
    />
  )
}

function TriageRoute({ fetchers }: { fetchers: RouteFetchers }) {
  const project = useParams()["*"]
  if (!project) return <Navigate to="/epics" replace />
  return (
    <TriagePage
      project={project}
      deps={{
        fetchTriage: fetchers.triage,
        fetchChat: fetchers.chat,
        post: fetchers.prompt,
        answer: fetchers.answer,
        open: fetchers.openTriage,
        stop: fetchers.stop,
        resume: fetchers.resume,
        fetchScreen: fetchers.screen,
        focus: fetchers.focus,
        abandon: fetchers.abandonTriage,
        fetchEpics: fetchers.epicList,
      }}
    />
  )
}

export interface RouteFetchers {
  abandonTriage?: AbandonTriage
  triage?: FetchTriage
  openTriage?: OpenFlow
  openDesign?: OpenDesign
  deleteDesign?: DeleteDesign
  stop?: StopSession
  resume?: ResumeSession
  screen?: FetchScreen
  mapImages?: FetchMapImages
  ticketScreen?: FetchScreen
  ticket?: FetchDetail
  epic?: FetchEpicDetail
  epicList?: FetchEpicList
  diffFiles?: FetchDiffFiles
  diffPatch?: FetchDiffPatch
  diffCommits?: FetchDiffCommits
  diffCommitPatches?: FetchDiffCommitPatches
  archived?: FetchArchived
  backlog?: FetchBacklog
  initiatives?: FetchInitiatives
  initiative?: FetchInitiative
  startDesign?: PostStartDesign
  finishMove?: PostFinishMove
  schedules?: FetchSchedules
  switchSchedule?: PostSwitch
  scheduleShow?: FetchScheduleShow
  createSchedule?: PostCreate
  editSchedule?: PostEdit
  runNowSchedule?: PostRunNow
  deleteSchedule?: PostDelete
  archivedEpic?: FetchEpicDetail
  checksStatus?: FetchChecksStatus
  saveChecks?: SaveChecks
  stats?: FetchStats
  cleanup?: FetchCleanup
  cleanupWrite?: PostCleanup
  tips?: FetchTips
  chat?: FetchChat
  prompt?: PostPrompt
  answer?: PostAnswer
  focus?: PostFocus
  fixAgent?: FixAgent
  authState?: () => Promise<AuthState>
  logins?: FetchLogins
  settings?: FetchSettings
  signOut?: () => void | Promise<void>
  fileResolve?: FetchFileResolve
  fileText?: FetchFileText
}

// An unknown path lands on /epics, which reports no span.
export const catchAllRoute: RouteObject = {
  path: "*",
  loader: () => {
    noteCatchAll()
    return redirect("/epics")
  },
}

export function appRoutes(fetchers: RouteFetchers = {}): RouteObject[] {
  return [
    { element: <AppShell fetchEpicList={fetchers.epicList} />, children: pageRoutes(fetchers) },
    catchAllRoute,
  ]
}

/** Every page route pattern, so a test can check each one has a normalized name. */
export const pageRoutePatterns = (): string[] =>
  pageRoutes({}).flatMap((r) => (r.path ? [r.path] : []))

function pageRoutes(fetchers: RouteFetchers): RouteObject[] {
  const settingsPage = (
    <SettingsPage
      fetchAuth={fetchers.authState}
      fetchList={fetchers.logins}
      fetchSettings={fetchers.settings}
      fetchEpics={fetchers.epicList}
      signOut={fetchers.signOut}
    />
  )
  return [
    {
      path: "/epics",
      element: (
        <EpicsPage
          fetchList={fetchers.epicList}
          fetchChecks={fetchers.checksStatus}
          saveChecks={fetchers.saveChecks}
        />
      ),
    },
    { path: "/archive", element: <ArchivePage fetchList={fetchers.archived} /> },
    {
      path: "/archive/*",
      element: (
        <EpicRoute readOnly fetchDetail={fetchers.archivedEpic} fetchList={fetchers.epicList} />
      ),
    },
    // "/agents/*" also matches "/agents"; the splat is the session label.
    {
      path: "/agents/*",
      element: <AgentsPage fetchChat={fetchers.chat} fetchScreen={fetchers.screen} />,
    },
    // The splat is the absolute path of the file, without its leading slash.
    {
      path: "/files/*",
      element: <FileViewerPage fetchResolve={fetchers.fileResolve} fetchText={fetchers.fileText} />,
    },
    {
      path: "/stats",
      element: <StatsPage fetchStats={fetchers.stats} fetchEpics={fetchers.epicList} />,
    },
    {
      path: "/cleanup",
      element: (
        <CleanupPage
          fetchCleanup={fetchers.cleanup}
          postCleanup={fetchers.cleanupWrite}
          fetchEpics={fetchers.epicList}
        />
      ),
    },
    {
      path: "/backlog",
      element: (
        <BacklogPage
          fetchList={fetchers.backlog}
          start={fetchers.startDesign}
          finish={fetchers.finishMove}
        />
      ),
    },
    { path: "/initiatives", element: <InitiativesPage fetchList={fetchers.initiatives} /> },
    // An Initiative's address holds a colon (project:slug), so it arrives as the splat.
    { path: "/initiatives/*", element: <InitiativePage fetchDetail={fetchers.initiative} /> },
    {
      path: "/schedules",
      element: <SchedulesPage fetchList={fetchers.schedules} post={fetchers.switchSchedule} />,
    },
    { path: "/schedules/new", element: <ScheduleRoute fetchers={fetchers} /> },
    // A Schedule's address holds a colon (project:slug), so it arrives as the splat.
    { path: "/schedules/*", element: <ScheduleRoute fetchers={fetchers} /> },
    { path: "/rest/break", element: <RestDemoPage kind={RestKindBreak} /> },
    { path: "/rest/bedtime", element: <RestDemoPage kind={RestKindBedtime} /> },
    { path: "/tips", element: <TipsPage fetchTips={fetchers.tips} /> },
    { path: "/settings", element: settingsPage },
    { path: "/settings/s/:section", element: settingsPage },
    { path: "/settings/p/:project", element: settingsPage },
    { path: "/settings/p/:project/s/:section", element: settingsPage },
    { path: "/settings/logins", element: <WebLoginsPage fetchList={fetchers.logins} /> },
    {
      path: "/settings/logins/:id",
      element: <WebLoginPage fetchList={fetchers.logins} signOut={fetchers.signOut} />,
    },
    {
      path: "/settings/password",
      element: <ChangePasswordPage fetchAuth={fetchers.authState} fetchList={fetchers.logins} />,
    },
    {
      path: "/settings/authenticator",
      element: <AuthenticatorPage fetchAuth={fetchers.authState} fetchList={fetchers.logins} />,
    },
    { path: "/settings/certificate", element: <CertificatePage fetchAuth={fetchers.authState} /> },
    { path: "/settings/agents", element: <Navigate to={sectionPath("agents")} replace /> },
    { path: "/settings/*", element: <UnknownSettingsPage /> },
    { path: "/d/*", element: <DesignRoute fetchers={fetchers} /> },
    { path: "/drafts/:id", element: <DraftPage /> },
    { path: "/chat-drafts/:id", element: <ChatDraftPage /> },
    { path: "/triage/*", element: <TriageRoute fetchers={fetchers} /> },
    {
      path: "/t/*",
      element: <TicketRoute fetchDetail={fetchers.ticket} fetchScreen={fetchers.ticketScreen} />,
    },
    {
      path: "/diff",
      element: (
        <DiffPage
          fetchFiles={fetchers.diffFiles}
          fetchPatch={fetchers.diffPatch}
          fetchCommits={fetchers.diffCommits}
          fetchCommitPatches={fetchers.diffCommitPatches}
          fetchEpic={fetchers.epic}
          fetchArchivedEpic={fetchers.archivedEpic}
        />
      ),
    },
    {
      path: "/e/*",
      element: (
        <EpicRoute
          fetchDetail={fetchers.epic}
          fetchList={fetchers.epicList}
          openDesign={fetchers.openDesign}
          fixAgent={fetchers.fixAgent}
        />
      ),
    },
  ]
}

export const createAppRouter = (fetchDetail?: FetchDetail) =>
  createHashRouter(appRoutes({ ticket: fetchDetail }))
