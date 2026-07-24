const CHARTFOX_ACCESS_TOKEN_KEY = 'chartfox.access-token';
const CHARTFOX_ACCESS_TOKEN_EXPIRY_KEY = 'chartfox.access-token-expiry';
const CHARTFOX_OAUTH_STATE_KEY = 'chartfox.oauth-state';
const CHARTFOX_OAUTH_VERIFIER_KEY = 'chartfox.oauth-verifier';
const CHARTFOX_OAUTH_RETURN_TO_KEY = 'chartfox.oauth-return-to';

const authorizationUrl = 'https://api.chartfox.org/oauth/authorize';
const tokenUrl = 'https://api.chartfox.org/oauth/token';
const requestedScope = 'charts:index charts:view charts:files';

interface ChartFoxTokenResponse {
  access_token: string;
  expires_in?: number;
  token_type?: string;
}

export function chartFoxRedirectUri() {
  return `${window.location.origin}/efb/chartfox/callback`;
}

function base64Url(bytes: Uint8Array) {
  let value = '';
  for (const byte of bytes) value += String.fromCharCode(byte);
  return btoa(value).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function randomValue(byteLength: number) {
  const bytes = new Uint8Array(byteLength);
  crypto.getRandomValues(bytes);
  return base64Url(bytes);
}

async function sha256(value: string) {
  const valueBytes = new TextEncoder().encode(value);
  return base64Url(new Uint8Array(await crypto.subtle.digest('SHA-256', valueBytes)));
}

function configuredClientId() {
  return window.__APP_CONFIG__?.chartfoxClientId?.trim() ?? '';
}

export function isChartFoxConfigured() {
  return configuredClientId() !== '';
}

export function getChartFoxAccessToken() {
  const token = window.sessionStorage.getItem(CHARTFOX_ACCESS_TOKEN_KEY);
  const expiresAt = Number(window.sessionStorage.getItem(CHARTFOX_ACCESS_TOKEN_EXPIRY_KEY) ?? 0);
  if (!token || !Number.isFinite(expiresAt) || expiresAt <= Date.now()) {
    clearChartFoxAccessToken();
    return null;
  }
  return token;
}

export function clearChartFoxAccessToken() {
  window.sessionStorage.removeItem(CHARTFOX_ACCESS_TOKEN_KEY);
  window.sessionStorage.removeItem(CHARTFOX_ACCESS_TOKEN_EXPIRY_KEY);
}

export async function startChartFoxAuthorization(returnTo = '/efb') {
  const clientId = configuredClientId();
  if (!clientId) throw new Error('ChartFox is not configured for this FlightStrips deployment.');

  const verifier = randomValue(64);
  const state = randomValue(32);
  window.sessionStorage.setItem(CHARTFOX_OAUTH_VERIFIER_KEY, verifier);
  window.sessionStorage.setItem(CHARTFOX_OAUTH_STATE_KEY, state);
  window.sessionStorage.setItem(CHARTFOX_OAUTH_RETURN_TO_KEY, returnTo);

  const params = new URLSearchParams({
    response_type: 'code',
    client_id: clientId,
    redirect_uri: chartFoxRedirectUri(),
    scope: requestedScope,
    state,
    code_challenge: await sha256(verifier),
    code_challenge_method: 'S256',
  });
  window.location.assign(`${authorizationUrl}?${params}`);
}

export async function completeChartFoxAuthorization(search: string) {
  const params = new URLSearchParams(search);
  const error = params.get('error');
  if (error) throw new Error(params.get('error_description') || error);

  const code = params.get('code');
  const state = params.get('state');
  const expectedState = window.sessionStorage.getItem(CHARTFOX_OAUTH_STATE_KEY);
  const verifier = window.sessionStorage.getItem(CHARTFOX_OAUTH_VERIFIER_KEY);
  const returnTo = window.sessionStorage.getItem(CHARTFOX_OAUTH_RETURN_TO_KEY) || '/efb';
  window.sessionStorage.removeItem(CHARTFOX_OAUTH_STATE_KEY);
  window.sessionStorage.removeItem(CHARTFOX_OAUTH_VERIFIER_KEY);
  window.sessionStorage.removeItem(CHARTFOX_OAUTH_RETURN_TO_KEY);

  if (!code || !state || !expectedState || state !== expectedState || !verifier) {
    throw new Error('The ChartFox authorization response could not be verified. Please try again.');
  }

  const body = new URLSearchParams({
    grant_type: 'authorization_code',
    client_id: configuredClientId(),
    code,
    redirect_uri: chartFoxRedirectUri(),
    code_verifier: verifier,
  });
  const response = await fetch(tokenUrl, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body,
  });
  const result = await response.json().catch(() => null) as ChartFoxTokenResponse | { error_description?: string; error?: string } | null;
  if (!response.ok || !result || !('access_token' in result)) {
    const message = result && 'error_description' in result ? result.error_description ?? result.error : undefined;
    throw new Error(message || 'ChartFox did not issue an access token.');
  }

  const expiresIn = typeof result.expires_in === 'number' ? result.expires_in : 3600;
  window.sessionStorage.setItem(CHARTFOX_ACCESS_TOKEN_KEY, result.access_token);
  window.sessionStorage.setItem(CHARTFOX_ACCESS_TOKEN_EXPIRY_KEY, String(Date.now() + Math.max(0, expiresIn - 30) * 1000));
  return returnTo;
}
