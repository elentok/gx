import { expect, test } from "vitest";
import { echoMatches, withEchoes } from "./SessionView";

const msg = (id: string) => ({ id });
const prompt = (text: string, images?: { id: string }[]) => ({ kind: "prompt", text, images });

test("an echo carries its image ids as thumbnails, also with no text", () => {
  const [e] = withEchoes<{ id: string; images?: { id: string }[] }>([], [{ text: "", images: ["i1", "i2"] }]);
  expect(e.images).toEqual([{ id: "i1" }, { id: "i2" }]);
});

test("the echo match ignores chips and edge whitespace", () => {
  const e = { text: "look", images: ["i1"] };
  expect(echoMatches([prompt("[Image #1] look ", [{ id: "i1" }])], e)).toBe(1);
  expect(echoMatches([prompt("other")], e)).toBe(0);
});

test("an images-only echo matches images-only prompts with the same count, not text-only ones", () => {
  const e = { text: "", images: ["i1", "i2"] };
  expect(echoMatches([prompt("[Image #1][Image #2]", [{ id: "a" }, { id: "b" }])], e)).toBe(1);
  expect(echoMatches([prompt("[Image #1]", [{ id: "a" }])], e)).toBe(0);
  expect(echoMatches([prompt("hello")], e)).toBe(0);
});
const ids = (all: { id: string }[]) => all.map((m) => m.id);

test("an echo sits after the message that was last when it was sent, so later replies land below it", () => {
  const out = withEchoes([msg("a"), msg("b"), msg("reply")], [{ text: "hi", after: "a" }]);
  expect(ids(out)).toEqual(["a", "echo-0", "b", "reply"]);
});

test("echoes with one anchor keep their send order", () => {
  const out = withEchoes([msg("a"), msg("b")], [{ text: "one", after: "a" }, { text: "two", after: "a" }]);
  expect(ids(out)).toEqual(["a", "echo-0", "echo-1", "b"]);
});

test("echoes with different anchors each land at their own place", () => {
  const out = withEchoes([msg("a"), msg("b"), msg("c")], [{ text: "one", after: "a" }, { text: "two", after: "c" }]);
  expect(ids(out)).toEqual(["a", "echo-0", "b", "c", "echo-1"]);
});

test("an echo sent into an empty chat goes first; one whose anchor is gone goes last", () => {
  const out = withEchoes([msg("a")], [{ text: "first" }, { text: "lost", after: "gone" }]);
  expect(ids(out)).toEqual(["echo-0", "a", "echo-1"]);
});
