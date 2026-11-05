import { expect, test } from "vitest";
import { renderThemed } from "../ui/render";
import { WideContext } from "../useWide";
import { PageHeader } from "./PageHeader";

const render = (node: React.ReactNode, wide = true) =>
  renderThemed(<WideContext.Provider value={wide}>{node}</WideContext.Provider>);

test("renders ring, title, line, meta bar and right slot", () => {
  const { getByRole, getByTestId } = render(
    <PageHeader title="slow-tests" ring={<i>ring</i>} line="a line" meta={<b>meta</b>} right={<u>menu</u>} />,
  );
  expect(getByRole("heading", { level: 1 }).textContent).toBe("slow-tests");
  expect(getByTestId("page-header-ring").textContent).toBe("ring");
  expect(getByTestId("page-header-line").textContent).toBe("a line");
  expect(getByTestId("page-header-meta").textContent).toBe("meta");
  expect(getByTestId("page-header-meta").parentElement?.textContent).toContain("menu");
});

test("the line truncates on one line", () => {
  const { getByTestId } = render(<PageHeader title="t" line={"x".repeat(300)} />);
  const style = getComputedStyle(getByTestId("page-header-line"));
  expect(style.whiteSpace).toBe("nowrap");
  expect(style.textOverflow).toBe("ellipsis");
  expect(style.overflow).toBe("hidden");
});

test("no line, no meta renders no empty rows", () => {
  const { queryByTestId } = render(<PageHeader title="t" ring={<i>r</i>} />);
  expect(queryByTestId("page-header-line")).toBeNull();
  expect(queryByTestId("page-header-meta")).toBeNull();
});

test("the ring box is 56px wide and 44px on a phone, with or without a line", () => {
  for (const line of ["a line", undefined]) {
    const view = render(<PageHeader title="t" ring={<i />} line={line} />);
    expect(getComputedStyle(view.getByTestId("page-header-ring")).height).toBe("56px");
    view.unmount();
  }
  const phone = render(<PageHeader title="t" ring={<i />} line="a line" />, false);
  expect(getComputedStyle(phone.getByTestId("page-header-ring")).height).toBe("44px");
});

test("the older lead form still renders the title and children", () => {
  const { getByText, getByRole } = render(
    <PageHeader title="old" lead={<i>lead</i>}>
      <p>child</p>
    </PageHeader>,
  );
  expect(getByRole("heading", { level: 1 }).textContent).toBe("old");
  expect(getByText("lead")).toBeTruthy();
  expect(getByText("child")).toBeTruthy();
});
