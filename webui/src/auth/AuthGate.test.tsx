import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { useEffect } from "react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { signOut } from "../apiFetch"
import { renderThemed } from "../ui/render"
import { AuthGate } from "./AuthGate"
import type { AuthState } from "./authState"
import { useCert } from "./cert"
import { TEST_CERT } from "./testCert"

const CertProbe = () => <p>cert:{useCert().cert?.cer_path}</p>

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  sessionStorage.clear()
})
beforeEach(() => {
  window.location.hash = ""
})

const stateOf = (state: AuthState["state"]): AuthState => ({
  ...TEST_CERT,
  state,
  totp_on: true,
  setup_code_required: false,
})

function renderGate(fetchState: () => Promise<AuthState>) {
  return renderThemed(
    <AuthGate fetchState={fetchState}>
      <p>the app</p>
    </AuthGate>,
  )
}

test("shows setup for a pending setup", async () => {
  renderGate(async () => stateOf("setup"))
  expect(await screen.findByRole("button", { name: "Continue" })).toBeTruthy()
})

test("shows login when signed out", async () => {
  renderGate(async () => stateOf("signed-out"))
  expect(await screen.findByRole("button", { name: "Sign in" })).toBeTruthy()
})

test("shows the app when signed in", async () => {
  renderGate(async () => stateOf("signed-in"))
  expect(await screen.findByText("the app")).toBeTruthy()
})

test("a failed auth read mounts the app", async () => {
  renderGate(async () => {
    throw new Error("offline")
  })
  expect(await screen.findByText("the app")).toBeTruthy()
})

test("after a failed read a 401 re-reads the state and shows login with the hash kept", async () => {
  window.location.hash = "#/stats"
  let fail = true
  renderGate(async () => {
    if (fail) throw new Error("offline")
    return stateOf("signed-out")
  })
  await screen.findByText("the app")
  fail = false
  act(() => signOut())
  expect(await screen.findByRole("button", { name: "Sign in" })).toBeTruthy()
  expect(window.location.hash).toBe("#/stats")
})

test("the signed-in app has the cert context", async () => {
  const state = stateOf("signed-in")
  renderThemed(
    <AuthGate fetchState={async () => state}>
      <CertProbe />
    </AuthGate>,
  )
  expect(await screen.findByText(`cert:${state.cer_path}`)).toBeTruthy()
})

test("the app is not remounted when unknown turns into signed-in", async () => {
  const mounts = vi.fn()
  const Child = () => {
    useEffect(mounts, [])
    return <p>the app</p>
  }
  let fail = true
  renderThemed(
    <AuthGate
      fetchState={async () => {
        if (fail) throw new Error("offline")
        return stateOf("signed-in")
      }}
    >
      <Child />
    </AuthGate>,
  )
  await screen.findByText("the app")
  fail = false
  act(() => signOut())
  await waitFor(() => expect(screen.getByText("the app")).toBeTruthy())
  await act(async () => {})
  expect(mounts).toHaveBeenCalledOnce()
})

test("login asks for the setup code only when the state says so", async () => {
  renderGate(async () => ({ ...stateOf("signed-out"), setup_code_required: true }))
  expect(await screen.findByLabelText("Setup code")).toBeTruthy()
})

test("a 401 in the app switches to login and keeps the hash", async () => {
  window.location.hash = "#/stats"
  let current = stateOf("signed-in")
  renderGate(async () => current)
  await screen.findByText("the app")
  current = stateOf("signed-out")
  act(() => signOut())
  expect(await screen.findByRole("button", { name: "Sign in" })).toBeTruthy()
  expect(window.location.hash).toBe("#/stats")
})

test("a 401 after a web reset switches to setup", async () => {
  let current = stateOf("signed-in")
  renderGate(async () => current)
  await screen.findByText("the app")
  current = stateOf("setup")
  act(() => signOut())
  expect(await screen.findByLabelText("Setup code")).toBeTruthy()
})

test("a successful login re-reads the state and keeps the hash", async () => {
  window.location.hash = "#/stats"
  let current = stateOf("signed-out")
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}")),
  )
  renderGate(async () => current)
  fireEvent.change(await screen.findByLabelText("Password"), { target: { value: "pw" } })
  current = stateOf("signed-in")
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }))
  expect(await screen.findByText("the app")).toBeTruthy()
  expect(window.location.hash).toBe("#/stats")
})

test("sign out clears the page caches", async () => {
  sessionStorage.setItem("gx.page.stats", "{}")
  renderGate(async () => stateOf("signed-in"))
  await screen.findByText("the app")
  act(() => signOut())
  await waitFor(() => expect(sessionStorage.getItem("gx.page.stats")).toBeNull())
})
