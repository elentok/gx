import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { CoarsePointerContext, WideContext } from "../../useWide";
import { renderThemed } from "../render";
import { LargeEditor } from "./LargeEditor";

const vi_state = vi.hoisted(() => ({ on: false, escape: undefined as unknown as ReturnType<typeof import("vitest").vi.fn> }));

vi.mock("../../vimode", () => ({ useViMode: () => vi_state.on }));

// The real editor needs CodeMirror; the layout only needs its handle and mode reports.
vi.mock("./CodeEditor", async () => {
  const { useImperativeHandle } = await import("react");
  return {
    default: ({ onModeChange, handle }: { onModeChange: (m: string) => void; handle: React.Ref<{ escape: () => void }> }) => {
      useImperativeHandle(handle, () => ({ escape: vi_state.escape }), []);
      return (
        <div data-testid="fake-editor">
          {["NORMAL", "VISUAL"].map((m) => (
            <button key={m} data-testid={`fake-mode-${m}`} onClick={() => onModeChange(m)} />
          ))}
        </div>
      );
    },
  };
});

beforeEach(() => {
  vi_state.escape = vi.fn();
});
afterEach(cleanup);

const TITLE = "Note on Q2 - Pick";

const cases = [
  { name: "wide, mouse", wide: true, coarse: false, header: true, footer: true, bar: false },
  { name: "wide, touch", wide: true, coarse: true, header: true, footer: false, bar: true },
  { name: "narrow, mouse", wide: false, coarse: false, header: true, footer: true, bar: false },
  { name: "narrow, touch", wide: false, coarse: true, header: false, footer: false, bar: true },
];

const mount = ({ wide, coarse }: { wide: boolean; coarse: boolean }, onClose = vi.fn()) => {
  renderThemed(
    <WideContext.Provider value={wide}>
      <CoarsePointerContext.Provider value={coarse}>
        <LargeEditor title={TITLE} value="" onChange={() => {}} selection={{ start: 0, end: 0 }} onClose={onClose} />
      </CoarsePointerContext.Provider>
    </WideContext.Provider>,
  );
  const dialog = screen.getByRole("dialog", { name: TITLE });
  return { dialog, backdrop: dialog.parentElement!, onClose };
};

describe.each(cases)("$name", (c) => {
  test("has the parts this layout needs, one Done and no x button", () => {
    const { dialog } = mount(c);
    expect(screen.queryByText(TITLE) !== null).toBe(c.header);
    expect(screen.queryByText("Saved as you type") !== null).toBe(c.footer);
    expect(screen.queryByTestId("editor-bar") !== null).toBe(c.bar);
    expect(screen.queryByLabelText("Close")).toBeNull();
    expect(screen.getAllByText("Done")).toHaveLength(1);
    expect(dialog.getAttribute("aria-label")).toBe(TITLE);
  });

  test("Done closes and Escape does not", () => {
    const { onClose } = mount(c);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("Done"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  test(`a backdrop press ${c.wide ? "closes" : "does not close"}`, () => {
    const { backdrop, onClose } = mount(c);
    fireEvent.mouseDown(backdrop);
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalledTimes(c.wide ? 1 : 0);
  });
});

test("the bar's Done is 44px high", () => {
  mount({ wide: false, coarse: true });
  expect(getComputedStyle(screen.getByText("Done")).height).toBe("44px");
});

describe("vi mode text and bar", () => {
  const setup = async (c: { wide: boolean; coarse: boolean }, vi_on = true) => {
    vi_state.on = vi_on;
    mount(c);
    await screen.findByTestId("fake-editor");
  };

  test.each(cases)("$name: one status region, in the footer or the bar", async (c) => {
    await setup(c);
    const status = screen.getAllByRole("status");
    expect(status).toHaveLength(1);
    expect(status[0].textContent).toBe("INSERT");
    const bar = screen.queryByTestId("editor-bar");
    expect(bar !== null).toBe(c.bar);
    if (bar) {
      expect(bar.contains(status[0])).toBe(true);
      const order = [status[0], screen.getByText("Esc"), screen.getByText("Done")];
      order.forEach((el, i) => {
        if (i > 0) expect(order[i - 1].compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      });
      for (const el of order) expect(getComputedStyle(el).height).toBe("44px");
    } else {
      expect(screen.queryByText("Esc")).toBeNull();
      const footer = screen.getByText("Saved as you type").parentElement!;
      expect(footer.firstElementChild).toBe(status[0]);
    }
  });

  test("the text follows the mode and its node changes once per change", async () => {
    await setup({ wide: false, coarse: true });
    const status = screen.getByRole("status");
    let mutations = 0;
    const observer = new MutationObserver((records) => (mutations += records.length));
    observer.observe(status, { childList: true, characterData: true, subtree: true });
    fireEvent.click(screen.getByTestId("fake-mode-NORMAL"));
    fireEvent.click(screen.getByTestId("fake-mode-NORMAL"));
    fireEvent.click(screen.getByTestId("fake-mode-VISUAL"));
    await Promise.resolve();
    observer.disconnect();
    expect(mutations).toBe(2);
    expect(status.textContent).toBe("VISUAL");
  });

  test("vi off: no status region and no Esc", async () => {
    await setup({ wide: false, coarse: true }, false);
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.queryByText("Esc")).toBeNull();
    expect(screen.getByText("Done")).toBeTruthy();
  });

  test("Esc calls escape() once and, like the pill, keeps focus in the editor", async () => {
    await setup({ wide: false, coarse: true });
    fireEvent.click(screen.getByText("Esc"));
    expect(vi_state.escape).toHaveBeenCalledTimes(1);
    for (const el of [screen.getByText("Esc"), screen.getByRole("status")]) {
      expect(fireEvent.mouseDown(el)).toBe(false);
    }
  });
});
