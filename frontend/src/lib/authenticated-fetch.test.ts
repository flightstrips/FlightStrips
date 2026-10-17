import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchWithAccessTokenRetry, requireAccessToken, requiresLoginPrompt } from "./authenticated-fetch";

describe("authenticated fetch", () => {
  afterEach(() => vi.unstubAllGlobals());

  it.each([undefined, ""])("requires a new login for a missing token (%s)", (token) => {
    try {
      requireAccessToken(token);
      expect.fail("Missing tokens must be rejected");
    } catch (error) {
      expect(requiresLoginPrompt(error)).toBe(true);
    }
  });

  it("does not send a request without an access token", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchWithAccessTokenRetry(vi.fn().mockResolvedValue(undefined), "/api/stand/status"))
      .rejects.toMatchObject({ error: "login_required" });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("does not replay an unauthorized request if token refresh returns no token", async () => {
    const getAccessToken = vi.fn().mockResolvedValueOnce("expired-token").mockResolvedValueOnce(undefined);
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 401 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchWithAccessTokenRetry(getAccessToken, "/api/stand/status"))
      .rejects.toMatchObject({ error: "login_required" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

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
