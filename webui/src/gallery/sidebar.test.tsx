import { cleanup, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import { renderThemed } from "../ui/render"
import { galleryEntries } from "./registry"

afterEach(() => {
  cleanup()
  localStorage.clear()
})

const render = (entry: string, label: string) => {
  const node = galleryEntries
    .find((e) => e.name === entry)
    ?.cases.find((c) => c.label === label)?.node
  expect(node, `${entry} ${label}`).toBeTruthy()
  renderThemed(<>{node}</>)
}

test.each(["desktop, docked", "tablet, slide-over open", "phone, slide-over open"])(
  "the sidebar %s lists its rows",
  async (label) => {
    render("Sidebar", label)
    expect(await screen.findByRole("link", { name: /Blog/ })).toBeTruthy()
    expect(screen.getByRole("button", { name: "On hold (1)" }).getAttribute("aria-expanded")).toBe(
      "false",
    )
  },
)

test.each(["tablet, slide-over open", "phone, slide-over open"])(
  "the slide-over %s starts open",
  async (label) => {
    render("Sidebar", label)
    await screen.findByRole("link", { name: /Blog/ })
    await waitFor(() =>
      expect(
        screen.getByLabelText("Sidebar", { selector: "aside" }).getAttribute("data-open"),
      ).toBe("true"),
    )
  },
)

test("the Other case opens the Other section", async () => {
  render("Sidebar", "desktop, Other open")
  expect(await screen.findByRole("link", { name: /scratch/ })).toBeTruthy()
})

test("the Fix agent rows case shows a wrench on each Fix agent row only", async () => {
  render("Sidebar", "desktop, Fix agent rows")
  await screen.findByRole("link", { name: /fix lint/ })
  expect(screen.getAllByTestId("fix-wrench")).toHaveLength(2)
  expect(
    screen.getByRole("link", { name: /builder/ }).querySelector("[data-testid=fix-wrench]"),
  ).toBeNull()
})

test("the On hold expanded case lists the held row", async () => {
  render("Sidebar", "desktop, On hold expanded")
  expect(await screen.findByRole("link", { name: /Old/ })).toBeTruthy()
})

test("the switcher menu case opens the menu", async () => {
  render("WorkspaceSwitcher", "menu open")
  expect(await screen.findByRole("menu")).toBeTruthy()
})

test("the Manage workspaces cases show the list, a rename in progress and the delete confirmation", async () => {
  render("ManageWorkspaces", "default")
  expect(await screen.findByRole("dialog", { name: "Manage workspaces" })).toBeTruthy()
  cleanup()
  render("ManageWorkspaces", "rename in progress")
  await waitFor(() =>
    expect((screen.getByLabelText("Rename Work") as HTMLInputElement).value).toBe("Client work"),
  )
  cleanup()
  render("ManageWorkspaces", "delete confirmation")
  expect(await screen.findByRole("dialog", { name: "Delete Work" })).toBeTruthy()
})
