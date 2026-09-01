type AuthErrorLike = {
  error?: string;
  error_description?: string;
  message?: string;
};

type AccessTokenGetter = (options?: { cacheMode?: "on" | "off" | "cache-only" }) => Promise<string>;

export function requiresLoginPrompt(error: unknown): boolean {
  const authError = typeof error === "object" && error !== null ? error as AuthErrorLike : null;
  const errorCode = typeof authError?.error === "string" ? authError.error.toLowerCase() : "";
  const errorDescription = typeof authError?.error_description === "string" ? authError.error_description.toLowerCase() : "";
  const errorMessage = error instanceof Error
    ? error.message.toLowerCase()
    : typeof authError?.message === "string"
      ? authError.message.toLowerCase()
      : typeof error === "string"
        ? error.toLowerCase()
        : "";
  const combinedMessage = `${errorCode} ${errorDescription} ${errorMessage}`;

  return errorCode === "invalid_grant"
    || errorCode === "login_required"
    || errorCode === "missing_refresh_token"
    || combinedMessage.includes("invalid refresh token")
    || combinedMessage.includes("unknown or invalid refresh token")
    || combinedMessage.includes("missing refresh token");
}

function requestWithToken(input: RequestInfo | URL, init: RequestInit | undefined, token: string): Promise<Response> {
  const headers = new Headers(init?.headers);
  headers.set("Authorization", `Bearer ${token}`);
  return fetch(input, {
    ...init,
    headers,
  });
}

// Auth0 normally refreshes expired tokens from its cache. If the API rejects a
// cached token anyway, bypass the cache once and replay the safe request.
export async function fetchWithAccessTokenRetry(
  getAccessToken: AccessTokenGetter,
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  let token = await getAccessToken();
  let response = await requestWithToken(input, init, token);
  if (response.status !== 401) return response;

  token = await getAccessToken({ cacheMode: "off" });
  response = await requestWithToken(input, init, token);
  return response;
}
