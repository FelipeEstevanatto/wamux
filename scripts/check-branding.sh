#!/usr/bin/env bash
# Fails when upstream brand assets or legacy upstream identifiers reappear in
# shipped paths. Attribution belongs in LICENSE/NOTICE/TRADEMARKS.md, not in
# the manager source, its built output, or static images.
set -euo pipefail
cd "$(dirname "$0")/.."

pattern='evolution-logo|evolution-pro|atendai|evolution-auth|egogo_dash|bmc_qr|picpay-qr|qrcode-pix|video-cover'

paths=(wamux-manager/src wamux-manager/dist manager/dist)
[ -d public ] && paths+=(public)

if grep -rniE "$pattern" "${paths[@]}"; then
	echo "brand-check: upstream brand assets found in shipped paths" >&2
	exit 1
fi

echo "brand-check: ok"
