import { fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderThemed } from "../render";
import { DiskStrip } from "./DiskStrip";

const base = { name: "Macintosh HD", free: "4.2 GB", held: false, cleaning: false, pending: false, onClean: () => {} };

describe("DiskStrip", () => {
  it("names the disk and its free space in the waiting tone", () => {
    const { getByRole } = renderThemed(<DiskStrip {...base} />);
    const strip = getByRole("status");
    expect(strip.getAttribute("data-tone")).toBe("waiting");
    expect(strip.textContent).toContain("Macintosh HD");
    expect(strip.textContent).toContain("4.2 GB");
  });

  it("shows a held disk in the paused tone and says tickets are held", () => {
    const { getByRole } = renderThemed(<DiskStrip {...base} held />);
    expect(getByRole("status").getAttribute("data-tone")).toBe("paused");
    expect(getByRole("status").textContent).toContain("held");
  });

  it("calls onClean from the button", () => {
    const onClean = vi.fn();
    const { getByRole } = renderThemed(<DiskStrip {...base} onClean={onClean} />);
    fireEvent.click(getByRole("button", { name: "Clean now" }));
    expect(onClean).toHaveBeenCalledOnce();
  });

  it.each([{ cleaning: true }, { pending: true }])("disables the button for %o", (busy) => {
    const { getByRole } = renderThemed(<DiskStrip {...base} {...busy} />);
    expect((getByRole("button", { name: "Clean now" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("shows a problem inline and keeps the strip", () => {
    const { getByRole } = renderThemed(<DiskStrip {...base} problem="clean already running" />);
    expect(getByRole("alert").textContent).toBe("clean already running");
    expect(getByRole("status")).toBeTruthy();
  });
});
