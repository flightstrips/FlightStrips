#!/bin/sh
deployment_version="${APP_VERSION:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"

cat > /usr/share/nginx/html/config.js <<EOF
window.__APP_CONFIG__ = {
  deploymentVersion: "${deployment_version}",
  wsUrl: "${WS_URL}",
  apiBaseUrl: "${API_BASE_URL}",
  clientId: "${OIDC_FRONTEND_CLIENT_ID}",
  audience: "${OIDC_AUDIENCE}",
  connection: "${OIDC_CONNECTION}",
};
EOF

cat > /usr/share/nginx/html/version.json <<EOF
{"deploymentVersion":"${deployment_version}"}
EOF

exec nginx -g "daemon off;"
