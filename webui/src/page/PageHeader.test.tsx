import { expect, test } from "vitest";
import { renderThemed } from "../ui/render";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { ShellContext } from "../shell/ShellContext";
import { DeviceContext, type Device, WideContext } from "../useWide";
import { PageHeader } from "./PageHeader";

const render = (node: React.ReactNode, wide = true) =>
  renderThemed(<WideContext.Provider value={wide}>{node}</WideContext.Provider>);

test("renders icon, title, subtitle, actions and meta bar", () => {
  const { getByRole, getByTestId } = render(
    <PageHeader title="slow-tests" icon={<i>icon</i>} subtitle="a line" actions={<u>menu</u>} meta={<b>meta</b>} />,
  );
  expect(getByRole("heading", { level: 1 }).textContent).toBe("slow-tests");
  expect(getByTestId("page-header-icon").textContent).toBe("icon");
  expect(getByTestId("page-header-subtitle").textContent).toBe("a line");
  expect(getByTestId("page-header-actions").textContent).toBe("menu");
  expect(getByTestId("page-header-meta").textContent).toBe("meta");
});

test("inside the shell the page icon is dropped on every device, leaving the mascot alone", () => {
  const shell = { sidebarOpen: false, toggle: () => {}, close: () => {}, claimHeader: () => () => {} };
  const state = { snapshot: { needs_you_total: 0 }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const at = (device: Device) =>
    renderThemed(
      <DeviceContext.Provider value={device}>
        <ConnectionContext.Provider value={conn}>
          <ShellContext.Provider value={shell}>
            <PageHeader title="t" icon={<i>icon</i>} />
          </ShellContext.Provider>
        </ConnectionContext.Provider>
      </DeviceContext.Provider>,
    );
  for (const device of ["desktop", "tablet", "phone"] as const) {
    const { queryByTestId, getByRole, unmount } = at(device);
    expect(queryByTestId("page-header-icon")).toBeNull();
    expect(getByRole("button", { name: "Show sidebar" })).toBeTruthy();
    unmount();
  }
});

test("the title is a 19px heading in the text font, not a button", () => {
  const { getByRole, queryByRole } = render(<PageHeader title="t" icon={<i />} />);
  const style = getComputedStyle(getByRole("heading", { level: 1 }));
  expect(style.fontSize).toBe("19px");
  expect(style.fontFamily).toBe("inherit");
  expect(queryByRole("button")).toBeNull();
});

test("the subtitle truncates on one line", () => {
  const { getByTestId } = render(<PageHeader title="t" subtitle={"x".repeat(300)} />);
  const style = getComputedStyle(getByTestId("page-header-subtitle"));
  expect(style.whiteSpace).toBe("nowrap");
  expect(style.textOverflow).toBe("ellipsis");
  expect(style.overflow).toBe("hidden");
});

test("the ring comes before the menu button in actions", () => {
  const { getByTestId } = render(<PageHeader title="t" actions={<><i>ring</i><u>menu</u></>} />);
  expect(getByTestId("page-header-actions").textContent).toBe("ringmenu");
});

test("no subtitle, no meta renders no empty rows", () => {
  const { queryByTestId } = render(<PageHeader title="t" icon={<i>i</i>} />);
  expect(queryByTestId("page-header-subtitle")).toBeNull();
  expect(queryByTestId("page-header-meta")).toBeNull();
  expect(queryByTestId("page-header-actions")).toBeNull();
});

test("a bare title renders children", () => {
  const { getByText, getByRole } = render(
    <PageHeader title="old">
      <p>child</p>
    </PageHeader>,
  );
  expect(getByRole("heading", { level: 1 }).textContent).toBe("old");
  expect(getByText("child")).toBeTruthy();
});
