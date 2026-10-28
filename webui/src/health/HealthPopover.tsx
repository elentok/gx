import { Box, Text } from "@chakra-ui/react";
import { useEffect, useState, type ReactNode } from "react";
import type { DisksReport, Health, HealthDisk, HealthReason, HealthReport, HealthState, HealthThresholds } from "../generated/server";
import { readPageCache, writePageCache } from "../pageCache";
import { fetchJson } from "../ticket/detail";
import { STATE_COLOR } from "../ui/tray/stateColor";

const POLL_MS = 5000;
const GIB = 1024 ** 3;

const TAG: Partial<Record<HealthState, string>> = { high: "High", critical: "Critical" };

const CPU_ICON = (
  <>
    <rect x="4.5" y="4.5" width="9" height="9" rx="1.5" />
    <rect x="7.2" y="7.2" width="3.6" height="3.6" rx=".5" />
    <path d="M7 2v2.5M11 2v2.5M7 13.5V16M11 13.5V16M2 7h2.5M2 11h2.5M13.5 7H16M13.5 11H16" />
  </>
);
const MEMORY_ICON = (
  <>
    <rect x="2" y="5" width="14" height="7" rx="1.2" />
    <path d="M5 8v1.5M8 8v1.5M11 8v1.5M14 8v1.5M4 12v2M7 12v2M11 12v2M14 12v2" />
  </>
);

// A report with the browser's clock at arrival, so the footer counts up without trusting that clock.
interface Received {
  report: HealthReport;
  at: number;
}

const gb = (bytes: number) => {
  const n = bytes / GIB;
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
};

// A disk entry shows its own age once its reading is this much older than the server's clock.
const STALE_SECONDS = 10;

const DISK_ICON = (
  <>
    <rect x="2" y="3.5" width="14" height="11" rx="2" />
    <path d="M2 10.5h14M12.5 12.5h.01" />
  </>
);

const UNKNOWN_TEXT: Record<string, string> = {
  waiting: "Waiting for a second reading",
  unavailable: "Not available in this build",
  error: "The last read failed",
};
const unknownText = (reason?: HealthReason) => UNKNOWN_TEXT[reason ?? ""] ?? "No reading yet";

// A bare "G" in the disk thresholds is decimal, so free space uses it too.
const diskGB = (bytes: number) => {
  const n = bytes / 1e9;
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
};

const thresholdText = (t: HealthThresholds) => `high at ${t.high}%, critical at ${t.critical}%`;

const CACHE_KEY = "health";

const METER_NAMES = [
  { key: "cpu", name: "CPU", icon: CPU_ICON },
  { key: "memory", name: "Memory", icon: MEMORY_ICON },
  { key: "disk", name: "Disk", icon: DISK_ICON },
] as const;

export function HealthPopover({ health, connected = true }: { health: Health; connected?: boolean }) {
  // The cache survives a reload, so an offline reload still has values.
  const [received, setReceived] = useState<Received | null>(() => readPageCache<Received>(CACHE_KEY));
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(tick);
  }, []);

  // Offline stops the polling; reconnecting runs this again, which fetches at once.
  useEffect(() => {
    if (!connected) return;
    let live = true;
    const load = () =>
      fetchJson<HealthReport>("/v1/health", "health")
        .then((res) => {
          if (!live || res.kind !== "ok") return;
          const next = { report: res.detail, at: Date.now() };
          writePageCache(CACHE_KEY, next);
          setReceived(next);
        })
        .catch(() => {});
    load();
    const poll = setInterval(load, POLL_MS);
    return () => {
      live = false;
      clearInterval(poll);
    };
  }, [connected]);

  if (!received) return connected ? null : <SnapshotRows health={health} />;
  const { report, at } = received;
  const { cpu, memory } = report;
  const pressure = memory.pressure ? ` · macOS pressure: ${memory.pressure}` : "";
  return (
    <Box py="1">
      <Box px="14px" pt="2" pb="6px" display="flex" justifyContent="space-between" alignItems="baseline" gap="2">
        <Text as="b" fontFamily="condensed" fontSize="15px" textTransform="uppercase" letterSpacing="0.05em" fontWeight="600" color="text.muted">
          gx server
        </Text>
        <Text as="span" fontSize="xs" color="text.dim">
          {report.host}
        </Text>
      </Box>
      <Row
        part="cpu"
        icon={CPU_ICON}
        name="CPU"
        state={cpu.state}
        value={cpu.state === "unknown" ? "—" : `${Math.round(cpu.percent)}% busy`}
        percent={cpu.state === "unknown" ? 0 : cpu.percent}
        sub={
          cpu.state === "unknown"
            ? unknownText(cpu.reason)
            : `1-minute average ${Math.round(cpu.average)}% · ${thresholdText(cpu.thresholds)}`
        }
      />
      <Row
        part="memory"
        icon={MEMORY_ICON}
        name="Memory"
        state={memory.state}
        value={memory.state === "unknown" ? "—" : `${Math.round(memory.percent)}% used`}
        percent={memory.state === "unknown" ? 0 : memory.percent}
        sub={
          memory.state === "unknown"
            ? unknownText(memory.reason)
            : `${gb(memory.used_bytes)} of ${gb(memory.total_bytes)} GB${pressure} · ${thresholdText(memory.thresholds)}`
        }
      />
      <DiskRow disks={report.disks} serverTime={report.time} />
      <Text px="14px" pt="2" pb="6px" borderTopWidth="1px" borderColor="hairline" fontSize="xs" color="text.faint" data-part="footer">
        {connected ? footer(report, at, now) : offlineFooter(at, now)}
      </Text>
    </Box>
  );
}

