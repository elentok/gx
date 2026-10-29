import { describe, expect, it } from "vitest";
import { formatBytes, memoryBytesText } from "./formatBytes";

describe("formatBytes", () => {
  it("shows whole gigabytes without a decimal", () => {
    expect(formatBytes(8 * 1024 ** 3, "binary")).toBe("8");
    expect(formatBytes(20e9, "decimal")).toBe("20");
  });

  it("shows one decimal otherwise, in the named unit", () => {
    expect(formatBytes(1.5 * 1024 ** 3, "binary")).toBe("1.5");
    expect(formatBytes(1.5e9, "decimal")).toBe("1.5");
    expect(formatBytes(1024 ** 3, "decimal")).toBe("1.1");
  });
});

describe("memoryBytesText", () => {
  it("uses GB from one GB up and MB below", () => {
    expect(memoryBytesText(2 * 1024 ** 3)).toBe("2 GB");
    expect(memoryBytesText(512 * 1024 ** 2)).toBe("512 MB");
  });
});
