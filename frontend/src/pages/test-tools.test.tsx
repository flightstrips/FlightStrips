import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import TestToolsPage from "./test-tools";

vi.mock("@auth0/auth0-react", () => ({
  useAuth0: () => ({ getAccessTokenSilently: vi.fn().mockResolvedValue("token") }),
}));

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  }));
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("local test console", () => {
  it("renders a backend-enabled SAT console and creates an editable preset", async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/api/test/status")) {
        return jsonResponse({
          enabled: true,
          simulated_time: "2026-07-17T12:00:00Z",
          sessions: [{ id: 7, name: "LOCAL", airport: "EKCH" }],
          sat: { enabled: true, ready: true },
        });
      }
      if (url.includes("/api/test/sat/scenarios") && init?.method === "POST") {
        return jsonResponse({ id: "one", callsign: "TST101" }, 201);
      }
      return jsonResponse({ scenarios: [], blocks: [], simulated_time: "2026-07-17T12:00:00Z" });
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<TestToolsPage />);
    expect(await screen.findByText("Local Test Console")).toBeInTheDocument();
    expect(await screen.findByText("Changes publish to LOCAL.")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Callsign"), { target: { value: "tst101" } });
    fireEvent.change(screen.getByLabelText("Preset"), { target: { value: "arrival" } });
    fireEvent.click(screen.getByRole("button", { name: /create scenario/i }));

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(([, init]) => init?.method === "POST");
      expect(createCall).toBeDefined();
      const body = JSON.parse(String(createCall?.[1]?.body));
      expect(body).toMatchObject({
        session_id: 7,
        preset: "arrival",
        callsign: "TST101",
        origin: "EGLL",
        destination: "EKCH",
      });
    });
    expect(screen.getByLabelText("Preset")).toHaveValue("arrival");
    expect(screen.getByLabelText("Origin")).toHaveValue("EGLL");
    expect(screen.getByLabelText("Destination")).toHaveValue("EKCH");
  });

  it("renders the normal not-found state when the backend route is disabled", async () => {
    vi.stubGlobal("fetch", vi.fn().mockImplementation(() => jsonResponse({ error: "Not Found" }, 404)));
    render(<TestToolsPage />);
    expect(await screen.findByText("404 Not Found")).toBeInTheDocument();
  });

  it("shows API failures instead of masking them as a disabled route", async () => {
    vi.stubGlobal("fetch", vi.fn().mockImplementation(() => jsonResponse({ error: "database unavailable" }, 500)));
    render(<TestToolsPage />);
    expect(await screen.findByText("Unable to load test console")).toBeInTheDocument();
    expect(screen.getByText("database unavailable")).toBeInTheDocument();
    expect(screen.queryByText("404 Not Found")).not.toBeInTheDocument();
  });
});
