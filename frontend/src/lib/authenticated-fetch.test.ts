import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchWithAccessTokenRetry, requiresLoginPrompt } from "./authenticated-fetch";

describe("authenticated fetch", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("force-refreshes the token and retries once after an unauthorized response", async () => {
    const getAccessToken = vi.fn()
      .mockResolvedValueOnce("expired-token")
      .mockResolvedValueOnce("fresh-token");
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const response = await fetchWithAccessTokenRetry(getAccessToken, "/api/stand/status");

    expect(response.status).toBe(200);
    expect(getAccessToken).toHaveBeenNthCalledWith(1);
    expect(getAccessToken).toHaveBeenNthCalledWith(2, { cacheMode: "off" });
    expect((fetchMock.mock.calls[0][1]?.headers as Headers).get("Authorization")).toBe("Bearer expired-token");
    expect((fetchMock.mock.calls[1][1]?.headers as Headers).get("Authorization")).toBe("Bearer fresh-token");
  });

  it("does not refresh a token for non-authentication failures", async () => {
    const getAccessToken = vi.fn().mockResolvedValue("token");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 500 })));

    const response = await fetchWithAccessTokenRetry(getAccessToken, "/api/stand/status");

    expect(response.status).toBe(500);
    expect(getAccessToken).toHaveBeenCalledTimes(1);
  });

  it("recognizes refresh-session errors that require a new login", () => {
    expect(requiresLoginPrompt({ error: "login_required" })).toBe(true);
    expect(requiresLoginPrompt(new Error("Unknown or invalid refresh token"))).toBe(true);
    expect(requiresLoginPrompt(new Error("Network request failed"))).toBe(false);
  });
});
