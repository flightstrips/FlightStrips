import { act, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";
import { CommunicationType, type FrontendStrip } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore } from "@/store/store";
import { WebSocketStoreContext } from "@/store/store-context";
import AppPage from "./app";

vi.mock("@auth0/auth0-react", () => ({ useAuth0: () => ({ isAuthenticated: true, isLoading: false }) }));
vi.mock("@/providers/websocket-provider", () => ({ WebSocketProvider: ({ children }: { children: ReactNode }) => children }));
vi.mock("@/providers/vacs-provider", () => ({ VacsProvider: ({ children }: { children: ReactNode }) => children }));
vi.mock("@/routes/AppRouter", () => ({ default: () => <div>Controller strips</div> }));
vi.mock("@/components/commandbar/CommandBar", () => ({ default: () => null }));
vi.mock("@/components/AppUpdateNotifier", () => ({ default: () => null }));
vi.mock("@/components/strip/ValidationStatusDialogOverlay", () => ({ ValidationStatusDialogOverlay: () => null }));

it("opens and cancels a text-pilot composer from the controller application", () => {
  const client = { on: vi.fn(), send: vi.fn(), isConnected: () => false };
  const store = createWebSocketStore(client as unknown as WebSocketClient);
  store.setState({ readOnly: false, strips: [{ callsign: "SAS123", communication_type: CommunicationType.Text, runway: "22R" }] as FrontendStrip[] });
  render(<WebSocketStoreContext.Provider value={store}><AppPage /></WebSocketStoreContext.Provider>);
  expect(screen.getByText("Controller strips")).toBeVisible();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  act(() => store.getState().openTextPilotMenu({ callsign: "SAS123", variant: "push", point: "J4" }));
  expect(screen.getByRole("dialog")).toBeVisible();
  expect(screen.getByRole("textbox")).toHaveValue("Push and start approved, Release point J4.");
  fireEvent.click(screen.getByRole("button", { name: "ESC" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(client.send).not.toHaveBeenCalled();
});
