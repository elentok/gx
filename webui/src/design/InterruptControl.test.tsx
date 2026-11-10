import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { Modal } from "../ui/modal/Modal";
import { renderThemed } from "../ui/render";
import { WideContext } from "../useWide";
import { InterruptControl } from "./InterruptControl";

afterEach(cleanup);

function mount(over: { wide?: boolean; connected?: boolean; extra?: React.ReactNode } = {}) {
  const onInterrupt = vi.fn(async () => ({}));
  renderThemed(
    <WideContext.Provider value={over.wide ?? true}>
      <InterruptControl connected={over.connected ?? true} pending={false} onInterrupt={onInterrupt} />
      {over.extra}
    </WideContext.Provider>,
  );
  return { onInterrupt };
}

const esc = () => fireEvent.keyDown(document.body, { key: "Escape" });
const button = () => screen.getByRole("button", { name: "Interrupt" });
const confirm = () => screen.queryByRole("alertdialog", { name: "Interrupt the agent?" });

test("Esc opens the confirm with Interrupt focused; Enter confirms once", async () => {
  const { onInterrupt } = mount();
  esc();
  const dialog = await screen.findByRole("alertdialog", { name: "Interrupt the agent?" });
  const yes = within(dialog).getByRole("button", { name: "Interrupt" });
  await waitFor(() => expect(document.activeElement).toBe(yes));
  fireEvent.click(yes);
  await waitFor(() => expect(onInterrupt).toHaveBeenCalledTimes(1));
});

test("Esc again closes the confirm, sends nothing and returns focus to the button", async () => {
  const { onInterrupt } = mount();
  esc();
  await screen.findByRole("alertdialog");
  fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
  await waitFor(() => expect(confirm()).toBeNull());
  expect(onInterrupt).not.toHaveBeenCalled();
  await waitFor(() => expect(document.activeElement).toBe(button()));
});

test("a click-opened confirm focuses Cancel", async () => {
  mount();
  fireEvent.click(button());
  const cancel = await screen.findByRole("button", { name: "Cancel" });
  await waitFor(() => expect(document.activeElement).toBe(cancel));
});

test("Esc does nothing under a dialog, a layer, a menu, a listbox or the Questions view", async () => {
  for (const extra of [
    <div key="d" role="dialog" />,
    <div key="m" role="menu" />,
    <div key="l" role="listbox" aria-label="Commands" />,
    <div key="q" data-questions-view="" />,
    <Modal key="o" label="Thing" onClose={() => {}}>
      x
    </Modal>,
  ]) {
    mount({ extra });
    esc();
    await new Promise((r) => setTimeout(r, 20));
    expect(confirm()).toBeNull();
    cleanup();
  }
});

test("Esc does nothing while disconnected", async () => {
  mount({ connected: false });
  esc();
  await new Promise((r) => setTimeout(r, 20));
  expect(confirm()).toBeNull();
});

test("on a phone Esc does nothing and the button has no title", async () => {
  mount({ wide: false });
  esc();
  await new Promise((r) => setTimeout(r, 20));
  expect(confirm()).toBeNull();
  expect(button().getAttribute("title")).toBeNull();
});

test("on a wide screen the title names Esc and the accessible name stays Interrupt", () => {
  mount();
  expect(button().getAttribute("title")).toContain("Esc");
});

test("no control, no listener: Esc with nothing rendered opens nothing", async () => {
  renderThemed(<div />);
  esc();
  expect(confirm()).toBeNull();
});
