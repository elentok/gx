import { act, cleanup, fireEvent, render, screen } from "@testing-library/react"
import { MemoryRouter, useLocation } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { PrototypeSwitcher } from "./PrototypeSwitcher"

afterEach(() => {
  cleanup()
  vi.unstubAllEnvs()
})

const variants = [
  { key: "A", name: "Dense list" },
  { key: "B", name: "Cards" },
  { key: "C", name: "Table" },
]

function Search() {
  return <output data-testid="search">{useLocation().search}</output>
}

function setup(initial = "/p") {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <PrototypeSwitcher variants={variants} />
      <input aria-label="field" />
      <Search />
    </MemoryRouter>,
  )
}

const search = () => screen.getByTestId("search").textContent

test("shows the first variant by default and cycles with prev and next", () => {
  const { container } = setup()
  const label = () => container.querySelector(".mk-label")?.textContent
  expect(label()).toBe("A of 3Dense list")
  fireEvent.click(screen.getByRole("button", { name: "Next variant" }))
  expect(search()).toBe("?variant=B")
  expect(label()).toBe("B of 3Cards")
  fireEvent.click(screen.getByRole("button", { name: "Previous variant" }))
  fireEvent.click(screen.getByRole("button", { name: "Previous variant" }))
  expect(search()).toBe("?variant=C")
  expect(label()).toBe("C of 3Table")
})

test("keeps other search params", () => {
  setup("/p?x=1&variant=B")
  fireEvent.click(screen.getByRole("button", { name: "Next variant" }))
  expect(search()).toBe("?x=1&variant=C")
})

test("arrow keys cycle outside a field and are ignored inside a text input", () => {
  setup()
  act(() => {
    fireEvent.keyDown(document.body, { key: "ArrowRight" })
  })
  expect(search()).toBe("?variant=B")
  fireEvent.keyDown(document.body, { key: "ArrowLeft" })
  expect(search()).toBe("?variant=A")
  fireEvent.keyDown(screen.getByLabelText("field"), { key: "ArrowRight" })
  expect(search()).toBe("?variant=A")
})

test("renders nothing outside dev", () => {
  vi.stubEnv("DEV", false)
  const { container } = setup()
  expect(container.querySelector(".mk-sw")).toBeNull()
})
