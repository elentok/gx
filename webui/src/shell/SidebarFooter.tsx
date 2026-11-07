import { Box, chakra, Portal, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { useNavigate } from "react-router";
import { fetchWorktrees, postCreateSession } from "../agents/newSession";
import { NewSessionForm } from "../agents/NewSessionForm";
import { useConnectionContext, useConnectionState } from "../connection";
import { NewDesignForm } from "../design/NewDesignForm";
import { useEpicList } from "../epics/list";
import type { Workspace } from "../generated/server";
import { designHref, STATS_HREF } from "../hrefs";
import { GxMenu } from "../ui/menu/Menu";
import { Tray } from "../ui/tray/Tray";
import { LimitsTray } from "../usage/LimitsTray";
import { GearMenu } from "./GearMenu";

const EPICS_HREF = "#/epics";

const icon = (children: ReactNode) => (
  <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
    {children}
  </svg>
);
const EpicsIcon = () => icon(<path d="M3.5 3.5h5v5h-5zM11.5 3.5h5v5h-5zM3.5 11.5h5v5h-5zM11.5 11.5h5v5h-5z" />);
const StatsIcon = () => icon(<path d="M4 16V9M10 16V4M16 16v-5" />);
const WarningIcon = () => icon(<path d="M10 3 18 17H2L10 3ZM10 8v4M10 14.5v.01" />);
const PausedIcon = () => icon(<path d="M7 5v10M13 5v10" />);
const OfflineIcon = () => icon(<path d="M3 3l14 14M5 9a7 7 0 0 1 4-2M15 9a7 7 0 0 0-2-1.5M8 12.5a3 3 0 0 1 4 0M10 16v.01" />);

const buttonStyle = {
  position: "relative",
  boxSize: "34px",
  borderRadius: "8px",
  display: "grid",
  placeItems: "center",
  color: "text.dim",
  cursor: "pointer",
  _hover: { bg: "surface.raised" },
} as const;

function FooterLink({ href, label, badge, children }: { href: string; label: string; badge?: number; children: ReactNode }) {
  return (
    <chakra.a href={href} title={label} aria-label={badge ? `${label} · ${badge} ${badge === 1 ? "needs" : "need"} you` : label} {...buttonStyle}>
      {children}
      {badge ? (
        <Text as="span" aria-hidden position="absolute" top="0" right="0" px="1.5" lineHeight="1.4" borderRadius="full" bg="tone.waiting.fg" color="surface" fontSize="xs" fontWeight="600">
          {badge}
        </Text>
      ) : null}
    </chakra.a>
  );
}

function Indicator({ label, tone, children }: { label: string; tone: "waiting" | "paused" | "muted"; children: ReactNode }) {
  return (
    <Box role="img" title={label} aria-label={label} color={`tone.${tone}.fg`} boxSize="34px" display="grid" placeItems="center">
      {children}
    </Box>
  );
}

type Dialog = "design" | "agent" | null;

export function SidebarFooter({ onNewWorkspace = () => {}, activeWorkspace }: { onNewWorkspace?: () => void; activeWorkspace?: Workspace }) {
  const conn = useConnectionContext();
  const { snapshot, connected } = useConnectionState(conn);
  const [dialog, setDialog] = useState<Dialog>(null);
  const health = snapshot?.health;
  const unhealthy = !!health && [health.cpu, health.memory, health.disk].some((m) => m.state === "high" || m.state === "critical");
  const needsYou = snapshot?.needs_you_total ?? 0;
  return (
    <Box as="footer" flex="none" display="flex" alignItems="center" gap="1" p="2" borderTopWidth="1px" borderColor="border">
      <GxMenu
        placement="top-start"
        trigger={
          <chakra.button type="button" h="34px" px="3" borderRadius="8px" bg="surface.raised" borderWidth="1px" borderColor="hairline" color="text" cursor="pointer">
            + New
          </chakra.button>
        }
        items={[
          { value: "design", label: "New design", onSelect: () => setDialog("design") },
          { value: "agent", label: "New chat", onSelect: () => setDialog("agent") },
          { kind: "separator" },
          { value: "workspace", label: "New workspace…", onSelect: onNewWorkspace },
        ]}
      />
      <Box flex="1" />
      <FooterLink href={EPICS_HREF} label="Epics" badge={needsYou}>
        <EpicsIcon />
      </FooterLink>
      <FooterLink href={STATS_HREF} label="Stats and health">
        <StatsIcon />
      </FooterLink>
      {health && (
        <>
          <Tray health={health} connected={connected} />
          <LimitsTray conn={conn} />
        </>
      )}
      {unhealthy && (
        <Indicator label="System health is high or critical" tone="waiting">
          <WarningIcon />
        </Indicator>
      )}
      {snapshot?.mode === "paused" && (
        <Indicator label="Server queue is paused" tone="paused">
          <PausedIcon />
        </Indicator>
      )}
      {!connected && (
        <Indicator label="Disconnected from the server" tone="muted">
          <OfflineIcon />
        </Indicator>
      )}
      <GearMenu activeWorkspace={activeWorkspace} />
      <Portal>
        {dialog === "design" && <DesignDialog onClose={() => setDialog(null)} />}
        {dialog === "agent" && <AgentDialog onClose={() => setDialog(null)} />}
      </Portal>
    </Box>
  );
}

function DesignDialog({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate();
  const { list } = useEpicList();
  const projects = [...new Set([...(list?.projects ?? []), ...(list?.epics ?? []).map((e) => e.project)])].sort();
  return (
    <NewDesignForm
      projects={projects}
      onStarted={(address, startFailure) => {
        onClose();
        navigate(designHref(address).slice(1), { state: { startFailure } });
      }}
      onClose={onClose}
    />
  );
}

function AgentDialog({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate();
  return (
    <NewSessionForm
      fetchTrees={fetchWorktrees}
      create={postCreateSession}
      onClose={onClose}
      onCreated={(id) => {
        onClose();
        navigate(`/agents/${id}`);
      }}
    />
  );
}
