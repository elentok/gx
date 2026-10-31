import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderThemed } from "../render";
import { BottomSheet } from "./BottomSheet";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const phone = (vv?: object) => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  if (vv) vi.stubGlobal("visualViewport", { addEventListener() {}, removeEventListener() {}, ...vv });
};

const mount = (props: object = {}, onClose = () => {}) => {
  renderThemed(
    <BottomSheet label="Thing" onClose={onClose} {...props}>
      <p>body</p>
    </BottomSheet>,
  );
  const dialog = screen.getByRole("dialog", { name: "Thing" });
  return { dialog, backdrop: dialog.parentElement! };
};

describe("BottomSheet", () => {
  it("is a plain dialog without a grab handle on a wide screen", () => {
    mount();
    expect(screen.queryByTestId("grab-handle")).toBeNull();
  });

  it("shows a grab handle on a phone", () => {
    phone();
    mount();
    expect(screen.getByTestId("grab-handle")).toBeTruthy();
  });

  it("sits below the TopBar's published height, and follows the visual viewport", () => {
    phone({ offsetTop: 10, height: 600 });
    const { backdrop } = mount({ belowTopBar: true });
    expect(backdrop.style.top || getComputedStyle(backdrop).top).toContain("var(--top-bar-height");
  });

  it("covers the whole screen when it is not below the TopBar", () => {
    phone();
    const { backdrop } = mount();
    expect(getComputedStyle(backdrop).top).not.toContain("--top-bar-height");
  });

  const press = (backdrop: HTMLElement) => {
    fireEvent.mouseDown(backdrop);
    fireEvent.click(backdrop);
  };

  it("closes on a backdrop press on a phone by default", () => {
    phone();
    const onClose = vi.fn();
    press(mount({}, onClose).backdrop);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("ignores a backdrop press on a phone with closeOnNarrowBackdrop off", () => {
    phone();
    const onClose = vi.fn();
    press(mount({ closeOnNarrowBackdrop: false }, onClose).backdrop);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("still closes on a backdrop press on a wide screen with closeOnNarrowBackdrop off", () => {
    const onClose = vi.fn();
    press(mount({ closeOnNarrowBackdrop: false }, onClose).backdrop);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
