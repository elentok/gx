import { describe, expect, it } from "vitest";
import type { SessionCommandEntry } from "../generated/server";
import { fillCommand, matchCommands, openToken } from "./match";

const entry = (name: string, aliases: string[] = []): SessionCommandEntry => ({
  name,
  aliases,
  arg_hint: "",
  description: "",
  source: "built-in",
});

describe("openToken", () => {
  it("opens / at the start of a draft with no whitespace", () => {
    expect(openToken("/he", 3, { harness: "claude" })).toEqual({ kind: "/", start: 0, end: 3, query: "he" });
    expect(openToken("/", 1, { harness: "codex" })).toEqual({ kind: "/", start: 0, end: 1, query: "" });
  });

  it("does not open / after whitespace, mid-sentence or with the cursor before it", () => {
    expect(openToken("/help me", 8, { harness: "claude" })).toBeNull();
    expect(openToken("hi /he", 6, { harness: "claude" })).toBeNull();
    expect(openToken("/he", 0, { harness: "claude" })).toBeNull();
  });

  it("opens $ on the word under the cursor, mid-sentence, in Codex only", () => {
    expect(openToken("run $gx-im now", 9, { harness: "codex" })).toEqual({ kind: "$", start: 4, end: 10, query: "gx-i" });
    expect(openToken("run $gx-im now", 9, { harness: "claude" })).toBeNull();
  });

  it("never opens for a disabled composer", () => {
    expect(openToken("/he", 3, { harness: "claude", disabled: true })).toBeNull();
    expect(openToken("$he", 3, { harness: "codex", disabled: true })).toBeNull();
  });
});

describe("matchCommands", () => {
  const all = [entry("review"), entry("clear", ["reset"]), entry("compact"), entry("preview"), entry("agents")];

  it("lists prefix matches, then substring matches, alphabetical in each", () => {
    expect(matchCommands(all, "re").map((m) => m.entry.name)).toEqual(["clear", "review", "preview"]);
  });

  it("matches aliases and highlights the alias", () => {
    const [m] = matchCommands(all, "res");
    expect(m.entry.name).toBe("clear");
    expect(m.matched).toBe("reset");
    expect(m.segments).toEqual([
      { text: "res", match: true },
      { text: "et", match: false },
    ]);
  });

  it("highlights the typed part of a substring match", () => {
    const m = matchCommands(all, "view").find((x) => x.entry.name === "preview")!;
    expect(m.segments).toEqual([
      { text: "pre", match: false },
      { text: "view", match: true },
    ]);
  });

  it("is case-insensitive and lists everything alphabetically for an empty query", () => {
    expect(matchCommands(all, "").map((m) => m.entry.name)).toEqual(["agents", "clear", "compact", "preview", "review"]);
    expect(matchCommands(all, "CO").map((m) => m.entry.name)).toEqual(["compact"]);
  });
});

describe("fillCommand", () => {
  it("replaces the token and puts the cursor after the space", () => {
    const token = openToken("run $gx-im now", 9, { harness: "codex" })!;
    expect(fillCommand("run $gx-im now", token, "gx-implement")).toEqual({
      text: "run $gx-implement now",
      cursor: 18,
    });
  });

  it("fills a leading /", () => {
    const token = openToken("/he", 3, { harness: "claude" })!;
    expect(fillCommand("/he", token, "help")).toEqual({ text: "/help ", cursor: 6 });
  });
});
