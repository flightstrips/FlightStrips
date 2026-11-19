import { act, cleanup, fireEvent, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LandingHeader } from "./LandingHeader";
import { useLandingAuth } from "./useLandingAuth";

const { loginWithRedirect, logout } = vi.hoisted(() => ({
  loginWithRedirect: vi.fn(),
  logout: vi.fn(),
}));
vi.mock("@auth0/auth0-react", () => ({
  useAuth0: () => ({ isAuthenticated: false, loginWithRedirect, logout }),
}));

beforeEach(() => {
  vi.stubGlobal("matchMedia", vi.fn(() => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })));
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value: function (this: HTMLDialogElement) { this.setAttribute("open", ""); } });
  Object.defineProperty(HTMLDialogElement.prototype, "close", { configurable: true, value: function (this: HTMLDialogElement) { this.removeAttribute("open"); } });
});
afterEach(() => {
  cleanup();
  Reflect.deleteProperty(HTMLDialogElement.prototype, "showModal");
  Reflect.deleteProperty(HTMLDialogElement.prototype, "close");
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.resetAllMocks();
});

it("prevents duplicate redirects and makes a rejected sign-in retryable", async () => {
  let rejectRedirect!: (reason: Error) => void;
  loginWithRedirect.mockImplementationOnce(() => new Promise<void>((_, reject) => { rejectRedirect = reject; }));
  const { result } = renderHook(() => useLandingAuth());
  let firstAttempt!: Promise<void>;
  act(() => { firstAttempt = result.current.signIn(); });
  expect(result.current.isSigningIn).toBe(true);
  await act(async () => { await result.current.signIn(); });
  expect(loginWithRedirect).toHaveBeenCalledTimes(1);

  await act(async () => {
    rejectRedirect(new Error("Network unavailable"));
    await firstAttempt;
  });
  expect(result.current.isSigningIn).toBe(false);
  expect(result.current.signInError).toContain("try again");

  loginWithRedirect.mockResolvedValueOnce(undefined);
  await act(async () => { await result.current.signIn(); });
  expect(loginWithRedirect).toHaveBeenCalledTimes(2);
  expect(loginWithRedirect).toHaveBeenLastCalledWith({ appState: { returnTo: "/app" } });
  expect(result.current.signInError).toBeNull();
  expect(result.current.isSigningIn).toBe(true);
});

it("also catches a synchronous redirect failure", async () => {
  loginWithRedirect.mockImplementationOnce(() => { throw new Error("Storage blocked"); });
  const { result } = renderHook(() => useLandingAuth());
  await act(async () => { await result.current.signIn(); });
  expect(result.current.signInError).toContain("Could not open VATSIM sign-in");
  expect(result.current.isSigningIn).toBe(false);
});

it("keeps the mobile menu open with an announced error and a working retry", async () => {
  loginWithRedirect.mockRejectedValueOnce(new Error("Network unavailable"));
  render(<MemoryRouter><LandingHeader /></MemoryRouter>);
  fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
  const menu = screen.getByRole("dialog", { name: "Navigation" });
  fireEvent.click(within(menu).getByRole("button", { name: "Sign in with VATSIM" }));
  await waitFor(() => expect(within(menu).getByRole("alert")).toHaveTextContent("try again"));
  expect(menu).toHaveAttribute("open");
  const retry = within(menu).getByRole("button", { name: "Sign in with VATSIM" });
  expect(retry).toBeEnabled();

  loginWithRedirect.mockImplementationOnce(() => new Promise<void>(() => {}));
  retry.focus();
  fireEvent.click(retry);
  expect(within(menu).queryByRole("alert")).not.toBeInTheDocument();
  expect(within(menu).getByRole("button", { name: "Connecting…" })).toHaveAttribute("aria-disabled", "true");
  expect(retry).toHaveFocus();
  fireEvent.click(retry);
  expect(loginWithRedirect).toHaveBeenCalledTimes(2);
});

it("shows a header sign-in error outside the closed menu", async () => {
  loginWithRedirect.mockRejectedValueOnce(new Error("Network unavailable"));
  render(<MemoryRouter><LandingHeader /></MemoryRouter>);
  fireEvent.click(within(screen.getByRole("banner")).getByRole("button", { name: "Sign in with VATSIM" }));
  await waitFor(() => expect(within(screen.getByRole("banner")).getByRole("alert")).toHaveTextContent("try again"));
});
