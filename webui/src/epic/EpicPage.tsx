import { Box, Text } from "@chakra-ui/react";
import { Fragment, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import type { OpenDesign } from "../design/open";
import { StartDesignButton } from "../design/SessionControls";
import { slugOf } from "../slug";
import { useWide } from "../useWide";
import { verdictText } from "../verdict";
import { hasDesignSession, liveSessions } from "../design/view";
import { ActionMerge, ActionStashMerge, type EpicDetailRow } from "../generated/server";
import { EpicRing } from "./EpicRing";
import { MergeButton, StashMergeButton } from "../epics/MergeButton";
import { designHref, epicHref } from "../hrefs";
import { QueueButton } from "../epics/QueueButton";
import { useEpicList, waitingCount, type EpicList, type FetchEpicList } from "../epics/list";
import { JumpMenu } from "../nav/JumpMenu";
import { Markdown } from "../markdown/Markdown";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { Badge } from "../ui/badge/Badge";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import { useEpicDetail, type EpicDetail, type FetchEpicDetail } from "./detail";
import type { FixAgent } from "./fixAgent";
import { MoreStats } from "./MoreStats";
import { TicketCard } from "./TicketCard";
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
} from "./rows";

const DONE_SHOWN = 3;

// The done cards under their badge: the newest few, then the rest on request (not remembered).
function DoneGroup({ rows, ctx, heads }: { rows: EpicDetailRow[]; ctx: RowContext; heads: boolean }) {
  const [all, setAll] = useState(false);
  if (rows.length === 0) return null;
  const done = doneRows(rows);
  const shown = all ? done : done.slice(0, DONE_SHOWN);
  return (
    <Box display="flex" flexDirection="column" gap="8px">
      <Badge data-done-divider alignSelf="flex-start" mt={heads ? "0" : "16px"}>
        {doneHeading(done)}
      </Badge>
      {shown.map((r) => (
        <TicketCard key={r.address} row={r} meta={doneMeta(r, ctx)} indent={0} />
      ))}
      {done.length > DONE_SHOWN && (
        <Button tone="muted" emphasis="ghost" size="sm" alignSelf="flex-start" onClick={() => setAll(!all)}>
          {all ? "Show fewer" : `Show ${done.length - DONE_SHOWN} more`}
        </Button>
      )}
    </Box>
  );
}

function EpicBody({ detail, now, designLive, fixAgent }: { detail: EpicDetail; now: number; designLive: boolean; fixAgent?: FixAgent }) {
  const ctx = rowContext(detail, now, designLive);
  const pending = useConnectionState(useConnectionContext()).snapshot?.pending;
  return (
    <>
      <Box mt="28px" display="flex" flexDirection="column" gap="8px">
        {nestLive(detail.rows).map(({ row: r, indent }) => (
          <TicketCard key={r.address} row={r} meta={cardMeta(r, ctx)} indent={indent} fixAgent={fixAgent} verdict={verdictText(pending, r.address)} />
        ))}
      </Box>
      <DoneGroup rows={detail.rows.filter(isFolded)} ctx={ctx} heads={detail.rows.every(isFolded)} />
    </>
  );
}

// Start while the Design has no session, Open after; neither on a Done Design. Open makes no call.
function DesignSessionButton({ detail, open }: { detail: EpicDetail; open?: OpenDesign }) {
  const navigate = useNavigate();
  const sessions = useConnectionState(useConnectionContext()).snapshot?.sessions ?? [];
  if (detail.steps?.find((s) => s.step === "done")?.state === "done") return null;
  const goToDesign = () => navigate(designHref(detail.address).slice(1));
  return hasDesignSession(detail.address, sessions) ? (
    <Button tone="running" onClick={goToDesign}>
      Open design session
    </Button>
  ) : (
    <StartDesignButton address={detail.address} open={open} onStarted={goToDesign} />
  );
}

const QUEUE_LOCKED = "Queue unlocks once all tickets are published";

