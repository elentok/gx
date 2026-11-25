import { Box, chakra, Popover, Portal, Text } from "@chakra-ui/react"
import { withKeys } from "../uniqueKeys"
import { GxMenu, menuGroups } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { Fragment, useEffect, useRef, useState } from "react"
import { useNavigate } from "react-router"
import { useConnectionContext, useConnectionState } from "../connection"
import type { OpenDesign } from "../design/open"
import { StartDesignButton } from "../design/SessionControls"
import { slugOf } from "../slug"
import { useWide } from "../useWide"
import { verdictText } from "../verdict"
import { hasDesignSession, liveSessions } from "../design/view"
import {
  ActionMerge,
  ActionStashMerge,
  CardActionMarkMerged,
  CardActionOpenPR,
  SectionOnHold,
  type EpicDetailRow,
  type EpicListRow,
} from "../generated/server"
import type { Entity } from "../actions/entity"
import { unarchiveOne } from "../actions/useActions"
import { useEntityActions } from "../workspaces/itemActions"
import { useWorkspaces } from "../workspaces/useActiveWorkspace"
import { BlockedBy } from "./BlockedBy"
import { EpicRing } from "./EpicRing"
import { MergeButton, StashMergeButton } from "../epics/MergeButton"
import { archiveHref, designHref, epicHref } from "../hrefs"
import { ViewDiffLink } from "../diff/ViewDiffLink"
import { formatDate } from "../formatDate"
import { QueueButton } from "../epics/QueueButton"
import { postEpicMarkMerged, postEpicMarkSeen } from "../epics/queue"
import { useEpicList, type EpicList, type FetchEpicList } from "../epics/list"
import { Markdown } from "../markdown/Markdown"
import { DetailPage } from "../page/DetailPage"
import { FileLinkProvider, NoFileLinks } from "../files/FileLinkContext"
import { BackLinkLine } from "../initiatives/BackLinkLine"
import { PageHeader } from "../page/PageHeader"
import { StateHeader } from "../page/StateHeader"
import { PageIcon } from "../page/PageIcon"
import { Badge } from "../ui/badge/Badge"
import { Button } from "../ui/button/Button"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { useDetailReady } from "../useReady"
import { useEpicDetail, type EpicDetail, type FetchEpicDetail } from "./detail"
import type { FixAgent } from "./fixAgent"
import { MoreStats } from "./MoreStats"
import { action, PrActions, PrActivity, PrNotice } from "./PrFlow"
import { TicketCard } from "./TicketCard"
import {
  cardMeta,
  designStatParts,
  doneHeading,
  doneMeta,
  doneRows,
  isFolded,
  loopStatParts,
  nestLive,
  rowContext,
  type RowContext,
} from "./rows"

const DONE_SHOWN = 3

// The done cards under their badge: the newest few, then the rest on request (not remembered).
function DoneGroup({
  rows,
  ctx,
  heads,
  plain,
}: {
  rows: EpicDetailRow[]
  ctx: RowContext
  heads: boolean
  plain?: boolean
}) {
  const [all, setAll] = useState(false)
  if (rows.length === 0) return null
  const done = doneRows(rows)
  const shown = all ? done : done.slice(0, DONE_SHOWN)
  return (
    <Box display="flex" flexDirection="column" gap="8px">
      <Badge data-done-divider alignSelf="flex-start" mt={heads ? "0" : "16px"}>
        {doneHeading(done)}
      </Badge>
      {shown.map((r) => (
        <TicketCard key={r.address} row={r} meta={doneMeta(r, ctx)} indent={0} plain={plain} />
      ))}
      {done.length > DONE_SHOWN && (
        <Button
          tone="muted"
          emphasis="ghost"
          size="sm"
          alignSelf="flex-start"
          onClick={() => setAll(!all)}
        >
          {all ? "Show fewer" : `Show ${done.length - DONE_SHOWN} more`}
        </Button>
      )}
    </Box>
  )
}

