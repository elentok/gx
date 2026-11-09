import { act, cleanup, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";
import { DeviceContext, type Device } from "../../useWide";
import { renderThemed } from "../render";
import { Modal } from "./Modal";
import { useFocusFirstField } from "./useFocusFirstField";

afterEach(cleanup);

let showLate: () => void = () => {};

function Dialog() {
  const ref = useFocusFirstField();
  const [late, setLate] = useState(false);
  showLate = () => setLate(true);
  return (
    <Modal label="Thing" onClose={() => {}}>
      <input aria-label="Other" />
      {late && <input aria-label="First" ref={ref} />}
    </Modal>
  );
}

function Immediate() {
  const ref = useFocusFirstField();
  return (
    <Modal label="Thing" onClose={() => {}}>
      <input aria-label="First" ref={ref} />
    </Modal>
  );
}

const mount = (ui: React.ReactNode, device: Device) => renderThemed(<DeviceContext.Provider value={device}>{ui}</DeviceContext.Provider>);

it("focuses the field on mount on a wide device", () => {
  mount(<Immediate />, "desktop");
  expect(document.activeElement).toBe(screen.getByLabelText("First"));
});

it("leaves focus on the panel on a phone", () => {
  mount(<Immediate />, "phone");
  expect(document.activeElement).toBe(screen.getByRole("dialog"));
});

it("focuses a field that mounts after the dialog", () => {
  mount(<Dialog />, "tablet");
  expect(document.activeElement).toBe(screen.getByRole("dialog"));
  act(() => showLate());
  expect(document.activeElement).toBe(screen.getByLabelText("First"));
});

it("takes focus from an element outside the dialog, such as a closing menu's item", () => {
  const outside = document.createElement("button");
  document.body.append(outside);
  outside.focus();
  mount(<Immediate />, "desktop");
  expect(document.activeElement).toBe(screen.getByLabelText("First"));
  outside.remove();
});

it("does not steal focus from a field the person already moved to", () => {
  mount(<Dialog />, "desktop");
  act(() => screen.getByLabelText("Other").focus());
  act(() => showLate());
  expect(document.activeElement).toBe(screen.getByLabelText("Other"));
});
