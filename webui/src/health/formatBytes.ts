// The disk thresholds are decimal ("20G"), so free disk space uses the decimal unit too.
const GIGA = { binary: 1024 ** 3, decimal: 1e9 };

/** Gigabytes without the unit: a whole number as is, otherwise one decimal. */
export function formatBytes(bytes: number, unit: keyof typeof GIGA): string {
  const n = bytes / GIGA[unit];
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

/** Binary GB from one GB up, whole MB below it. */
export function memoryBytesText(bytes: number): string {
  return bytes >= GIGA.binary ? `${formatBytes(bytes, "binary")} GB` : `${Math.round(bytes / 1024 ** 2)} MB`;
}
