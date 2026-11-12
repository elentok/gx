import { Box, chakra, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { useEpicList } from "../epics/list";
import type { Workspace } from "../generated/server";
import { STATS_HREF } from "../hrefs";
import { GxMenu, type MenuEntry } from "../ui/menu/Menu";
import { Tray } from "../ui/tray/Tray";
import { LimitsTray } from "../usage/LimitsTray";
import { badgeLine, badgeText, badgeTotal, badgeWaiting, inWorkspace } from "../workspaces/followUpsBadge";
import { useDevice } from "../useWide";
import { FooterButton, footerHover, interactiveStyle } from "./FooterButton";
import { GearMenu } from "./GearMenu";
import { openNewAgent, openNewDesign } from "./newItemOpen";
import { useShell } from "./ShellContext";


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

function FooterLink({ href, label, badge, children }: { href: string; label: string; badge?: number; children: ReactNode }) {
  return (
    <FooterButton
      as="a"
      href={href}
      title={label}
      aria-label={badge ? `${label} · ${badge} ${badge === 1 ? "needs" : "need"} you` : label}
      badge={badge ? { tone: "waiting", text: badge } : undefined}
      {...interactiveStyle}
    >
      {children}
    </FooterButton>
  );
}

/** The Epics icon: a menu of the Epics page and the Archive, with the needs-you count as its badge. */
function EpicsMenu({ needsYou }: { needsYou: number }) {
  const navigate = useNavigate();
  const shell = useShell();
  const conn = useConnectionContext();
  const { list } = useEpicList(undefined, conn);
  const go = (path: string) => () => {
    navigate(path);
    shell?.closeSliding();
  };
  return (
    <GxMenu
      placement="top-end"
      trigger={
        <FooterButton
          title="Epics"
          aria-label={needsYou ? `Epics · ${needsYou} ${needsYou === 1 ? "needs" : "need"} you` : "Epics"}
          badge={needsYou ? { tone: "waiting", text: needsYou } : undefined}
          {...interactiveStyle}
        >
          <EpicsIcon />
        </FooterButton>
      }
      items={[
        { value: "epics", label: "Epics", hint: list?.epics?.length, inlineHint: true, onSelect: go("/epics") },
        { value: "archive", label: "Archive", onSelect: go("/archive") },
      ]}
    />
  );
}

function Indicator({ label, tone, children }: { label: string; tone: "waiting" | "paused" | "muted"; children: ReactNode }) {
  return (
    <FooterButton as="div" role="img" title={label} aria-label={label} color={`tone.${tone}.fg`}>
      {children}
    </FooterButton>
  );
}

const InboxIcon = () => icon(<path d="M3 11l2-6h10l2 6v4H3zM3 11h4l1 2h4l1-2h4" />);

/** The inbox button: the summed follow-ups as a badge, and a popover with a line per project. */
function FollowUpsButton({ workspace }: { workspace?: string }) {
  const conn = useConnectionContext();
  const navigate = useNavigate();
  const shell = useShell();
  const { list } = useEpicList(undefined, conn);
  const counts = inWorkspace(list?.follow_ups ?? [], workspace);
  const total = badgeTotal(counts);
  const label = total ? `Follow-ups · ${total} open` : "Follow-ups";
  const lines = counts.map(badgeLine);
  const heading: MenuEntry = {
    kind: "content",
    value: "heading",
    content: (
      <Text px="3" py="1" fontSize="xs" letterSpacing="0.08em" textTransform="uppercase" color="text.muted">
        Follow-ups
      </Text>
    ),
  };
  const items: MenuEntry[] = lines.length
    ? [heading, ...lines.map((l): MenuEntry => ({
        value: l.project,
        label: (
          <Box as="span" display="flex" alignItems="center" gap="3" minW="240px">
            {l.live && <Box as="span" aria-hidden boxSize="16px" borderRadius="full" borderWidth="2px" borderColor="tone.waiting.fg" borderTopColor="hairline" data-testid="triage-ring" />}
            <Text as="span" flex="1">
              {l.project}
            </Text>
            <Text as="span" fontSize="xs" fontWeight="400" color="text.muted">
              {l.text}
            </Text>
            {l.live ? (
              <Text as="span" px="2" borderRadius="6px" borderWidth="1px" borderColor="border" fontSize="xs">
                Open
              </Text>
            ) : (
              <Text as="span" px="2" borderRadius="full" bg="surface" borderWidth="1px" borderColor="border" fontSize="xs">
                {l.count}
              </Text>
            )}
          </Box>
        ),
        onSelect: () => {
          navigate(l.path, l.state && { state: l.state });
          shell?.closeSliding();
        },
      }))]
    : [{ kind: "content", value: "none", content: <Text px="3" py="1" color="text.muted">No follow-ups</Text> }];
  return (
    <GxMenu
      placement="top-end"
      trigger={
        <FooterButton
          title={label}
          aria-label={label}
          badge={total ? { tone: badgeWaiting(counts) ? "waiting" : "running", text: badgeText(total) } : undefined}
          {...interactiveStyle}
        >
          <InboxIcon />
        </FooterButton>
      }
      items={items}
    />
  );
}

export function SidebarFooter({ onNewWorkspace = () => {}, activeWorkspace }: { onNewWorkspace?: () => void; activeWorkspace?: Workspace }) {
  const conn = useConnectionContext();
  const { snapshot, connected } = useConnectionState(conn);
  const health = snapshot?.health;
  const unhealthy = !!health && [health.cpu, health.memory, health.disk].some((m) => m.state === "high" || m.state === "critical");
  const needsYou = snapshot?.needs_you_total ?? 0;
  const desktop = useDevice() === "desktop";
  const indicators = (
    <>
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
    </>
  );
  const anyIndicator = unhealthy || snapshot?.mode === "paused" || !connected;
  return (
    <Box as="footer" data-part="sidebar-footer" flex="none" display="flex" flexDirection="column" gap="1" p="2" borderTopWidth="1px" borderColor="border">
      {/* At the 280px sidebar the optional indicators cannot share the row without shrinking a box, so they take a line above it. */}
      {desktop && anyIndicator && (
        <Box display="flex" flexWrap="wrap" gap="1" data-part="indicators">
          {indicators}
        </Box>
      )}
      {/* Off desktop the boxes keep their size; when the row is too narrow for them all, the indicators and the menu wrap together. */}
      <Box display="flex" alignItems="center" gap="1" flexWrap={desktop ? "nowrap" : "wrap"}>
      <GxMenu
        placement="top-start"
        trigger={
          <chakra.button type="button" h="34px" px="3" flex="none" whiteSpace="nowrap" borderRadius="8px" bg="surface.raised" borderWidth="1px" borderColor="hairline" color="text" cursor="pointer" {...footerHover}>
            + New
          </chakra.button>
        }
        items={[
          { value: "design", label: "New design", onSelect: () => openNewDesign() },
          { value: "agent", label: "New chat", onSelect: openNewAgent },
          { kind: "separator" },
          { value: "workspace", label: "New workspace…", onSelect: onNewWorkspace },
        ]}
      />
      <Box flex="1" />
      <EpicsMenu needsYou={needsYou} />
      <FollowUpsButton workspace={activeWorkspace?.name} />
      <FooterLink href={STATS_HREF} label="Stats and health">
        <StatsIcon />
      </FooterLink>
      {health && <Tray health={health} connected={connected} />}
      <LimitsTray conn={conn} />
      <Box display="flex" alignItems="center" gap="1" flex="none" ml="auto">
        {!desktop && indicators}
        <GearMenu activeWorkspace={activeWorkspace} />
      </Box>
      </Box>
    </Box>
  );
}
