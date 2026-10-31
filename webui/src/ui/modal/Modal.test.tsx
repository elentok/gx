import { ChakraProvider } from "@chakra-ui/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderThemed } from "../render";
import { system } from "../theme";
import { Modal } from "./Modal";

afterEach(cleanup);

function mount() {
  const onClose = vi.fn();
  renderThemed(
    <Modal label="Thing" onClose={onClose}>
      <input aria-label="Field" />
    </Modal>,
  );
  const dialog = screen.getByRole("dialog", { name: "Thing" });
  return { onClose, dialog, backdrop: dialog.parentElement! };
}

describe("Modal", () => {
  it("closes on a press and release on the backdrop", () => {
    const { onClose, backdrop } = mount();
    fireEvent.mouseDown(backdrop);
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("stays open when a press in the panel is released on the backdrop", () => {
    const { onClose, backdrop } = mount();
    fireEvent.mouseDown(screen.getByRole("textbox", { name: "Field" }));
    fireEvent.click(backdrop);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("stays open on a click in the panel", () => {
    const { onClose, dialog } = mount();
    fireEvent.mouseDown(dialog);
    fireEvent.click(dialog);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("closes on Escape", () => {
    const { onClose } = mount();
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes only the topmost Modal on Escape", () => {
    const under = vi.fn();
    const over = vi.fn();
    const ui = (showOver: boolean) => (
      <ChakraProvider value={system}>
        <Modal label="Under" onClose={under}>
          <p>under</p>
        </Modal>
        {showOver && (
          <Modal label="Over" onClose={over}>
            <p>over</p>
          </Modal>
        )}
      </ChakraProvider>
    );
    const { rerender } = render(ui(false));
    rerender(ui(true));
    fireEvent.keyDown(window, { key: "Escape" });
    expect(over).toHaveBeenCalledTimes(1);
    expect(under).not.toHaveBeenCalled();
    rerender(ui(false));
    fireEvent.keyDown(window, { key: "Escape" });
    expect(under).toHaveBeenCalledTimes(1);
  });

  it("ignores Escape with closeOnEscape off, but still closes on the backdrop", () => {
    const onClose = vi.fn();
    renderThemed(
      <Modal label="Thing" onClose={onClose} closeOnEscape={false}>
        <p>body</p>
      </Modal>,
    );
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
    const backdrop = screen.getByRole("dialog").parentElement!;
    fireEvent.mouseDown(backdrop);
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
