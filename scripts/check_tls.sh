#!/usr/bin/env bash
set -euo pipefail

HOST=${1:?usage: check_tls.sh <host>}
PORT=${PORT:-443}
failures=0

check_tls_version() {
	local version=$1
	if timeout 10 openssl s_client -connect "$HOST:$PORT" -servername "$HOST" "$version" \
		</dev/null >/dev/null 2>&1; then
		echo "PASS $version $HOST:$PORT"
	else
		echo "FAIL $version $HOST:$PORT"
		failures=$((failures + 1))
	fi
}

check_tls_version -tls1_2
check_tls_version -tls1_3

echo "certificate:"
timeout 10 openssl s_client -connect "$HOST:$PORT" -servername "$HOST" </dev/null 2>/dev/null |
	openssl x509 -noout -subject -issuer -dates || {
	echo "FAIL cannot read certificate"
	failures=$((failures + 1))
}

headers=$(mktemp)
trap 'rm -f "$headers"' EXIT
if curl -sS -D "$headers" -o /dev/null "https://$HOST/healthz"; then
	echo "PASS https healthz"
else
	echo "FAIL https healthz"
	failures=$((failures + 1))
fi
if grep -qi '^Strict-Transport-Security:' "$headers"; then
	echo "PASS HSTS"
else
	echo "FAIL HSTS header is missing"
	failures=$((failures + 1))
fi

if ((failures > 0)); then
	echo "tls checks failed: $failures"
	exit 1
fi
echo "tls checks passed"