// The values are as old as the server's last response, plus the browser time since it arrived.
function offlineFooter(receivedAt: number, now: number): string {
  const seconds = Math.max(0, Math.floor((now - receivedAt) / 1000));
  return `Offline. Values from ${seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m`} ago.`;
}

// Offline with nothing cached: the Tray's snapshot states are all there is.
function SnapshotRows({ health }: { health: Health }) {
  return (
    <Box py="1">
      {METER_NAMES.map(({ key, name, icon }) => (
        <Row key={key} part={key} icon={icon} name={name} state={health[key].state} value="—" sub={`State: ${health[key].state}`} />
      ))}
      <Text px="14px" pt="2" pb="6px" borderTopWidth="1px" borderColor="hairline" fontSize="xs" color="text.faint" data-part="footer">
        Offline. No values were fetched.
      </Text>
    </Box>
  );
}

// The age of the older of the CPU and memory readings at the server's clock, plus the time since it arrived.
function footer(report: HealthReport, receivedAt: number, now: number): string {
  const readAt = Math.min(Date.parse(report.cpu.read_at), Date.parse(report.memory.read_at));
  const age = Date.parse(report.time) - readAt + (now - receivedAt);
  return `Updated ${Math.max(0, Math.floor(age / 1000))}s ago`;
}

function Row({
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
  sub: string;
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
      <Box aria-hidden color={raised ? color : "text.dim"} display="flex">
        <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round">
          {icon}
        </svg>
      </Box>
      <Text as="span" fontWeight="500">
        {name}
        {tag && (
          <Text as="span" ml="6px" px="6px" fontSize="11px" borderRadius="full" bg={color.replace(".fg", ".bg")} color={color}>
            {tag}
          </Text>
        )}
      </Text>
      <Text as="span" textAlign="right" fontWeight="500" fontVariantNumeric="tabular-nums" color={raised ? color : undefined}>
        {value}
      </Text>
      {percent !== undefined && <Bar percent={percent} color={color} />}
      {children}
      <Text as="span" gridColumn="2 / 4" fontSize="xs" lineHeight="1.45" color="text.dim">
        {sub}
      </Text>
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

// Orders states for picking a row's worst disk.
const SEVERITY: Record<HealthState, number> = { unknown: 0, normal: 1, high: 2, critical: 3 };

function DiskRow({ disks, serverTime }: { disks: DisksReport; serverTime: string }) {
  const list = disks.disks;
  const read = list.filter((d) => d.state !== "unknown");
  const worst = read.reduce<HealthDisk | undefined>((w, d) => (!w || d.free_bytes < w.free_bytes ? d : w), undefined);
  const state = list.reduce<HealthState>((s, d) => (SEVERITY[d.state] > SEVERITY[s] ? d.state : s), "unknown");
  return (
    <Row
      part="disk"
      icon={DISK_ICON}
      name="Disk"
      state={state}
      value={worst ? `${diskGB(worst.free_bytes)}G free` : "—"}
      sub={`warns under ${disks.warn_below}, cleans under ${disks.clean_below}`}
      tagText={{ high: "Under warn", critical: "Under clean" }}
    >
      <Box gridColumn="2 / 4" display="flex" flexDirection="column" gap="6px" mt="4px">
        {list.map((d) => (
          <DiskEntry key={d.id} disk={d} serverTime={serverTime} />
        ))}
      </Box>
    </Row>
  );
}

function DiskEntry({ disk, serverTime }: { disk: HealthDisk; serverTime: string }) {
  const color = STATE_COLOR[disk.state];
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
          {unknown ? "—" : `${diskGB(disk.free_bytes)}G free`}
        </Text>
      </Box>
      {!unknown && <Bar part="disk-bar" percent={usedPercent} color={color} />}
      <Text fontSize="xs" lineHeight="1.45" color="text.dim">
        {unknown ? "No reading for this disk" : projects}
      </Text>
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
