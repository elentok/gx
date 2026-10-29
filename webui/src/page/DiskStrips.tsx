import type { CleanNowResult, DiskStatus } from "../generated/server";
import { apiFetch } from "../apiFetch";
import { useConnectionContext, useConnectionState } from "../connection";
import { DiskStrip } from "../ui/disk-strip/DiskStrip";
import { refusalText, useWrite, WriteError } from "../useWrite";

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export async function postCleanDisk(disk: string): Promise<CleanNowResult> {
  const res = await apiFetch("/v1/disk/clean", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ disk }),
  });
  if (!res.ok) throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`);
  return res.json();
}

const UNITS = ["B", "KB", "MB", "GB", "TB"];

function formatFree(bytes: number): string {
  let n = bytes;
  let i = 0;
  while (n >= 1000 && i < UNITS.length - 1) {
    n /= 1000;
    i++;
  }
  return `${i === 0 || n >= 100 ? Math.round(n) : n.toFixed(1)} ${UNITS[i]}`;
}

function ConnectedDiskStrip({ disk }: { disk: DiskStatus }) {
  const conn = useConnectionContext();
  const { run, pending, refusal } = useWrite();
  // A failed clean is read from the snapshot, so an automatic one shows too and a reload keeps it.
  const failed = disk.last_clean.startsWith("failed") ? disk.last_clean : undefined;
  return (
    <DiskStrip
      name={disk.name}
      free={formatFree(disk.free_bytes)}
      held={disk.held}
      cleaning={disk.cleaning}
      pending={pending}
      problem={refusal ? refusalText(refusal) : failed}
      onClean={() => run(() => postCleanDisk(disk.id), { onDone: () => conn.refresh() })}
    />
  );
}

// One strip per low disk in the snapshot, below the Offline strip.
export function DiskStrips() {
  const { snapshot } = useConnectionState(useConnectionContext());
  return (
    <>
      {(snapshot?.disks ?? []).map((d) => (
        <ConnectedDiskStrip key={d.id} disk={d} />
      ))}
    </>
  );
}
