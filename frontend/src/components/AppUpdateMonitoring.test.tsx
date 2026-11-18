import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Link, MemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { startAppUpdateMonitoring } from "@/lib/app-update";
import { AppUpdateMonitoring } from "./AppUpdateMonitoring";

vi.mock("@/lib/app-update", () => ({ startAppUpdateMonitoring: vi.fn() }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it("leaves public browsing alone and starts offline/update support when the board opens", () => {
  render(
    <MemoryRouter>
      <AppUpdateMonitoring />
      <Link to="/about">About</Link>
      <Link to="/app">Open board</Link>
    </MemoryRouter>,
  );
  expect(startAppUpdateMonitoring).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("About"));
  expect(startAppUpdateMonitoring).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("Open board"));
  expect(startAppUpdateMonitoring).toHaveBeenCalledTimes(1);
});

it("starts monitoring on a direct operational subpage visit", () => {
  render(
    <MemoryRouter initialEntries={["/dashboard/profile"]}>
      <AppUpdateMonitoring />
    </MemoryRouter>,
  );
  expect(startAppUpdateMonitoring).toHaveBeenCalledTimes(1);
});
