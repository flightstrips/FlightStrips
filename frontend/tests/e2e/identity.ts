import type {Page} from "@playwright/test";

// Seed only the external identity provider's SDK cache. The normal Auth0
// provider, app route, WebSocket provider and server-side roles remain in use.
export async function identity(page: Page, backend: string, token = "__TEST_TOKEN__") {
  await page.addInitScript(({token}) => {
    const clientId = "mIjRYlbKHpTwnNAkhcu9plQP541Klwvn";
    const scope = "openid profile email offline_access";
    const claims = {sub: "auth0|browser-e2e", name: "Browser E2E", exp: Math.floor(Date.now() / 1000) + 86400};
    const idToken = `${btoa('{"alg":"RS256"}')}.${btoa(JSON.stringify(claims))}.fixture`;
    const decodedToken = {claims, user: {sub: claims.sub, name: claims.name}, encoded: {header: "", payload: "", signature: ""}, header: {alg: "RS256"}};
    localStorage.setItem(`@@auth0spajs@@::${clientId}::@@user@@`, JSON.stringify({id_token: idToken, decodedToken}));
    localStorage.setItem(`@@auth0spajs@@::${clientId}::backend-dev::${scope}`, JSON.stringify({expiresAt: claims.exp, body: {access_token: token, id_token: idToken, decodedToken, scope, audience: "backend-dev", client_id: clientId, expires_in: 86400, token_type: "Bearer"}}));
  }, {token});
  await page.route("**/config.js", route => route.fulfill({contentType: "text/javascript", body: `window.__APP_CONFIG__={wsUrl:"${backend.replace("http", "ws")}/frontEndEvents",apiBaseUrl:"${backend}",clientId:"mIjRYlbKHpTwnNAkhcu9plQP541Klwvn",audience:"backend-dev",connection:"vatsim-dev"};`}));
  await page.route(/^https:\/\/fonts\.(googleapis|gstatic)\.com\//, route => route.fulfill({body: "", contentType: "text/css"}));
  await page.route("**/version.json", route => route.fulfill({json: {deploymentVersion: "browser-e2e"}}));
}