function EpicBody({
  detail,
  now,
  designLive,
  fixAgent,
  plain,
}: {
  detail: EpicDetail
  now: number
  designLive: boolean
  fixAgent?: FixAgent
  plain?: boolean
}) {
  const ctx = rowContext(detail, now, designLive)
  const pending = useConnectionState(useConnectionContext()).snapshot?.pending
  return (
    <>
      <Box mt="28px" display="flex" flexDirection="column" gap="8px">
        {nestLive(detail.rows).map(({ row: r, indent }) => (
          <TicketCard
            key={r.address}
            row={r}
            meta={cardMeta(r, ctx)}
            indent={indent}
            plain={plain}
            fixAgent={fixAgent}
            verdict={
              r.holder_unreachable
                ? "holder unreachable, retrying"
                : verdictText(pending, r.address, r.launch_step)
            }
          />
        ))}
      </Box>
      <DoneGroup
        rows={detail.rows.filter(isFolded)}
        ctx={ctx}
        heads={detail.rows.every(isFolded)}
        plain={plain}
      />
    </>
  )
}

// Start while the Design has no session, Open after; neither on a Done Design. Open makes no call.
function DesignSessionButton({ detail, open }: { detail: EpicDetail; open?: OpenDesign }) {
  const navigate = useNavigate()
  const sessions = useConnectionState(useConnectionContext()).snapshot?.sessions ?? []
  if (detail.steps?.find((s) => s.step === "done")?.state === "done") return null
  const goToDesign = () => navigate(designHref(detail.address).slice(1))
  return hasDesignSession(detail.address, sessions) ? (
    <Button tone="running" onClick={goToDesign}>
      Open design session
    </Button>
  ) : (
    <StartDesignButton address={detail.address} open={open} onStarted={goToDesign} />
  )
}

const QUEUE_LOCKED = "Queue unlocks once all tickets are published"

// The stats line stacks under the title in the header.
function StatsLine({
  detail,
  now,
  designLive,
}: {
  detail: EpicDetail
  now: number
  designLive: boolean
}) {
  // The server's wall clock is as of the fetch, so a running epic adds the time since then.
  // A new detail object is the trigger to take a new fetch time.
  const stamp = useRef({ detail, at: Date.now() })
  if (stamp.current.detail !== detail) stamp.current = { detail, at: Date.now() }
  const fetchedAt = stamp.current.at
  const wallClock =
    detail.state === "running" && detail.wall_clock !== undefined
      ? detail.wall_clock + Math.max(0, (now - fetchedAt) / 1000)
      : detail.wall_clock
  const parts =
    detail.kind === "map"
      ? designStatParts(detail, designLive, now)
      : loopStatParts({ ...detail, wall_clock: wallClock })
  return (
    <Text
      data-testid="stats-line"
      fontSize="inherit"
      lineHeight="normal"
      color="text.dim"
      overflow="hidden"
      textOverflow="ellipsis"
    >
      {withKeys(parts, (p) => p.text).map(({ item: p, index: i, key }) => (
        <Fragment key={key}>
          {i > 0 && " · "}
          {p.tone || p.bright ? (
            <Text as="span" color={p.tone ? `tone.${p.tone}.fg` : "text.muted"}>
              {p.text}
            </Text>
          ) : (
            p.text
          )}
        </Fragment>
      ))}
    </Text>
  )
}

// One bar under a hairline holding the buttons, right-aligned. On phones the buttons wrap to
// their own row at a touch-friendly height. A merge-blocked epic's notice sits under the bar, and it has no Merge.
function EpicBar({
  address,
  detail,
  wide,
  partner,
  openDesign,
  onChanged,
  onMarkMerged,
}: {
  address: string
  detail: EpicDetail
  wide: boolean
  partner?: string
  openDesign?: OpenDesign
  onChanged: () => void
  onMarkMerged: { disabled: boolean; run: (anchor: HTMLElement) => void }
}) {
  const navigate = useNavigate()
  const isImpl = address.endsWith("-impl")
  return (
    <>
      <Box display="flex" flexWrap="wrap" alignItems="center" justifyContent="flex-end" gap="3">
        <Box
          display="flex"
          flexWrap="wrap"
          alignItems="center"
          gap="2"
          w={wide ? undefined : "full"}
          css={wide ? undefined : { "& button": { height: "44px", flex: "1" } }}
        >
          {partner && (
            <Button
              tone="muted"
              emphasis="secondary"
              onClick={() => navigate(epicHref(partner).slice(1))}
            >
              {isImpl ? "Open design" : "Open implementation epic"}
            </Button>
          )}
          {detail.draft && isImpl && (
            <Box
              as="span"
              title={QUEUE_LOCKED}
              display="flex"
              css={wide ? undefined : { flex: "1" }}
            >
              <Button tone="running" disabled>
                Queue
              </Button>
            </Box>
          )}
          {detail.shows_view_diff && (
            <ViewDiffLink epic={address} base={detail.diff_base} tip={detail.diff_tip} />
          )}
          {detail.kind === "map" && <DesignSessionButton detail={detail} open={openDesign} />}
          {detail.queueable && (
            <QueueButton address={address} state={detail.state ?? ""} onQueued={onChanged} />
          )}
          {detail.state === "awaiting-merge" && (
            <MergeButton address={address} state={detail.state} onMerged={onChanged} />
          )}
          <PrActions detail={detail} onChanged={onChanged} onMarkMerged={onMarkMerged} />
        </Box>
      </Box>
      {detail.pr_reason && !detail.pr_notice && (
        <Text data-testid="pr-reason" mt="2" fontSize="14px" color="text.dim" textAlign="right">
          {detail.pr_reason}
        </Text>
      )}
      <PrNotice detail={detail} wide={wide} onChanged={onChanged} />
      {mergeBlocked(detail) && (
        <MergeBlockNotice address={address} detail={detail} wide={wide} onChanged={onChanged} />
      )}
    </>
  )
}

