// @vitest-environment node
import { describe, expect, it } from "vitest";
import { session } from "../agents/agentsTestKit";
import {
  GROUP_ON_HOLD,
  countLabel,
  enter,
  fit,
  groupState,
  initialSelect,
  leave,
  prune,
  rowOfAgent,
  rowOfCard,
  selectAll,
  selectNone,
  skipLines,
  toggle,
  toggleGroup,
  visibleActions,
  type SelectRow,
  type SelectState,
} from "./selectModel";
import { CardKindDesign, CardKindImplementation, SectionFinished, SectionOnHold, type EpicListRow } from "../generated/server";

const r = (key: string, over: Partial<SelectRow> = {}): SelectRow => ({ key, kind: "epic", visible: true, selectable: true, ...over });
const on = (...keys: string[]): SelectState => ({ on: true, keys });

describe("mode", () => {
  it("enters with nothing selected and leaves clearing the selection", () => {
    expect(enter(initialSelect)).toEqual(on());
    expect(leave(on("a", "b"))).toEqual(initialSelect);
  });

  it("toggles a key in select mode only", () => {
    expect(toggle(on(), "a")).toEqual(on("a"));
    expect(toggle(on("a"), "a")).toEqual(on());
    expect(toggle(initialSelect, "a")).toEqual(initialSelect);
  });

  it("a run that leaves no selected rows leaves select mode", () => {
    expect(prune(on("a"), [r("b")], true)).toEqual(initialSelect);
    expect(prune(on("a", "b"), [r("b")], true)).toEqual(on("b"));
  });
});

describe("select all and none", () => {
  const rows = [r("a"), r("b", { visible: false }), r("q", { kind: "agent", selectable: false }), r("c")];

  it("covers only visible, selectable rows", () => {
    expect(selectAll(on(), rows).keys).toEqual(["a", "c"]);
  });

  it("select none clears", () => {
    expect(selectNone(on("a", "c"))).toEqual(on());
  });
});

describe("section checkbox", () => {
  const rows = [r("a", { group: GROUP_ON_HOLD }), r("b", { group: GROUP_ON_HOLD }), r("c")];

  it("shows none, mixed and all", () => {
    expect(groupState(on(), rows, GROUP_ON_HOLD)).toBe("none");
    expect(groupState(on("a"), rows, GROUP_ON_HOLD)).toBe("some");
    expect(groupState(on("a", "b"), rows, GROUP_ON_HOLD)).toBe("all");
  });

  it("counts rows of a collapsed section, hidden ones included, but not unselectable rows", () => {
    const hidden = [r("a", { group: GROUP_ON_HOLD, visible: false }), r("b", { group: GROUP_ON_HOLD, visible: false }), r("q", { group: GROUP_ON_HOLD, selectable: false })];
    expect(groupState(on("a"), hidden, GROUP_ON_HOLD)).toBe("some");
    expect(groupState(on("a", "b"), hidden, GROUP_ON_HOLD)).toBe("all");
    expect(toggleGroup(on(), hidden, GROUP_ON_HOLD).keys).toEqual(["a", "b"]);
  });

  it("selects every row of the section, then clears them", () => {
    expect(toggleGroup(on("c", "a"), rows, GROUP_ON_HOLD).keys).toEqual(["c", "a", "b"]);
    expect(toggleGroup(on("c", "a", "b"), rows, GROUP_ON_HOLD).keys).toEqual(["c"]);
  });
});

describe("prune", () => {
  it("drops the keys whose row left the list", () => {
    expect(prune(on("a", "b"), [r("a")])).toEqual(on("a"));
  });
});

