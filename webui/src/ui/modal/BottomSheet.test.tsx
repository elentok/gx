import { cleanup, screen } from "@testing-library/react";
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

const mount = (props: object = {}) => {
  renderThemed(
    <BottomSheet label="Thing" onClose={() => {}} {...props}>
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
});
