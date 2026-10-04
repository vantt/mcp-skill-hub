#!/usr/bin/env bash
set -euo pipefail

for port in 7421 5421; do
	if command -v ss >/dev/null 2>&1; then
		if ss -ltn "sport = :$port" | grep -q LISTEN; then
			echo "Port $port is already in use by:"
			ss -ltnp "sport = :$port" || true
			echo "Stop that process and retry. This script never picks another port."
			exit 1
		fi
	else
		if lsof -iTCP:"$port" -sTCP:LISTEN -t >/dev/null 2>&1; then
			echo "Port $port is already in use by:"
			lsof -iTCP:"$port" -sTCP:LISTEN || true
			echo "Stop that process and retry. This script never picks another port."
			exit 1
		fi
	fi
done

mkdir -p web/.e2e
go build -o web/.e2e/skillhub-dev ./cmd/skillhub
web/.e2e/skillhub-dev serve web --dev --no-open "$@" &
API_PID=$!
trap 'kill "$API_PID" 2>/dev/null || true' EXIT INT TERM

cd web && npm run dev