// One bar under a hairline: the stats line on the left, the buttons on the right. On phones the buttons wrap to
// their own row at a touch-friendly height. A merge-blocked epic's notice sits under the bar, and it has no Merge.
function EpicBar({
  address,
  detail,
  now,
  wide,
  designLive,
  partner,
  openDesign,
  onChanged,
}: {
  address: string;
  detail: EpicDetail;
  now: number;
  wide: boolean;
  designLive: boolean;
  partner?: string;
  openDesign?: OpenDesign;
  onChanged: () => void;
}) {
  const navigate = useNavigate();
  const isImpl = address.endsWith("-impl");
  // The server's wall clock is as of the fetch, so a running epic adds the time since then.
  const fetchedAt = useMemo(() => Date.now(), [detail]);
  const wallClock =
    detail.state === "running" && detail.wall_clock !== undefined
      ? detail.wall_clock + Math.max(0, (now - fetchedAt) / 1000)
      : detail.wall_clock;
  const parts =
    detail.kind === "map"
      ? designStatParts(detail, designLive, now)
      : loopStatParts({ ...detail, wall_clock: wallClock });
  return (
    <Box mt={wide ? "14px" : "12px"} borderTopWidth="1px" borderColor="hairline" pt={wide ? "14px" : "12px"}>
      <Box display="flex" flexWrap="wrap" alignItems="center" justifyContent="space-between" gap="3">
        <Text data-testid="stats-line" fontSize="14px" lineHeight="normal" color="text.dim">
          {parts.map((p, i) => (
            <Fragment key={i}>
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
        <Box
          display="flex"
          flexWrap="wrap"
          alignItems="center"
          gap="2"
          w={wide ? undefined : "full"}
          css={wide ? undefined : { "& button": { height: "44px" } }}
        >
          {partner && (
            <Button tone="muted" emphasis="secondary" onClick={() => navigate(epicHref(partner).slice(1))}>
              {isImpl ? "Open design" : "Open implementation epic"}
            </Button>
          )}
          {detail.draft && isImpl && (
            <Box as="span" title={QUEUE_LOCKED}>
              <Button tone="running" disabled>
                Queue
              </Button>
            </Box>
          )}
          {detail.kind === "map" && <DesignSessionButton detail={detail} open={openDesign} />}
          {detail.queueable && <QueueButton address={address} state={detail.state ?? ""} onQueued={onChanged} />}
          {detail.state === "awaiting-merge" && (
            <MergeButton address={address} state={detail.state} onMerged={onChanged} />
          )}
        </Box>
      </Box>
      {mergeBlocked(detail) && <MergeBlockNotice address={address} detail={detail} wide={wide} onChanged={onChanged} />}
    </Box>
  );
}

const mergeBlocked = (detail: EpicDetail) => detail.state === "paused" && !!detail.land_block_reason;

const DIRTY_TRUNK = "dirty-trunk";

// The merge block's reason with its fix buttons. A dirty trunk says it short and lists every clashing file.
function MergeBlockNotice({
  address,
  detail,
  wide,
  onChanged,
}: {
  address: string;
  detail: EpicDetail;
  wide: boolean;
  onChanged: () => void;
}) {
  const state = detail.state ?? "";
  const dirty = detail.land_block_kind === DIRTY_TRUNK;
  const reason = dirty ? "Trunk has local changes" : detail.land_block_reason;
  const info = (action: string) => ({ label: detail.action_info?.[action]?.label, tooltip: detail.action_info?.[action]?.tooltip });
  // Only the verbs the epic address can run. Start over needs a ticket id, so it is left out.
  const runnable = [ActionStashMerge, ActionMerge].filter((a) => detail.actions?.includes(a));
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
          <Box mt="2" display="flex" flexWrap="wrap" gap="2" css={wide ? undefined : { "& button": { height: "44px" } }}>
            {runnable.map((a) => {
              const Btn = a === ActionStashMerge ? StashMergeButton : MergeButton;
              return <Btn key={a} address={address} state={state} onMerged={onChanged} {...info(a)} />;
            })}
          </Box>
        )}
      </Box>
    </Box>
  );
}

type EpicPageProps = {
  address: string;
  fetchDetail?: FetchEpicDetail;
  openDesign?: OpenDesign;
  fetchList?: FetchEpicList;
  fixAgent?: FixAgent;
};

// The body reads `useWide`, so it renders inside DetailPage's page mode.
export function EpicPage(props: EpicPageProps) {
  const { list } = useEpicList(props.fetchList, useConnectionContext());
  return (
    <DetailPage
      mode="narrow"
      title={{ kind: "Epic", name: slugOf(props.address) }}
      waiting={list ? waitingCount(list.epics ?? [], props.address) : 0}
    >
      <EpicPageBody {...props} list={list} />
    </DetailPage>
  );
}

function EpicPageBody({ address, fetchDetail, openDesign, fixAgent, list }: EpicPageProps & { list: EpicList | null }) {
  const conn = useConnectionContext();
  // Subscribing re-renders the page every tick, which keeps the running clocks counting.
  const sessions = useConnectionState(conn).snapshot?.sessions ?? [];
  const wide = useWide();
  const now = Date.now();
  const designLive = liveSessions(address, sessions).length > 0;
  const { detail, notFound, refetch } = useEpicDetail(address, conn, fetchDetail);
  // The payload has no name field: the epic name is the address after the project.
  const name = slugOf(address);
  // A joined card lists both of its epics; the other one is the Design or its -impl epic.
  const joinedCard = list?.epics?.find((e) => e.addresses?.includes(address));
  const partner = joinedCard?.addresses?.find((a) => a !== address);
  // The card speaks for this page only while its active (first) address is this epic.
  const card = joinedCard?.addresses?.[0] === address ? joinedCard : undefined;
  const summary = detail?.summary && (
    <Markdown mt={wide ? "1" : "12px"} fontSize="15px" lineHeight="1.5" color="text.muted">
      {detail.summary}
    </Markdown>
  );

  return (
    <>
      {notFound ? (
        <EmptyState title="Epic not found">{address} does not exist.</EmptyState>
      ) : detail ? (
        <>
          <PageHeader
            title={name}
            lead={<EpicRing detail={detail} card={card} sessions={sessions} size={wide ? "md" : "sm"} />}
            heading={
              <>
                <JumpMenu name={name} current={address} epics={list?.epics ?? []} />
                {wide && summary}
              </>
            }
          >
            {!wide && summary}
          </PageHeader>
          <EpicBar
            address={address}
            detail={detail}
            now={now}
            wide={wide}
            designLive={designLive}
            partner={partner}
            openDesign={openDesign}
            onChanged={refetch}
          />
          {detail.total === 0 ? (
            <Text mt="4" color="text.muted">
              No tickets yet
            </Text>
          ) : (
            <EpicBody detail={detail} now={now} designLive={designLive} fixAgent={fixAgent} />
          )}
          <MoreStats detail={detail} />
        </>
      ) : (
        <Spinner label="Loading epic" />
      )}
    </>
  );
}
