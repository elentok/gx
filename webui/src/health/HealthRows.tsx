import { Box, Link, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { CauseList as CauseListData, CauseMarker, CauseRow, DisksReport, HealthDisk, HealthReason, HealthState } from "../generated/server";
import { ticketHref } from "../hrefs";
import { STATE_COLOR } from "../ui/tray/stateColor";
import { formatBytes } from "./formatBytes";

const TAG: Partial<Record<HealthState, string>> = { high: "High", critical: "Critical" };

// A disk entry shows its own age once its reading is this much older than the server's clock.
const STALE_SECONDS = 10;

export const CPU_ICON = (
  <>
    <rect x="4.5" y="4.5" width="9" height="9" rx="1.5" />
    <rect x="7.2" y="7.2" width="3.6" height="3.6" rx=".5" />
    <path d="M7 2v2.5M11 2v2.5M7 13.5V16M11 13.5V16M2 7h2.5M2 11h2.5M13.5 7H16M13.5 11H16" />
  </>
);
export const MEMORY_ICON = (
  <>
    <rect x="2" y="5" width="14" height="7" rx="1.2" />
    <path d="M5 8v1.5M8 8v1.5M11 8v1.5M14 8v1.5M4 12v2M7 12v2M11 12v2M14 12v2" />
  </>
);
export const DISK_ICON = (
  <>
    <rect x="2" y="3.5" width="14" height="11" rx="2" />
    <path d="M2 10.5h14M12.5 12.5h.01" />
  </>
);

// Keyed by every non-empty value, so a new reason or marker fails to compile until it has text.
const UNKNOWN_TEXT: Record<Exclude<HealthReason, "">, string> = {
  waiting: "Waiting for a second reading",
  unavailable: "Not available in this build",
  error: "The last read failed",
};
export const unknownText = (reason?: HealthReason) => (reason ? UNKNOWN_TEXT[reason] : "No reading yet");
// The disk meter has no second reading to wait for.
const diskUnknownText = (reason?: HealthReason) => (reason === "waiting" ? "No disk reading yet" : unknownText(reason));

const CAUSE_MARKER_TEXT: Record<Exclude<CauseMarker, "">, string> = { unavailable: "processes unavailable", reading: "reading…" };

export function Row({
  part,
  icon,
  name,
  state,
  value,
  percent,
  sub,
  tagText,
  children,
}: {
  part: string;
  icon: ReactNode;
  name: string;
  state: HealthState;
  value: string;
  /** Omitted when the row's bars are in its children. */
  percent?: number;
  /** Omitted when the row has no note under it. */
  sub?: string;
  /** Replaces the state's default tag wording. */
  tagText?: Partial<Record<HealthState, string>>;
  children?: ReactNode;
}) {
  const color = STATE_COLOR[state];
  const tag = (tagText ?? TAG)[state];
  // Only high and critical tint the icon and value; normal keeps the mock's neutral text.
  const raised = tag !== undefined;
  return (
    <Box
      px="14px"
      py="2"
      borderTopWidth="1px"
      borderColor="hairline"
      display="grid"
      gridTemplateColumns="18px 1fr auto"
      gap="2px 10px"
      alignItems="center"
      data-part={part}
      data-state={state}
    >
      <Box aria-hidden color={raised ? color.fg : "text.dim"} display="flex">
        <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round">
          {icon}
        </svg>
      </Box>
      <Text as="span" fontWeight="500">
        {name}
        {tag && (
          <Text as="span" ml="6px" px="6px" fontSize="11px" borderRadius="full" bg={color.bg} color={color.fg}>
            {tag}
          </Text>
        )}
      </Text>
      <Text as="span" textAlign="right" fontWeight="500" fontVariantNumeric="tabular-nums" color={raised ? color.fg : undefined}>
        {value}
      </Text>
      {percent !== undefined && <Bar percent={percent} color={color.fg} />}
      {children}
      {sub && (
        <Text as="span" gridColumn="2 / 4" fontSize="xs" lineHeight="1.45" color="text.dim">
          {sub}
        </Text>
      )}
    </Box>
  );
}

function Bar({ percent, color, part }: { percent: number; color: string; part?: string }) {
  const clamped = Math.min(Math.max(percent, 0), 100);
  return (
    <Box gridColumn="2 / 4" h="4px" borderRadius="2px" bg="hairline" overflow="hidden" mt="3px" mb="1px" data-part={part} data-percent={Math.round(clamped)}>
      <Box h="100%" w={`${clamped}%`} bg={color} />
    </Box>
  );
}

export function CauseList({ part, list, value }: { part: string; list: CauseListData; value: (row: CauseRow) => string }) {
  const marker = list.marker ? CAUSE_MARKER_TEXT[list.marker] : undefined;
  return (
    <Box px="14px" pb="2" fontSize="xs" lineHeight="1.45" color="text.dim" data-part={part}>
      {marker ??
        list.rows.map((row, i) => (
          <Box key={i} display="flex" gap="2" data-part="cause-row">
            <Text as="span" flex="1" minW="0" overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap" data-part="cause-label">
              {row.ticket ? <Link href={ticketHref(row.ticket)} color="text.dim">{row.label}</Link> : row.label}
              {row.ticket && row.busiest && (
                <>
                  {" · "}
                  <Text as="span" color="text.faint" data-part="cause-busiest">
                    {row.busiest}
                  </Text>
                </>
              )}
            </Text>
            <Text as="span" textAlign="right" fontVariantNumeric="tabular-nums" data-part="cause-value">
              {value(row)}
            </Text>
          </Box>
        ))}
    </Box>
  );
}

// The server picks the worst disk, so the row and the Tray's D bar agree.
export function DiskRow({ disks, serverTime }: { disks: DisksReport; serverTime: string }) {
  const list = disks.disks;
  const worst = list.find((d) => d.id === disks.worst_id);
  // With every project on one disk, its project list names them all and says nothing.
  const showProjects = list.filter((d) => d.projects.length > 0).length > 1;
  return (
    <Row
      part="disk"
      icon={DISK_ICON}
      name="Disk"
      state={disks.state}
      value={worst && disks.state !== "unknown" ? `${formatBytes(worst.free_bytes, "decimal")}G free` : "—"}
      sub={disks.state === "unknown" ? diskUnknownText(disks.reason) : undefined}
      tagText={{ high: "Under warn", critical: "Under clean" }}
    >
      <Box gridColumn="2 / 4" display="flex" flexDirection="column" gap="6px" mt="4px">
        {list.map((d) => (
          <DiskEntry key={d.id} disk={d} serverTime={serverTime} showProjects={showProjects} />
        ))}
      </Box>
    </Row>
  );
}

function DiskEntry({ disk, serverTime, showProjects }: { disk: HealthDisk; serverTime: string; showProjects: boolean }) {
  const color = STATE_COLOR[disk.state].fg;
  const unknown = disk.state === "unknown";
  const ageSeconds = Math.floor((Date.parse(serverTime) - Date.parse(disk.read_at)) / 1000);
  const projects = disk.projects.join(", ");
  const usedPercent = disk.total_bytes > 0 ? (1 - disk.free_bytes / disk.total_bytes) * 100 : 0;
  return (
    <Box data-part="disk-entry" data-state={disk.state}>
      <Box display="flex" justifyContent="space-between" gap="2" fontSize="13px">
        <Text as="span" fontSize="12px" color="text.muted" overflowWrap="anywhere">
          {disk.name}
        </Text>
        <Text as="span" fontVariantNumeric="tabular-nums" color={color}>
          {unknown ? "—" : `${formatBytes(disk.free_bytes, "decimal")}G free`}
        </Text>
      </Box>
      {!unknown && <Bar part="disk-bar" percent={usedPercent} color={color} />}
      {(unknown || showProjects) && (
        <Text fontSize="xs" lineHeight="1.45" color="text.dim" data-part="disk-projects">
          {unknown ? "No reading for this disk" : projects}
        </Text>
      )}
      {disk.held && (
        <Text fontSize="xs" lineHeight="1.45" color={color}>
          {`Dispatch held for ${projects}. Clean now is in the banner.`}
        </Text>
      )}
      {ageSeconds > STALE_SECONDS && (
        <Text fontSize="xs" color="text.faint">
          {`read ${ageSeconds}s ago`}
        </Text>
      )}
    </Box>
  );
}
