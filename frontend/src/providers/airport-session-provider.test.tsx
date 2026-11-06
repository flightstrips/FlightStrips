import {act, cleanup, fireEvent, render, screen, within} from "@testing-library/react";
import {useState, type ReactNode} from "react";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
import {AirportSessionProvider} from "./airport-session-provider";
import {AirportSwitcher} from "@/components/AirportSwitcher";
import type {AuthorizedSession, OwnMembership} from "@/api/sessions";

const mocks = vi.hoisted(() => ({discover: vi.fn(), getToken: vi.fn(), logout: vi.fn()}));
vi.mock("@auth0/auth0-react", () => ({useAuth0: () => ({getAccessTokenSilently: mocks.getToken, logout: mocks.logout, isAuthenticated: true, isLoading: false})}));
vi.mock("@/api/sessions", async original => ({...await original<typeof import("@/api/sessions")>(), discoverSessions: mocks.discover}));
vi.mock("./websocket-provider", () => ({WebSocketProvider: ({children, selection}: {children: ReactNode; selection: OwnMembership}) => {
  const [value, setValue] = useState(0);
  return <div data-testid="board" data-session={selection.session_id}><button onClick={() => setValue(v => v + 1)}>Local {value}</button>{children}</div>;
}}));

const member = (overrides: Partial<OwnMembership> = {}): OwnMembership => ({session_id: 1, airport: "AAAA", environment: "LIVE", instance_id: "own", membership_id: "m", transport_generation: "1", generation: "2", master_epoch: "1", view: "ekch", definition_revision: "r", actions: ["strip.move"], capabilities: {cdm: false, stands: false, text_pilot: false, pdc: false, aman: false}, ready: true, read_only: false, role: "slave", callsign: "OWN", position: "118.000", ...overrides});
const session = (m: OwnMembership, ...others: OwnMembership[]): AuthorizedSession => ({session_id: m.session_id, airport: m.airport, environment: m.environment, instances: [m, ...others]});
const mount = async () => {await act(async () => {render(<AirportSessionProvider url="ws://test"><AirportSwitcher/></AirportSessionProvider>);});};

beforeEach(() => {sessionStorage.clear(); vi.clearAllMocks(); vi.useFakeTimers(); mocks.discover.mockResolvedValue([]);});
afterEach(() => {cleanup(); vi.useRealTimers();});

describe("per-tab airport selection", () => {
  it("keeps the existing no-plugin waiting flow and automatically enters a single session", async () => {
    await mount();
    expect(screen.getByText("Waiting for ES connection")).toBeInTheDocument();
    expect(screen.queryByTestId("board")).not.toBeInTheDocument();
    mocks.discover.mockResolvedValue([session(member())]);
    await act(async () => {await vi.advanceTimersByTimeAsync(5000);});
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "1");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("requires an airport choice for multiple sessions and clears the old board on switch", async () => {
    mocks.discover.mockResolvedValue([session(member()), session(member({session_id: 2, airport: "BBBB"}))]);
    await mount();
    fireEvent.click(screen.getByRole("button", {name: /AAAA/}));
    fireEvent.click(screen.getByRole("button", {name: "Local 0"}));
    expect(screen.getByText("Local 1")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: "Switch airport session"}));
    fireEvent.click(screen.getByRole("button", {name: /BBBB/}));
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "2");
    expect(screen.getByText("Local 0")).toBeInTheDocument();
    expect(sessionStorage.getItem("flightstrips.airport-session.v1")).toContain('"session":2');
  });
  it("requires an own-instance choice only when it is ambiguous", async () => {
    mocks.discover.mockResolvedValue([session(member(), member({instance_id: "second", callsign: "SECOND"}))]);
    await mount();
    expect(screen.getByText("Choose your plugin instance")).toBeInTheDocument();
    expect(screen.queryByTestId("board")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: /SECOND/}));
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "1");
    expect(sessionStorage.getItem("flightstrips.airport-session.v1")).toContain("second");
  });
  it("keeps two mounted tabs independent and does not fall back after own-plugin loss", async () => {
    mocks.discover.mockResolvedValue([session(member()), session(member({session_id: 2, airport: "BBBB"}))]);
    await act(async () => {render(<><section data-testid="tab-a"><AirportSessionProvider url="ws://test"><AirportSwitcher/></AirportSessionProvider></section><section data-testid="tab-b"><AirportSessionProvider url="ws://test"><AirportSwitcher/></AirportSessionProvider></section></>);});
    const a = within(screen.getByTestId("tab-a")), b = within(screen.getByTestId("tab-b"));
    fireEvent.click(a.getByRole("button", {name: /AAAA/}));
    fireEvent.click(b.getByRole("button", {name: /BBBB/}));
    mocks.discover.mockResolvedValue([session(member({session_id: 2, airport: "BBBB"}))]);
    await act(async () => {await vi.advanceTimersByTimeAsync(5000);});
    expect(a.getByTestId("board")).toHaveAttribute("data-session", "1");
    expect(b.getByTestId("board")).toHaveAttribute("data-session", "2");
  });
});
