#!/usr/bin/env bash
# Copy the handler into the GSX profile folder, keeping whatever API_BASE is
# already deployed. Copying the file straight over resets it to the shipped
# default, which points at the live server - so a test flight silently stops
# talking to the mock and the next log looks like a code fault.
#
#     gsx-client/mock/deploy.sh                       # keep the deployed API_BASE
#     gsx-client/mock/deploy.sh http://localhost:8787 # ...or set a new one
set -euo pipefail

SRC="$(dirname "$0")/../EKCH-Simnord-Sonnich_handler.py"
DEST="$APPDATA/Virtuali/GSX/MSFS/EKCH-Simnord-Sonnich_handler.py"

base="${1:-}"
if [ -z "$base" ] && [ -f "$DEST" ]; then
  base="$(sed -n 's/^API_BASE = "\(.*\)"$/\1/p' "$DEST" | head -1)"
fi

cp "$SRC" "$DEST"
if [ -n "$base" ]; then
  sed -i "s|^API_BASE = .*|API_BASE = \"$base\"|" "$DEST"
fi

echo "deployed -> $DEST"
sed -n 's/^API_BASE = /  API_BASE = /p' "$DEST"
echo "  press F5 in the GSX Handler Editor to reload"