// The archived page's bar: the stamp, the link to the Design's -impl archive page, and Unarchive. No other action.
function ArchivedBar({
  address,
  detail,
  unarchive,
}: {
  address: string
  detail: EpicDetail
  unarchive: { disabled: boolean; run: () => void }
}) {
  const navigate = useNavigate()
  return (
    <Box display="flex" flexWrap="wrap" alignItems="center" justifyContent="flex-end" gap="3">
      {detail.archived_at && (
        <Badge data-testid="archived-stamp">{`Archived ${formatDate(detail.archived_at)}`}</Badge>
      )}
      {detail.kind === "map" && detail.impl_address && (
        <Button
          tone="muted"
          emphasis="secondary"
          onClick={() => navigate(archiveHref(detail.impl_address ?? "").slice(1))}
        >
          Open implementation epic
        </Button>
      )}
      {detail.shows_view_diff && (
        <ViewDiffLink epic={address} base={detail.diff_base} tip={detail.diff_tip} />
      )}
      <Button tone="running" disabled={unarchive.disabled} onClick={() => unarchive.run()}>
        Unarchive
      </Button>
    </Box>
  )
}

// The line under the title: the summary on one truncated line. It is a button that opens the full text.
function DescriptionLine({ summary }: { summary: string }) {
  return (
    <Popover.Root lazyMount unmountOnExit positioning={{ placement: "bottom-start", gutter: 8 }}>
      <Popover.Trigger asChild>
        <chakra.button
          type="button"
          aria-label="Show full description"
          display="block"
          maxW="full"
          textAlign="left"
          cursor="pointer"
          overflow="hidden"
          textOverflow="ellipsis"
          whiteSpace="nowrap"
          color="inherit"
        >
          <NoFileLinks>
            <Markdown inline>{summary}</Markdown>
          </NoFileLinks>
        </chakra.button>
      </Popover.Trigger>
      <Portal>
        <Popover.Positioner>
          <Popover.Content
            aria-label="Epic description"
            data-testid="description-popover"
            bg="surface.raised"
            borderWidth="1px"
            borderColor="border"
            borderRadius="12px"
            boxShadow="lg"
            color="text"
            w="min(560px, calc(100vw - 32px))"
            maxH="min(70vh, 480px)"
            overflowY="auto"
            css={{ "--popover-bg": "colors.surface.raised" }}
          >
            <Markdown px="4" py="3" fontSize="15px" lineHeight="1.5">
              {summary}
            </Markdown>
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  )
}

