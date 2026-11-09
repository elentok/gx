// The disk thresholds are decimal ("20G"), so free disk space uses the decimal unit too.
const GIGA = { binary: 1024 ** 3, decimal: 1e9 };

/** Gigabytes without the unit: a whole number as is, otherwise one decimal. */
export function formatBytes(bytes: number, unit: keyof typeof GIGA): string {
  const n = bytes / GIGA[unit];
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

const SIZE_UNITS = ["B", "KB", "MB", "GB", "TB"];
const STEP = { binary: 1024, decimal: 1000 };

/** Size with an auto-picked unit: whole at 100 and above (and for bytes), otherwise one decimal. */
export function sizeText(bytes: number, unit: keyof typeof STEP): string {
  let n = bytes;
  let i = 0;
  while (n >= STEP[unit] && i < SIZE_UNITS.length - 1) {
    n /= STEP[unit];
    i++;
  }
  return `${i === 0 || n >= 100 ? Math.round(n) : n.toFixed(1)} ${SIZE_UNITS[i]}`;
}

/** Binary GB from one GB up, whole MB below it. */
export function memoryBytesText(bytes: number): string {
  return bytes >= GIGA.binary ? `${formatBytes(bytes, "binary")} GB` : `${Math.round(bytes / 1024 ** 2)} MB`;
}
