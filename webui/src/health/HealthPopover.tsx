import { Box, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import type { Health, HealthReport } from "../generated/server";
import { METERS } from "../ui/tray/meters";
import { formatBytes, memoryBytesText } from "./formatBytes";
import { CauseList, CPU_ICON, DISK_ICON, DiskRow, MEMORY_ICON, Row, unknownText } from "./HealthRows";
import { useHealthReport } from "./useHealthReport";

const ICONS = { cpu: CPU_ICON, memory: MEMORY_ICON, disk: DISK_ICON };
const title = (name: string) => name.charAt(0).toUpperCase() + name.slice(1);

export function HealthPopover({ health, connected = true }: { health: Health; connected?: boolean }) {
  const received = useHealthReport(connected);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(tick);
  }, []);

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
            : `1-minute average ${Math.round(cpu.average)}%`
        }
      />
      <CauseList part="cpu-causes" list={report.causes.cpu} value={(r) => `${Math.round(r.cpu)}%`} />
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
            : `${formatBytes(memory.used_bytes, "binary")} of ${formatBytes(memory.total_bytes, "binary")} GB${pressure}`
        }
      />
      <CauseList part="memory-causes" list={report.causes.memory} value={(r) => memoryBytesText(r.mem_bytes)} />
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
      {METERS.map(({ key, name }) => (
        <Row key={key} part={key} icon={ICONS[key]} name={title(name)} state={health[key].state} value="—" sub={`State: ${health[key].state}`} />
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