// The page's registry actions: the ⋯ menu, plus the footer buttons that run the same actions.
// The card's own verbs (Move, Hold, Archive) need the active card in a workspace; the detail
// alone gives Open PR, Mark as merged (when the server offers it) and Copy ID.
function useEpicActions(
  address: string,
  detail: EpicDetail | null | undefined,
  card: EpicListRow | undefined,
  onChanged: () => void,
) {
  const { workspaces } = useWorkspaces(useConnectionContext())
  const navigate = useNavigate()
  const wrap = useRef<HTMLSpanElement>(null)
  const markMerged = detail ? action(detail, CardActionMarkMerged) : undefined
  const prLink = detail ? (action(detail, CardActionOpenPR)?.link ?? detail.pr?.url) : undefined
  const entity: Entity | undefined = !detail
    ? undefined
    : {
        kind: "epic",
        key: address,
        name: slugOf(address),
        surface: "page",
        workspace: card?.workspace,
        held: card?.section === SectionOnHold,
        queued: card?.queued ?? detail.queued,
        prUrl: prLink,
        archived: !!detail.archived_at,
      }
  const hide = markMerged ? [] : ["mark-merged"]
  const { groups, get } = useEntityActions(
    entity,
    workspaces,
    {
      hide,
      anchor: () => wrap.current?.querySelector("button"),
    },
    {
      state: detail?.state ?? "",
      writers: {
        markMerged: async (body) => {
          const res = await postEpicMarkMerged({
            ...body,
            address: markMerged?.target ?? body.address,
          })
          if (!res.refused) onChanged()
          return res
        },
        unarchive: async (a) => {
          const res = await unarchiveOne(a)
          if (!res.refused) navigate(epicHref(res.new_address ?? a).slice(1))
          return res
        },
      },
    },
  )
  const fromFooter = (id: string) => (anchor?: HTMLElement) => get(id)?.run(anchor ?? null)
  const menu = (
    <span ref={wrap} style={{ display: "contents" }}>
      <GxMenu trigger={<MoreActionsButton />} items={menuGroups(groups)} />
    </span>
  )
  return {
    menu,
    markMerged: { disabled: get("mark-merged")?.disabled ?? true, run: fromFooter("mark-merged") },
    unarchive: { disabled: get("unarchive")?.disabled ?? true, run: fromFooter("unarchive") },
  }
}

const mergeBlocked = (detail: EpicDetail) => detail.state === "paused" && !!detail.land_block_reason

const DIRTY_TRUNK = "dirty-trunk"

// The merge block's reason with its fix buttons. A dirty trunk says it short and lists every clashing file.
function MergeBlockNotice({
  address,
  detail,
  wide,
  onChanged,
}: {
  address: string
  detail: EpicDetail
  wide: boolean
  onChanged: () => void
}) {
  const state = detail.state ?? ""
  const dirty = detail.land_block_kind === DIRTY_TRUNK
  const reason = dirty ? "Trunk has local changes" : detail.land_block_reason
  const info = (action: string) => ({
    label: detail.action_info?.[action]?.label,
    tooltip: detail.action_info?.[action]?.tooltip,
  })
  // Only the verbs the epic address can run. Start over needs a ticket id, so it is left out.
  const runnable = [ActionStashMerge, ActionMerge].filter((a) => detail.actions?.includes(a))
  return (
    <Box
      role="region"
      aria-label="Merge blocked"
      data-testid="merge-block-notice"
      mt="14px"
      display="flex"
      alignItems="flex-start"
      gap="10px"
      bg="rgba(255,112,102,0.10)"
      borderRadius="10px"
      px="14px"
      py="10px"
    >
      <svg
        aria-hidden="true"
        viewBox="0 0 16 16"
        width="16"
        height="16"
        fill="none"
        stroke="var(--chakra-colors-tone-paused-fg)"
        strokeWidth="1.5"
        style={{ flexShrink: 0, marginTop: 3 }}
      >
        <circle cx="8" cy="8" r="6.5" />
        <path d="M8 7.2v4M8 4.8v.1" strokeLinecap="round" />
      </svg>
      <Box flex="1" fontSize="14px" lineHeight="1.5" color="tone.paused.text">
        <Text data-reason>{`Can't merge: ${reason}`}</Text>
        {dirty && (detail.land_block_files?.length ?? 0) > 0 && (
          <Box as="ul" data-testid="merge-block-files" pl="5" fontFamily="mono">
            {detail.land_block_files?.map((f) => (
              <li key={f}>{f}</li>
            ))}
          </Box>
        )}
        {runnable.length > 0 && (
          <Box
            mt="2"
            display="flex"
            flexWrap="wrap"
            gap="2"
            css={wide ? undefined : { "& button": { height: "44px" } }}
          >
            {runnable.map((a) => {
              const Btn = a === ActionStashMerge ? StashMergeButton : MergeButton
              return (
                <Btn key={a} address={address} state={state} onMerged={onChanged} {...info(a)} />
              )
            })}
          </Box>
        )}
      </Box>
    </Box>
  )
}

