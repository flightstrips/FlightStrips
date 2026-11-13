import {act, cleanup, fireEvent, render, screen, within} from "@testing-library/react";
import {useState, type ReactNode} from "react";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
import {AirportSessionProvider} from "./airport-session-provider";
import {AirportSwitcher} from "@/components/AirportSwitcher";
import type {AuthorizedSession, OwnMembership} from "@/api/sessions";

const mocks = vi.hoisted(() => ({discover: vi.fn(), getToken: vi.fn(), logout: vi.fn()}));
vi.mock("@auth0/auth0-react", () => ({useAuth0: () => ({getAccessTokenSilently: mocks.getToken, logout: mocks.logout, isAuthenticated: true, isLoading: false})}));
vi.mock("@/api/sessions", async original => ({...await original<typeof import("@/api/sessions")>(), discoverSessions: mocks.discover}));
vi.mock("./websocket-provider", () => ({WebSocketProvider: ({children, selection, ownPluginAvailable}: {children: ReactNode; selection: OwnMembership; ownPluginAvailable: boolean}) => {
  const [value, setValue] = useState(0);
  return <div data-testid="board" data-session={selection.session_id} data-instance={selection.instance_id} data-membership={selection.membership_id} data-available={ownPluginAvailable}><button onClick={() => setValue(v => v + 1)}>Local {value}</button>{children}</div>;
}}));

const member = (overrides: Partial<OwnMembership> = {}): OwnMembership => ({session_id: 1, airport: "AAAA", environment: "LIVE", instance_id: "own", membership_id: "m", transport_generation: "1", generation: "2", master_epoch: "1", view: "ekch", definition_revision: "r", actions: ["strip.move"], capabilities: {cdm: false, stands: false, text_pilot: false, pdc: false, aman: false}, ready: true, read_only: false, role: "slave", callsign: "OWN", position: "118.000", ...overrides});
const session = (m: OwnMembership, ...others: OwnMembership[]): AuthorizedSession => ({session_id: m.session_id, airport: m.airport, environment: m.environment, instances: [m, ...others]});
const mount = async () => {await act(async () => {render(<AirportSessionProvider url="ws://test"><AirportSwitcher/></AirportSessionProvider>);});};
const selectionKey = "flightstrips.airport-session.v1";
const saveHint = (session: number, instance = "own") => sessionStorage.setItem(selectionKey, JSON.stringify({session, instance}));
const refresh = async () => {await act(async () => {await vi.advanceTimersByTimeAsync(5000);});};

beforeEach(() => {sessionStorage.clear(); window.history.replaceState(null, "", "/"); vi.clearAllMocks(); vi.useFakeTimers(); mocks.discover.mockResolvedValue([]);});
afterEach(() => {cleanup(); vi.useRealTimers();});

