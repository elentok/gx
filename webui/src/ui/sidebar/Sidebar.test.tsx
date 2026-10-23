import { fireEvent } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { Button } from "../button/Button";
import { Sidebar } from "./Sidebar";
import { SidebarItem } from "./SidebarItem";
import { SidebarSection } from "./SidebarSection";

describe("Sidebar", () => {
  it.each(TONES)("renders title and children in tone %s", (tone) => {
    const { container, getByText } = renderThemed(
      <Sidebar title="Designs" tone={tone}>
        body
      </Sidebar>,
    );
    expect(getByText("Designs")).toBeTruthy();
    expect(getByText("body")).toBeTruthy();
    expect(container.querySelector("[data-tone]")!.getAttribute("data-tone")).toBe(tone);
  });

  it("renders the action in the title row", () => {
    const { getByText } = renderThemed(
      <Sidebar title="Designs" action={<Button variant="ghost">+ New</Button>}>
        body
      </Sidebar>,
    );
    expect(getByText("+ New")).toBeTruthy();
  });
});

describe("SidebarSection", () => {
  it.each(TONES)("shows title and count in tone %s", (tone) => {
    const { container, getByText } = renderThemed(
      <SidebarSection title="In progress" count={3} tone={tone}>
        rows
      </SidebarSection>,
    );
    expect(getByText("In progress")).toBeTruthy();
    expect(getByText("3")).toBeTruthy();
    expect(container.querySelector("[data-tone]")!.getAttribute("data-tone")).toBe(tone);
  });

  it("is expanded by default and collapses on click", () => {
    const { getByRole, queryByText } = renderThemed(<SidebarSection title="Ready">rows</SidebarSection>);
    const toggle = getByRole("button");
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(queryByText("rows")).toBeTruthy();
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(queryByText("rows")).toBeNull();
  });

  it("starts collapsed and keeps the count visible", () => {
    const { getByText, queryByText } = renderThemed(
      <SidebarSection title="Done" count={7} defaultCollapsed>
        rows
      </SidebarSection>,
    );
    expect(queryByText("rows")).toBeNull();
    expect(getByText("7")).toBeTruthy();
  });

  it("omits the count when none is given", () => {
    const { getByRole } = renderThemed(<SidebarSection title="Ready">rows</SidebarSection>);
    expect(getByRole("button").textContent).toBe("▾Ready");
  });
});

describe("SidebarItem", () => {
  it.each(TONES)("renders label in tone %s", (tone) => {
    const { container, getByText } = renderThemed(<SidebarItem label="webui-design" tone={tone} />);
    expect(getByText("webui-design")).toBeTruthy();
    expect(container.querySelector("[data-tone]")!.getAttribute("data-tone")).toBe(tone);
  });

  it("marks the current item and not a plain one", () => {
    const { getByText } = renderThemed(
      <>
        <SidebarItem label="current" current />
        <SidebarItem label="plain" />
      </>,
    );
    expect(getByText("current").closest("button")!.getAttribute("aria-current")).toBe("page");
    expect(getByText("plain").closest("button")!.hasAttribute("aria-current")).toBe(false);
  });

  it("renders marks and fires onClick", () => {
    let clicked = 0;
    const { getByText } = renderThemed(
      <SidebarItem label="row" marks={<span>mark</span>} onClick={() => (clicked += 1)} />,
    );
    expect(getByText("mark")).toBeTruthy();
    fireEvent.click(getByText("row"));
    expect(clicked).toBe(1);
  });
});
