import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderThemed } from "../render";
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
});
