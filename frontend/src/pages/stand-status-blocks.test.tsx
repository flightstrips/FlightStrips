import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import StandStatusPage from "./stand-status";

vi.mock("@auth0/auth0-react", () => {
  const auth = { getAccessTokenSilently: vi.fn().mockResolvedValue("token"), logout: vi.fn() };
  return { useAuth0: () => auth };
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("stand status manual blocks", () => {
  async function renderBlock({ expired = false, outsideHistory = false, neighbors = true } = {}) {
    const now = Date.now();
    const minute = 60_000;
    const createdAt = new Date(now - (outsideHistory ? 240 : 30) * minute).toISOString();
    const expiresAt = expired ? new Date(now - (outsideHistory ? 180 : 10) * minute).toISOString() : undefined;
    vi.stubGlobal("fetch", vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({
      sessions: [{
        session_id: 7, name: "LIVE", airport: "EKCH", assignments: [],
        blocks: [{
          id: 21, stand: "A19", block_type: "CLOSURE", source: "CONTROLLER",
          ...(neighbors ? { blocks: ["A18", " a20 ", "A18", "A19", ""] } : {}),
          reason: "maintenance", manual: true, version: 1, created_at: createdAt,
          updated_at: createdAt, expires_at: expiresAt,
        }],
      }],
    }), { status: 200, headers: { "Content-Type": "application/json" } }))));
    render(<StandStatusPage compactSessionId={7} />);
    await screen.findByText("Section A stand timeline");
  }

  function standRow(stand: string) {
    return within(screen.getByText(stand).parentElement!);
  }

  it("shows manual-block neighbors as blocked while keeping unrelated stands free", async () => {
    await renderBlock();

    for (const stand of ["A18", "A19", "A20"]) {
      expect(standRow(stand).queryByText("FREE")).not.toBeInTheDocument();
    }
    for (const stand of ["A18", "A20"]) {
      expect(standRow(stand).getAllByText("CLOSURE blocks")).toHaveLength(1);
      expect(standRow(stand).getByTitle(/blocked by A19.*maintenance.*open-ended/)).toBeInTheDocument();
    }
    expect(standRow("A19").getAllByText("CLOSURE")).toHaveLength(1);
    expect(standRow("A17").getByText("FREE")).toBeInTheDocument();
  });

  it("shows neighboring blocks as history after the original block expires", async () => {
    await renderBlock({ expired: true });

    for (const stand of ["A18", "A19", "A20"]) {
      expect(standRow(stand).getByText("History")).toBeInTheDocument();
      expect(standRow(stand).getByTitle(/maintenance.*ends /)).toBeInTheDocument();
    }
  });

  it("removes neighboring blocks from view when the original block leaves the history window", async () => {
    await renderBlock({ expired: true, outsideHistory: true });

    for (const stand of ["A18", "A19", "A20"]) {
      expect(standRow(stand).getByText("FREE")).toBeInTheDocument();
    }
  });

  it("accepts older snapshots without neighboring block data", async () => {
    await renderBlock({ neighbors: false });

    expect(standRow("A19").getByText("CLOSURE")).toBeInTheDocument();
    expect(standRow("A18").getByText("FREE")).toBeInTheDocument();
  });
});
