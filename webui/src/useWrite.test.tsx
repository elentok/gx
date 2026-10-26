import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { renderThemed } from "./ui/render";
import { WriteError, useWrite, type Refusable } from "./useWrite";

afterEach(cleanup);

function Probe({ call, success }: { call: () => Promise<Refusable>; success?: boolean }) {
  const { run, pending, refusal } = useWrite();
  return (
    <>
      <button disabled={pending} onClick={() => run(call, { success: success ? () => ({ type: "success", title: "Done it" }) : undefined })}>
        {pending ? "spinner" : "go"}
      </button>
      {refusal && <span data-testid="refusal">{refusal.reason}</span>}
    </>
  );
}

test("the button is disabled with a spinner while the call runs", async () => {
  let finish!: (r: Refusable) => void;
  renderThemed(<Probe call={() => new Promise((r) => (finish = r))} />);
  fireEvent.click(screen.getByRole("button"));
  const busy = await screen.findByRole("button", { name: "spinner" });
  expect((busy as HTMLButtonElement).disabled).toBe(true);
  await act(async () => finish({}));
  expect((await screen.findByRole("button", { name: "go" }) as HTMLButtonElement).disabled).toBe(false);
});

test("a non-200 answer shows its plain-text body in a toast", async () => {
  renderThemed(<Probe call={async () => Promise.reject(new WriteError("epic is archived"))} />);
  fireEvent.click(screen.getByRole("button"));
  expect(await screen.findByText("epic is archived")).toBeTruthy();
});

test("a network failure says gx could not be reached", async () => {
  renderThemed(<Probe call={async () => Promise.reject(new TypeError("Failed to fetch"))} />);
  fireEvent.click(screen.getByRole("button"));
  expect(await screen.findByText("Could not reach gx. Try again.")).toBeTruthy();
});

test("a refusal toasts its message, falls back to the reason, and is exposed to the caller", async () => {
  renderThemed(<Probe call={async () => ({ refused: true, reason: "no-message-reason" })} />);
  fireEvent.click(screen.getByRole("button"));
  expect(await screen.findByText("no-message-reason", { selector: "[data-part='title']" })).toBeTruthy();
  expect(screen.getByTestId("refusal").textContent).toBe("no-message-reason");
});

test("a success toasts only when the caller asks", async () => {
  renderThemed(<Probe call={async () => ({})} />);
  fireEvent.click(screen.getByRole("button"));
  await screen.findByRole("button", { name: "go" });
  expect(screen.queryByText("Done it")).toBeNull();
  cleanup();
  renderThemed(<Probe call={async () => ({})} success />);
  fireEvent.click(screen.getByRole("button"));
  expect(await screen.findByText("Done it")).toBeTruthy();
});