describe("fit rules", () => {
  const live = session("live", "p");
  const exited = session("done", "p", { status: { state: "exited" } });
  const queueAgent = session("qa", "p", { queue: true });
  const rows: SelectRow[] = [
    r("epic", { kind: "epic" }),
    r("design", { kind: "design" }),
    r("chat", { kind: "chat", session: live }),
    r("queued", { queued: true }),
    r("held", { held: true }),
    r("fin", { archivable: true }),
    r("a-live", { kind: "agent", session: live }),
    r("a-exited", { kind: "agent", session: exited }),
    r("a-queue", { kind: "agent", session: queueAgent }),
    r("arch", { kind: "archived" }),
  ];
  const all = on(...rows.map((x) => x.key));
  const fits = (a: Parameters<typeof fit>[0]) => fit(a, all, rows).fits;

  it("Move fits epics, Designs and chats", () => {
    expect(fits("move")).toEqual(["epic", "design", "chat", "queued", "held", "fin"]);
  });

  it("Hold fits rows not queued and not held", () => {
    expect(fits("hold")).toEqual(["epic", "design", "fin"]);
    expect(fit("hold", all, rows).skipped).toContainEqual({ key: "queued", reason: "queued" });
  });

  it("Take off hold fits held rows", () => {
    expect(fits("unhold")).toEqual(["held"]);
    expect(fit("unhold", on("epic"), rows).skipped).toEqual([{ key: "epic", reason: "not on hold" }]);
  });

  it("Stop fits live agents the queue does not manage", () => {
    expect(fits("stop")).toEqual(["chat", "a-live"]);
    const f = fit("stop", all, rows);
    expect(f.skipped).toContainEqual({ key: "a-exited", reason: "not live" });
    expect(f.skipped).toContainEqual({ key: "a-queue", reason: "managed by the queue" });
  });

  it("Remove fits agents the queue does not manage", () => {
    expect(fits("remove")).toEqual(["chat", "a-live", "a-exited"]);
  });

  it("Archive fits finished loop epics and done Designs", () => {
    expect(fits("archive")).toEqual(["fin"]);
    expect(fit("archive", on("epic"), rows).skipped).toEqual([{ key: "epic", reason: "unfinished" }]);
  });

  it("Unarchive fits archive rows", () => {
    expect(fits("unarchive")).toEqual(["arch"]);
  });

  it("hides an action that fits no selected row", () => {
    expect(visibleActions(on("arch"), rows).map((a) => a.action)).toEqual(["unarchive"]);
  });
});

describe("labels", () => {
  const rows = [r("a", { archivable: true }), r("b", { archivable: true }), r("c")];

  it("says n of m when some rows are skipped", () => {
    expect(countLabel("archive", fit("archive", on("a", "b", "c"), rows))).toBe("Archive 2 of 3");
  });

  it("says n when all fit", () => {
    expect(countLabel("archive", fit("archive", on("a", "b"), rows))).toBe("Archive 2");
  });

  it("lists each skipped row with its reason", () => {
    expect(skipLines(fit("archive", on("a", "c"), rows), (k) => k.toUpperCase())).toEqual(["C: unfinished"]);
  });
});

describe("row builders", () => {
  const card = (over: Partial<EpicListRow>): EpicListRow => ({ address: "gx:x", name: "x", queued: false, ...over }) as EpicListRow;

  it("marks a finished loop epic archivable and an unfinished one not", () => {
    expect(rowOfCard(card({ card_kind: CardKindImplementation, section: SectionFinished })).archivable).toBe(true);
    expect(rowOfCard(card({ card_kind: CardKindImplementation, section: "implementing" })).archivable).toBe(false);
  });

  it("archives a Design only at the done step", () => {
    const d: Partial<EpicListRow> = { card_kind: CardKindDesign, section: SectionFinished };
    expect(rowOfCard(card({ ...d, progress_step: "done" })).archivable).toBe(true);
    expect(rowOfCard(card({ ...d, progress_step: "spec" })).archivable).toBe(false);
  });

  it("puts On hold cards in the on-hold group, held", () => {
    const row = rowOfCard(card({ card_kind: CardKindImplementation, section: SectionOnHold }));
    expect(row).toMatchObject({ held: true, group: GROUP_ON_HOLD });
  });

  it("makes a Queue agent row unselectable", () => {
    expect(rowOfAgent(session("q", "p", { queue: true })).selectable).toBe(false);
    expect(rowOfAgent(session("a", "p")).selectable).toBe(true);
  });
});