describe("per-tab airport selection", () => {
  it.each(["storage", "url"])("enters the sole EKCH SWEATBOX session when the deleted session hint comes from %s", async source => {
    if (source === "storage") saveHint(2);
    else window.history.replaceState(null, "", "/?session=2&instance=own");
    mocks.discover.mockResolvedValue([session(member({session_id: 3, airport: "EKCH", environment: "SWEATBOX"}))]);
    await mount();
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "3");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(JSON.parse(sessionStorage.getItem(selectionKey)!)).toEqual({session: 3, instance: "own"});
  });

  it("uses the only own instance in a saved session when its saved instance has gone", async () => {
    saveHint(1, "deleted");
    mocks.discover.mockResolvedValue([session(member()), session(member({session_id: 2, airport: "BBBB"}))]);
    await mount();
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "1");
    expect(screen.getByTestId("board")).toHaveAttribute("data-instance", "own");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("restores the saved own instance on reload even when both session and instance choices are ambiguous", async () => {
    mocks.discover.mockResolvedValue([session(member(), member({instance_id: "second", callsign: "SECOND"})), session(member({session_id: 2, airport: "BBBB"}))]);
    await mount();
    fireEvent.click(screen.getByRole("button", {name: /AAAA/}));
    fireEvent.click(screen.getByRole("button", {name: /SECOND/}));
    cleanup();
    await mount();
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "1");
    expect(screen.getByTestId("board")).toHaveAttribute("data-instance", "second");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("waits for authorized discovery before restoring a saved selection after reconnect", async () => {
    saveHint(1, "second");
    await mount();
    expect(screen.getByText("Waiting for ES connection")).toBeInTheDocument();
    expect(screen.queryByTestId("board")).not.toBeInTheDocument();
    mocks.discover.mockResolvedValue([session(member(), member({instance_id: "second"})), session(member({session_id: 2, airport: "BBBB"}))]);
    await refresh();
    expect(screen.getByTestId("board")).toHaveAttribute("data-instance", "second");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("discards a stale hint when the only session appears on a later discovery", async () => {
    saveHint(2);
    await mount();
    expect(screen.getByText("Waiting for ES connection")).toBeInTheDocument();
    mocks.discover.mockResolvedValue([session(member({session_id: 3}))]);
    await refresh();
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "3");
    cleanup();
    await mount();
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "3");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("requires a session choice when a stale hint leaves multiple sessions, even with the same instance ID", async () => {
    saveHint(3);
    mocks.discover.mockResolvedValue([session(member()), session(member({session_id: 2, airport: "BBBB"}))]);
    await mount();
    expect(screen.queryByTestId("board")).not.toBeInTheDocument();
    expect(screen.getByText("Choose airport session")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: /BBBB/}));
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "2");
  });

  it("narrows a saved session to an instance choice and still allows choosing another airport", async () => {
    saveHint(1, "deleted");
    mocks.discover.mockResolvedValue([session(member(), member({instance_id: "second", callsign: "SECOND"})), session(member({session_id: 2, airport: "BBBB"}))]);
    await mount();
    expect(screen.queryByTestId("board")).not.toBeInTheDocument();
    expect(screen.getByText("Choose your plugin instance")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: "Back to airports"}));
    fireEvent.click(screen.getByRole("button", {name: /BBBB/}));
    expect(screen.getByTestId("board")).toHaveAttribute("data-session", "2");
  });

  it("requires an instance choice in the only session when the saved session no longer exists", async () => {
    saveHint(2);
    mocks.discover.mockResolvedValue([session(member(), member({instance_id: "second", callsign: "SECOND"}))]);
    await mount();
    expect(screen.queryByTestId("board")).not.toBeInTheDocument();
    expect(screen.getByText("Choose your plugin instance")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: /SECOND/}));
    expect(screen.getByTestId("board")).toHaveAttribute("data-instance", "second");
  });

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

  it("retains the active board through plugin loss and rebinds only the same physical instance", async () => {
    saveHint(1);
    mocks.discover.mockResolvedValue([session(member())]);
    await mount();
    fireEvent.click(screen.getByRole("button", {name: "Local 0"}));
    mocks.discover.mockResolvedValue([session(member({instance_id: "other"}))]);
    await refresh();
    expect(screen.getByTestId("board")).toHaveAttribute("data-instance", "own");
    expect(screen.getByTestId("board")).toHaveAttribute("data-available", "false");
    expect(screen.getByText("Local 1")).toBeInTheDocument();
    mocks.discover.mockResolvedValue([]);
    await refresh();
    expect(screen.getByTestId("board")).toHaveAttribute("data-instance", "own");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    mocks.discover.mockResolvedValue([session(member({membership_id: "rejoined", generation: "42", transport_generation: "43"}), member({instance_id: "other"}))]);
    await refresh();
    expect(screen.getByTestId("board")).toHaveAttribute("data-instance", "own");
    expect(screen.getByTestId("board")).toHaveAttribute("data-membership", "rejoined");
    expect(screen.getByTestId("board")).toHaveAttribute("data-available", "true");
    expect(JSON.parse(sessionStorage.getItem(selectionKey)!)).toEqual({session: 1, instance: "own"});
  });
});