type EpicPageProps = {
  address: string
  fetchDetail?: FetchEpicDetail
  openDesign?: OpenDesign
  fetchList?: FetchEpicList
  fixAgent?: FixAgent
  /** The archived epic: read from the archive, one Unarchive action, ticket rows as plain text. */
  readOnly?: boolean
}

// The body reads `useWide`, so it renders inside DetailPage's page mode.
export function EpicPage(props: EpicPageProps) {
  return (
    <DetailPage title={{ kind: "Epic", name: slugOf(props.address) }}>
      {props.readOnly ? <EpicPageBody {...props} list={null} /> : <ActiveEpicPageBody {...props} />}
    </DetailPage>
  )
}

// Only the active page reads the epic list: an archived address can equal an active one.
function ActiveEpicPageBody(props: EpicPageProps) {
  const { list } = useEpicList(props.fetchList, useConnectionContext())
  return <EpicPageBody {...props} list={list} />
}

function EpicPageBody({
  address,
  fetchDetail,
  openDesign,
  fixAgent,
  list,
  readOnly,
}: EpicPageProps & { list: EpicList | null }) {
  const conn = useConnectionContext()
  // Subscribing re-renders the page every tick, which keeps the running clocks counting.
  const sessions = useConnectionState(conn).snapshot?.sessions ?? []
  const wide = useWide()
  const now = Date.now()
  const designLive = liveSessions(address, sessions).length > 0
  const { detail, notFound, source, refetch } = useEpicDetail(address, conn, fetchDetail, readOnly)
  useDetailReady({ detail, notFound, source })
  // The payload has no name field: the epic name is the address after the project.
  const name = slugOf(address)
  // A joined card lists both of its epics; the other one is the Design or its -impl epic.
  const joinedCard = list?.epics?.find((e) => e.addresses?.includes(address))
  const partner = joinedCard?.addresses?.find((a) => a !== address)
  // The card speaks for this page only while its active (first) address is this epic.
  const card = joinedCard?.addresses?.[0] === address ? joinedCard : undefined
  // Opening an unread epic marks it seen. A later change to Needs you or Merged raises it again.
  const unread = !readOnly && !!card?.unread
  useEffect(() => {
    if (unread) void postEpicMarkSeen(address).catch(() => {})
  }, [unread, address])
  const page = useEpicActions(address, detail, card, refetch)

  return (
    <FileLinkProvider base={{ project: address.split(":")[0] }}>
      {notFound ? (
        <>
          <StateHeader title={name} />
          <EmptyState title="Epic not found">{address} does not exist.</EmptyState>
        </>
      ) : detail ? (
        <>
          <PageHeader
            title={name}
            icon={<PageIcon kind="epic" />}
            subtitle={
              <>
                <StatsLine detail={detail} now={now} designLive={designLive} />
                <BackLinkLine link={detail} />
                {detail.summary ? <DescriptionLine summary={detail.summary} /> : null}
              </>
            }
            actions={
              <>
                <EpicRing
                  detail={detail}
                  card={card}
                  sessions={sessions}
                  size={wide ? "header" : "headerSm"}
                />
                {page.menu}
              </>
            }
            meta={
              readOnly ? (
                <ArchivedBar address={address} detail={detail} unarchive={page.unarchive} />
              ) : (
                <EpicBar
                  address={address}
                  detail={detail}
                  wide={wide}
                  partner={partner}
                  openDesign={openDesign}
                  onChanged={refetch}
                  onMarkMerged={page.markMerged}
                />
              )
            }
          />
          <BlockedBy detail={detail} address={address} refetch={refetch} readOnly={readOnly} />
          <PrActivity entries={detail.pr_activity} />
          {detail.total === 0 ? (
            <Text mt="4" color="text.muted">
              No tickets yet
            </Text>
          ) : (
            <EpicBody
              detail={detail}
              now={now}
              designLive={designLive}
              fixAgent={fixAgent}
              plain={readOnly}
            />
          )}
          <MoreStats detail={detail} />
        </>
      ) : (
        <>
          <StateHeader title={name} />
          <FullScreenLoader label="Loading epic" />
        </>
      )}
    </FileLinkProvider>
  )
}
