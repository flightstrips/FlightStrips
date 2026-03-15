#!/bin/sh
cat > /usr/share/nginx/html/config.js <<EOF
window.__APP_CONFIG__ = {
  wsUrl: "${WS_URL}",
  clientId: "${OIDC_FRONTEND_CLIENT_ID}",
  audience: "${OIDC_AUDIENCE}",
  connection: "${OIDC_CONNECTION}",
};
EOF
exec nginx -g "daemon off;"
