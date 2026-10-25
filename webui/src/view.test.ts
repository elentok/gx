import { describe, expect, it } from "vitest";
import { epicBadge } from "./view";

describe("epicBadge", () => {
  it("labels each state", () => {
    expect(epicBadge("paused")).toBe("Paused");
    expect(epicBadge("running")).toBe("Running");
    expect(epicBadge("waiting")).toBe("Waiting for a slot");
    expect(epicBadge("awaiting-merge")).toBe("Awaiting merge");
  });

  it("labels Waiting by queue mode and leaves Running unchanged", () => {
    expect(epicBadge("waiting", "paused")).toBe("Waiting: queue paused");
    expect(epicBadge("waiting", "running")).toBe("Waiting for a slot");
    expect(epicBadge("running", "paused")).toBe("Running");
  });
});
