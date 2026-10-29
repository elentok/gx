import { describe, expect, it } from "vitest";
import { slugOf } from "./slug";

describe("slugOf", () => {
  it("drops the project", () => {
    expect(slugOf("gx:foo")).toBe("foo");
  });

  it("keeps everything after the first colon", () => {
    expect(slugOf("gx:foo:bar/01")).toBe("foo:bar/01");
  });
});
