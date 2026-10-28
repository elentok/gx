import { Box, Text } from "@chakra-ui/react";
import { useEffect, useState, type ReactNode } from "react";
import type { HealthReport, HealthState, HealthThresholds } from "../generated/server";
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

const thresholdText = (t: HealthThresholds) => `high at ${t.high}%, critical at ${t.critical}%`;

export function HealthPopover() {
  const [received, setReceived] = useState<Received | null>(null);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    let live = true;
    const load = () =>
      fetchJson<HealthReport>("/v1/health", "health")
        .then((res) => live && res.kind === "ok" && setReceived({ report: res.detail, at: Date.now() }))
        .catch(() => {});
    load();
    const poll = setInterval(load, POLL_MS);
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => {
      live = false;
      clearInterval(poll);
      clearInterval(tick);
    };
  }, []);

  if (!received) return null;
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
        value={`${Math.round(cpu.percent)}% busy`}
        percent={cpu.percent}
        sub={`1-minute average ${Math.round(cpu.average)}% · ${thresholdText(cpu.thresholds)}`}
      />
      <Row
        part="memory"
        icon={MEMORY_ICON}
        name="Memory"
        state={memory.state}
        value={`${Math.round(memory.percent)}% used`}
        percent={memory.percent}
        sub={`${gb(memory.used_bytes)} of ${gb(memory.total_bytes)} GB${pressure} · ${thresholdText(memory.thresholds)}`}
      />
      <Box data-part="disk-slot" />
      <Text px="14px" pt="2" pb="6px" borderTopWidth="1px" borderColor="hairline" fontSize="xs" color="text.faint" data-part="footer">
        {footer(report, at, now)}
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
}: {
  part: string;
  icon: ReactNode;
  name: string;
  state: HealthState;
  value: string;
  percent: number;
  sub: string;
}) {
  const color = STATE_COLOR[state];
  const tag = TAG[state];
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
      <Box gridColumn="2 / 4" h="4px" borderRadius="2px" bg="hairline" overflow="hidden" mt="3px" mb="1px">
        <Box h="100%" w={`${Math.min(Math.max(percent, 0), 100)}%`} bg={color} />
      </Box>
      <Text as="span" gridColumn="2 / 4" fontSize="xs" lineHeight="1.45" color="text.dim">
        {sub}
      </Text>
    </Box>
  );
}
