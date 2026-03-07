#!/bin/sh
cat > /usr/share/nginx/html/config.js <<EOF
window.__APP_CONFIG__ = {
  wsUrl: "${WS_URL}",
};
EOF
exec nginx -g "daemon off;"
