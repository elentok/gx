import { expect, test } from "vitest";
import { withEchoes } from "./SessionView";

const msg = (id: string) => ({ id });
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
