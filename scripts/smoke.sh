#!/usr/bin/env bash
set -euo pipefail

AUTOCONFIG_URL=${AUTOCONFIG_URL:-http://127.0.0.1:8080}
APEX_URL=${APEX_URL:-$AUTOCONFIG_URL}
AUTOCONFIG_HOST=${AUTOCONFIG_HOST:-}
APEX_HOST=${APEX_HOST:-}
DOMAIN=${DOMAIN:-example.com}
EMAIL=${EMAIL:-user@$DOMAIN}
EXPECT_APPLE=${EXPECT_APPLE:-false}
CURL_OPTS=${CURL_OPTS:-}

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

failures=0

HOST_ARGS=()

build_host_args() {
	HOST_ARGS=()
	if [[ -n "$1" ]]; then
		HOST_ARGS=(-H "Host: $1")
	fi
}

request() {
	local method=$1
	local url=$2
	local host=$3
	shift 3

	build_host_args "$host"

	# CURL_OPTS is intentionally word-split for optional flags such as -k.
	# shellcheck disable=SC2086
	curl -sS $CURL_OPTS ${HOST_ARGS[@]+"${HOST_ARGS[@]}"} -o "$TMP_DIR/body" -w '%{http_code}' \
		-X "$method" "$@" "$url"
}

check_status() {
	local name=$1
	local want=$2
	local got=$3
	if [[ "$got" == "$want" ]]; then
		echo "PASS $name ($got)"
	else
		echo "FAIL $name (got $got, want $want)"
		failures=$((failures + 1))
	fi
}

check_contains() {
	local name=$1
	local needle=$2
	if grep -qF -- "$needle" "$TMP_DIR/body"; then
		echo "PASS $name"
	else
		echo "FAIL $name (missing: $needle)"
		failures=$((failures + 1))
	fi
}

check_header() {
	local name=$1
	local header=$2
	local expected=${3:-}
	if ! grep -qi "^$header:" "$TMP_DIR/headers"; then
		echo "FAIL $name (missing header: $header)"
		failures=$((failures + 1))
		return
	fi
	if [[ -n "$expected" ]] && ! grep -qi "^$header: *$expected" "$TMP_DIR/headers"; then
		echo "FAIL $name (unexpected value for $header)"
		failures=$((failures + 1))
		return
	fi
	echo "PASS $name"
}

echo "smoke: autoconfig=$AUTOCONFIG_URL apex=$APEX_URL domain=$DOMAIN"

status=$(request GET "$AUTOCONFIG_URL/healthz" "$AUTOCONFIG_HOST")
check_status "healthz" 200 "$status"
check_contains "healthz body" "ok"

status=$(request GET "$AUTOCONFIG_URL/version" "$AUTOCONFIG_HOST")
check_status "version" 200 "$status"
check_contains "version body" '"go"'

status=$(request GET "$AUTOCONFIG_URL/" "$AUTOCONFIG_HOST")
check_status "index page" 200 "$status"
check_contains "index project" "go-autoconfig"
check_contains "index manual setup" "Manual setup"

status=$(request GET "$AUTOCONFIG_URL/mail/config-v1.1.xml" "$AUTOCONFIG_HOST")
check_status "thunderbird" 200 "$status"
check_contains "thunderbird domain" "<domain>$DOMAIN</domain>"

status=$(request GET "$APEX_URL/.well-known/autoconfig/mail/config-v1.1.xml" "$APEX_HOST")
check_status "thunderbird well-known" 200 "$status"
check_contains "well-known domain" "<domain>$DOMAIN</domain>"

outlook_body="<?xml version=\"1.0\"?><Autodiscover><Request><EMailAddress>$EMAIL</EMailAddress><AcceptableResponseSchema>http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a</AcceptableResponseSchema></Request></Autodiscover>"
status=$(request POST "$AUTOCONFIG_URL/autodiscover/autodiscover.xml" "$AUTOCONFIG_HOST" \
	-H 'Content-Type: text/xml' --data-binary "$outlook_body")
check_status "outlook" 200 "$status"
check_contains "outlook smtp address" "<AutoDiscoverSMTPAddress>$EMAIL</AutoDiscoverSMTPAddress>"

status=$(request POST "$AUTOCONFIG_URL/autodiscover/autodiscover.xml" "$AUTOCONFIG_HOST" \
	-H 'Content-Type: text/xml' --data-binary \
	'<Autodiscover><Request><EMailAddress>user@example.com</EMailAddress><AcceptableResponseSchema>https://unknown/schema</AcceptableResponseSchema></Request></Autodiscover>')
check_status "unknown schema" 400 "$status"

status=$(request GET "$AUTOCONFIG_URL/mail/config-v1.1.xml" "unknown.test")
check_status "unknown host" 404 "$status"

if [[ "$EXPECT_APPLE" == "true" ]]; then
	status=$(request GET "$AUTOCONFIG_URL/email.mobileconfig" "$AUTOCONFIG_HOST")
	check_status "apple profile" 200 "$status"
	check_contains "apple payload" "com.apple.mail.managed"
else
	status=$(request GET "$AUTOCONFIG_URL/email.mobileconfig" "$AUTOCONFIG_HOST")
	check_status "apple disabled" 404 "$status"
fi

# Header checks: keep the last response headers.
build_host_args "$AUTOCONFIG_HOST"
# shellcheck disable=SC2086
curl -sS $CURL_OPTS ${HOST_ARGS[@]+"${HOST_ARGS[@]}"} -D "$TMP_DIR/headers" -o /dev/null \
	"$AUTOCONFIG_URL/mail/config-v1.1.xml"
check_header "cache-control" "Cache-Control" "no-store"
check_header "content-type options" "X-Content-Type-Options" "nosniff"
if [[ "$AUTOCONFIG_URL" == https://* ]]; then
	check_header "hsts" "Strict-Transport-Security"
fi

dd if=/dev/zero bs=1024 count=1025 2>/dev/null | tr '\0' 'a' > "$TMP_DIR/big"
status=$(request POST "$AUTOCONFIG_URL/autodiscover/autodiscover.xml" "$AUTOCONFIG_HOST" \
	-H 'Content-Type: text/xml' --data-binary "@$TMP_DIR/big")
check_status "oversized body" 413 "$status"

if ((failures > 0)); then
	echo "smoke failed: $failures check(s)"
	exit 1
fi
echo "smoke passed"
