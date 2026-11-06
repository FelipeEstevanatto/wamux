#!/usr/bin/env bash
# Fails when the docs stop matching the code: a path or make target AGENTS.md names
# no longer exists, or an environment variable is documented nowhere.
set -euo pipefail
cd "$(dirname "$0")/.."

failed=0
complain() {
	echo "check-docs: $1" >&2
	failed=1
}

# Paths in backticks: anything with a slash or a known file extension.
while read -r path; do
	[ -e "$path" ] || complain "AGENTS.md names $path, which does not exist"
done < <(grep -oE '`[A-Za-z0-9_.][A-Za-z0-9_./-]*`' AGENTS.md | tr -d '`' |
	grep -E '/|\.(md|sh|go|yml)$')

while read -r target; do
	grep -qE "^${target}:" Makefile || complain "AGENTS.md names make $target, which does not exist"
done < <(grep -oE '`make [a-z-]+`' AGENTS.md | tr -d '`' | awk '{print $2}')

while read -r name; do
	grep -rqsE "\b${name}\b" .env.example README.md docs/wiki ||
		complain "$name is read by the server but documented nowhere"
done < <(grep -oE '^\s+[A-Z][A-Z0-9_]+\s+= "[A-Z0-9_]+"' pkg/config/env/env.go | grep -oE '"[^"]+"' | tr -d '"')

# The other way round: a documented variable the server never reads does nothing.
known="$(grep -oE '"[A-Z0-9_]+"' pkg/config/env/env.go | tr -d '"')"
while read -r name; do
	grep -qx "$name" <<<"$known" || git grep -q "\"$name\"" -- pkg cmd ||
		complain "$name is documented but the server never reads it"
done < <(
	grep -ohE '^\s*#? ?[A-Z][A-Z0-9_]+=' .env.example | tr -d '#= \t'
	grep -rohE '^\s*(- )?(NATS|AMQP|MINIO|DATABASE|WEBHOOK|QRCODE|LOG|MEDIA)_[A-Z0-9_]+=' docs/wiki README.md |
		sed -E 's/^\s*(- )?//; s/=$//'
)

[ "$failed" -eq 0 ] && echo "check-docs: ok"
exit "$failed"
